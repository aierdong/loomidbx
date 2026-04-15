# Requirements Document

## Introduction

本规格定义 **spec-03-generator-framework（生成器框架与字段规则）** 的需求边界：在 `spec-02` 已完成当前 schema 扫描与同步的前提下，系统提供“单字段/单表层面”的可生成能力，包括生成器接口、注册机制、类型生成器、字段级配置与预览接口。

**范围与边界（冻结）**

- **包含**：Generator 统一接口、注册与发现机制、按字段类型选择生成器、字段级规则配置、预览能力、配置校验与错误模型。
- **不包含**：跨表依赖排序、批量写入事务、失败回滚编排（由 `spec-04` 接续）。
- **间断交付**：交付“可配置、可预览、可扩展的字段生成内核”；不承担跨表执行与持久化写入。
- **依赖**：上游依赖 `spec-02` 提供可信 schema；下游 `spec-04/spec-06/spec-08/spec-09` 消费生成器能力与契约。

## Requirements

### Requirement 1: 生成器接口与注册机制

**Objective:** 作为平台开发者，我希望以统一接口接入和管理生成器，以便新增规则时无需改动执行主干代码。

#### Acceptance Criteria

1. The LoomiDBX Generator Framework shall 定义统一 `Generator` 抽象（至少包括元信息、输入上下文、输出值与可分类错误）。
2. The LoomiDBX Generator Framework shall 提供注册中心能力，支持按生成器 ID、版本与类型标签注册、查询、启停与冲突检测。
3. When 同一生成器 ID 被重复注册且版本策略不兼容, the LoomiDBX Generator Framework shall 返回稳定错误并拒绝覆盖有效注册项。
4. The LoomiDBX Generator Framework shall 支持通过能力声明（例如支持字段类型、是否需要外部 feed、是否可确定性复现）进行筛选，供 UI/FFI 与执行层消费。

### Requirement 2: 类型生成器与字段类型映射

**Objective:** 作为用户，我希望系统可基于字段类型快速选择可用生成器，以便减少手工配置成本。

#### Acceptance Criteria

1. The LoomiDBX Generator Framework shall 内置 MVP 必需的基础类型生成器映射（如字符串、整数、浮点、布尔、日期时间、枚举/集合类）。
2. When 字段 schema 类型发生变化, the LoomiDBX Generator Framework shall 基于最新 schema 重新判定候选生成器集合，并输出不兼容提示。
3. The LoomiDBX Generator Framework shall 支持“类型默认生成器 + 字段显式覆盖”的优先级策略，且行为可预测。
4. If 字段类型无可用生成器, the LoomiDBX Generator Framework shall 返回可定位错误（包含字段标识、类型与建议动作），不得静默降级为随机值。
5. The LoomiDBX Generator Framework shall 提供通用 `ENUM` 候选值生成能力：允许配置候选集合（如 `[1,2,3]`、`["x","y","z"]`），并根据字段类型进行一致性约束。

### Requirement 3: 字段级配置模型与校验

**Objective:** 作为配置者，我希望为每个字段声明生成规则参数，并在保存前完成校验，以便避免执行时失败。

#### Acceptance Criteria

1. The LoomiDBX Generator Framework shall 提供字段级配置模型（至少包含字段标识、生成器 ID、参数、空值策略、种子策略与启用状态）。
2. When 用户提交字段配置, the LoomiDBX Generator Framework shall 执行结构化校验（必填参数、参数类型、取值范围、与字段 schema 兼容性）。
3. If 校验失败, the LoomiDBX Generator Framework shall 返回字段级错误清单，包含错误码、错误路径与修复建议。
4. The LoomiDBX Generator Framework shall 支持按连接/表维度查询与更新字段配置，并保留最小必要审计信息（修改时间、修改来源）。
5. The LoomiDBX Generator Framework shall 对 `modified_source` 采用固定枚举并进行校验，非法取值必须拒绝保存并返回字段级 `INVALID_ARGUMENT` 错误。

### Requirement 4: 预览能力与可复现性

**Objective:** 作为用户，我希望在真正执行写入前预览字段生成结果，并在指定种子时得到可复现输出。

#### Acceptance Criteria

1. The LoomiDBX Generator Framework shall 提供预览接口，支持单字段与单表范围的样本生成，不触发真实写入。
2. When 预览请求包含固定种子, the LoomiDBX Generator Framework shall 在同一配置与输入上下文下返回可复现结果。
3. The LoomiDBX Generator Framework shall 在预览响应中附带元数据（生成器 ID、参数摘要、是否确定性、警告信息）。
4. If 生成器依赖外部 feed 或计算表达式但依赖未就绪, the LoomiDBX Generator Framework shall 返回 `FAILED_PRECONDITION` 类错误并指示对应上游依赖。

### Requirement 5: 边界控制、安全与契约一致性

**Objective:** 作为系统维护者，我希望生成器框架严格保持职责边界和安全输出，以便下游执行层可以安全复用。

#### Acceptance Criteria

1. The LoomiDBX Generator Framework shall 仅负责值生成与预览，不执行跨表顺序编排、批量写入或事务提交。
2. If 外部调用请求涉及跨表执行或写入动作, the LoomiDBX Generator Framework shall 返回范围外错误并提示由 `spec-04` 处理。
3. The LoomiDBX Generator Framework shall 通过 FFI/服务契约输出统一错误模型，不暴露连接凭据、密钥、token 等敏感信息。
4. The LoomiDBX Generator Framework shall 暴露对 `spec-08/spec-09` 的扩展点（外部 feed 与计算字段），且不破坏 MVP 既有接口语义。
