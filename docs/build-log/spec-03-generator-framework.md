> /validate-design: 当前spec: spec-03-generator-framework，请检查设计一致性

## Design Review Summary

`spec-03-generator-framework` 的设计整体方向与需求边界基本一致：单字段/单表生成、注册中心、配置校验、预览链路都已覆盖，且与 `spec-02`、`spec-04`、`spec-08/09` 的上下游边界有明确声明。
同时，当前版本在“注册契约细化、审计字段完整性、类型映射覆盖度”上还有 3 个会影响实现一致性的关键缺口。
综合判断：可进入实现，但建议先做小幅设计修订（带条件 GO），避免后续返工。

## Critical Issues (≤3)

🔴 Critical Issue 1: 注册中心契约未完整体现版本与生命周期操作
Concern: 需求要求支持按 `ID + 版本 + 类型标签` 注册/查询，并支持启停与冲突检测；设计里的 API 列表未显式给出 `Register/Enable/Disable` 等生命周期接口，仅体现查询类接口。
Impact: 实现阶段可能出现“先写运行时，再补注册治理”的逆向开发，导致插件扩展和冲突策略不稳定。
Suggestion: 在 API Contract 增补 `RegisterGenerator`、`EnableGenerator`、`DisableGenerator`、`ResolveConflictPolicy`（含版本兼容策略字段）。
Traceability: Requirement 1（1.2、1.3）
Evidence: `design.md` 的 “API Contract（逻辑签名）”“Components and Interfaces/GeneratorRegistry”

🔴 Critical Issue 2: 字段配置审计模型缺少“修改来源”落位定义
Concern: 需求要求最小审计信息包含“修改时间 + 修改来源”；设计数据模型只明确了更新时间，没有给出 `modified_by/source` 的字段或来源枚举。
Impact: 后续 UI/FFI 回显与问题追溯能力不足，尤其在多人协作或自动映射覆盖场景中难以判责。
Suggestion: 在 `field_generator_configs` 逻辑模型增加 `modified_source`（如 `ui/manual`、`automap`、`migration`）与 `modified_by`（可选）字段定义，并在 `SaveFieldGeneratorConfig` 返回中透出。
Traceability: Requirement 3（3.4）
Evidence: `design.md` 的 “Data Models/Logical Data Model”“API Contract/SaveFieldGeneratorConfig”

🔴 Critical Issue 3: MVP 类型映射未显式覆盖“枚举/集合类”
Concern: 需求要求内置基础类型映射包含“枚举/集合类”；设计只给出 5 类抽象类型，未说明 enum/set 在 resolver 中的归约策略与默认生成器。
Impact: 遇到 MySQL `ENUM/SET` 或等价类型时候选集不确定，可能触发错误或不一致行为。
Suggestion: 在 `GeneratorTypeResolver` 规则补充“数据库原生枚举/集合 -> 抽象类型（通常 string）-> 默认生成器”的显式映射表，并在候选 API 中返回“映射依据”。
Traceability: Requirement 2（2.1、2.3）
Evidence: `design.md` 的 “Data Models/Capability Model”“Components/GeneratorTypeResolver”；`requirements.md` 的 2.x 条款

## Design Strengths

- 边界控制做得很清楚：明确“不做跨表编排/写入”，与 `execution-engine` steering 和 Requirement 5 高一致。
- 可测试性与可演进性良好：已给出单测/集成/契约/跨 spec 联调策略，并预留 `spec-08/spec-09` 扩展点，风险可控。

## Final Assessment

Decision: GO（带条件）
Rationale: 架构主干、职责分层、流程闭环已达到可实施标准；3 个问题都属于“契约细化缺口”，修订成本低但收益高，不会推翻当前设计。
Next Steps: 先快速修订 `design.md`（补 API、审计字段、枚举映射说明）后进入任务实现；如你愿意，我可以下一步直接给出这 3 处的最小改动草案（按章节逐条可贴回文档）。

## Interactive Discussion

为保证你后续实现顺畅，我建议你先确认两点：

- 你希望 `modified_source` 采用固定枚举（推荐）还是自由文本？
- `ENUM/SET` 默认是否统一走 `string` 生成器，还是希望单独加 `enum` 类型生成器入口？

---

> 问题澄清：
>
> 1. modified_source 采用固定枚举
> 2. ENUM 是一个通用生成器，它可以对应任何类型的生成器，取决于字段的类型。例如字段值从这些枚举中获取： [1, 2, 3...]，['x', 'y', 'z']
>
> 请根据你给出的问题，给出你的建议，也可以直接调整设计文件

已根据你的澄清，直接更新了设计文件：`e:/git/loomidbx/.kiro/specs/spec-03-generator-framework/design.md`。

## 我已做的调整

- `modified_source` 固定枚举化
  - 在数据模型中新增审计约束，`modified_source` 必填且固定枚举：
  - `ui_manual` / `automap` / `schema_sync_migration` / `import_restore` / `system_patch`
  - 同时补充 `modified_by`（可选）语义。
- ENUM 按你定义改为“通用生成器”
  - 新增 `EnumValueGenerator`（Builtin）作为通用候选值生成器。
  - 明确它不绑定单一抽象类型，而是要求 `params.values[]` 与字段类型兼容（例如 `int` 用 `[1,2,3]`，`string` 用 `["x","y","z"]`）。
  - 增加不兼容/混合类型时的字段级错误约束（`INVALID_ARGUMENT` + 错误路径定位）。
- 补齐注册中心契约缺口
  - 在 API Contract 增加 `RegisterGenerator`、`EnableGenerator`、`DisableGenerator`。
  - 在错误策略中补充 `GENERATOR_CONFLICT`（版本策略不兼容时拒绝覆盖）。

## 建议（可选，下一步）

- 建议在后续 `tasks.md`里明确 2 条实现任务：
  1. `modified_source` 的枚举校验与默认值策略（建议不设默认，强制调用方传入）
  2. `EnumValueGenerator` 的类型验证器（含 `params.values[*]` 精确报错路径）

如果你愿意，我下一步可以直接把这两条落到 `tasks.md`（含测试用例粒度）。

如需，我可以继续把你这次确认的两点同步到 `requirements.md`（补一条“`modified_source` 固定枚举”验收标准，和一条“ENUM 通用生成器类型兼容”验收标准），这样需求-设计-任务链路会更闭环。

> 请调整 tasks.md 和 requirements.md，落地这两点

已完成落地，两个文件都已更新并与当前 `design.md` 保持一致。

## 本次修改

- 更新 `e:/git/loomidbx/.kiro/specs/spec-03-generator-framework/requirements.md`
  - 在 Requirement 2 新增`2.5`：
    - 明确 `ENUM` 是通用候选值生成能力，可配置候选集合，并按字段类型做一致性约束。
  - 在 Requirement 3 新增`3.5`：
    - 明确 `modified_source` 必须为固定枚举，非法值拒绝保存并返回字段级 `INVALID_ARGUMENT`。
- 更新 `e:/git/loomidbx/.kiro/specs/spec-03-generator-framework/tasks.md`
  - 新增任务 `2.4`：实现 `EnumValueGenerator` 与参数模型（支持按字段类型的候选值集合）。
  - 新增任务 `3.4`：实现 `modified_source` 固定枚举校验与存储约束。
  - 新增测试任务：`6.5/6.6`：
    - `EnumValueGenerator` 类型一致性与混合类型拦截测试；
    - `modified_source` 合法/非法取值的契约与集成测试。
  - 更新 Requirements Coverage Matrix：
    - `2.1–2.5`、`3.1–3.5` 覆盖关系已补齐。

如果你希望，我可以下一步再帮你把 `tasks.md` 拆成“先做最小可运行子集（MVP）”和“增强项”两批，方便直接开工。

> 当前Spec: spec-03-generator-framework
>
> 请查看下列3个问题，考虑他们是否成为问题，如果是，请修正：
>
> --
>
> 关键问题 (3)
>
> 🔴 问题 1: 数据模型缺少具体 DDL 定义
>
> 问题: generator_definitions 和 field_generator_configs 仅描述为"逻辑模型"，未提供具体列定义、数据类型和约束。也未明确与 steering/database-schema.md 中 ldb_column_gen_configs 表名的对齐关系。
>
> 建议: 补充具体表结构定义，包括关键字段、数据类型、约束条件。
>
> --
>
> 🔴 问题 2: Generator 接口未与 Steering 规范对齐
>
> 问题: 设计定义了 GeneratorRegistry 和 GeneratorRuntime，但未明确 Generator 接口签名。Steering (generator.md) 已规定统一接口：Generate(ctx), GenerateBatch(ctx, count), Reset(), Type()。
>
> 建议: 在 Components and Interfaces 章节显式添加 Generator 接口定义，并说明 GeneratorRuntime 如何调用此接口。
>
> --
>
> 🔴 问题 3: 种子策略与确定性复现机制未详述
>
> 问题: 需求要求"同一配置与输入上下文下返回可复现结果"，但设计未说明：
>
>  种子如何传递至 Generator
>
>  全局种子与字段级种子的优先级关系
>
>  预览请求中的种子如何影响 GenerateBatch 调用
>
> 建议: 补充种子策略章节，定义 seed_policy 字段取值、预览种子覆盖机制、与 steering/generator.md 中"全局 seed"的对齐方式。

3 个问题都成立，我已在 `\.kiro/specs/spec-03-generator-framework/design.md` 完成修正。

- 问题 1（DDL 缺失 + 表名未对齐）：成立
  已新增 `Physical DDL Alignment` 章节，补了可执行层面的 DDL 草案与关键约束，并明确：
  - `generator_definitions` 物理落地为 `ldb_generator_definitions`
  - `field_generator_configs` 不新建同名表，直接对齐 `ldb_column_gen_configs`
  - 说明唯一键、枚举约束、JSON 字段约束、运行时会话不落持久化主表
- 问题 2（Generator 接口未对齐 steering）：成立
已新增 `Generator Interface（与 steering 对齐）`，显式定义接口签名： `Generate(ctx)` / `GenerateBatch(ctx, count)` / `Reset()` / `Type()`，并补充 `GeneratorRuntime` 调用约束（单样本与批量预览调用规则、Reset 使用边界、职责边界）。
- 问题 3（种子策略与复现机制不完整）：成立
  已新增 `Seed Strategy` 章节，补齐：
- `seed_policy` 结构与 `mode` 取值（`inherit_global | fixed`）
  - 全局 seed、字段 seed、预览 seed 的优先级
  - 预览请求覆盖机制
  - `GenerateBatch` 的批次根种子与行级派生规则（含前缀一致性约束）

已完成第二轮检查，并做了两处修正，当前这 3 个关键问题相关内容是闭环的。

- 修正 1：API 表格歧义
  - 文件：`\.kiro/specs/spec-03-generator-framework/design.md`
  - 将断裂的 `PreviewGeneration` 行修正为完整签名：`scope(field|table), seed?, sample_size`
  - 同时统一了配置参数命名映射：`generator_opts(params)`，避免和 DDL 列名 `generator_opts` 脱节
- 修正 2：任务覆盖矩阵遗漏
  - 文件：`\.kiro/specs/spec-03-generator-framework/tasks.md`
  - 在覆盖矩阵中补入：
    - `2.5` -> `6.5`（EnumValueGenerator 类型一致性测试）
    - `3.5` -> `6.6`（`modified_source` 固定枚举契约测试）
    - `4.4` -> `6.4`（跨 spec 依赖联调）
  - 这样矩阵与任务正文一致，不会在评审时出现“写了任务但矩阵没覆盖”的问题

当前结论：
你最初提的 3 个问题仍然是有效问题，并且现在在 `design.md` 中已经得到明确修复；本轮额外把文档内一致性问题也清掉了。若你愿意，我可以继续做第三轮“实现前检查”（仅检查是否可直接进入 `spec-impl`，不改内容）。

---

## Design Review Summary

本轮 `spec-03-generator-framework` 设计整体质量较高：边界清晰、核心接口与种子策略已经明显补强，并且与 `spec-02/spec-04/spec-08/spec-09` 的协作关系有明确说明。
但按 `design-review` 的“实现就绪”标准，仍有 2 个会直接影响落地一致性的关键缺口，建议先修完再进入实现。

## Critical Issues (≤3)

🔴 Critical Issue 1: 字段配置模型缺少“启用状态”落地定义
Concern: 需求 `3.1` 明确要求字段配置包含“启用状态”，但设计的数据模型与 DDL 草案未明确该字段（如 `is_enabled`）的语义、默认值与约束。
Impact: 实现阶段会出现“禁用字段如何参与候选解析/预览”的歧义，可能导致 UI/FFI/存储行为不一致。
Suggestion: 在 `Data Models` 与 `Physical DDL Alignment` 补充 `is_enabled`（或等价字段）定义：默认值、读写规则、与预览行为关系（禁用是否跳过、返回何种 warning）。
Traceability: Requirement `3.1`
Evidence: `design.md` 的 `Logical Data Model` 与 `Physical DDL Alignment` 章节

🔴 Critical Issue 2: `Generator.Type()` 返回类型在 steering 与设计间不一致
Concern: 设计里 `Type() string`，而技术 steering 中示例为 `Type() GeneratorType`；若不统一会形成接口漂移。
Impact: 直接影响插件实现、注册中心匹配以及 FFI 契约稳定性，后续重构成本高。
Suggestion: 在设计中明确“最终采用 `string` 还是 `GeneratorType`”，并在同一处说明与 `steering/tech.md`、`steering/generator.md` 的对齐决策（必要时同步 steering 文档）。
Traceability: Requirement `1.1`, `1.4`
Evidence: `design.md` 的 `Generator Interface`；`steering/tech.md` 的 `Generator` 接口示例

## Design Strengths

- 新增的 `Generator Interface`、`Seed Strategy`、`DDL Alignment` 三块已经把之前核心风险（接口缺失、复现机制不清、逻辑模型过虚）显著收敛。
- 架构边界控制到位：`spec-03` 明确不越界到跨表调度/写入，和 `spec-04` 职责分层清楚，后续实现更可控。

## Final Assessment

Decision: NO-GO（暂缓实现）
Rationale: 当前剩余问题不多，但都属于“接口与数据模型契约级”问题，会影响多模块一致性，建议先修文档再进 `spec-impl`。
Next Steps:

1. 在 `design.md` 补齐 `is_enabled` 的模型与 DDL 约束。
2. 统一 `Type()` 返回类型并同步相关 steering。
3. 完成后再跑一次 `/validate-design spec-03-generator-framework`（预计可转 GO）。

## Interactive Discussion

你如果同意，我可以直接帮你把这两处文档改完（最小改动版），然后你再执行一遍 `/validate-design` 基本就能过。

我会直接落地这两处修复：一是补齐 `is_enabled` 的模型/DDL/API 语义，二是统一 `Type()` 返回为 `GeneratorType` 并注明 FFI 序列化策略，随后给你一个简短变更说明。

已按你要求直接改完，修改文件：`\.kiro/specs/spec-03-generator-framework/design.md`。

本次改动点：

- 补齐 `is_enabled` 全链路定义
  - API：`SaveFieldGeneratorConfig` 请求/响应加入 `is_enabled`
  - 逻辑模型：`field_generator_configs` 明确包含 `is_enabled`
  - 语义：`is_enabled=false` 时跳过生成调用，并返回 `GENERATOR_DISABLED` warning
  - DDL：在 `ldb_column_gen_configs` 草案中新增
  `is_enabled BOOLEAN NOT NULL DEFAULT TRUE`
  - 约束：补充 `is_enabled` 默认值及运行时行为说明
- 统一 `Generator.Type()` 类型
  - 接口从 `Type() string` 改为 `Type() GeneratorType`
  - 新增对齐说明：领域层用强类型 `GeneratorType`，FFI JSON 层序列化为稳定字符串（如 `sequence`、`enum`）

这两处就是上一轮评审的关键 NO-GO 项，现在已修复。
你可以直接再跑一次 `/validate-design spec-03-generator-framework`。

<<<<<<< HEAD


---

> 1. @.kiro/specs/spec-03-generator-framework/design.md:181  生成器指的是对什么类型字段的生成什么匹配的数据。与  connection_id 无关。因此  ListGeneratorCapabilities 只与 field_type 相关，与 connection_id 无关。 
> 2. @.kiro/specs/spec-03-generator-framework/design.md:258 生成器是”硬编码“实现的，因此它没有所谓状态。
> 3. 生成器是通过编码实现的，例如根据用户输入的“正则表达式”生成数据。生成器注册是为多个不同类型的生成器扩展而设计的。设计中通过“ldb_generator_definitions”表保存，这让我不能理解。我认为注册是通过“注册表”的形式对外公布已有的生成器及其能力，它不需要数据库保存，这没有意义。请检查设计，是我理解有误，还是设计有误

你的理解基本正确，设计里确实有几处概念混淆，我逐条核对如下：

- 结论 1（你说得对）：`ListGeneratorCapabilities` 不应依赖 `connection_id`
  这个接口语义是“列出生成器能力”，本质应按 `field_type`（或空表示全部）过滤。`connection_id` 属于 schema 上下文，应该放在 `GetFieldGeneratorCandidates` 这类“按具体字段推候选”接口里。当前 `design.md` 把两者混在一起了。
- 结论 2（你部分对）：`lifecycle_state` 这类“定义状态”不该作为持久化主模型，但“生成器无状态”不能一概而论
  你说“硬编码实现没有状态”对注册定义层是成立的；但对运行时实例不一定成立，比如序列类生成器是可能有状态的。
  设计里真正不协调的是：把“编译时注册”同时又建成了“数据库定义 + 启停 API”的运行时控制体系，层次混了。
- 结论 3（你说得对）：`ldb_generator_definitions` 这张表在当前方案下意义很弱，甚至是反模式
  既然基线是“编译时注册（非动态插件）”，注册表应该是代码内存结构（元数据 + 构造器），不是数据库主数据。
  数据库更应该只存字段配置（例如 `ldb_column_gen_configs`），不应再存“生成器定义事实”。

建议你把 `spec-03` 改成这套更一致的口径：

- `ListGeneratorCapabilities(field_type?)`：去掉 `connection_id`
- 删除 `EnableGenerator` / `DisableGenerator`（若保留，仅做进程内策略开关，不落库）
- 删除逻辑模型 `generator_definitions` 与物理表 `ldb_generator_definitions`
- 保留并强化 `field_generator_configs -> ldb_column_gen_configs`
- 文档里明确“状态分层”：
  - 注册定义：静态、编译时、无持久化
  - 运行实例：可有状态（如 sequence），通过 `Reset()` 管理

如果你愿意，我可以直接帮你把 `design.md` 按上述口径改一版（包括接口表、数据模型、DDL 对齐和术语说明）。



> 关于结论2：“lifecycle_state 这类“定义状态”不该作为持久化主模型，但“生成器无状态”不能一概而论... 比如序列类生成器是可能有状态的”。我所说的 lifecycle_state 指的是生命周期状态：enabled, disabled, deprecated。它与 “序列化生成器" 的”状态“是两个概念（这里的状态是指持有可变数据），不要混淆了。我指的是**不需要生命周期状态**，EnableGenerator，DisableGenerator 这些都是不必要的。并不需要 generator_definitions 的”lifecycle_state“， 也不需要表 ldb_generator_definitions。
>
> 其它的调整，请按照你的理解来进行（可能不仅仅是 design.md, 还包括 requirements.md 和 tasks.md

已按你的口径完成调整，并同步到了 3 份文档，核心是：移除生命周期状态模型与相关启停接口，保留编译时注册 + 运行时能力查询。

已修改文件：

- `/.kiro/specs/spec-03-generator-framework/design.md`
- `/.kiro/specs/spec-03-generator-framework/requirements.md`
- `/.kiro/specs/spec-03-generator-framework/tasks.md`

具体改动：

- 在 `design.md` 中：
  - 删除 `EnableGenerator`、`DisableGenerator`
  - `RegisterGenerator` 请求参数移除 `lifecycle_state`
  - `ListGeneratorCapabilities` 改为仅 `field_type?`（去掉 `connection_id?`）
  - 删除 `generator_definitions` 逻辑主模型与 `ldb_generator_definitions` DDL 方案
  - 新增并明确：生成器注册信息是进程内 `generator_registry_snapshot`，不落库
  - 保留并强化：`generator_id + version` 的冲突检测在注册表内完成（返回 `GENERATOR_CONFLICT`）
- 在 `requirements.md` 中：
  - Requirement 1.2 从“注册、查询、启停、冲突检测”改为“注册、查询、冲突检测”
- 在 `tasks.md` 中：
  - 任务 1.2 从“支持注册、查询、启停、重复冲突检测”改为“支持注册、查询、重复冲突检测”

自检结果：

- `spec-03` 目录内已无 `EnableGenerator`、`DisableGenerator`、`lifecycle_state`、`启停` 残留。
- 现在三份文档在该议题上已一致。

如果你希望，我可以再做一版“术语统一清理”（例如把 `generator_registry_snapshot` 在全篇再统一成更短名称，如 `registered_generators`），让文档更易读。



> 好的,请”术语统一清理“



术语统一清理已完成。

这次统一结果：

- 将 `generator_registry_snapshot / 注册表快照` 统一为
  `registered_generators / 已注册生成器清单`
- 保持 `GeneratorRegistry` 作为组件名不变（这是类型名，不是业务术语）

已更新位置：`/.kiro/specs/spec-03-generator-framework/design.md`

  - 逻辑数据模型条目改名为 `registered_generators`
  - 能力查询说明改为“直接读取已注册生成器清单”

一致性检查结果：

- `spec-03` 三份文档中不再出现 `generator_registry_snapshot` 或“注册表快照”旧术语。
- `requirements.md` 和 `tasks.md` 当前无需额外术语替换，和 `design.md` 语义一致。



---

## Design Review Summary

`spec-03-generator-framework` 的设计整体质量较高，范围边界、职责拆分、接口契约和可扩展点与需求文档及 steering 上下文基本一致。
从可实现性看，已具备直接进入实现阶段的条件，风险主要集中在“schema 变更后候选生成器重判定”的触发闭环是否足够明确。
结论倾向 GO（建议先补一个小的设计澄清项）。

## Critical Issues (≤3)

🔴 Critical Issue 1: Schema 变更后的“重判定触发路径”不够显式
Concern: 设计明确了保存配置时会做候选解析与校验，但对“`spec-02` 同步后如何自动触发既有字段配置重判定并输出不兼容提示”的流程未形成独立流程/状态迁移约束。
Impact: 可能出现旧配置在 schema 已变化时未及时暴露风险，影响 Requirement 2.2 的一致性与可预期性。
Suggestion: 在设计中新增“Schema Sync -> Revalidate Existing Field Configs -> Emit Incompatibility Report”的显式流程（可挂在 `GeneratorConfigService` 或单独 `CompatibilityRecheckService`），并定义触发时机（同步后立即、首次读取时惰性、或两者结合）。
Traceability: Requirement 2.2、2.4（类型变化重判定与可定位错误）
Evidence: `design.md` 的“字段规则配置与校验流程”“Requirements Traceability”“Error Handling”章节（已有校验与错误码，但缺少 schema 变更后的主动重判定闭环描述）。

## Design Strengths

- 边界控制非常清晰：`GeneratorRegistry` / `GeneratorConfigService` / `GeneratorPreviewService` / `GeneratorRuntime` 的职责拆分与 `spec-04` 执行域隔离明确，符合 steering 的分层与职责约束。
- 契约与可复现性设计扎实：`seed_policy` 结构、优先级和 runtime 传递规则定义完整，且与 `generator.md` 中接口与“全局 seed 可追溯”原则保持一致。

## Final Assessment

Decision: GO（带1项实现前澄清）

Rationale: 当前设计不存在架构级冲突，绝大多数需求有明确落点且实现路径清楚；仅有一个高影响但低改动成本的闭环澄清项。补齐后可显著降低 schema 变更场景下的运行风险。

Next Steps:

- 在 `design.md` 补充“schema 同步后重判定”流程与触发时机说明（建议加一段 sequence diagram 或状态机补充）。
- 补充后可直接执行 `/kiro/spec-tasks spec-03-generator-framework`（如你希望我继续，我可以直接帮你起草这段补丁文本）。

## Interactive Discussion

你更倾向哪种触发策略？

- A：schema 同步成功后立即全量重判定（更稳健）
- B：按读取/预览时惰性重判定（更省资源）
- C：A+B 结合（同步后预计算 + 运行时兜底）

你选定后，我可以按该策略给出一版最小改动的设计补充内容。



已按 A 方案完成设计文档更新，改动在 `/.kiro/specs/spec-03-generator-framework/design.md`。

主要补充点：

- 新增章节 “Schema 同步后立即全量重判定流程（A 方案）”（含 Mermaid 时序图）。
- 明确 4 条约束：
  - `ApplySchemaSync` 成功后立即触发，不允许仅靠惰性触发。
  - 作用范围是该连接下全部字段配置。
  - 不兼容必须返回字段定位和修复建议，禁止静默跳过。
  - 阻断风险与 `spec-02` 可信度状态联动，进入 `pending_adjustment`。
- 在 Requirements Traceability 中把 Requirement 2.x 扩展为包含 `CompatibilityRecheckService` 与 `RevalidateAllConfigs`。
- 在 Components and Interfaces 中新增 `CompatibilityRecheckService` 组件定义。
- 在 Error Strategy 中新增“schema 同步后重判定不兼容”错误语义（`FAILED_PRECONDITION`）。
- 在 Testing Strategy 中将跨 spec 联调改为验证“同步成功后立即触发重判定并输出报告”。

另外我已做过一次文档诊断检查，当前无新增 linter 问题。
如果你需要，我可以继续基于这个版本再跑一轮 `/validate-design spec-03-generator-framework` 的最终评审口径（只给结论和风险清单）。

已按 A 方案完成设计文档更新，改动在 `/.kiro/specs/spec-03-generator-framework/design.md`。

主要补充点：

- 新增章节 “Schema 同步后立即全量重判定流程（A 方案）”（含 Mermaid 时序图）。
- 明确 4 条约束：
  - `ApplySchemaSync` 成功后立即触发，不允许仅靠惰性触发。
  - 作用范围是该连接下全部字段配置。
  - 不兼容必须返回字段定位和修复建议，禁止静默跳过。
  - 阻断风险与 `spec-02` 可信度状态联动，进入 `pending_adjustment`。
- 在 Requirements Traceability 中把 Requirement 2.x 扩展为包含 `CompatibilityRecheckService` 与 `RevalidateAllConfigs`。
- 在 Components and Interfaces 中新增 `CompatibilityRecheckService` 组件定义。
- 在 Error Strategy 中新增“schema 同步后重判定不兼容”错误语义（`FAILED_PRECONDITION`）。
- 在 Testing Strategy 中将跨 spec 联调改为验证“同步成功后立即触发重判定并输出报告”。

另外我已做过一次文档诊断检查，当前无新增 linter 问题。
如果你需要，我可以继续基于这个版本再跑一轮 `/validate-design spec-03-generator-framework` 的最终评审口径（只给结论和风险清单）。
=======
---

> 1. 首先需要把“生成器”的概念澄清并写入设计文档中：「生成器是一段生成单一/一组模拟数据的代码，它满足 Generator 接口。Generator 需要一个“配置”参数，例如“整数序列生成器”需要的配置是 {"start": 1, "step": 1}。系统有很多这样的生成器，例如随机字符串生成器，UUID 生成器等等。为了统一暴露、调用这些生成器，因此需要“Register” 作为统一管理，例如查询不同数据类型的字段可以使用哪些生成器。」。明确了生成器及Register的概念后，就知道 Issue 1 中：把生成器设计为一个“数据模型” 并且用 ldb_generator_definitions 持久化是一件非常可笑的事情。需要持久化的是用户传递的配置参数，例如为一个 ID 字段设置了generator_type=IntSequenceGenerator，参数= {"start": 1, "step": 1}
> 2. Issue 2 中，关于JSON类型的落地：统一使用 String 类型（varchar, TEXT 等）。因为 `{"start": 1, "step": 1}` 这样的参数需要在代码中解析，SQLite/MySQL/Postgres 这样的后端只需要负责存储即可
> 3. Issue 3，请根据你的理解调整

已按你的三点意见改完 `design.md`，摘要如下。

## 1. 概念澄清与去掉「生成器定义表」

- 在 「核心概念：生成器、配置与注册」 中写明：生成器是实现 `Generator` 的代码；持久化的是字段级「选用哪个生成器 + 参数字符串」（如 `IntSequenceGenerator` + `{"start":1,"step":1}`）；Register 是进程内登记与发现，不是把生成器做成库表。
- 删除原 `ldb_generator_definitions` 整段 DDL；逻辑模型改为 内置目录仅在代码 + Registry 内存，不落「生成器定义」实体。
- API：对外 FFI 表去掉可被误解为「对外注册并落库」的命名；进程内用 `RegisterBuiltinGenerator`，并说明 `SetGeneratorLifecycleState` 为可选、仅内存态。

## 2. JSON 落地 → 统一字符串列

- 在 Physical DDL 中明确：`generator_opts`、`seed_policy` 物理类型为 `TEXT`/`VARCHAR`，存 UTF-8 JSON 文本；由应用解析，存储只负责透明存取。
- Seed Strategy 小节补充：持久化形态为字符串，文中 JSON 为解析后的逻辑结构。

## 3. Issue 3（schema 可信度）

- 新增章节「Schema 可信度与 spec-02 闸门」：
  - `trusted` / `pending_rescan` / `pending_adjustment` 与 保存、校验、候选、预览 的允许/拒绝关系；
  - 与 `CURRENT_SCHEMA_NOT_FOUND` 的区分表（事实缺失 vs 治理门闩）；
  - 建议稳定 `reason`：`SCHEMA_TRUST_PENDING_RESCAN` / `SCHEMA_TRUST_PENDING_ADJUSTMENT`；
  - `GetFieldGeneratorConfig`：允许在 `pending_*` 下带 `warnings` 只读展示，或统一 `FAILED_PRECONDITION`（需 FFI 二选一固定）；
  - `ListGeneratorCapabilities`：可保留为能力说明，但 UI 应结合连接 trust 提示风险。
- Error Strategy 已接上上述语义；Testing Strategy 补了 trust 分支与集成场景。

若你希望 `GetFieldGeneratorConfig` 只保留一种策略（总是带 `warnings` 或总是失败），定案后我可以再把设计里括号里的「二选一」改成单一路径，避免实现分叉。

> GetFieldGeneratorConfig  在 pending_* 下带 warnings 只读展示

已按你的选择改好：`design.md` 里 `GetFieldGeneratorConfig` 在 `pending_`* 下固定为成功返回 + `warnings[]` 只读展示，并去掉「可统一 `FAILED_PRECONDITION`」的歧义。

本次调整要点：

1. 调用范围：明确该接口不属于「必须 `FAILED_PRECONDITION`」集合；`pending_*` 时 `ok: true`，带已持久化 `config` 与 `warnings[]`（含 `SCHEMA_TRUST_*` 类稳定 `reason`）。
2. 状态表：在 `pending_*` 两行中补充 例外：`GetFieldGeneratorConfig` 成功 + `warnings[]`。
3. 错误模型表：新增一行区分「其它接口」与 `GetFieldGeneratorConfig`；并说明 trust 语义在 `warnings[]`，不用顶层 `error`。
4. 流程说明 / Error Strategy / FFI 表：与上述一致；API 表中 `GetFieldGeneratorConfig` 的 Errors 列标明 `pending_*` 不使用 `FAILED_PRECONDITION`，无配置时仍为 `NOT_FOUND`。

---

> /validate-design spec-03-generator-framework

## Design Review Summary

`spec-03-generator-framework` 的设计文档结构完整：职责边界（单字段/单表、不跨表写入）、组件拆分、schema 可信度闸门与 `GetFieldGeneratorConfig` 只读例外等表述清晰，且与 `steering/database-schema.md` 的状态机高度一致。主要风险集中在与权威文档 `docs/generator.md` 的接口定义不一致、FFI 逻辑名与既有 `SaveColumnGenConfig` 等导出名的关系未钉死，以及可复现性里“批次前缀不变”对 Runtime 算法的隐含约束。整体仍具备可实施路径，属可接受风险，但建议在进入编码前收敛上述三点。

---

## Critical Issues（≤3）

### Critical Issue 1：Generator 接口与权威详设不一致

Concern：设计在「Generator Interface（与 steering 对齐）」中采用 `Generate(ctx GenerateContext) (GeneratedValue, error)`，而 `steering/generator.md` 明确写明与 `docs/generator.md` 冲突时以 `docs/generator.md` 为准；该文档 §1.1 定义为 `Generate(ctx context.Context) (interface{}, error)` 等。

Impact：实现者可能按 spec-03 做强类型上下文，也可能按 `docs/generator.md` 沿用 `context.Context` + `interface{}`，造成生成器层与 Runtime/预览分叉，违背「统一接口」与可维护性。

Suggestion：在设计与实现前做一次权威口径选择：要么在 `docs/generator.md` 中正式升级为 `GenerateContext`/`GeneratedValue`（并说明迁移理由），要么将 spec-03 设计改回与 `docs/generator.md` 一致，并在 Runtime 内做类型封装；同时在 `steering/tech.md` 中同步，避免三处各写一套。

Traceability：需求 1.1（统一 `Generator` 抽象）；与 4.x 可复现执行路径相关。

Evidence：`design.md` 中「Generator Interface（与 steering 对齐）」代码块；`docs/generator.md` §1.1。

---

### Critical Issue 2：FFI 契约与既有导出函数的关系未写清

Concern：设计列出 `SaveFieldGeneratorConfig` / `GetFieldGeneratorConfig` 等逻辑方法名，而 `docs/schema.md` 已存在 `SaveColumnGenConfig` 等导出；`steering/database-schema.md` 亦用 `Save*GenConfig` 概括。

Impact：Flutter/Go 两端可能出现「新 JSON 路由名 vs 旧 C 导出名」双轨或重复实现，契约测试与文档维护成本上升，甚至产生行为不一致。

Suggestion：在设计中增加一小节：逻辑契约名 ↔ 实际 `LDB_*` / `SaveColumnGenConfig` 映射表（是保留旧导出仅扩展 JSON 载荷，还是弃用旧名并列迁移步骤）。实现阶段保证 `docs/schema.md` §六清单与 spec-03 单一真源。

Traceability：需求 3.x、5.3（统一错误模型与契约一致性）。

Evidence：`design.md`「API Contract（逻辑签名，非最终实现）」表；`docs/schema.md` 中 `SaveColumnGenConfig` 导出。

---

### Critical Issue 3：`sample_size` 变化时的「前缀一致」缺少可验证算法约定

Concern：「Seed Strategy」要求：在相同上下文下，当 `sample_size` 变化时，前 `min(n1,n2)` 条样本应保持前缀一致；但未规定 按行下标派生子种子 的确定性算法（例如与 `GenerateBatch` 内部游标、`Reset` 调用次序的关系）。

Impact：部分有状态或批量优化路径的生成器可能无法满足需求 4.2 与 4.x 的元数据预期，集成测试难以写成稳定断言。

Suggestion：在 `GeneratorRuntime` 小节补充可测试的伪代码或不变量（例如：批次内第 `i` 条使用 `f(effective_seed, row_index=i)`，且预览路径禁止在样本循环中破坏生成器状态）；并为「仅实现 `GenerateBatch` 的生成器」规定与单条路径的等价性。

Traceability：需求 4.2、4.3。

Evidence：`design.md`「Seed Strategy」中「Runtime 传递规则」与「当 sample_size 变化时…」段落。

---

## Design Strengths

1. Schema 可信度闸门：对 `Save*` / `Validate*` / `GetFieldGeneratorCandidates` / `PreviewGeneration` 与 `GetFieldGeneratorConfig` 的成败分流写得很细，与 `steering/database-schema.md` 的 `schema_trust_state` 一致，有利于 UI 引导与稳定 `reason` 分支。
2. 持久化边界清晰：内置生成器走进程内 Registry、用户配置落 `ldb_column_gen_configs`、参数 JSON 以字符串列存储等决策，与 steering 中的元数据隔离和方言策略一致，降低过度建模风险。

---

## Final Assessment

结论：GO（附条件）

理由：未发现与需求或架构的根本性冲突；上述三点属于文档权威对齐与契约命名类问题，可在不动大架构的前提下通过补充说明或小幅修订设计/`docs/generator.md` 解决，失败风险可控。

建议下一步：

1. 收敛 Generator 接口的单一权威定义（`docs/generator.md` ↔ spec-03），并更新 `steering/tech.md` 中的示例接口，避免实现期分叉。
2. 在 `design.md` 或 `docs/schema.md` 中明确 FFI 导出名与 JSON 方法名的对应关系。
3. 为种子/批次前缀不变量补一段可测试规范后再按现有任务推进实现（`spec.json` 已为 `tasks-generated`，若任务已拆完可直接进入实现与契约测试）。

---

## Interactive Discussion

若你倾向于保持 `context.Context` + `interface{}` 以少改既有生成器代码，建议在 spec-03 中把 `GenerateContext` 写成 Runtime 对 `context.Context` 的包装层命名，并在文档里写一句「与 `docs/generator.md` 表面签名一致」。若你更倾向强类型，则应以 `docs/generator.md` 修订 + steering 引用更新 为前置，否则评审里「与 steering 对齐」的表述会长期引发争议。

如需要，我可以根据你选定的接口口径，帮你在 `design.md` 里改一版「与 `docs/generator.md` 的显式差异表」草稿（仅在你确认要改文档时再动文件）。

---

> Issue 1:  以docs/generator.md 为准：沿用 context.Context + interface{}
>
> Issue 2:  应修改docs/schema.md，使其名称、参数、返回值等与 design.md 中一致（design.md 中的 SaveFieldGeneratorConfig 比 SaveColumnGenConfig 更友好）
>
> Issue 3: 去掉 “当 sample_size 变化时，前 min(n1,n2) 条样本在相同上下文下应保持前缀一致“ 的相关表述。feed 返回什么数据由外部数据源决定

已完成两份文档修改，正在做一致性复查，确保旧名称不再残留在关键接口段落。

已按你的 3 点全部改完，并做了关键项复查。

- `design.md` 已改为以`docs/generator.md`为准：
  - `Generator` 接口改回 `context.Context + interface{}` 口径
  - 删除了 `GenerateContext` / `GeneratedValue` 的接口定义表述
- `design.md`已删除你指定约束：
  - 去掉了“`sample_size` 变化时前缀一致”的整条规则
- `docs/schema.md` 已按`design.md`对齐接口命名与契约语义：
  - 用 `SaveFieldGeneratorConfig` 替代 `SaveColumnGenConfig`
  - 在 FFI 导出清单中补齐并统一为：
    - `ListGeneratorCapabilities`
    - `GetFieldGeneratorCandidates`
    - `SaveFieldGeneratorConfig`
    - `GetFieldGeneratorConfig`
    - `ValidateFieldGeneratorConfig`
    - `PreviewGeneration`
  - 同步更新了该节中的入参/返回说明与流程图节点命名

本次修改文件：

- `.kiro/specs/spec-03-generator-framework/design.md`
- `docs/schema.md`

如果你要，我下一步可以继续做一轮“全仓检索一致性修正”（例如把其余文档里可能残留的 `SaveColumnGenConfig`/旧命名也统一掉）。

---

> /validate-design spec-03-generator-framework

我已经完成评审分析，下面会直接给出结论；同时我会在末尾抛出 2 个确认问题，方便你决定是“先修订 design 再进实现”，还是“带风险进入 tasks/impl”。## Design Review Summary

`spec-03-generator-framework` 的设计整体质量较高，边界意识清晰（单字段/单表、无跨表编排），并且与 `spec-02` 的 schema trust gate、以及 `spec-04/spec-08/spec-09` 的职责分离基本一致。对配置校验、预览可复现、错误模型和扩展点也给出了较完整的机制。
当前存在 2 个需要在实现前收敛的关键问题，但不构成架构级阻塞，建议 GO（附带修正项）。

## Critical Issues (≤3)

🔴 Critical Issue 1: Generator 抽象未完整承载“分类错误 + 输入上下文语义”
Concern: Requirement 1.1 要求统一抽象至少覆盖元信息、输入上下文、输出值、可分类错误；当前接口主要是 `Generate(ctx) (interface{}, error)`，分类错误语义更多落在 FFI 层，领域接口层约束偏弱。
Impact: 生成器实现方容易出现“同类错误不同编码/不同包装”，影响注册中心筛选、预览诊断一致性和后续扩展治理。
Suggestion: 在领域层补充标准错误分类契约（例如统一 error type/code 枚举与包装器），并明确 `ctx` 中必备输入键（seed、field schema、dependency readiness）。
Traceability: Requirement 1.1
Evidence: `design.md` 的 `Generator Interface（与 steering 对齐）`、`Error Handling`、`Capability Model` 章节。

🔴 Critical Issue 2: 生成器启停与版本策略的“持久语义”未完全定稿
Concern: Requirement 1.2 要求按 ID/版本/类型标签支持注册、查询、启停与冲突检测；设计中 `SetGeneratorLifecycleState` 标注为“可选、仅进程内语义”，但未明确重启后策略及与配置查询的一致性约束。
Impact: 可能出现“同一配置在不同进程实例可见性不一致”，导致 UI 候选列表与运行时行为偏差。
Suggestion: 明确 MVP 最终策略二选一：

- A. 明确“仅编译/发布期开关，不提供运行时启停”；或
- B. 提供最小持久化启停快照并定义重启恢复规则。
并把该策略写入 FFI 能力响应元数据。
Traceability: Requirement 1.2, 1.3, 1.4
Evidence: `design.md` 的 `进程内注册（仅 Go 启动/内置模块 init）` 与 `SetGeneratorLifecycleState（可选）` 段落。

## Design Strengths

- 边界与分层非常清楚：`Registry/ConfigService/PreviewService/Runtime` 职责拆分合理，且多处明确“不做跨表执行、事务写入”，与 steering 和需求边界一致。
- 可追溯性强：有完整 Requirements Traceability、schema trust gate 状态映射、错误码区分与测试策略，落地可操作性较好。

## Final Assessment

Decision: GO（建议带修正进入下一阶段）
Rationale: 设计不存在根本性架构冲突，需求覆盖度高，实现路径清晰；当前问题主要是接口契约精度与生命周期语义收敛，不属于推翻式重做。
Next Steps:

- 先补齐上述 2 项修订（建议直接更新 `design.md` 对应章节）；
- 然后执行 `/kiro/spec-tasks spec-03-generator-framework`（或已生成任务则更新任务与验收点）；
- 实现阶段将“错误分类一致性 + 启停语义一致性”设为优先验收项。

## Interactive Discussion

为了避免后续返工，我建议你先确认这两点取舍：

1. 你希望 MVP 的生成器启停是纯进程内（重启失效）还是可恢复（最小持久化）？
2. 你是否同意把“分类错误契约”上移到领域层（而不是主要依赖 FFI 映射层）？

你确认后，我可以直接帮你产出一版可落地的 `design.md` 修订稿（最小改动版）。

---

> /validate-design spec-03-generator-framework

我已经完成评审分析，下面会直接给出结论；同时我会在末尾抛出 2 个确认问题，方便你决定是“先修订 design 再进实现”，还是“带风险进入 tasks/impl”。## Design Review Summary

`spec-03-generator-framework` 的设计整体质量较高，边界意识清晰（单字段/单表、无跨表编排），并且与 `spec-02` 的 schema trust gate、以及 `spec-04/spec-08/spec-09` 的职责分离基本一致。对配置校验、预览可复现、错误模型和扩展点也给出了较完整的机制。
当前存在 2 个需要在实现前收敛的关键问题，但不构成架构级阻塞，建议 GO（附带修正项）。

## Critical Issues (≤3)

🔴 Critical Issue 1: Generator 抽象未完整承载“分类错误 + 输入上下文语义”
Concern: Requirement 1.1 要求统一抽象至少覆盖元信息、输入上下文、输出值、可分类错误；当前接口主要是 `Generate(ctx) (interface{}, error)`，分类错误语义更多落在 FFI 层，领域接口层约束偏弱。
Impact: 生成器实现方容易出现“同类错误不同编码/不同包装”，影响注册中心筛选、预览诊断一致性和后续扩展治理。
Suggestion: 在领域层补充标准错误分类契约（例如统一 error type/code 枚举与包装器），并明确 `ctx` 中必备输入键（seed、field schema、dependency readiness）。
Traceability: Requirement 1.1
Evidence: `design.md` 的 `Generator Interface（与 steering 对齐）`、`Error Handling`、`Capability Model` 章节。

🔴 Critical Issue 2: 生成器启停与版本策略的“持久语义”未完全定稿
Concern: Requirement 1.2 要求按 ID/版本/类型标签支持注册、查询、启停与冲突检测；设计中 `SetGeneratorLifecycleState` 标注为“可选、仅进程内语义”，但未明确重启后策略及与配置查询的一致性约束。
Impact: 可能出现“同一配置在不同进程实例可见性不一致”，导致 UI 候选列表与运行时行为偏差。
Suggestion: 明确 MVP 最终策略二选一：

- A. 明确“仅编译/发布期开关，不提供运行时启停”；或
- B. 提供最小持久化启停快照并定义重启恢复规则。
并把该策略写入 FFI 能力响应元数据。
Traceability: Requirement 1.2, 1.3, 1.4
Evidence: `design.md` 的 `进程内注册（仅 Go 启动/内置模块 init）` 与 `SetGeneratorLifecycleState（可选）` 段落。

## Design Strengths

- 边界与分层非常清楚：`Registry/ConfigService/PreviewService/Runtime` 职责拆分合理，且多处明确“不做跨表执行、事务写入”，与 steering 和需求边界一致。
- 可追溯性强：有完整 Requirements Traceability、schema trust gate 状态映射、错误码区分与测试策略，落地可操作性较好。

## Final Assessment

Decision: GO（建议带修正进入下一阶段）
Rationale: 设计不存在根本性架构冲突，需求覆盖度高，实现路径清晰；当前问题主要是接口契约精度与生命周期语义收敛，不属于推翻式重做。
Next Steps:

- 先补齐上述 2 项修订（建议直接更新 `design.md` 对应章节）；
- 然后执行 `/kiro/spec-tasks spec-03-generator-framework`（或已生成任务则更新任务与验收点）；
- 实现阶段将“错误分类一致性 + 启停语义一致性”设为优先验收项。

## Interactive Discussion

为了避免后续返工，我建议你先确认这两点取舍：

1. 你希望 MVP 的生成器启停是纯进程内（重启失效）还是可恢复（最小持久化）？
2. 你是否同意把“分类错误契约”上移到领域层（而不是主要依赖 FFI 映射层）？

你确认后，我可以直接帮你产出一版可落地的 `design.md` 修订稿（最小改动版）。

> Issue 1: 请补充 Generator 的完整定义
>
> Issue 2: 生成器是一段“逻辑代码”。在 design.md 的 “### 核心概念：生成器、配置与注册（必须先读）”章节中已经描述的很清晰了，它没有启停功能，也没有“版本”这个概念。删掉相关的内容

已按你的两点要求完成 `design.md` 修改，文件是：

- `.kiro/specs/spec-03-generator-framework/design.md`

本次改动：

- 补充了 Generator 完整定义，现在在 Generator 完整定义（与 steering 对齐）章节中明确包含：
  - 元信息：`GeneratorMeta`
  - 输入上下文：`GeneratorContext`
  - 输出值：`Generate/GenerateBatch` 返回值
  - 分类错误：`GeneratorErrorCode` + `GeneratorError`
- 删除了“版本/启停” 相关设计内容：
  - 去掉 `SetGeneratorLifecycleState`
  - 去掉 `RegisterBuiltinGenerator` 中 `version` / `lifecycle_state`
  - 去掉“禁用某版本生成器”的说明段
  - 数据模型里去掉“生成器元数据含版本”表述
  - 错误策略里把“同 ID 版本策略不兼容”改为“同 ID 重复注册拒绝覆盖”

---

> 再跑一轮 validate-design

## Design Review Summary

这版 `spec-03` 设计明显更一致了：你要求的“Generator 完整定义”已经补齐，且“生成器是逻辑代码、无版本/启停”的边界表达清晰，架构分层与职责边界也保持稳定。
从实现可行性看，主链路（注册→候选解析→配置校验→预览）已经具备落地条件。
当前仅剩 1 个关键一致性问题，主要是 requirements 与 design 的契约对齐。

## Critical Issues (≤3)

🔴 Critical Issue 1: requirements 与 design 在“版本/启停”语义上冲突
Concern: `requirements.md` 的 Requirement 1.2/1.3 仍要求“按版本注册、启停、版本策略冲突”，但 `design.md` 已按你的决策删除版本/启停概念。
Impact: 后续 tasks/impl/验收会出现“按需求判不通过、按设计又不该实现”的双重标准，直接影响交付一致性。
Suggestion: 立即同步修订 Requirement 1 的 AC（尤其 1.2/1.3），改为“按 generator_type + capability 注册/冲突检测”，去掉版本与生命周期措辞。
Traceability: Requirement 1.2, 1.3
Evidence: `requirements.md` 的 Requirement 1 AC；`design.md` 的“核心概念：生成器、配置与注册”“进程内注册”“Generator 完整定义”章节。

## Design Strengths

- `Generator` 抽象已覆盖元信息、输入上下文、输出与分类错误，接口契约比之前更完整。
- 与 `spec-02` 的 schema trust gate、以及 `spec-04` 执行边界衔接清晰，错误模型和测试策略可执行性较高。

## Final Assessment

Decision: GO（带 1 项前置修订）
Rationale: 设计本身已可进入实现，但文档体系存在需求-设计不一致，需先修正文档基线以避免验收争议。
Next Steps:

- 先改 `requirements.md` 中 Requirement 1 的版本/启停相关 AC；
- 再执行 `/kiro/spec-tasks spec-03-generator-framework`（或更新已生成任务）；
- 实现阶段按“无版本/无启停”的统一语义推进。

## Interactive Discussion

你要我现在直接帮你把 `requirements.md` 的 Requirement 1（1.2/1.3）同步改掉吗？
如果你同意，我会按当前 design 语义给出最小改动版，确保下一步 tasks 与 impl 不再冲突。

---

> 同步调整 requirements.md 和 tasks.md

已同步完成 `requirements.md` 和 `tasks.md`，并对齐到“生成器是逻辑代码、无版本/无启停”的语义。

修改文件：

- `.kiro/specs/spec-03-generator-framework/requirements.md`
- `.kiro/specs/spec-03-generator-framework/tasks.md`

关键变更：

- `requirements.md`
  - Requirement 1.2：从“ID、版本、类型标签注册 + 启停”改为“ID、类型标签注册/查询/冲突检测”。
  - Requirement 1.3：从“版本策略不兼容冲突”改为“同 ID 重复注册冲突”。
- `tasks.md`
  - 1.1 子任务说明：移除“版本语义”。
  - 1.2 子任务：移除“启停”能力，仅保留注册/查询/重复冲突检测。
  - 3.1 子任务说明：将“版本字段”改为“配置修订号字段”。

我还做了复查：两份文件里已无“版本/启停/lifecycle”相关残留表述。
如果你要，我可以继续帮你再跑一次轻量一致性检查（`requirements/design/tasks` 三者逐条对照表）。

---

> /validate-design spec-03-generator-framework

## Design Review Summary

`spec-03-generator-framework` 的设计整体完整度较高，需求覆盖面（接口、配置、预览、错误模型、边界）与 `requirements.md` 基本对齐，且对 `spec-02` 的 schema trust gate 约束落地比较到位。
不过存在 2 个会直接影响实现一致性和前后端协作的关键缺口，建议在进入实现前先修正。基于当前状态，我给出 NO-GO（需小幅返工后再进实现）。

## Critical Issues (≤3)

🔴 Critical Issue 1: Generator 接口契约与项目既有约束存在漂移
Concern: 设计里定义的 `Generator` 接口为 `Meta()+Generate(ctx,in)+GenerateBatch(ctx,in,count)+Reset()`，而 steering/tech 中长期约束接口为 `Generate(ctx)+GenerateBatch(ctx,count)+Reset()+Type()`；签名与能力出口不一致。
Impact: 会导致已有/内置生成器实现改造成本不确定，FFI/UI 能力发现依赖点不稳定，容易出现“文档正确但代码接口冲突”。
Suggestion: 在设计中明确“唯一权威接口版本”（建议显式声明与 `docs/generator.md` 对齐），并给出兼容策略：保留 `Type()` 还是改为 `Meta().Type`、旧实现如何适配。
Traceability: Requirement 1.1, 1.4
Evidence: `design.md` 的 “Generator 完整定义（与 steering 对齐）” 与 `.kiro/steering/generator.md`/`.kiro/steering/tech.md` 接口描述不一致。

🔴 Critical Issue 2: “类型默认生成器 + 字段显式覆盖”缺少可执行决策规则
Concern: 需求要求优先级可预测，但设计只描述了存在默认与候选，未给出明确决策算法（冲突场景、禁用场景、schema 变更后的回退行为）。
Impact: UI 展示、配置保存、预览结果可能出现不一致（同字段在不同入口选出不同 generator），导致行为不可预测。
Suggestion: 在 `GeneratorTypeResolver` 章节补一段确定性规则：`field explicit > table default > type default`（示例），并定义当 explicit 不兼容时是“失败”还是“回退+警告”。
Traceability: Requirement 2.3, 2.2
Evidence: `design.md` “Components and Interfaces / GeneratorTypeResolver” 与 API 表未给出完整优先级决策矩阵。

## Design Strengths

- 对边界控制做得很好：明确“只做生成与预览，不做跨表编排/写入”，并且通过 `OUT_OF_SCOPE_EXECUTION_REQUEST` 与 `spec-04` 解耦。
- schema trust gate 与错误模型区分清晰，尤其 `GetFieldGeneratorConfig` 在 `pending_*` 下“成功+warnings”这一点兼顾了可用性与安全性。

## Final Assessment

Decision: NO-GO
Rationale: 当前问题不在“功能缺失”，而在“契约一致性与行为确定性”。这两点若不先收敛，会在实现阶段放大为接口震荡和前后端行为分叉风险。
Next Steps:

- 先修订 `design.md` 两处：统一 Generator 接口权威版本；补全 default/override 决策矩阵。
- 修订后重新执行：`/kiro/validate-design spec-03-generator-framework`。
- 通过后再进入：`/kiro/spec-tasks spec-03-generator-framework`（或 `-y`）。

## Interactive Discussion

你更倾向哪种接口收敛方案？

- A: 保持 steering 现状（`Type()`），`Meta` 仅作为扩展信息
- B: 升级为 `Meta()` 主出口，同时更新 steering/权威文档并给出兼容层

如果你选定方案，我可以下一步直接帮你产出一版“最小改动的 design 修订清单”。

> Issue 1: 升级为 Meta() 主出口（这是我们刚添加的能力，修改了 design.md 但没有同步。保持 design.md，.kiro/steering/generator.md/.kiro/steering/tech.md 三者一致
>
> Issue 2: 不需要“明确决策算法”。类型默认生成器基于很简单的策略：只需要建立 5 种数据类型对应的默认生成器即可。它用于在 schema 扫描后，给每个字段赋给一个默认生成器，或通过字段名称+类型进行揣测（例如 name 字段赋给一个“姓名生成器”），用户可以随后接受或者调整它。这与冲突、禁用、scheme 变更等动作毫无关系。请调整“支持“类型默认生成器 + 字段显式覆盖”的优先级策略，且行为可预测。” 这样的文字表述，让它更容易理解

已按你的两点意见完成修改，并做了一次一致性复核。

- 已同步 Meta() 主出口到：
  - `.kiro/steering/generator.md`
  - `.kiro/steering/tech.md`
- `design.md` 保持不变（与你要求一致）。
- 已重写 `Requirement 2` 的 AC-3（`requirements.md`）为更直白语义：
“扫描后按 5 种抽象类型先分配默认/推测生成器（如 `name`→姓名生成器），用户可接受或调整”，去掉了容易引发误解的“优先级策略/冲突语义”表达。

如果你愿意，我可以下一步直接基于这版内容再跑一次 `/validate-design spec-03-generator-framework`，给你新的 GO/NO-GO 结论。

> 好的，再跑一次

## Design Review Summary

本轮修订后，`spec-03-generator-framework` 的设计与 steering 已达成关键一致：`Meta()` 主出口已在 `design.md`、`.kiro/steering/generator.md`、`.kiro/steering/tech.md` 对齐。
同时，Requirement 2 的“默认生成器 + 用户调整”语义已变得清晰、可实现，避免了此前“优先级算法”表述带来的歧义。整体看已具备进入实现阶段的质量。

## Critical Issues (≤3)

本次复审未发现需要阻断实现的关键问题（0 项）。

## Design Strengths

- 接口与边界定义清晰：Generator 抽象、Registry、Config/Preview 服务职责分离明确，且与 `spec-04` 的执行边界切分到位。
- schema trust gate 设计成熟：`pending_*` 场景下的失败路径与 `GetFieldGeneratorConfig` 的只读告警路径区分合理，兼顾安全与可用性。

## Final Assessment

Decision: GO

Rationale: 关键一致性问题已修复，需求可追踪、架构边界清楚、错误模型和可复现策略具备实现可操作性，当前风险处于可接受范围。
Next Steps: 直接进入实现或任务执行：`/kiro/spec-impl spec-03-generator-framework`（可按任务分批执行）。

## Interactive Discussion

建议你在实现前只做一个轻量补强（非阻断）：在 `design.md` 的“类型映射规则补充”里补一行“默认/推测分配发生在扫描后初始建议阶段，最终以用户确认配置为准”，这样能进一步减少后续误解。
如果你同意，我可以直接帮你补这句。



---

> 问题 1：PreviewGeneration 表级预览响应结构不明确，API 契约定义 samples[] 为平铺数组，适用于字段级预览。但对于 scope=table 场景，响应结构未明确。请补充 PreviewGeneration 响应结构的 scope 分支定义：
>
> \- scope.type=field → samples: [value1, value2, ...]
>
> \- scope.type=table → samples: {column_name: [value1, ...]}
>
> 
>
> 问题 2：生成器 Reset() 调用时机未明确
>
> 对于序列生成器等有状态生成器，Reset() 是复现性的关键。若在预览/生产请求开始时不调用 Reset()，序列游标可能从上次位置继续，导致相同 seed 无法复现。应在预览/生成流程中明确：每次请求开始时对有状态生成器调用 Reset()。



已按你的两点要求完成文档修订，改动如下：

- 更新 `.kiro/specs/spec-03-generator-framework/design.md`
  - 在 PreviewGeneration 的 API Contract 中把 samples[] 改为按 scope.type 分支定义：
    - `field -> []interface{}`
    - `table -> map[string][]interface{}`
  - 在 GeneratorRuntime 调用约束中明确 Reset() 时机：
    - 每次预览请求开始时，对有状态生成器先执行一次 `Reset()`
    - 每次生成请求开始时（生产执行入口，引用 `spec-04`）同样先执行一次 `Reset()`
    - 同一请求内禁止重复 `Reset()`
  - 在 PreviewGeneration 示例后新增显式结构说明：
    - `scope.type=field -> samples: [value1, value2, ...]`
    - `scope.type=table -> samples: {"column_name":[value1,...]}`
- 更新 `docs/schema.md`
  - `PreviewGeneration` 的返回注释改为分支结构说明，和 spec 保持一致（`field` 为数组，`table` 为按列名映射的对象）。

另外我做了自检，当前这两个改动文件没有 linter 报错



---

> 问题1：@ ```e:\git\loomidbx\.kiro\specs\spec-03-generator-framework\requirements.md:57:61 ```
> ```1. The LoomiDBX Generator Framework shall 提供预览接口，支持单字段与单表范围的样本生成，不触发真实写入。 ```
> ```2. When 预览请求包含固定种子, the LoomiDBX Generator Framework shall 在同一配置与输入上下文下返回可复现结果。 ```
> ```3. The LoomiDBX Generator Framework shall 在预览响应中附带元数据（`generator_type`、参数摘要、是否确定性、警告信息）。 ```
> ```4. If 生成器依赖外部 feed 或计算表达式但依赖未就绪, the LoomiDBX Generator Framework shall 返回 `FAILED_PRECONDITION` 类错误并指示对应上游依赖。 ``` 
> Req 4.1/4.3，table scope 预览的部分失败处理策略不明确，用户无法预知哪些字段被跳过或失败，请增加预览异常处理策略
>
> 问题2：@ ```e:\git\loomidbx\.kiro\specs\spec-03-generator-framework\requirements.md:34:34 2. When 字段 schema 类型发生变化, the LoomiDBX Generator Framework shall 基于最新 schema 重新判定候选生成器集合，并输出不兼容提示。 ``` ，
> 请补充一个默认生成器映射表（抽象类型 → 默认生成器） ``` 



已按你提的两个问题完成修改，且只改了 `requirements.md`。

- 问题1（Req 4 table scope 异常策略）
  - 在 `Requirement 4` 新增两条验收标准（4.5/4.6）：
  - 明确 `scope=table` 下允许“部分成功”，但失败/跳过字段不得静默丢失
  - 强制响应包含字段级结果清单，至少有：`field`、`status`(`ok|skipped|failed`)、`error_code?`、`warning?`
  - 这样 UI/调用方可以明确知道哪些字段成功、跳过、失败
- 问题2（默认生成器映射表）
  - 在 `Requirement 2` 新增 2.6，并补了“抽象类型 → 默认 `generator_type`”映射表：
  - `int` → `int_range_random`
  - `decimal` → `decimal_range_random`
  - `string` → `string_random_chars`
  - `boolean` → `boolean_ratio`
  - `datetime` → `datetime_range_random`

如果你愿意，我下一步可以把同样的两处约束同步到 `design.md`（接口响应结构与映射来源章节），保证需求-设计完全对齐。



已同步完成到 `design.md` 和 `tasks.md`，并补了必要测试任务。

### 已更新内容

- `\.kiro/specs/spec-03-generator-framework/design.md`
  - 在 FFI 契约表中增强 PreviewGeneration 响应：
    - 新增 `field_results[]`（`scope=table` 必填）
  - 在 Runtime 规则中补充 table scope 异常策略：
    - 明确“部分成功”语义（成功字段可返回，失败/跳过字段不得静默丢失）
    - 明确 field_results[] 最小字段：
      - `field`
      - `status`=`ok|skipped|failed`
      - `error_code?`
      - `warning?`
    - 明确 `status=ok` 字段集合需与 `samples` 对齐
  - 在“类型映射规则补充（MVP）”中补充默认映射基线：
    - `int -> int_range_random`
    - `decimal -> decimal_range_random`
    - `string -> string_random_chars`
    - `boolean -> boolean_ratio`
    - `datetime -> datetime_range_random`
- `\.kiro/specs/spec-03-generator-framework/tasks.md`
  - Requirement 2 新增实现任务：
    - `2.5` 固化默认映射表并暴露可查询结果（覆盖 `2.6`）
  - Requirement 4 新增实现任务：
    - `4.4` table scope 部分失败策略（覆盖 `4.5`）
    - `4.5` 输出 `field_results[]` 并做一致性校验（覆盖 `4.6`）
  - 新增测试任务：
    - `6.11` 契约/集成测试：`scope=table` 的 `ok/skipped/failed` 混合场景与 `field_results[]` 完整性
    - `6.12` 单元测试：默认映射表稳定性与回归保护
  - 覆盖矩阵已同步：
    - `2.1–2.6`
    - `4.1–4.6`

当前需求-设计-任务三层已经对齐，你可以直接进入实现阶段。



---

> 当前spec: spec-03-generator-framework. 请把 tasks.md 再细化为可并行执行的“Batch A/B/C”实施批次以及模板，我在上一个 Specs 任务模版为：
>
> \---
>
> /spec-impl 你现在执行 spec-03 的实现批次 {批次编号}，任务范围仅限：{任务号列表}。
>
> 
>
> 注意：以最新 design.md 为准，覆盖 tasks.md 中旧表述。
>
> 
>
> 必须实现/重点验收：
>
> \- ...
>
> \- ...
>
> 
>
> 完成后必须按以下格式输出：
>
> \- 将已完成任务在 tasks.md 中标记完成
>
> \- 已完成任务：<逐条列出任务号>
>
> \- 变更文件：<路径列表>
>
> \- 验证结果：<测试命令与结果摘要>
>
> \- 未解决问题/风险：<如无写“无”>
>
> \- 下一批前置条件：<满足/不满足 + 原因>


>>>>>>> b7aedc3 (docs(spec): refine spec-03 docs and add batch templates)



