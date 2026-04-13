# 计算字段执行引擎研究

> **研究日期**：2026-04-12
> **研究目标**：明确 SQL 表达式与 Python 表达式的执行策略、求值顺序、安全边界
> **产品决议（2026-04-13）**：MVP 阶段仅落地 SQL 表达式；Python 表达式延期到后续阶段，原因是阶段复杂度控制，而非方案缺陷

---

## 一、求值顺序：多计算字段的 DAG 与错误信息

### 1.1 问题背景

根据 `generator.md` 的设计：

> "SQL计算表达式和Python计算表达式可以引用其它字段。表达式的计算在所有字段数据生成后才开始计算，如果一条语句中包含多个表达式生成器。则需要评估字段生成顺序"

这意味着：
- 一个表内可能存在多个计算字段
- 计算字段 A 可能引用计算字段 B 的值
- 需要确定计算字段之间的求值顺序

### 1.2 依赖类型分析

| 字段类型 | 依赖特性 | 求值时机 |
|---------|---------|---------|
| 普通生成器（序列、随机、枚举等） | 无依赖 | 第一阶段：并行生成 |
| 外键字段 | 依赖父表 ID 池 | 第二阶段：从池采样 |
| SQL 计算表达式 | 仅可引用本表其他字段 | 第三阶段：表达式求值 |
| Python 计算表达式 | 仅可引用本表其他字段 | 第三阶段：表达式求值 |

**关键约束**：计算字段只能引用**本表**字段，不能跨表引用，此为产品边界。
SQL 与 Python 计算字段存在互相依赖的情况，二者将统一进一张 DAG，并不区别对待（因此并不是先计算 SQL 表达式再计算 Python 表达式）

### 1.3 字段级 DAG 构建方案

```go
// engine/planner/field_dependency.go

type FieldDependencyGraph struct {
    nodes   map[string]*FieldNode   // column_id → node
    edges   map[string][]string     // from_id → []to_id (依赖方向)
    order   []string                // 求值顺序
}

type FieldNode struct {
    ColumnID       string
    ColumnName     string
    GeneratorType  GeneratorType    // sequence / sql_expr / python_expr 等
    ExprReferences []string         // 表达式引用的字段列表（仅表达式生成器有值）
    DepType        FieldDepType     // primary / computed / dependent_computed
}

// 字段依赖类型
type FieldDepType int
const (
    FieldPrimary     FieldDepType = iota  // 普通生成器，无依赖
    FieldComputed                         // 计算字段，依赖 primary 字段
    FieldDependentComputed                // 计算字段，依赖其他计算字段
)
```

### 1.4 表达式引用解析

#### 方案对比

| 方案 | 实现方式 | 优点 | 缺点 |
|-----|---------|-----|-----|
| **词法分析** | 遍历字段名，检查是否出现在表达式中 | 实现简单 | 误匹配（如 `concat(name, 'test')` 中 `'test'` 被误判） |
| **解析器分析** | SQL/Python AST 解析 | 精准识别 | 实现复杂，需维护多方言 parser |
| **占位符语法**（推荐） | 强制 `${column}` 格式包裹字段引用 | 精准、简单、无歧义 | 语法略显冗长 |

#### 推荐方案：占位符语法

强制要求表达式中的字段引用使用 `${column_name}` 格式：

**命名约束**：`column_name` 仅支持 `\w+`（字母、数字、下划线），不支持特殊字符或中文列名。

| 表达式类型 | 示例 |
|----------|------|
| **SQL** | `CONCAT('ORD-', DATE_FORMAT(${order_time}, '%Y%m%d'), LPAD(${seq}, 4, '0'))` |
| **Python** | `${name}.upper() + '_' + str(${id})` |

**解析实现**：
```go
// 统一的引用解析（适用于 SQL 和 Python）
var columnRefPattern = regexp.MustCompile(`\$\{(\w+)\}`)

func ParseExpressionReferences(expr string) []string {
    matches := columnRefPattern.FindAllStringSubmatch(expr, -1)
    refs := make([]string, 0, len(matches))
    seen := make(map[string]bool)

    for _, match := range matches {
        colName := match[1]  // 提取 ${column} 中的 column
        if !seen[colName] {
            refs = append(refs, colName)
            seen[colName] = true
        }
    }
    return refs
}
```

**求值时替换**：
```go
// 生成实际执行的表达式（替换占位符为实际值）
func BuildExecutableExpression(expr string, row map[string]interface{}) string {
    result := expr
    for colName, value := range row {
        placeholder := "${" + colName + "}"
        // SQL: 值需要转义；Python: 值需要类型转换
        replacement := formatValueForExecution(value, exprType)
        result = strings.ReplaceAll(result, placeholder, replacement)
    }
    return result
}
```

**隐式依赖与配置一致性**：除 `${...}` 显式引用外，若其它生成器配置（如模板字符串）间接引用列名，应单独建模或同样强制占位符，避免仅靠「全文扫描列名」产生漏检。

**列重命名**：列改名后占位符仍指向旧名时，保存/校验阶段应报「引用不存在字段」，并在 UI 侧提供批量查找替换（产品体验，可与元数据迁移 spec 对齐）。

**设计理由**：
1. **精准识别**：正则 `\$\{(\w+)\}` 无歧义，不会误匹配字符串常量
2. **实现简单**：几十行代码替代复杂的 SQL parser
3. **前端友好**：IDE 可提示语法规则，输入时有明确约束
4. **统一处理**：SQL 和 Python 表达式使用相同解析逻辑

### 1.5 环检测与错误信息设计

**检测逻辑**：
```go
func DetectFieldCycle(fdg *FieldDependencyGraph) ([]string, error) {
    // Kahn 算法检测环
    // 若存在未排序节点，返回环中字段列表
}
```

**错误信息模板**：

| 场景 | 错误信息 |
|-----|---------|
| **简单环**（A→B→A） | "字段 '{A}' 与 '{B}' 存在循环依赖，无法计算。请修改其中一个表达式的引用关系。" |
| **多字段环**（A→B→C→A） | "检测到循环依赖链：{A} → {B} → {C} → {A}。请断开其中一个引用关系。" |
| **自引用**（A 引用 A） | "字段 '{A}' 的表达式引用了自身，这是不允许的。" |
| **引用不存在字段** | "字段 '{A}' 的表达式引用了 '{B}'，但 '{B}' 不存在于本表或未配置生成器。" |

### 1.6 求值顺序算法

```go
func CalculateFieldOrder(fdg *FieldDependencyGraph) ([]string, error) {
    // 1. 拓扑排序，得到求值顺序
    order, cycles := TopologicalSort(fdg)
    if len(cycles) > 0 {
        return nil, fmt.Errorf("field cycle detected: %v", cycles)
    }

    // 2. 分组并行执行（可选优化）
    // 同层级（入度同时为 0）的字段可并行计算
    groups := GroupByLevel(order, fdg)

    return order, nil
}
```

### 1.7 推荐方案

| 决策项 | 方案 |
|-------|-----|
| **依赖检测时机** | 配置保存时预检测（前端实时验证），生成执行前二次校验 |
| **环处理策略** | 禁止保存/执行，强制用户修改 |
| **跨表引用** | 禁止，提示用户改用外键关联机制 |
| **引用不存在字段** | 禁止保存，提示用户检查字段名 |
| **并行优化** | 同层字段可并行（单行内），但收益有限，可先串行实现 |

---

## 二、SQL 方言：表达式解析策略

### 2.0 核对：「SQL 表达式解析」口径

| 层次 | 做什么 | 不做什么（MVP） |
|-----|--------|----------------|
| **依赖 / 引用解析** | 仅用 `${column}` 提取依赖边，供 DAG 与校验入参（与 §1.4 一致） | 不对裸列名做全文扫描，避免误匹配 |
| **方言与函数合法性** | 连目标库做 `SELECT … LIMIT 1`（或等价）+ 模拟值代入 | 不把「多方言 AST」作为引用解析前提 |
| **静态结构约束** | 产品规则层禁止子查询等（§2.4） | 若需自动证明「无子查询」，再评估 SQL parser，与占位符方案正交 |

**结论**：「解析」在本文中拆成 **占位符级（必有）** 与 **目标库运行时验证（推荐）**；二者与 §3 的 Python 合法性（ParseExpr + AST）对称，无混为一谈。

### 2.1 问题背景

用户设想：
> "按目标库解析，例如执行 SELECT concat(x, y) LIMIT 1，如果执行没有报错，则认为此表达式正确。如果进一步能够代入 x, y 的模拟值，则可以直观看见 SQL 表达式的结果，更好。"

**核心问题**：
- 不同数据库方言函数名不同（MySQL `CONCAT` vs Postgres `||`）
- 函数参数语义不同（MySQL `DATE_FORMAT` vs Postgres `TO_CHAR`）
- 如何在配置阶段验证表达式有效性？

**语法约定**：字段引用使用 `${column}` 格式，例如：
```sql
CONCAT('ORD-', DATE_FORMAT(${order_time}, '%Y%m%d'), LPAD(${seq}, 4, '0'))
```

### 2.2 方案对比

| 方案 | 实现方式 | 优点 | 缺点 |
|-----|---------|-----|-----|
| **方案 A：抽象层限定** | 定义通用函数集，映射到各方言 | 跨方言一致性，前端易验证 | 限制灵活性，需维护映射表 |
| **方案 B：目标库验证**（推荐） | 连接目标库执行验证 SQL | 用户可使用方言特有函数 | 需数据库连接 |
| **方案 C：解析器验证** | 使用 SQL parser 检查语法 | 不需连接，速度快 | 无法验证函数是否存在 |

### 2.3 推荐方案：目标库验证 + 模拟值代入

**验证流程**：

```
配置阶段（前端）：
  1. 用户输入 SQL 表达式（如：CONCAT('ORD-', ${order_time}, ${seq})）
  2. 前端解析 ${...} 占位符，提取字段引用列表
  3. 前端调用 FFI: ValidateSQLExpression(expr, columnRefs)

Go 后端验证：
  4. 连接目标数据库
  5. 为每个引用字段生成模拟值（根据字段类型）
  6. 将 `${column}` 替换为模拟值（见 §2.3.1），构建可执行 SQL
  7. 执行 `SELECT expr AS result LIMIT 1`（或等价方言）并捕获错误

结果返回：
  - 成功：返回模拟值结果，用户可直观验证
  - 失败：返回数据库错误信息，提示修正
```

**Go 实现伪代码**：

```go
// backend/generator/common/sql_expression.go

type SQLExpressionValidator struct {
    connector Connector
}

func (v *SQLExpressionValidator) Validate(
    ctx context.Context,
    expr string,
    columnRefs []ColumnRef,
) (*ValidationResult, error) {

    // 1. 为引用字段生成模拟值
    mockValues := make(map[string]interface{})
    for _, ref := range columnRefs {
        mockValues[ref.ColumnName] = generateMockValue(ref.DataType)
    }

    // 2. 替换 ${column} 占位符为实际值（SQL 需转义）
    executableExpr := substitutePlaceholders(expr, mockValues, "sql", v.connector.DriverName())

    // 3. 构建验证 SQL
    // MySQL: SELECT {expr} AS result LIMIT 1
    // Postgres: SELECT {expr} AS result LIMIT 1
    fullSQL := fmt.Sprintf("SELECT %s AS result LIMIT 1", executableExpr)

    // 4. 执行并捕获错误
    row := v.connector.QueryRow(fullSQL)
    var result interface{}
    err := row.Scan(&result)
    if err != nil {
        return &ValidationResult{
            Valid:   false,
            Error:   err.Error(),
            MockResult: nil,
        }, nil
    }

    return &ValidationResult{
        Valid:      true,
        Error:      "",
        MockResult: result,
        MockValues: mockValues,
    }, nil
}

// 根据数据类型生成模拟值
func generateMockValue(dtype DataType) interface{} {
    switch dtype {
    case TypeInt:
        return 12345
    case TypeString:
        return "mock_string"
    case TypeDecimal:
        return 123.45
    case TypeDatetime:
        return time.Now()
    case TypeBoolean:
        return true
    }
}
```

#### 2.3.1 模拟值、代入安全与验证语义

**模拟值生成策略**：

| 数据类型 | 模拟值 | 理由 |
|---------|-------|-----|
| int | `12345` | 典型整数，便于验证数值运算 |
| string | `"mock_string"` | 典型字符串，便于验证 concat/substr |
| decimal | `123.45` | 典型小数，便于验证 round/ceil |
| datetime | `NOW()` 或固定日期 | 便于验证 date_format/year 等 |
| boolean | `true` | 典型布尔值 |

**占位符代入与安全**：验证与批量生成时，将 `${column}` 展开为 SQL 字面量必须经过**按类型转义**（或等价地：用驱动参数绑定拼出 `SELECT ? AS expr` 形态，由占位符映射到参数），禁止未转义字符串拼接导致注入。文档级伪代码用 `fmt.Sprintf` 仅为示意，实现以参数化为准。

**无连接 / 离线编辑**：目标库验证依赖连接。**产品决策：禁止保存**——用户必须先建立数据库连接才能保存 SQL 表达式配置。将错误扼杀在设计阶段而非运行阶段，避免生成执行时报数据库函数不存在等错误。

**验证通过 ≠ 全表语义等价**：`LIMIT 1` 与模拟值只能排除大量语法/函数错误；真实行上仍可能因 NULL、排序规则、时区、隐式类型转换等与预览不一致，UI 可提示用户以目标库实际类型为准。

### 2.4 方言差异处理

**数据库特定语法**：

| 方言 | 字符串连接 | 日期格式化 | 条件表达式 |
|-----|----------|----------|----------|
| MySQL | `CONCAT(a, b)` | `DATE_FORMAT(dt, '%Y-%m-%d')` | `IF(cond, a, b)` |
| Postgres | `a || b` | `TO_CHAR(dt, 'YYYY-MM-DD')` | `CASE WHEN cond THEN a ELSE b END` |
| Oracle | `a || b` | `TO_CHAR(dt, 'YYYY-MM-DD')` | `CASE WHEN ...` |
| MSSQL | `a + b` | `FORMAT(dt, 'yyyy-MM-dd')` | `IIF(cond, a, b)` |
| SQLite | `a || b` | `strftime('%Y-%m-%d', dt)` | `IIF(cond, a, b)` 或 `CASE` |

**安全处理**：
- 所有表达式在 `SELECT` 子句中执行，不涉及 INSERT/UPDATE/DELETE
- 表达式不允许包含子查询（防止数据泄露）；该规则为 **MVP 产品约束**，后续若放开需另定义数据访问边界与白名单。
- 禁止使用危险函数（如 `LOAD_FILE`、`INTO OUTFILE`）

### 2.5 前端交互设计

```
表达式编辑器：
┌─────────────────────────────────────────────┐
│ SQL 表达式:                                  │
│ CONCAT('ORD-', DATE_FORMAT(${order_time},   │
│        '%Y%m%d'), LPAD(${seq}, 4, '0'))     │
│                                             │
│ 💡 提示: 字段引用使用 ${column_name} 格式    │
│                                             │
│ [验证表达式] [预览结果]                       │
├─────────────────────────────────────────────┤
│ 检测到引用字段: order_time, seq              │
│                                             │
│ 模拟值代入:                                  │
│   ${order_time} → '2026-04-12 10:30:00'     │
│   ${seq} → 12345                             │
│                                             │
│ 实际执行:                                    │
│ CONCAT('ORD-', DATE_FORMAT('2026-04-12 ...'│
│        '%Y%m%d'), LPAD(12345, 4, '0'))      │
│                                             │
│ 结果预览: "ORD-2026041200012345" ✓           │
├─────────────────────────────────────────────┤
│ ⚠️ 注意: 此表达式使用 MySQL 方言，           │
│    Postgres 需改为 || 连接符                 │
└─────────────────────────────────────────────┘
```

---

## 三、Python 沙箱：安全边界设计

### 3.1 gpython 概述

`github.com/go-python/gpython` 是一个嵌入式 Python 解释器：

- **特点**：无需外部 Python 运行时，纯 Go 实现
- **支持**：Python 语法、标准库子集
- **适用场景**：计算字段表达式求值

### 3.2 沙箱需求

| 需求项 | 原因 |
|-------|-----|
| **限制标准库范围** | 防止文件操作、网络请求等危险行为 |
| **禁用特定模块** | os, sys, subprocess, socket 等高风险模块 |
| **性能上限** | 防止死循环、无限递归导致资源耗尽 |
| **内存上限** | 防止大对象创建导致内存溢出 |

### 3.3 推荐方案：受限执行环境

**方案 A：gpython 内置限制**（调研需验证）

gpython 可能提供：
- `__import__` 钩子：控制模块导入
- `RestrictedPython` 类似机制：AST 过滤

**方案 B：自定义沙箱**（推荐实现）

```go
// backend/generator/common/python_expression.go

type PythonSandbox struct {
    preInjectedModules map[string]bool   // 预注入到 scope 的模块（用户可直接使用）
    bannedFunctions    map[string]bool   // 禁止调用的函数名
    timeout            time.Duration
    maxMemory          int64
}

var DefaultSandbox = PythonSandbox{
    // 这些模块在执行前注入到 scope，用户可直接调用如 math.floor(3.14)
    // 注意：用户无法 import，只能使用预注入的模块
    preInjectedModules: map[string]bool{
        "math":     true,   // 数学运算：math.floor, math.sqrt
        "random":   true,   // 随机数：random.randint, random.choice
        "datetime": true,   // 日期处理：datetime.datetime.now()
        "string":   true,   // 字符串处理：string.Template
        "re":       true,   // 正则表达式：re.sub, re.match
        "json":     true,   // JSON：json.dumps, json.loads
    },
    // 禁止的函数名（内置函数）
    bannedFunctions: map[string]bool{
        // 文件/IO 操作
        "open":         true,
        "input":        true,
        // 动态执行
        "exec":         true,
        "eval":         true,
        "compile":      true,
        // 动态导入（虽然语法层拒绝 import 语句，但 __import__('os') 这种调用仍可能）
        "__import__":   true,
        // 环境访问
        "globals":      true,
        "locals":       true,
        "vars":         true,
        // 属性操作（宽封禁；若需 isinstance 等，可对「顶格 Name 为 getattr」做例外或提供安全包装函数）
        "getattr":      true,
        "setattr":      true,
        "delattr":      true,
        "hasattr":      true,
        // 直接调用 type()/object() 常用于逃逸；若封禁导致合法 isinstance 不可用，可将 isinstance 放入 injectSafeBuiltins 白名单实现
        "type":         true,
        "object":       true,
    },
    timeout:   5 * time.Second,  // 单表达式超时
    maxMemory: 10 * 1024 * 1024,  // 10MB 内存上限
}
```

**设计说明**：

| 机制 | 作用 | 示例 |
|-----|-----|------|
| **语法层** | 拒绝语句（import/def/class） | `import os` → 语法错误 |
| **preInjectedModules** | 预注入安全模块到 scope | 用户可直接用 `math.floor(3.14)` |
| **bannedFunctions** | 禁止危险内置函数 | `open('file')` → 安全违规 |

> `importlib` 不在 bannedFunctions 中，因为它是一个模块而非函数。由于语法层拒绝 import 且我们不预注入 importlib，用户无法访问它。

### 3.4 执行流程

**两层验证机制**：

```
输入字符串
    │
    ▼
┌─────────────────────────────────┐
│ 第 1 层：语法验证                │
│ - 使用 gpython.Eval() 模式       │
│ - 仅接受表达式，拒绝语句         │
│ - import/def/class 等语句报错    │
└─────────────────────────────────┘
    │ 合法表达式
    ▼
┌─────────────────────────────────┐
│ 第 2 层：安全检查                │
│ - 遍历 AST，检查危险函数调用     │
│ - 检查危险属性访问               │
└─────────────────────────────────┘
    │ 安全表达式
    ▼
    执行求值
```

```go
func (s *PythonSandbox) Execute(
    ctx context.Context,
    expr string,
    variables map[string]interface{},
) (interface{}, error) {

    // 第 1 层：语法验证（使用 Eval 模式，仅接受表达式）
    ast, err := gpython.ParseExpr(expr)  // ParseExpr = 仅解析表达式
    if err != nil {
        // 输入不是合法表达式（可能是语句，如 import os）
        return nil, fmt.Errorf("语法错误：输入必须是 Python 表达式，而非语句。\n详情：%v", err)
    }

    // 第 2 层：安全检查（检查表达式中的危险元素）
    if err := s.validateAST(ast); err != nil {
        return nil, fmt.Errorf("安全违规：%v", err)
    }

    // 创建受限执行环境
    scope := gpython.NewScope()

    // 注入用户变量（${column} 占位符对应的值）
    for name, value := range variables {
        scope.Set(name, value)
    }

    // 预注入安全模块（用户可直接使用 math.floor 等）
    s.injectSafeModules(scope)

    // 注入受限内置函数（仅安全函数）
    s.injectSafeBuiltins(scope)

    // 超时执行
    result, err := s.executeWithTimeout(ctx, expr, scope, s.timeout)
    if err != nil {
        return nil, err
    }

    return result, nil
}
```

**Eval vs Exec 模式对比**：

| 模式 | 接受内容 | 示例 | 本项目选择 |
|-----|---------|------|----------|
| `Eval` | 仅表达式 | `a + b`, `func(x)` | ✅ **使用此模式** |
| `Exec` | 语句 + 表达式 | `import os; print(x)` | ❌ 不使用 |

**设计理由**：使用 `Eval` 模式时，`import os` 等语句会在语法层直接报错，无需在安全层额外检查 Import 节点。

### 3.5 AST 安全检查

```go
func (s *PythonSandbox) validateAST(ast *gpython.AST) error {
    // 遍历 AST，检查表达式中的危险元素
    // 注意：Import 等语句已在语法层被拒绝，此处仅需检查表达式层面的危险操作

    for _, node := range ast.Walk() {
        switch node.Type {

        // 检查函数调用：函数名是否在黑名单
        case "Call":
            funcName := extractFuncName(node)
            if s.bannedFunctions[funcName] {
                return fmt.Errorf("函数 '%s' 不被允许", funcName)
            }

        // 检查属性访问：是否访问危险属性
        case "Attribute":
            attrName := node.Attribute
            if isDangerousAttribute(attrName) {
                return fmt.Errorf("属性 '%s' 访问不被允许", attrName)
            }
        }
    }

    return nil
}

// 提取函数名（支持简单调用和链式调用）
func extractFuncName(callNode *gpython.ASTNode) string {
    if callNode.Func.Type == "Name" {
        return callNode.Func.Name  // 如：open()
    }
    if callNode.Func.Type == "Attribute" {
        // 链式调用，如：math.floor(3.14) → 返回 "math.floor"
        return callNode.Func.FullPath
    }
    return ""
}

// 危险属性列表
var dangerousAttributes = []string{
    "__builtins__", "__class__", "__base__", "__bases__",
    "__mro__", "__subclasses__", "__globals__", "__code__",
}
```

**检查逻辑说明**：

| AST 节点类型 | 处理策略 | 示例 |
|-------------|---------|------|
| `Call` | **检查函数名** | `open('file')` → 检查 `open` 是否在黑名单 |
| `Attribute` | **检查属性名** | `obj.__class__` → 拒绝危险属性 |

> **语法层已拒绝**：`import`、`def`、`class` 等语句会在 `ParseExpr` 阶段直接报错，无需在安全层检查。

**安全函数示例**：`min()`、`max()`、`abs()`、`math.floor()` 是 `Call` 节点，但函数名不在 `bannedFunctions` 中，故允许。

**与 §3.3、§3.6 的一致性**：§3.3 的 `bannedFunctions` 为「禁止的内置名」清单；§3.5 对 **每个** `Call` 做解析后比对黑名单（并非禁止所有调用）；§3.6 用表格说明用户可见的允许/禁止语义。三者分工为：清单定义 → AST 落实 → 文档说明，无「禁止 Call 节点」类矛盾。

**合法性判断小结**：Python 是否可执行 = **(1) ParseExpr 通过**（仅为表达式）∧ **(2) AST 安全遍历通过**（无黑名单调用、无危险属性等）∧ **(3) 运行时**（超时、未定义名等）。实现前需以 gpython 真实 API 核对 `ParseExpr` / `Eval` 名称。

### 3.6 允许的内置函数

| 函数 | 允许 | 理由 |
|-----|-----|-----|
| `abs`, `min`, `max`, `sum` | ✅ | 数学运算，安全 |
| `len`, `range`, `enumerate` | ✅ | 序列操作，安全 |
| `str`, `int`, `float`, `bool` | ✅ | 类型转换，安全 |
| `list`, `dict`, `tuple`, `set` | ✅ | 数据结构，安全 |
| `sorted`, `reversed` | ✅ | 序列处理，安全 |
| `round`, `pow` | ✅ | 数学函数，安全 |
| `format`, `f-string` | ✅ | 格式化，安全 |
| `map`, `filter`, `zip` | ✅ | 函数式操作，安全 |
| `chr`, `ord` | ✅ | 字符转换，安全 |
| `any`, `all` | ✅ | 逻辑运算，安全 |
| `open`, `input` | ❌ | 文件/IO 操作，危险 |
| `exec`, `eval`, `compile` | ❌ | 动态执行，危险 |
| `__import__` | ❌ | 模块导入，危险 |
| `globals`, `locals`, `vars` | ❌ | 环境访问，危险 |
| `getattr`, `setattr`, `delattr`, `hasattr` | ❌ | 属性操作，危险 |
| `type`, `object` | ❌（直接调用） | 类型篡改风险；`isinstance` 等若需支持见 §3.3 注释，由安全内置注入实现 |

> 完整禁止列表见 `DefaultSandbox.bannedFunctions`（§3.3）

**预注入 `random` 模块**：用户可在表达式中使用 `random.randint()`、`random.choice()` 等函数。若产品对生成可复现性有要求，预注入的 `random` 应绑定任务级种子（与全局生成器体系的种子策略一致），具体实现由执行引擎统一管理。

### 3.7 性能防护

**超时机制**：
```go
func (s *PythonSandbox) executeWithTimeout(
    ctx context.Context,
    expr string,
    scope *gpython.Scope,
    timeout time.Duration,
) (interface{}, error) {

    done := make(chan interface{})
    errChan := make(chan error)

    go func() {
        result, err := gpython.Eval(expr, scope)
        if err != nil {
            errChan <- err
        } else {
            done <- result
        }
    }()

    select {
    case result := <-done:
        return result, nil
    case err := <-errChan:
        return nil, err
    case <-time.After(timeout):
        return nil, fmt.Errorf("execution timeout after %v", timeout)
    case <-ctx.Done():
        return nil, ctx.Err()
    }
}
```

**超时实现的工程说明**：上述模式在超时返回后，子 goroutine 内的 `gpython.Eval` **可能仍在运行**（若解释器无取消 API，则存在 goroutine 泄漏与 CPU 占用风险）。MVP 可接受「超时后仅返回错误、后台仍跑一小段」时须在文档与 spec 中声明；更稳妥方向是：解释器级取消、进程隔离、或限制表达式复杂度（禁止长时间循环）等，择一在实现阶段敲定。

**内存限制**（Go 层面）：
```go
// 在执行前设置内存监控
func (s *PythonSandbox) setMemoryMonitor() {
    var m runtime.MemStats
    runtime.ReadMemStats(&m)
    startMem := m.Alloc

    // 执行后检查内存增长
    runtime.ReadMemStats(&m)
    if m.Alloc - startMem > s.maxMemory {
        panic("memory limit exceeded")
    }
}
```

**内存上限语义**：`runtime.MemStats` 反映的是 **Go 进程堆**，不能等价于 gpython 子解释器的隔离配额；上述片段仅作粗监控示意，**不应**在对外承诺中写为「硬沙箱内存上限」。若需强保证，应评估进程级隔离或不在 MVP 承诺内存封顶。

### 3.8 错误信息设计

| 场景 | 错误信息 |
|-----|---------|
| **语法层拒绝（import/def/class）** | "语法错误：输入必须是 Python 表达式，而非语句。'{detail}'" |
| **禁止函数** | "安全违规：函数 '{func}' 不被允许。请检查表达式。" |
| **危险属性访问** | "安全违规：属性 '{attr}' 访问不被允许。" |
| **执行超时** | "表达式执行超时（{timeout}秒）。请简化表达式或减少循环次数。" |
| **语法错误（解析失败）** | "Python 语法错误：{details}" |
| **运行时错误（类型/未定义）** | "{details}。请确保引用的字段 '{var}' 存在且有值。" |

---

## 四、综合建议

### 4.1 设计决策汇总

| 决策项 | 方案 | 实现优先级 |
|-------|-----|----------|
| **MVP 范围** | MVP 仅启用 SQL 表达式；Python 表达式保留研究结论并延期落地 | P0 |
| **字段依赖 DAG** | 表级依赖 + 字段级依赖，双重拓扑排序 | P1 |
| **环检测** | 配置保存时预检 + 执行前二次校验 | P0 |
| **字段引用语法** | `${column}` 占位符格式（SQL/Python 统一） | P0 |
| **SQL 引用解析** | 正则/扫描占位符生成依赖；与 SQL AST 解析解耦 | P0 |
| **SQL 方言** | 目标库验证 + 模拟值代入（§2.3.1：参数化、离线策略、语义边界） | P1 |
| **Python 沙箱** | 语法层(Eval模式) + 模块预注入 + 函数黑名单 + 超时（后续阶段） | P2 |

### 4.2 下一步行动

1. ~~**占位符语法规范化**~~：✅ 已确认 `\w+`（字母、数字、下划线），不支持特殊字符或中文列名
2. **调研 gpython 实际能力**：验证 `ParseExpr` API、AST 遍历、scope 注入、超时机制是否可行（后续阶段任务，不阻塞 MVP）
3. **原型验证**：实现占位符解析器 + SQL 表达式验证器（参数化代入）；Python 沙箱原型纳入后续阶段
4. ~~**完善文档**~~：✅ 已在 `docs/execution-engine.md` §2.5 补充双层依赖关系
5. **前端交互设计**：设计表达式编辑器 UI，支持 `${column}` 占位符提示、实时验证和模拟值预览
6. ~~**产品边界确认**~~：✅ 已确认：
   - 无连接场景：禁止保存（错误扼杀在设计阶段）
   - 子查询禁令：MVP 产品约束（§2.4 已写明）
   - `random` 模块：预注入，种子与全局生成器统一

---

## 五、附录：表达式示例

### SQL 表达式示例（MySQL）

| 场景 | 表达式 |
|-----|-------|
| **订单编号** | `CONCAT('ORD-', DATE_FORMAT(${order_time}, '%Y%m%d'), LPAD(${seq}, 4, '0'))` |
| **邮箱拼接** | `CONCAT(${username}, '@', ${domain})` |
| **金额格式化** | `FORMAT(${amount}, 2)` |
| **状态映射** | `CASE WHEN ${status}=1 THEN 'active' ELSE 'inactive' END` |
| **日期提取** | `YEAR(${created_at})` |

### Python 表达式示例

| 场景 | 表达式 |
|-----|-------|
| **数值运算** | `${base_salary} * (1 + ${bonus_rate})` |
| **字符串处理** | `${name}.upper() + '_' + str(${id})` |
| **日期计算** | `datetime.datetime.now().strftime('%Y%m%d')` |
| **正则匹配** | `re.sub(r'\s+', '_', ${title})` |
| **条件表达式** | `'VIP' if ${score} > 100 else 'Normal'` |

---

_Document decisions and patterns, not just implementation._