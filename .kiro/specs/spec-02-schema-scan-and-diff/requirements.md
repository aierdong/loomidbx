# Requirements Document

## Introduction

本规格定义 **spec-02-schema-scan-and-diff（Schema 扫描、Diff 与同步）** 的需求边界：在已具备稳定连接能力（spec-01）的前提下，系统只持久化“当前生效的 schema 元数据与生成器配置”，每次扫描仅在内存中形成临时快照，用于与当前持久化 schema 对比并提示兼容性风险。

**范围与边界（调整后）**

- **包含**：全库/单表扫描、内存快照构建、与当前 schema 的 Diff、兼容性风险提示、UI Diff 展示、按风险级别自动/手动同步当前 schema。
- **不包含**：历史快照留存、快照审计表、快照版本回溯、字段生成规则执行、数据写入事务与执行编排（spec-03/spec-04）。
- **间断交付**：交付“当前结构可同步、变化可提示、不兼容可感知”；不承担数据生成与执行。
- **依赖**：上游依赖 spec-01 提供连接与凭据；下游 spec-03/spec-04/spec-05/spec-07 消费当前 schema 与兼容性状态。

## Requirements

### Requirement 1: 扫描任务创建与执行边界

**Objective:** 作为用户，我希望发起全库或单表扫描并看到实时状态，以便快速发现数据库结构变化。

#### Acceptance Criteria

1. When 用户选择“全库扫描”或“单表扫描”, the LoomiDBX Schema 扫描子系统 shall 基于指定连接创建扫描任务运行时上下文，并记录任务 ID、目标范围、发起时间与执行状态（仅运行时维护，不落库为独立任务历史表）。
2. The LoomiDBX Schema 扫描子系统 shall 在同一连接上下文内按确定性顺序读取表、列、主键、唯一约束、外键元数据，并在内存中生成统一逻辑 Schema 结构。
3. If 扫描过程中发生权限不足、连接中断或方言不支持, the LoomiDBX Schema 扫描子系统 shall 返回可分类错误信息，且不得将不完整结果标记为成功完成。
4. While 扫描任务执行中, the LoomiDBX Schema 扫描子系统 shall 暴露可查询状态（至少包含 running/completed/failed/cancelled），供 UI 与 FFI 消费。

### Requirement 2: 当前 Schema 持久化语义

**Objective:** 作为系统维护者，我希望系统只维护一份当前生效 schema 元数据，以便下游模块始终基于同一真实状态工作。

#### Acceptance Criteria

1. The LoomiDBX Schema 扫描子系统 shall 持久化“当前生效 schema 元数据”（按连接维度），并支持按连接快速查询当前表/字段结构。
2. When 本次 Diff 满足同步条件（无阻断风险，自动或单击确认）, the LoomiDBX Schema 扫描子系统 shall 以事务方式更新当前 schema 元数据，替换旧状态为新状态。
3. If 同步写入失败, the LoomiDBX Schema 扫描子系统 shall 返回明确存储错误，且当前 schema 元数据保持同步前状态不变。
4. The LoomiDBX Schema 扫描子系统 shall 不持久化每次扫描产生的完整临时快照，也不维护独立快照/审计历史表；仅维护当前 schema 元数据与必要状态字段（如最后扫描时间、最后同步时间、可信度状态）。

### Requirement 3: 内存快照 Diff 与变化分类

**Objective:** 作为用户，我希望将“本次扫描结果”与“当前持久化 schema”对比，并清楚知道新增、删除、修改项。

#### Acceptance Criteria

1. When 扫描完成, the LoomiDBX Diff 子系统 shall 以“内存扫描快照”对比“当前持久化 schema”，输出新增/删除/修改三级变化分类。
2. The LoomiDBX Diff 子系统 shall 在“修改”类别下细分列级变更（类型、可空性、默认值、约束关联变化）并输出机器可消费结构。
3. If 当前持久化 schema 缺失或损坏, the LoomiDBX Diff 子系统 shall 返回可分类错误或初始化提示（首扫场景），且不得输出上下文缺失的半成品 Diff。
4. The LoomiDBX Diff 子系统 shall 支持“全库 Diff”与“单表 Diff”两种视角，且同一算法语义保持一致。

### Requirement 4: 生成器兼容性提示与用户确认

**Objective:** 作为用户，我希望在 schema 变化影响生成器配置时得到明确提示，并在调整后再完成同步。

#### Acceptance Criteria

1. When Diff 结果影响已有生成器配置（例如字段删除、类型不兼容、约束变化）, the LoomiDBX Schema 扫描子系统 shall 产出可定位的风险提示清单（包含对象、原因、建议动作）。
2. The LoomiDBX Schema 扫描子系统 shall 始终将 Diff 结果通过 UI 呈现给用户（无论是否可自动同步），至少包含变更摘要与风险级别。
3. If 本次 Diff 不包含阻断级兼容性风险, the LoomiDBX Schema 扫描子系统 shall 允许直接落库覆盖当前 schema 元数据（自动或单击确认），并将状态标记为已同步。
4. If 存在阻断级兼容性风险且用户未完成调整, the LoomiDBX Schema 扫描子系统 shall 阻止落库与后续生成执行流程，并返回稳定状态错误与调整建议。
5. The LoomiDBX Schema 扫描子系统 shall 在“无生成器配置”场景返回“无风险/未配置”状态，而非将其视为错误。
6. The LoomiDBX Schema 扫描子系统 shall 在契约层保证不暴露凭据明文与连接敏感信息，仅输出结构变化与兼容性元数据。

### Requirement 5: 重扫策略与范围控制

**Objective:** 作为用户，我希望在结构变化后按需重扫，并确保扫描模块不会越界执行数据生成或写入。

#### Acceptance Criteria

1. When 用户触发重扫, the LoomiDBX Schema 扫描子系统 shall 支持“全量重扫”与“按受影响表重扫”策略，并记录触发原因与范围。
2. The LoomiDBX Schema 扫描子系统 shall 在重扫后生成新的内存快照并重新计算 Diff，供用户决定是否同步到当前 schema 元数据。
3. The LoomiDBX Schema 扫描子系统 shall 明确不执行任何字段值生成或数据写入操作；若外部调用要求执行写入，系统应返回范围外错误并提示由 spec-03/spec-04 负责。
4. Where 连接配置变化导致当前 schema 可信度下降, the LoomiDBX Schema 扫描子系统 shall 进入明确的可信度状态机（至少包含 `trusted`、`pending_rescan`、`pending_adjustment`），并在恢复到 `trusted` 前提示或阻断下游执行。

