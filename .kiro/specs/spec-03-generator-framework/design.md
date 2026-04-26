# Design Document: spec-03-generator-framework

## Overview

本设计说明“生成器框架与字段规则”的实现方式。系统基于 `spec-02` 同步后的可信 schema，提供可扩展的 Generator 接口、进程内注册、字段配置模型和预览 API。这里的范围只到单字段/单表取值生成，不处理跨表依赖编排，也不负责批量写入事务。

**用户**：配置数据生成规则的业务用户、平台开发者、FFI/UI 调用方。  
**影响**：新增生成器注册中心、类型映射和配置校验能力；下游执行层可以拿到统一的生成上下文和错误模型。

### 核心概念：生成器、配置与注册（必须先读）

- **生成器（Generator）**：满足 `Generator` 接口的一段代码，负责为**单个或一批**字段生成模拟数据，例如整数序列、随机字符串、UUID。生成器**不是**业务库里的实体，也不是需要单独落库的系统表；它随二进制或模块发布，遵循 `steering/generator.md` 中“编译时集成、非运行时动态插件”的约束。
- **生成器配置（用户侧参数）**：调用某个生成器时传入的参数对象，用来约束生成行为。例如“整数序列生成器”可以接收 `{"start": 1, "step": 1}`。系统在**字段维度**持久化的是“选了哪种生成器 + 这组参数”，不是把生成器本身做成一行数据。
- **注册（Register）**：在**进程内**登记内置生成器实现及其元数据，包括 ID、能力、参数 schema 等。`GeneratorTypeResolver`、`ListGeneratorCapabilities`、`GetFieldGeneratorCandidates` 通过 Registry 查询“某类字段当前可用哪些生成器”。Registry 只负责**发现与路由**，不把生成器定义当作持久化业务数据。
- **持久化边界**：系统只持久化 **`ldb_column_gen_configs`（及同类字段规则表）中的用户配置**。例如 `generator_type` = `IntSequenceGenerator`，`generator_opts` 保存参数序列化后的**字符串**（如上述 JSON 文本）。这里**不**引入“生成器定义表”来重复保存代码级生成器。

### Goals

- 提供统一 Generator 抽象，以及**进程内**注册和发现能力（登记内置实现，不在 DB 中镜像生成器定义）。
- 支持字段级规则配置、校验与快速预览。
- 保证确定性场景可以复现，并保持安全边界和契约稳定。
- 支持 ENUM/集合值场景：通用枚举值生成器接收候选值集合，并按字段类型检查类型一致性。

### Non-Goals

- 不实现跨表依赖拓扑排序（由 `spec-04` 负责）。
- 不实现批量写入、事务提交与回滚机制（由 `spec-04` 负责）。
- 不实现完整 LLM 供给编排（由 `spec-11` 在后续扩展）。

## Architecture

### Existing Architecture Analysis

- `spec-02` 已提供当前 schema 和兼容性闸门，可作为字段配置校验的输入。
- 现有代码已有 schema 与 FFI 基础模块，可以复用错误码和 JSON 响应包装。
- 本 spec 补齐“生成规则定义与预览”这一层，避免把执行职责提前放进来。

### Architecture Pattern & Boundary Map

**选定模式**：领域服务 + 注册中心 + 配置仓储 + 预览应用服务。

```mermaid
flowchart TB
  subgraph ui [Flutter_UI]
    FieldRulePanel[FieldRulePanel]
    PreviewPanel[GeneratorPreviewPanel]
    FFICall[FFI_Bindings]
  end

  subgraph core [Go_libloomidbx]
    FFI[FFI_JSON_Adapters]
    PreviewSvc[GeneratorPreviewService]
    ConfigSvc[GeneratorConfigService]
    Registry[GeneratorRegistry]
    TypeResolver[GeneratorTypeResolver]
    Validator[GeneratorConfigValidator]
    Runtime[GeneratorRuntime]
    SchemaRepo[CurrentSchemaRepository_spec02]
    ConfigRepo[GeneratorConfigRepository]
  end

  subgraph ext [Extensions]
    BuiltinGen[BuiltinGenerators]
    ExternalFeedAdapter[ExternalFeedAdapter_spec08]
    ComputedAdapter[ComputedFieldAdapter_spec09]
  end

  FieldRulePanel --> FFICall
  PreviewPanel --> FFICall
  FFICall --> FFI
  FFI --> ConfigSvc
  FFI --> PreviewSvc
  ConfigSvc --> SchemaRepo
  ConfigSvc --> Validator
  ConfigSvc --> TypeResolver
  ConfigSvc --> Registry
  ConfigSvc --> ConfigRepo
  PreviewSvc --> Runtime
  PreviewSvc --> Registry
  PreviewSvc --> TypeResolver
  PreviewSvc --> ConfigRepo
  Registry --> BuiltinGen
  Runtime --> ExternalFeedAdapter
  Runtime --> ComputedAdapter
```



**边界约束**：

- `GeneratorRegistry`：在进程生命周期内注册和发现**内置生成器实现**，并提供能力声明和冲突检测；不把“生成器目录”持久化成业务数据，也不做跨字段编排。
- `GeneratorConfigService`：负责字段规则读写和校验，不产生最终写库动作。
- `GeneratorPreviewService`：只生成样本和诊断信息，不调用执行写入引擎。
- `GeneratorRuntime`：按单字段/单表上下文调用生成器，不负责跨表排序。

# System Flows

### 字段规则配置与校验流程

```mermaid
sequenceDiagram
  participant UI as Flutter
  participant FFI as FFI
  participant Cfg as GeneratorConfigService
  participant Schema as CurrentSchemaRepository(spec-02)
  participant Resolve as GeneratorTypeResolver
  participant Validate as GeneratorConfigValidator
  participant Repo as GeneratorConfigRepository
  UI->>FFI: SaveFieldGeneratorConfig(request)
  FFI->>Cfg: SaveConfig(request)
  Cfg->>Schema: GetCurrentSchema(connection_id, table, column)
  Schema-->>Cfg: field schema
  Cfg->>Resolve: ResolveCandidates(field schema)
  Resolve-->>Cfg: candidate generators
  Cfg->>Validate: Validate(config, schema, candidates)
  alt 校验通过
    Cfg->>Repo: UpsertFieldConfig(config)
    Repo-->>Cfg: saved
    Cfg-->>FFI: ok + config summary
  else 校验失败
    Cfg-->>FFI: INVALID_ARGUMENT + field errors[]
  end
```



### 预览生成流程（单字段/单表）

```mermaid
sequenceDiagram
  participant UI as Flutter
  participant FFI as FFI
  participant Preview as GeneratorPreviewService
  participant Repo as GeneratorConfigRepository
  participant Registry as GeneratorRegistry
  participant Runtime as GeneratorRuntime
  UI->>FFI: PreviewGeneration(connection_id, scope, seed, sample_size)
  FFI->>Preview: Preview(request)
  Preview->>Repo: LoadConfigs(scope)
  Repo-->>Preview: field configs
  Preview->>Registry: Resolve(config.generator_type)
  Registry-->>Preview: generator instance + capability
  Preview->>Runtime: GenerateSample(config, seed, sample_size)
  Runtime-->>Preview: values + warnings
  Preview-->>FFI: preview result + metadata
  FFI-->>UI: render samples and diagnostics
```



### Schema 同步后立即全量重判定流程（A 方案）

```mermaid
sequenceDiagram
  participant Sync as SchemaSync(spec-02)
  participant FFI as FFI
  participant Recheck as CompatibilityRecheckService
  participant Schema as CurrentSchemaRepository(spec-02)
  participant Repo as GeneratorConfigRepository
  participant Resolve as GeneratorTypeResolver
  participant Validate as GeneratorConfigValidator
  participant Risk as CompatibilityRiskReporter
  Sync->>FFI: ApplySchemaSync(connection_id, diff)
  FFI->>Sync: 持久化 current schema（成功）
  Sync-->>FFI: sync_applied
  FFI->>Recheck: RevalidateAllConfigs(connection_id)
  Recheck->>Repo: ListConfigsByConnection(connection_id)
  Repo-->>Recheck: all field configs
  loop 每个字段配置
    Recheck->>Schema: GetCurrentSchema(connection_id, table, column)
    Recheck->>Resolve: ResolveCandidates(field schema)
    Recheck->>Validate: Validate(config, schema, candidates)
  end
  Recheck->>Risk: BuildIncompatibilityReport(results)
  Risk-->>FFI: report + blocking/non_blocking summary
  FFI-->>Sync: schema_sync_result + incompatibility_report
```

约束：

- `ApplySchemaSync` 成功后必须立即触发重判定，不能只依赖后续读取时惰性触发。
- 重判定覆盖该 `connection_id` 下的全部字段配置，结果按字段聚合返回。
- 发现不兼容时，必须返回字段定位信息（`connection_id + table + column`）和建议动作，不能静默跳过。
- 如果风险达到阻断级别，需要联动 `spec-02` 的可信度状态，进入 `pending_adjustment`，并阻断后续执行链路。

## Requirements Traceability


| Requirement | Summary     | Components                                                                  | Interfaces                                            | Flows      |
| ----------- | ----------- | --------------------------------------------------------------------------- | ----------------------------------------------------- | ---------- |
| 1.x         | 统一接口与注册     | GeneratorRegistry, GeneratorRuntime                                         | `RegisterBuiltinGenerator`（进程内）, `ListGeneratorCapabilities` | 配置与校验、预览生成 |
| 2.x         | 类型映射与候选选择   | GeneratorTypeResolver, GeneratorRegistry                                    | `GetFieldGeneratorCandidates`                         | 配置与校验      |
| 3.x         | 字段配置与校验     | GeneratorConfigService, GeneratorConfigValidator, GeneratorConfigRepository | `SaveFieldGeneratorConfig`, `GetFieldGeneratorConfig` | 配置与校验      |
| 4.x         | 预览与可复现性     | GeneratorPreviewService, GeneratorRuntime                                   | `PreviewGeneration`                                   | 预览生成       |
| 5.x         | 边界、安全、契约一致性 | FFI JSON Adapters, GeneratorPreviewService                                  | `PreviewGeneration`, `SaveFieldGeneratorConfig`       | 全流程        |


## Components and Interfaces

### Summary


| Component                   | Domain   | Intent                      | Req Coverage       | Key Dependencies                    | Contracts      |
| --------------------------- | -------- | --------------------------- | ------------------ | ----------------------------------- | -------------- |
| GeneratorRegistry           | Go 领域层   | 进程内登记内置生成器、能力查询与冲突检测（非持久化「生成器目录表」） | 1.x, 2.x           | BuiltinGenerators, extensions       | Domain Service |
| GeneratorTypeResolver       | Go 领域层   | 根据字段 schema 解析候选生成器         | 2.x, 3.x           | Current schema (spec-02), Registry  | Domain Service |
| CompatibilityRecheckService | Go 应用层   | 在 schema 同步成功后立即全量重判定字段配置兼容性 | 2.x, 3.x, 5.x      | Current schema, Validator, ConfigRepository | Service        |
| GeneratorConfigValidator    | Go 领域层   | 校验字段级配置合法性                  | 3.x                | TypeResolver, schema metadata       | Domain Service |
| GeneratorConfigService      | Go 应用层   | 编排配置读写与错误映射                 | 1.x, 2.x, 3.x, 5.x | Validator, ConfigRepository         | Service        |
| GeneratorPreviewService     | Go 应用层   | 生成样本、汇总预览元数据                | 4.x, 5.x           | Runtime, Registry, ConfigRepository | Service        |
| GeneratorRuntime            | Go 运行时层  | 执行单字段生成调用                   | 1.x, 4.x, 5.x      | Generator plugins, adapters         | Runtime        |
| EnumValueGenerator（Builtin） | Go 生成器层  | 基于候选值集合输出枚举值，支持数值/字符串等类型化输出 | 2.x, 3.x, 4.x      | Runtime, Validator                  | Generator      |
| FFI JSON Adapters           | Go FFI 层 | 输出稳定 JSON 契约与脱敏错误           | 3.x, 4.x, 5.x      | ConfigService, PreviewService       | API            |


### API Contract（逻辑签名，非最终实现）

#### 进程内注册（仅 Go 启动/内置模块 init，非 FFI 持久化）

内置生成器在进程启动阶段调用注册入口，把**实现与元数据**登记到 `GeneratorRegistry`。这和“用户保存字段规则”无关，也**不**向应用元数据表写入独立的“生成器定义”实体。

| Method                     | Request 要点                              | Response      | Errors                                   |
| -------------------------- | ----------------------------------------- | ------------- | ---------------------------------------- |
| `RegisterBuiltinGenerator` | `generator_type`, `type_tags[]`, `capability` | `registered` | `INVALID_ARGUMENT`, `GENERATOR_CONFLICT` |

#### 对外 FFI / JSON 契约（稳定）

| Method                         | Request 要点                                                                                                    | Response                                                   | Errors                                    |
| ------------------------------ | ------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------- | ----------------------------------------- |
| `ListGeneratorCapabilities`    | `connection_id?`, `field_type?`                                                                               | `generators[]`                                             | `INVALID_ARGUMENT`, `FAILED_PRECONDITION` |
| `GetFieldGeneratorCandidates`  | `connection_id`, `table`, `column`                                                                            | `candidates[]`, `default_generator`                        | `CURRENT_SCHEMA_NOT_FOUND`, `FAILED_PRECONDITION` |
| `SaveFieldGeneratorConfig`     | `connection_id`, `table`, `column`, `generator_type`, `generator_opts(params)`, `seed_policy`, `null_policy`, `is_enabled`, `modified_source` | `saved`, `config_version`, `is_enabled`, `modified_source`, `warnings[]` | `INVALID_ARGUMENT`, `FAILED_PRECONDITION` |
| `GetFieldGeneratorConfig`      | `connection_id`, `table`, `column`                                                                            | `config`，`pending_*` 时另含 `warnings[]`（只读提示） | `NOT_FOUND`（无已存配置）；`pending_*` **不**用 `FAILED_PRECONDITION`，见 Schema 闸门章节 |
| `PreviewGeneration`            | `connection_id`, `scope(field|table)`, `seed?`, `sample_size`                                                | `samples`（按 `scope.type` 分支：`field -> []interface{}`；`table -> map[string][]interface{}`）, `metadata`, `warnings[]`, `field_results[]`（`scope=table` 必填） | `INVALID_ARGUMENT`, `FAILED_PRECONDITION` |
| `ValidateFieldGeneratorConfig` | `connection_id`, `draft_config`                                                                               | `valid`, `errors[]`                                        | `INVALID_ARGUMENT`, `FAILED_PRECONDITION`                        |

`FAILED_PRECONDITION` 在以下场景包含 **schema 可信度门禁**（见“Schema 可信度与 spec-02 闸门”）。它和 `CURRENT_SCHEMA_NOT_FOUND` 不同：前者表示元数据还在，但当前连接/表暂时不能继续配置或预览。


### Generator 完整定义（与 steering 对齐）

为满足 Requirement 1.1，`spec-03` 在领域层统一约定四类内容：元信息、输入上下文、输出值、分类错误。具体实现仍以 `docs/generator.md` 为准。

```go
// GeneratorMeta 描述生成器静态元信息（逻辑代码能力声明）。
type GeneratorMeta struct {
    // Type: 生成器逻辑类型（强类型枚举），FFI 层序列化为稳定字符串。
    Type GeneratorType
    // TypeTags: 能力标签（如 supports_types、requires_external_feed）。
    TypeTags []string
    // Deterministic: 是否支持确定性复现（固定 seed 下稳定输出）。
    Deterministic bool
}

// GeneratorContext 定义单次生成输入上下文。
type GeneratorContext struct {
    // ConnectionID/Table/Column: 字段定位信息，用于校验与诊断。
    ConnectionID string
    Table        string
    Column       string
    // FieldType: 字段抽象类型（int/string/decimal/datetime/boolean）。
    FieldType string
    // Params: 字段级 generator_opts 解析后的参数对象。
    Params map[string]interface{}
    // Seed: 本次调用有效种子（可能来自 preview 覆盖/字段策略/全局策略）。
    Seed *int64
}

// GeneratorErrorCode 定义可分类错误码，供 Runtime/FFI 统一映射。
type GeneratorErrorCode string

const (
    GeneratorErrInvalidArgument    GeneratorErrorCode = "INVALID_ARGUMENT"
    GeneratorErrFailedPrecondition GeneratorErrorCode = "FAILED_PRECONDITION"
    GeneratorErrUnsupported        GeneratorErrorCode = "UNSUPPORTED_GENERATOR"
    GeneratorErrNotRegistered      GeneratorErrorCode = "GENERATOR_NOT_REGISTERED"
)

// GeneratorError 为生成器领域错误的统一封装。
type GeneratorError struct {
    // Code: 机器可判定的稳定错误码。
    Code GeneratorErrorCode
    // Path: 可选错误路径（如 params.values[2]）。
    Path string
    // Message: 面向用户的可读描述（不得包含敏感信息）。
    Message string
}

func (e *GeneratorError) Error() string { return e.Message }

// Generator 是统一的值生成接口。
type Generator interface {
    // Meta: 返回生成器静态元信息。
    Meta() GeneratorMeta
    // Generate: 基于单次上下文生成一个值；用于单值预览或运行时逐条调用。
    Generate(ctx context.Context, in GeneratorContext) (interface{}, error)
    // GenerateBatch: 基于同一上下文批量生成 count 个值；用于预览批量样本。
    GenerateBatch(ctx context.Context, in GeneratorContext, count int) ([]interface{}, error)
    // Reset: 重置内部状态（如序列游标、缓存随机源）；用于新会话或重试前清理。
    Reset() error
}
```

说明：业务逻辑返回分类错误时，建议使用 `*GeneratorError`。未知错误交给 Runtime 统一处理。FFI JSON 契约层不判断错误原因，只负责把错误映射成稳定输出，并在输出前完成脱敏。

术语对齐：`generator_type` 是 `GeneratorType` 的稳定字符串序列化值。配置、预览和能力查询契约都使用这个字符串。

注册语义：`RegisterGenerator` 是进程启动期或模块初始化阶段使用的内部注册抽象，属于编译时集成；它不提供运行时热插拔注册能力。

`GeneratorRuntime` 调用约束：

- `PreviewGeneration(sample_size=1)`：优先调用 `Generate(ctx)`；如果生成器只优化了批量路径，可以等价路由到 `GenerateBatch(ctx, 1)`。
- `PreviewGeneration(sample_size>1)`：调用 `GenerateBatch(ctx, count)`，并保证返回顺序和请求上下文一致。
- 每次**预览请求**开始时，Runtime 必须先对有状态生成器执行一次 `Reset()`，再按 seed 策略初始化上下文随机源，避免沿用上次游标，导致相同 seed 无法复现。
- 每次**生成请求**（生产执行入口，见 `spec-04`）开始时也必须先执行一次 `Reset()`；同一请求内禁止重复 `Reset()`，避免序列意外回退。
- Runtime 只负责调用生成器和统一处理错误，不承载跨表调度与写入行为（边界仍归 `spec-04`）。
- 当 `scope=table` 预览出现字段级失败时，服务返回“部分成功”结果：成功字段样本照常返回，失败或跳过的字段不能静默丢失。
- `scope=table` 响应必须包含 `field_results[]`，至少包含：`field`、`status`（`ok|skipped|failed`）、`error_code?`、`warning?`；其中 `status=ok` 的字段必须与 `samples` 中返回的字段集合一致。

`scope=table` 标准契约样例（`PREVIEW_TABLE_PARTIAL_SUCCESS_V1`）：

```json
{
  "samples": {
    "id": [1001, 1002, 1003],
    "name": ["Alice", "Bob", "Carol"]
  },
  "metadata": {
    "scope": {
      "type": "table",
      "table": "users"
    },
    "sample_size": 3,
    "seed": 42,
    "generated_at": "2026-04-16T10:15:30Z",
    "partial_success": true
  },
  "warnings": [
    {
      "code": "GENERATOR_DISABLED",
      "field": "email",
      "message": "field generator is disabled by config"
    }
  ],
  "field_results": [
    {
      "field": "id",
      "status": "ok",
      "sample_count": 3,
      "error_code": null,
      "warning": null
    },
    {
      "field": "name",
      "status": "ok",
      "sample_count": 3,
      "error_code": null,
      "warning": null
    },
    {
      "field": "email",
      "status": "skipped",
      "sample_count": 0,
      "error_code": null,
      "warning": "GENERATOR_DISABLED"
    },
    {
      "field": "phone",
      "status": "failed",
      "sample_count": 0,
      "error_code": "INVALID_ARGUMENT",
      "warning": "params.pattern is invalid"
    }
  ]
}
```

### 类型映射规则补充（MVP）

- 数据库原生 `ENUM/SET`（或等价集合类型）映射为“集合约束字段”，候选生成器集合中必须包含 `EnumValueGenerator`。
- 默认/推测生成器只在 schema 扫描后的初始建议阶段产生，例如按抽象类型或列名语义给出建议。最终生效的规则以用户确认后的字段配置为准。
- MVP 默认映射基线（抽象类型 -> 默认 `generator_type`）：
  - `int` -> `int_range_random`
  - `decimal` -> `decimal_range_random`
  - `string` -> `string_random_chars`
  - `boolean` -> `boolean_ratio`
  - `datetime` -> `datetime_range_random`
- `EnumValueGenerator` 是通用生成器，不绑定单一抽象类型；它的 `params.values[]` 元素类型必须与字段抽象类型兼容。
- 兼容性规则示例：
  - `int` 字段可使用 `[1,2,3]`；
  - `string` 字段可使用 `["x","y","z"]`；
  - `decimal/datetime/boolean` 字段同理按目标抽象类型校验。
- 当 `params.values[]` 出现混合类型，或与字段类型不兼容时，校验阶段返回字段级 `INVALID_ARGUMENT`，并给出错误路径（如 `params.values[2]`）。

## Data Models

### Logical Data Model

- **内置生成器目录（非持久化）**：各生成器实现及其元数据（ID、能力、参数 schema 等）随代码发布；进程启动时写入 `GeneratorRegistry` 内存结构。这里**不**落库成“生成器定义”业务实体。
- `field_generator_configs`（逻辑模型）：按连接/表/字段维度保存配置。内容包括：**选用的生成器类型标识**（`generator_type`，与 `docs/schema.md` 一致）、**用户参数**（序列化字符串，如 `{"start":1,"step":1}`）、空值策略、种子策略、启用状态（`is_enabled`）、配置修订号、更新时间、修改来源和可选修改人。
- `generation_preview_sessions`（运行时模型）：保存预览请求上下文、样本结果和警告信息，只在运行时使用，不作为执行历史的主数据源。

字段定位语义：FFI/API 入参可以使用 `connection_id + table + column`，方便调用侧定位字段。仓储层写入 `ldb_column_gen_configs` 前，必须先解析出唯一的 `column_schema_id`，并以它作为持久化唯一键。

`field_generator_configs` 审计字段约束（固定枚举）：

- `modified_source`（必填，固定枚举）：
  - `ui_manual`：用户在 UI 手工修改；
  - `automap`：来自扫描后的自动映射建议/应用；
  - `schema_sync_migration`：schema 同步过程中的迁移调整；
  - `import_restore`：来自导入配置或恢复操作；
  - `system_patch`：系统级修复任务写入。
- `modified_by`（可选）：操作者标识（本地单机可为空）。
- `is_enabled`（必填，布尔）：表示字段规则是否启用；为 `false` 时，该字段在预览和执行准备阶段跳过生成器调用，并在响应 `warnings[]` 中返回 `GENERATOR_DISABLED` 提示。

### Physical DDL Alignment（MVP 持久化草案）

说明：本节把逻辑模型映射到 steering 已存在的表；完整方言细节最终以 `docs/schema.md` 和 migration 为准。

**参数与策略列的存储类型（跨 SQLite / MySQL / Postgres 统一）**：`generator_opts`、`seed_policy` 等在逻辑上是 JSON 对象，但**物理列统一使用字符串类型**（如 `VARCHAR`/`TEXT`），内容是 UTF-8 JSON 文本（示例：`{"start":1,"step":1}`）。应用层负责解析和校验；存储层只负责透明存取，**不**依赖各数据库的原生 `JSON` 类型，以减少方言差异和迁移成本。

#### 1) `field_generator_configs` -> 对齐 `ldb_column_gen_configs`

实现时不新增名为 `field_generator_configs` 的实体，而是直接落到 `ldb_column_gen_configs`，保证与 `docs/schema.md` 一致。

```sql
-- 仅列出 spec-03 必需字段（表可能包含其他既有列）
-- generator_opts / seed_policy：TEXT/VARCHAR，存 JSON 文本而非 JSON 原生类型
-- 注意：`generator_type`、`generator_opts` 等列已由 docs/schema.md 中 ldb_column_gen_configs 定义；此处仅为 spec-03 追加列的迁移示意
ALTER TABLE ldb_column_gen_configs ADD COLUMN generator_opts TEXT NOT NULL DEFAULT '{}';
ALTER TABLE ldb_column_gen_configs ADD COLUMN null_policy VARCHAR(32) NOT NULL DEFAULT 'respect_nullable';
ALTER TABLE ldb_column_gen_configs ADD COLUMN seed_policy TEXT NULL;
ALTER TABLE ldb_column_gen_configs ADD COLUMN is_enabled BOOLEAN NOT NULL DEFAULT TRUE;
ALTER TABLE ldb_column_gen_configs ADD COLUMN config_version BIGINT NOT NULL DEFAULT 1;
ALTER TABLE ldb_column_gen_configs ADD COLUMN modified_source VARCHAR(32) NOT NULL DEFAULT 'ui_manual';
ALTER TABLE ldb_column_gen_configs ADD COLUMN modified_by VARCHAR(128) NULL;
ALTER TABLE ldb_column_gen_configs ADD COLUMN updated_at BIGINT NOT NULL;
```

关键约束：

- 唯一键：`UNIQUE(column_schema_id)`（与 `docs/schema.md` 对齐）。
- `modified_source` 固定枚举：`ui_manual | automap | schema_sync_migration | import_restore | system_patch`。
- `null_policy` 建议固定枚举：`respect_nullable | force_non_null | force_null_ratio`。
- `is_enabled` 默认 `TRUE`；为 `FALSE` 时，不参与运行时生成调用。
- `seed_policy` 为 JSON **文本**（结构见“Seed Strategy”章节），用于表达 global/field/preview 的优先级和覆盖关系；读取后先解析成对象，再参与运行时。
- 迁移必须幂等：`StorageDriver`/migration 负责 `IF NOT EXISTS`/版本表策略，避免多环境重复执行 `ADD COLUMN` 失败；具体语句以 `docs/schema.md` 为准。

#### 2) 运行时模型 `generation_preview_sessions`

不新增持久化主表。该模型作为运行时对象返回给 `PreviewGeneration`；必要审计交给下游执行历史体系承接，避免和 `ldb_generation_runs*` 重叠。

### Seed Strategy（可复现机制）

为满足 Requirement 4.2 中“同一配置与输入上下文可复现”的要求，这里统一定义 `seed_policy` 语义。

#### `seed_policy` 结构（字段配置层）

持久化形态为**单列字符串**（JSON 文本）；解析后的逻辑结构如下：

```json
{
  "mode": "inherit_global | fixed",
  "seed": 123456
}
```

- `inherit_global`：字段使用请求或会话的全局 seed。
- `fixed`：字段始终使用自身 `seed`，不随全局 seed 变化。

#### 优先级与覆盖规则

1. 预览请求显式 `seed`（最高优先级，仅本次请求生效）。
2. 字段 `seed_policy.mode=fixed` 且提供 `seed`。
3. 全局 seed（来自运行会话/任务上下文；与 `steering/generator.md` “MVP 支持全局 seed”对齐）。
4. 无 seed：允许非确定性输出，但响应元数据必须标记 `deterministic=false`。

#### Runtime 传递规则

- `GeneratorPreviewService` 构造运行时上下文时注入 `effective_seed`、`seed_source`（preview_override/field_fixed/global/default）。
- `GenerateBatch(ctx, count)` 必须使用同一个 `effective_seed` 作为批次根种子；行级随机序列通过 `row_index` 派生，确保“同 seed + 同配置 + 同输入上下文”得到一致结果。

### Capability Model

- `supports_types[]`：支持的逻辑字段类型。
- `deterministic_mode`：是否支持固定种子复现。
- `requires_external_feed`：是否依赖外部 feed（`spec-08`）。
- `requires_computed_context`：是否依赖计算字段上下文（`spec-09`）。
- `accepts_enum_values`：是否支持“候选值集合”输入（用于 `EnumValueGenerator` 及兼容实现）。

## Schema 可信度与 spec-02 闸门

本模块依赖 `spec-02` 产出的**当前 schema**，把它作为字段类型和约束的事实来源。同时，模块必须遵守 `steering/database-schema.md` 中的 **`schema_trust_state` 状态机**：只要状态不是 `trusted`，下游就不能假定 schema 可信，也不能继续配置或预览生成规则。

### 调用范围

以下入口在门禁未通过时，必须返回**稳定、可机器识别**的失败结果（不得静默降级），方便 UI/FFI 引导用户完成重扫或风险处理：

- `SaveFieldGeneratorConfig`
- `ValidateFieldGeneratorConfig`
- `GetFieldGeneratorCandidates`
- `PreviewGeneration`

`GetFieldGeneratorConfig` **不在**上述“必须返回错误”的集合中：当连接处于 `pending_rescan` / `pending_adjustment` 时，它仍返回 **成功响应**（`ok: true`），并带上已持久化的 `config`，供 UI **只读展示**。同时，响应必须附带 `warnings[]`（至少包含稳定 `code`/`reason`，如 `SCHEMA_TRUST_PENDING_RESCAN` / `SCHEMA_TRUST_PENDING_ADJUSTMENT`），明确说明当前配置不能用于保存、校验通过或预览生成，**不得**在成功体中暗示可以进入执行写入。

`ListGeneratorCapabilities` 不绑定具体连接、表或列，只枚举内置能力，因此可以保持可用，方便离线展示能力说明。仍建议 UI 在同屏展示 trust 风险，由 UI 聚合连接状态。

### 状态映射（与门禁语义）

| `schema_trust_state`（连接/会话上下文） | 用户可见含义 | 本 spec 行为 |
| --- | --- | --- |
| `trusted` | 当前 schema 与治理流程处于可使用状态 | 允许基于 `CurrentSchemaRepository` 拉取列定义并执行校验/预览 |
| `pending_rescan` | 连接等关键配置已变化，需要重扫后才能信任 | 受影响的 `connection_id`：**拒绝**依赖表/列 schema 的写配置、校验、候选与预览；返回 `FAILED_PRECONDITION`。**例外**：`GetFieldGeneratorConfig` 成功 + `warnings[]`（只读）。 |
| `pending_adjustment` | 扫描 Diff 存在待处理风险，需要先调整生成配置再同步 | **拒绝**保存新规则、校验、候选与预览；返回 `FAILED_PRECONDITION`。**例外**：`GetFieldGeneratorConfig` 成功 + `warnings[]`（只读）。 |

### 错误模型（与 `CURRENT_SCHEMA_NOT_FOUND` 区分）

| 场景 | 说明 | 主错误码 | 建议稳定 `reason` / `subcode` |
| --- | --- | --- | --- |
| 持久化的当前 schema 中找不到表/列，或同步未完成导致没有列定义 | 数据事实缺失 | `CURRENT_SCHEMA_NOT_FOUND` 或约定下的 `NOT_FOUND` | `CURRENT_SCHEMA_COLUMN_MISSING`（示例，最终以错误码表为准） |
| schema 行仍存在，但连接处于 `pending_rescan` / `pending_adjustment`（`Save*` / `Validate*` / `GetFieldGeneratorCandidates` / `PreviewGeneration`） | 治理/流程 Gate | `FAILED_PRECONDITION` | `SCHEMA_TRUST_PENDING_RESCAN` / `SCHEMA_TRUST_PENDING_ADJUSTMENT` |
| 同上 `pending_*` 状态，但调用 `GetFieldGeneratorConfig` | 只读展示已存配置 | **成功**（`ok: true`） | `warnings[]` 中携带同上 `reason`（非顶层 `error`） |

FFI JSON 响应中：失败路径在 `error` 对象中携带 **主码 + `reason`**；`GetFieldGeneratorConfig` 在 `pending_*` 下走成功路径，trust 语义放在 **`warnings[]`**。调用方**不得**依赖纯英文 `message` 做分支。

### 与流程图的关系

`GeneratorConfigService` / `GeneratorPreviewService` 调用 `CurrentSchemaRepository` 前后，都应向连接上下文查询 trust 状态。对 **保存、校验、候选、预览** 路径：如果处于 `pending_*`，直接**短路返回** `FAILED_PRECONDITION`。对 **`GetFieldGeneratorConfig`**：`pending_*` 时仍装载并返回配置，**只**附加 `warnings[]`，不把 trust 问题提升为顶层错误。

## Error Handling

### Error Strategy

- 配置参数错误：`INVALID_ARGUMENT`。
- schema 行缺失，或列不在当前持久化 schema 内：`CURRENT_SCHEMA_NOT_FOUND`（与上表“事实缺失”一致）。
- schema **可信度**未满足（`pending_rescan` / `pending_adjustment`）：保存、校验、候选、预览等路径返回 `FAILED_PRECONDITION` + 稳定 `reason`；**`GetFieldGeneratorConfig` 例外**：返回成功 + `warnings[]`（见“Schema 可信度与 spec-02 闸门”）。
- 生成器不可用：`UNSUPPORTED_GENERATOR`、`GENERATOR_NOT_REGISTERED`。
- 生成器注册冲突：`GENERATOR_CONFLICT`（同 ID 重复注册时拒绝覆盖）。
- 枚举值类型不兼容或混合类型：`INVALID_ARGUMENT`（字段级错误路径定位到 `params.values[*]`）。
- 扩展依赖未就绪：`FAILED_PRECONDITION`（需要明确提示对应的上游 spec）。
- 边界外调用：`OUT_OF_SCOPE_EXECUTION_REQUEST`（提示交由 `spec-04`）。

### Monitoring

- 指标：配置校验通过率、预览请求耗时、生成器命中率、错误码分布。
- 日志脱敏：只记录连接 ID、表字段标识、生成器类型和请求 ID，不记录凭据或明文敏感参数。

## Testing Strategy

- 单元测试：覆盖注册冲突检测、类型候选解析、配置校验、固定种子复现、schema trust 短路分支。
- 集成测试：覆盖“字段配置保存 -> 预览调用 -> 错误映射”全链路，以及 `trusted` / `pending_rescan` / `pending_adjustment` 下的 FFI 行为。
- 契约测试：验证 FFI JSON 响应结构和错误码稳定性。
- 跨 spec 联调：与 `spec-02` 验证 `ApplySchemaSync` 成功后立即触发 `RevalidateAllConfigs` 并输出不兼容报告；与 `spec-04` 验证边界错误传播；与 `spec-08/spec-09` 验证扩展点兼容。

### Schema Trust 门禁测试场景清单（状态 × 接口）

说明：

- 以下 JSON 是**契约骨架**（字段可按实现补充），测试断言以结构和稳定码为主，不依赖 `message` 文案。
- `error.reason` 与 `warnings[].reason` 必须可机器判定，建议固定为：`SCHEMA_TRUST_PENDING_RESCAN` / `SCHEMA_TRUST_PENDING_ADJUSTMENT`。
- 统一请求上下文示例：`connection_id="c1"`, `table="users"`, `column="name"`。

#### A. `trusted` 状态（门禁通过）

1) `SaveFieldGeneratorConfig`：保存成功

- 输入（示例）：

```json
{
  "connection_id": "c1",
  "table": "users",
  "column": "name",
  "generator_type": "NameGenerator",
  "generator_opts": "{\"locale\":\"zh-CN\"}",
  "seed_policy": "{\"mode\":\"inherit_global\"}",
  "null_policy": "respect_nullable",
  "is_enabled": true,
  "modified_source": "ui_manual"
}
```

- 期望输出（JSON 结构）：

```json
{
  "ok": true,
  "data": {
    "saved": true,
    "config_version": 2,
    "is_enabled": true,
    "modified_source": "ui_manual",
    "warnings": []
  },
  "error": null
}
```

2) `ValidateFieldGeneratorConfig`：校验成功

- 期望输出：

```json
{
  "ok": true,
  "data": {
    "valid": true,
    "errors": []
  },
  "error": null
}
```

3) `GetFieldGeneratorCandidates`：返回候选

- 期望输出：

```json
{
  "ok": true,
  "data": {
    "candidates": [
      {
        "generator_type": "NameGenerator",
        "supports_types": ["string"]
      }
    ],
    "default_generator": "NameGenerator"
  },
  "error": null
}
```

4) `PreviewGeneration`：返回样本

- 期望输出：

```json
{
  "ok": true,
  "data": {
    "samples": ["张三", "李四"],
    "metadata": {
      "deterministic": true,
      "effective_seed": 123456
    },
    "warnings": []
  },
  "error": null
}
```

`samples` 结构分支（由 `scope.type` 决定）：

- `scope.type=field`：`samples: [value1, value2, ...]`
- `scope.type=table`：`samples: {"column_name": [value1, value2, ...], "...": [...]}`

5) `GetFieldGeneratorConfig`：正常读取配置

- 期望输出：

```json
{
  "ok": true,
  "data": {
    "config": {
      "connection_id": "c1",
      "table": "users",
      "column": "name",
      "generator_type": "NameGenerator",
      "generator_opts": "{\"locale\":\"zh-CN\"}",
      "is_enabled": true
    },
    "warnings": []
  },
  "error": null
}
```

#### B. `pending_rescan` 状态（门禁拒绝写/校验/候选/预览）

1) `SaveFieldGeneratorConfig` / `ValidateFieldGeneratorConfig` / `GetFieldGeneratorCandidates` / `PreviewGeneration`

- 期望输出（统一失败骨架）：

```json
{
  "ok": false,
  "data": null,
  "error": {
    "code": "FAILED_PRECONDITION",
    "reason": "SCHEMA_TRUST_PENDING_RESCAN",
    "message": "schema trust gate blocked"
  }
}
```

2) `GetFieldGeneratorConfig`（只读例外：成功 + warnings）

- 期望输出：

```json
{
  "ok": true,
  "data": {
    "config": {
      "connection_id": "c1",
      "table": "users",
      "column": "name",
      "generator_type": "NameGenerator",
      "generator_opts": "{\"locale\":\"zh-CN\"}",
      "is_enabled": true
    },
    "warnings": [
      {
        "code": "FAILED_PRECONDITION",
        "reason": "SCHEMA_TRUST_PENDING_RESCAN"
      }
    ]
  },
  "error": null
}
```

#### C. `pending_adjustment` 状态（门禁拒绝写/校验/候选/预览）

1) `SaveFieldGeneratorConfig` / `ValidateFieldGeneratorConfig` / `GetFieldGeneratorCandidates` / `PreviewGeneration`

- 期望输出（统一失败骨架）：

```json
{
  "ok": false,
  "data": null,
  "error": {
    "code": "FAILED_PRECONDITION",
    "reason": "SCHEMA_TRUST_PENDING_ADJUSTMENT",
    "message": "schema trust gate blocked"
  }
}
```

2) `GetFieldGeneratorConfig`（只读例外：成功 + warnings）

- 期望输出：

```json
{
  "ok": true,
  "data": {
    "config": {
      "connection_id": "c1",
      "table": "users",
      "column": "name",
      "generator_type": "NameGenerator",
      "generator_opts": "{\"locale\":\"zh-CN\"}",
      "is_enabled": true
    },
    "warnings": [
      {
        "code": "FAILED_PRECONDITION",
        "reason": "SCHEMA_TRUST_PENDING_ADJUSTMENT"
      }
    ]
  },
  "error": null
}
```

#### D. `ListGeneratorCapabilities`（补充约定）

该接口不绑定具体表列，可在三种状态下都保持 `ok: true`，用于 UI 离线展示能力。测试只断言结构稳定，不参与 trust gate 失败断言。

- 期望输出：

```json
{
  "ok": true,
  "data": {
    "generators": [
      {
        "generator_type": "NameGenerator",
        "supports_types": ["string"],
        "deterministic_mode": true
      }
    ]
  },
  "error": null
}
```

## Supporting References

- 规划来源：`[SPECS_PLANNING.md](../../../SPECS_PLANNING.md)`
- 上游依赖：`[spec-02-schema-scan-and-diff](../spec-02-schema-scan-and-diff/spec.json)`

