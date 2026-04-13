# LoomiDBX 数据生成器架构设计方案

## Context

**问题背景**：用户需要设计一套数据生成器架构，用于生成逼真的模拟数据。核心挑战包括：

- 统一接口设计
- 函数式 vs 对象式生成方式的选择
- 公共配置的处理
- 生成器类型的完整性
- 扩展性设计

**项目约束**：

- Go 后端 + Flutter 前端，通过 FFI + JSON 通信
- 支持 5 种抽象数据类型：int、string、decimal、datetime、boolean
- 性能目标：≥10,000 条/秒

---

## 1. 统一接口设计

### 1.1 核心接口定义

采用**分层接口设计**，同时支持单值生成和批量生成：

```go
// Generator 生成器接口
type Generator interface {
    Generate(ctx context.Context) (interface{}, error)
    GenerateBatch(ctx context.Context, count int) ([]interface{}, error) 
    Reset() error // 用于持有状态的生成器，例如：序列/自增场景，对其它生成器无效果（不报错）
    Type() GeneratorType
}
```

### 1.2 设计决策理由


| 决策                   | 理由                       |
| -------------------- | ------------------------ |
| `interface{}` 返回值    | 支持 5 种抽象类型，避免泛型复杂性       |
| 单值 + 批量双接口           | 批量满足预览(10条)场景，此外都会使用单值接口 |
| `context.Context` 参数 | 支持超时控制、取消操作、并发协调         |


---

## 2. 生成方式选择：对象式为主、函数式为辅

### 2.1 两种方式对比


| 维度    | 函数式`Generate(config)` | 对象式`NewGenerator(config).Generate()` |
| ----- | --------------------- | ------------------------------------ |
| 初始化开销 | 每次调用解析配置              | 一次解析，多次使用                            |
| 状态管理  | 无状态                   | 有状态（如序列号）                            |
| 并发安全  | 天然安全                  | 需要额外处理                               |
| 适用场景  | 一次性调用、预览              | 批量生成、有状态生成器                          |


### 2.2 推荐方案：混合模式

```go
// 对象式：创建生成器实例（核心流程）
generator := NewIntSequenceGenerator(config)
values := generator.GenerateBatch(ctx, 10000)

// 函数式：快捷调用（预览、测试场景）
// 它只是对象式的语法糖
values := Preview(config, 10)  // 生成10条预览数据
```

**选择依据**：

- 有状态生成器（序列、雪花ID）**必须**使用对象式
- 批量生成避免重复解析配置，使用对象式更高效
- 预览功能（生成10条样本）可使用函数式快捷调用
- 未保持简单，预览功能被设计为对象式的语法糖

---

## 3. 公共配置处理：嵌入组合模式

用户确认公共配置在生成器配置中**显式指定**，而非从 Schema 推断（扫描 Schema 时允许推断，但需要用户确认）。

### 3.1 公共配置项

```go
type CommonOptions struct {
    NullPercentage float64 // Null值比例 (0-1)
    IsArray        bool    // 是否生成数组
    ArrayMin       int     // 数组最小长度
    ArrayMax       int     // 数组最大长度
    ArrayStyle     string  // 数组形式: JSON, 逗号分割, 数据库原生格式（Postgres/Hive）
    Unique         bool    // 唯一性约束
    Formatter      string  // 格式化模板，仅针对输出类型为 string(varchar/text) 的列
    Padding        PaddingConfig // 填充配置
}
```

### 3.2 组合模式实现

```go
type BaseGenerator struct {
    config       *GeneratorConfig
    uniquePool   *UniquePool
    formatter    Formatter
    nullRand     *rand.Rand
}

// ApplyCommonOptions 由具体生成器调用
func (b *BaseGenerator) ApplyCommonOptions(value interface{}) (interface{}, error) {
    // 1. Null值处理
    // 2. 唯一性检查
    // 3. 格式化输出
    // 4. 数组处理
    return value, nil
}

// 具体生成器嵌入 BaseGenerator
type IntSequenceGenerator struct {
    BaseGenerator        // 嵌入组合
    specific   *IntSequenceConfig
    current    int64
}
```

**选择理由**：Go 没有继承，组合是惯用模式；公共处理逻辑集中，各生成器可选择性调用。

---

## 4. 完整生成器类型

```
数据生成器体系
├── 整数生成器
│   ├── 序列生成器 - 自增ID、流水号
│   ├── 范围随机 - 随机整数
│   ├── 概率分布 - 正态/帕累托/对数正态
│   ├── 雪花ID (Snowflake) - 分布式唯一ID
│   └── 位掩码 - 权限位生成
│
├── 浮点生成器
│   ├── 范围随机
│   └── 概率分布
│
├── 字符串生成器
│   ├── 随机字符 - 指定字符集
│   ├── 正则生成 - 按正则规则生成
│   ├── ULID/UUID - 唯一标识
│   ├── 姓名 - 中英文姓名
│   ├── 联系方式 - Email/电话/手机
│   ├── 证件 - 身份证/护照
│   ├── 地理位置 - 国家/城市/地址/邮编
│   ├── 网络 - IP/MAC/URL
│   ├── 商业 - 公司/职位/银行账号/信用卡
│   ├── 外观 - 颜色/头像URL
│   └── 文本 - Lorem/中文段落
│
├── 布尔生成器
│   └── 等概率 - 可配置为 true/false 或自定义字符/数字（例如: 1/0, YES/NO, 是/否）, 可以为 true/false 指定比例
│
├── 时间生成器
│   ├── 序列 - 按步长递增
│   ├── 范围随机 - 指定区间内随机
│   ├── 概率分布 - 时间戳分布
│   ├── 相对时间 - now +/- N天/时/分/秒
│   └── 当前时间
│
└── 通用生成器
    ├── 常数 - 固定值
    ├── 枚举 - 带权重枚举，如果没有指定权重则所有值具有相同权重
    ├── SQL 计算表达式 - 可以引用其它字段
    ├── Python 计算表达式 - 可以引用其它字段
    ├── 外键关联 - 从关联表取值
    ├── 自引用 - 树结构父ID
    ├── 外部数据源 - SQL/JSON文件/REST API
    └── AI生成 - LLM 生成语义化内容
```

说明:

- 字符串生成器: 除姓名、城市外，地址、公司名等均为中国（未来将扩展至其它国家）
- 非字符串生成器，可以通过 Formatter 变成字符串输出。字符串类型的字段（varchar/text 等），可以指定非字符串生成器，但必须要指定 Formatter
- SQL计算表达式和Python计算表达式: 可以引用其它字段。表达式的计算在所有字段数据生成后才开始计算，如果一条语句中包含多个表达式生成器。则需要评估字段生成顺序
- 整数类型或字符串类型也可以使用布尔生成器，此时必须指定 true/false 对应的字符/数字

---

## 5. 扩展性设计

### 5.1 注册机制（编译时集成）

用户选择采用注册机制而非 Go plugin 动态加载，优点：

- 编译时集成，性能最优
- 无运行时加载风险
- 类型安全，编译检查

```go
// GeneratorRegistry 全局生成器注册表
type GeneratorRegistry struct {
    constructors map[GeneratorType]GeneratorConstructor
    metadata     map[GeneratorType]*GeneratorMetadata
}

// GeneratorMetadata 生成器元数据（用于前端UI渲染）
type GeneratorMetadata struct {
    Type         GeneratorType `json:"type"`
    DisplayName  string        `json:"display_name"`
    Description  string        `json:"description"`
    DataType     string        `json:"data_type"`
    ConfigSchema *ConfigSchema `json:"config_schema"`  // JSON Schema，前端动态表单
    Supports     []string      `json:"supports"`       // 支持的特性
    Examples     []ConfigExample `json:"examples"`
}

// 注册生成器
func RegisterGenerator(
    gtype GeneratorType,
    constructor GeneratorConstructor,
    metadata *GeneratorMetadata,
) error
```

### 5.2 配置序列化（FFI通信）

```go
// GeneratorConfig 生成器配置（JSON序列化载体）
type GeneratorConfig struct {
    Type            GeneratorType `json:"type"`
    CommonOptions   CommonOptions `json:"common_options"`
    SpecificOptions interface{}   `json:"specific_options"`
}

// FromJSON 从JSON反序列化（支持多态）
func FromJSON(data []byte) (*GeneratorConfig, error) {
    // 第一步：解析类型字段
    // 第二步：根据类型分发到具体配置解析
}
```

### 5.3 ExternalFeed 配置 Schema（同步已决策基线）

> 对齐 `docs/external-data-and-ai-research.md`：统一 5 类 feed，MVP 启用文件/HTTP/SQL，LLM 预留但默认关闭。

```json
{
  "type": "external_feed",
  "enabled": true,
  "kind": "embedded_json|uploaded_json|uploaded_csv|http|sql|llm",
  "mvp_enabled": true,
  "row_limit": 10000,
  "row_limit_env": "LOOMIDBX_EXTERNAL_ROW_LIMIT",
  "on_overflow": "truncate_warn",
  "failure_policy": "hard_fail",
  "config": {
    "auth": {},
    "request": {},
    "extract": {}
  }
}
```

#### 5.3.1 通用字段


| 字段               | 含义    | 说明                                                                |
| ---------------- | ----- | ----------------------------------------------------------------- |
| `kind`           | 数据源类型 | `embedded_json`/`uploaded_json`/`uploaded_csv`/`http`/`sql`/`llm` |
| `row_limit`      | 拉取上限  | 默认 10000，可由环境变量覆盖                                                 |
| `on_overflow`    | 超限策略  | 固定 `truncate_warn`（截断并告警）                                         |
| `failure_policy` | 失败策略  | 固定 `hard_fail`（整任务失败并回滚）                                          |


#### 5.3.2 HTTP 类型

```json
{
  "kind": "http",
  "config": {
    "request": {
      "method": "GET|POST",
      "url": "https://example.com/dataset",
      "headers": {"X-API-Key": "${ENV:API_KEY}"},
      "query": {"tenant": "demo"},
      "timeout_ms": 10000
    },
    "auth": {
      "type": "none|api_key|bearer|oauth2|basic|hmac|digest",
      "in": "header|query",
      "token_url": "https://example.com/oauth/token",
      "token_body": {"grant_type": "client_credentials"}
    },
    "extract": {
      "response_format": "json_array",
      "field_path": "$.name"
    }
  }
}
```

规则：

- HTTP 返回必须可解析为 JSON 数组。
- 通过 `extract.field_path` 提取单列值。
- 任一元素提取结果为 `null`，或最终数组为空，判定失败。

#### 5.3.3 SQL 类型

```json
{
  "kind": "sql",
  "config": {
    "connection": {
      "driver": "mysql|postgres|sqlite|sqlserver|oracle",
      "dsn": "${ENV:SQL_DSN}",
      "username": "${ENV:SQL_USER}",
      "password": "${ENV:SQL_PASSWORD}"
    },
    "auth": {
      "type": "password|dsn|env"
    },
    "request": {
      "query": "SELECT id, name FROM sales_rep WHERE active = 1"
    },
    "extract": {
      "target_column": "name"
    }
  }
}
```

规则：

- SQL 可返回单列或多列，但只保留 `target_column` 对应列。
- `target_column` 不存在、值为 `null`、结果为空时判定失败。

#### 5.3.4 JSON/CSV 文件类型

```json
{
  "kind": "uploaded_csv",
  "config": {
    "request": {
      "path": "E:/data/dict/city.csv",
      "encoding": "utf-8"
    },
    "extract": {
      "column_index": 1,
      "column_name": "city_name"
    }
  }
}
```

规则：

- JSON 文件要求为 JSON 数组，通过 `field_path` 提取单列。
- CSV 文件要求 UTF-8；支持 `column_index` 或 `column_name`（二者同时配置时优先 `column_name`）。

#### 5.3.5 LLM 类型（非 MVP）

- `kind=llm` 保留 schema 但默认不可在 MVP 启用。
- 参数支持直接输入或环境变量引用：`api_key`、`model_name`、`base_url`。
- 仅要求 OpenAI 协议兼容，不纳入当前性能承诺。

### 5.4 扩展步骤

1. 定义新的 `GeneratorType` 常量
2. 定义具体配置结构体（如 `NewGeneratorConfig`）
3. 实现 `Generator` 接口
4. 在 `init()` 中调用 `RegisterGenerator()`

---

## 6. 关键文件结构

```
backend/generator/
├── interface.go            # 核心接口定义（Generator, BatchGenerator等）
├── registry.go             # 注册机制、工厂模式
├── base.go                 # BaseGenerator、CommonOptions、UniquePool
├── serialization.go        # JSON序列化/反序列化（FromJSON、ToJSON）
├── types.go                # GeneratorType 枚举定义
├── metadata.go             # GeneratorMetadata、ConfigSchema定义
│
├── int/
│   ├── sequence.go         # 整数序列生成器
│   ├── range.go            # 整数范围随机生成器
│   ├── distribution.go     # 整数概率分布生成器
│   ├── snowflake.go        # 雪花ID生成器
│   └── bitmask.go          # 位掩码生成器
│
├── decimal/
│   ├── range.go            # 浮点范围随机生成器
│   └── distribution.go     # 浮点概率分布生成器
│
├── string/
│   ├── random.go           # 随机字符生成器
│   ├── regex.go            # 正则生成器
│   ├── ulid.go             # ULID生成器
│   ├── uuid.go             # UUID生成器
│   ├── name.go             # 姓名生成器
│   ├── phone.go            # 电话生成器
│   ├── email.go            # Email生成器
│   ├── idcard.go           # 身份证生成器
│   ├── passport.go         # 护照号码生成器
│   ├── country.go          # 国家生成器
│   ├── province.go         # 省份生成器
│   ├── city.go             # 城市生成器
│   ├── postcode.go         # 邮编生成器
│   ├── address.go          # 地址生成器
│   ├── ip.go               # IP地址生成器
│   ├── mac.go              # MAC地址生成器
│   ├── url.go              # URL生成器
│   ├── company.go          # 公司名生成器
│   ├── job.go              # 职位生成器
│   ├── bank.go             # 银行账号生成器
│   ├── cc.go               # 信用卡号生成器
│   ├── color.go            # 颜色生成器
│   ├── avatar.go           # 头像URL生成器
│   └── lorem.go            # Lorem文本生成器
│
├── datetime/
│   ├── sequence.go         # 时间序列生成器
│   ├── range.go            # 时间范围随机生成器
│   ├── distribution.go     # 时间概率分布生成器
│   ├── relative.go         # 相对时间生成器
│   └── current.go          # 当前时间生成器
│
├── boolean/
│   └── boolean.go          # 布尔生成器
│
└── common/
    ├── constant.go          # 常数生成器
    ├── enum.go              # 带权重的枚举生成器
    ├── sql_expression.go    # sql计算表达式生成器
    ├── python_expression.go # python计算表达式生成器
    ├── foreign_key.go       # 外键关联生成器
    ├── self_ref.go          # 子引用生成器
    ├── json_external.go     # 外部数据源（JSON）生成器
    ├── sql_external.go      # 外部数据源（数据库SQL）生成器
    ├── api_external.go      # 外部数据源（API）生成器
    └── ai.go                # AI生成器
```

---

## 7. 实现优先级


| 优先级 | 生成器                                | 理由         |
| --- | ---------------------------------- | ---------- |
| P0  | interface.go, registry.go, base.go | 基础架构，必须先实现 |
| P1  | int/sequence.go                    | 最常用，自增主键   |
| P1  | string/regex.go                    | 灵活通用       |
| P1  | common/enum.go                     | 枚举字段必备     |
| P1  | common/foreign_key.go              | 外键约束核心功能   |
| P2  | int/snowflake.go                   | 分布式ID需求    |
| P2  | datetime/*.go                      | 时间字段常见     |
| P2  | string/name.go, phone.go           | 逼真数据核心卖点   |
| P3  | 其他生成器                              | 按需逐步实现     |


---

## 8. 验证方案

### 单元测试验证

- 每个生成器独立测试：配置解析 → 生成结果验证
- 批量生成性能测试：≥10,000 条/秒
- 并发安全测试：多 goroutine 同时调用

### 集成验证

- JSON 序列化/反序列化测试：Go → Flutter → Go 全链路
- FFI 调用测试：Dart 调用 Go 动态库
- 预览功能测试：前端触发预览，验证返回数据

### 端到端验证

- 连接真实数据库，配置生成规则，生成并写入数据
- 验证外键关系正确性
- 验证唯一性约束

