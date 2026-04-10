---
name: Data Generator Architecture
description: LoomiDBX 数据生成器核心架构设计，包括统一接口、类型体系和扩展机制
type: reference
---

# 数据生成器架构

## 核心接口设计

生成器采用分层接口设计，支持单值和批量生成：

```go
type Generator interface {
    Generate(ctx context.Context) (interface{}, error)
    GenerateBatch(ctx context.Context, count int) ([]interface{}, error)
    Reset() error  // 用于有状态生成器（序列、自增）
    Type() GeneratorType
}
```

**设计决策**：

| 决策 | 理由 |
|------|------|
| `interface{}` 返回值 | 支持 5 种抽象类型（int/string/decimal/datetime/boolean），避免泛型复杂性 |
| 单值 + 批量双接口 | 批量用于预览(10条)场景，单值用于主流程 |
| `context.Context` 参数 | 支持超时控制、取消操作、并发协调 |

---

## 生成方式：对象式为主、函数式为辅

```go
// 对象式：核心流程（有状态生成器必须使用）
generator := NewIntSequenceGenerator(config)
values := generator.GenerateBatch(ctx, 10000)

// 函数式：预览快捷调用（对象式的语法糖）
values := Preview(config, 10)
```

**选择依据**：
- 有状态生成器（序列、雪花ID）**必须**使用对象式
- 批量生成避免重复解析配置，使用对象式
- 预览功能可使用函数式快捷调用

---

## 公共配置处理：嵌入组合模式

```go
type CommonOptions struct {
    NullPercentage float64      // Null值比例 (0-1)
    IsArray        bool         // 是否生成数组
    ArrayMin/Max   int          // 数组长度范围
    ArrayStyle     string       // JSON/逗号分割/数据库原生格式
    Unique         bool         // 唯一性约束
    Formatter      string       // 格式化模板
    Padding        PaddingConfig
}

type BaseGenerator struct {
    config     *GeneratorConfig
    uniquePool *UniquePool
    formatter  Formatter
}

// 具体生成器嵌入 BaseGenerator
type IntSequenceGenerator struct {
    BaseGenerator           // 嵌入组合
    specific    *IntSequenceConfig
    current     int64
}
```

**理由**：Go 没有继承，组合是惯用模式；公共处理逻辑集中，各生成器可选择性调用。

---

## 生成器类型体系

```
数据生成器
├── 整数：序列/范围随机/概率分布/雪花ID/位掩码
├── 浮点：范围随机/概率分布
├── 字符串：随机字符/正则/ULID/UUID/姓名/联系方式/证件/地理位置/网络/商业/文本
├── 布尔：等概率（可配置比例和输出形式：true/false、1/0、YES/NO、是/否）
├── 时间：序列/范围随机/概率分布/相对时间/当前时间
└── 通用：常数/枚举/SQL表达式/Python表达式/外键关联/自引用/外部数据源/AI生成
```

**关键约束**：
- 非字符串生成器通过 Formatter 变成字符串输出
- 字符串字段使用非字符串生成器时必须指定 Formatter
- SQL/Python 表达式可引用其它字段，计算在所有字段生成后执行

---

## 扩展性设计

注册机制（编译时集成，非动态加载）：

```go
type GeneratorRegistry struct {
    constructors map[GeneratorType]GeneratorConstructor
    metadata     map[GeneratorType]*GeneratorMetadata  // 前端UI动态表单
}

func RegisterGenerator(
    gtype GeneratorType,
    constructor GeneratorConstructor,
    metadata *GeneratorMetadata,
) error
```

**扩展步骤**：
1. 定义新的 `GeneratorType` 常量
2. 定义具体配置结构体
3. 实现 `Generator` 接口
4. 在 `init()` 中调用 `RegisterGenerator()`

---

## 文件结构模式

```
backend/generator/
├── interface.go        # 核心接口定义
├── registry.go         # 注册机制、工厂模式
├── base.go             # BaseGenerator、CommonOptions
├── serialization.go    # JSON序列化（FFI通信）
├── types.go            # GeneratorType 枚举
├── metadata.go         # 前端动态表单元数据
├── int/                # 按类型分目录
├── string/
├── datetime/
├── boolean/
└── common/             # 通用生成器
```

---

## 实现优先级

| 优先级 | 生成器 | 理由 |
|--------|--------|------|
| P0 | interface.go, registry.go, base.go | 基础架构，必须先实现 |
| P1 | int/sequence, string/regex, common/enum, common/foreign_key | 最常用、核心功能 |
| P2 | int/snowflake, datetime/*, string/name, string/phone | 分布式ID、时间字段、逼真数据 |
| P3 | 其他生成器 | 按需逐步实现 |

---

## 性能与验证

**性能目标**：>=10,000 条/秒

**验证要点**：
- 单元测试：配置解析 -> 生成结果验证
- 并发安全：多 goroutine 同时调用
- 集成验证：Go <-> Flutter JSON 序列化全链路
- 端到端：真实数据库写入、外键关系、唯一性约束