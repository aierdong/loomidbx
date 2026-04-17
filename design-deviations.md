> 请查看我们最初的设计，从当前实现，以及你对 IntSequenceGenerator  的规划来看似乎并没有遵从这个设计（例如文件结构，CommonOptions的设计等）。我想知道造成这个偏差的原因，是因为 steering 中 generator.md 丢失了太多细节，或者缺乏足够的文档引用？
> 1. 请首先阅读 /docs/generator.md /.kiro/steering/generator.md
> 2. 回顾当前的实现（/backend/generator/*.go）
> 3. 分析当前的实现与原始设计的偏差有哪些
> 4. 你的 IntSequenceGenerator 与原始设计的偏差有哪些
> 总结这些偏差的原因



你这个质疑非常准确。基于你要求的 4 步，我做了对照审计，结论是：当前实现确实与最初 `docs/generator.md` 存在结构性偏差，而我之前给的 `IntSequenceGenerator` 方案也继承了这些偏差（偏“贴现状增量接入”）。

## 1) 设计基线回顾（`docs/generator.md` + `steering/generator.md`）

`docs/generator.md` 的原始设计强调的是一套“完整生成器体系”，关键包括：

- 分层文件结构（`interface.go`、`registry.go`、`base.go`、`serialization.go`、`types.go`、`metadata.go` + `int/sequence.go` 等）
- `CommonOptions` + `BaseGenerator` 组合复用
- `GeneratorConfig{type, common_options, specific_options}` 的统一配置载体
- `FromJSON` 多态反序列化
- `GeneratorMetadata.ConfigSchema`（给 UI 动态表单）
- 明确推荐 `int/sequence.go` 作为优先实现之一

`.kiro/steering/generator.md` 明确写了：这是长期记忆，冲突时以 `docs/generator.md` 为准，并且只保留稳定约束，不保留实现细节。

------

## 2) 当前实现现状（`backend/generator/*.go`）

当前代码主干集中在：

- `backend/generator/framework.go`：接口、注册、候选解析、配置校验、配置服务
- `backend/generator/preview.go`：预览服务、runtime、seed 规则
- 子目录仅有 `doc.go` 占位（`int`/`string`/`decimal`/`datetime`/`boolean`）

也就是说目前是“框架能力先落地，具体生成器基本未实现”的状态。

------

## 3) 当前实现 vs 原始设计：主要偏差

### A. 目录与模块拆分偏差（明显）

- 设计：多文件分层 + 各类型子目录具体实现
- 现状：核心逻辑主要堆在 `framework.go` / `preview.go`，子目录仅占位

### B. 公共配置模型偏差（核心）

- 设计：`CommonOptions`、`BaseGenerator.ApplyCommonOptions`
- 现状：没有 `CommonOptions/BaseGenerator` 统一抽象；参数主要是 `GeneratorOpts map[string]interface{}`

### C. 配置序列化与多态偏差

- 设计：`GeneratorConfig + FromJSON(type 分发)`
- 现状：走字段级配置模型 + map 参数，未形成设计中的统一多态配置对象

### D. 元数据能力偏差

- 设计：`GeneratorMetadata` 包含 `ConfigSchema`、`Examples` 等，用于 UI 动态渲染
- 现状：`GeneratorMeta` 仅有 `Type/TypeTags/Deterministic`，没有完整 schema 驱动能力

### E. 生成器实现覆盖偏差

- 设计：完整类型体系，且 `int/sequence.go` 为优先项
- 现状：仅看到通用框架 + `enum_value` 相关校验路径，具体生成器基本未落地

------

## 4) 我之前 IntSequenceGenerator 规划 vs 原始设计：偏差点

你说得对，我之前的方案偏向“顺着现有实现加一层”，因此与原始设计有这些差异：

- 倾向在 `backend/generator/` 增加实现，而不是严格落到 `backend/generator/int/sequence.go`
- 方案没有先引入 `CommonOptions/BaseGenerator`，而是直接走 `GeneratorOpts map` 参数
- 方案围绕现有注册/校验/FFI 链路增量接入，没有先补 `GeneratorConfig + FromJSON + ConfigSchema` 这一套
- 目标是“最小改动可用”，不是“回归原始架构蓝图一致性”

------

## 偏差原因总结（你问的核心）

结论不是单一原因，而是两层叠加：

- 不是 steering 丢细节导致“看不到设计”
  `steering/generator.md` 本来就定位为长期记忆摘要，并明确“以 `docs/generator.md` 为准”。它不是详细设计文档，缺细节是有意设计。
- 主要原因是实施阶段采用了“spec-03 的收敛范围 + 渐进落地”
  `spec-03` 实际落地优先了“可配置/可预览/可校验的框架闭环”，未强制按 `docs/generator.md` 的完整目录和 `CommonOptions` 架构一次性实现，导致现状与最初蓝图出现结构漂移。
  我之前的 IntSequence 方案又进一步遵循“当前代码现实”，所以继续放大了这种漂移。

如果你愿意，我下一步可以给你一份“对齐原始设计的 IntSequenceGenerator 方案”（不是最小改动版），把 `CommonOptions/BaseGenerator/int/sequence.go` 一并纳入，并标注和现有代码的兼容迁移路径。

---

> 我所希望的工程实践是：尽量把框架、关键技术、Spec 拆分、执行计划放在前期解决（/docs下相关产物），编码阶段能严格按照这些产物的约定按部就班的演进。为了达到这个目标，你认为应该如何改进当前流程？例如：我们是否应该在 CLAUDE.md, steering 中增加相应的说明或强制约束？



你的目标非常合理，而且是能落地的。
当前偏差的根因本质上是：“文档是参考”而不是“文档是约束”。要改进流程，需要把 `docs`/spec 从“建议”升级为“带闸门的执行契约”。

先给一个 Todo List（按优先级）：

- 1. 在 `CLAUDE.md` 增加“文档先行 + 偏差审批”强约束
- 1. 在 `.kiro/steering/*.md` 增加“架构冻结点（不可绕过）”
- 1. 在每个 spec 增加“实现前检查清单 + 覆盖映射 + 偏差记录”
- 1. 在 PR/CI 增加自动校验（缺文档、缺映射、未声明偏差则阻断）
- 1. 在实现阶段执行“先校对再编码”的固定节奏

------

## 建议的流程改进（核心）

### 1) 建立“三道闸门”

- Gate A: 设计冻结闸门
  没有完成 `requirements/design/tasks` 且未确认，不进入编码。
- Gate B: 编码前一致性闸门
  编码前必须输出“本次改动涉及的设计锚点清单”（文件+章节），并声明“严格遵循/有偏差”。
- Gate C: 合并前偏差闸门
  若实现与设计不一致，必须有 `deviation record`（偏差原因、影响、回滚/迁移方案），否则禁止合并。

### 2) 把“偏差”从隐式变显式

新增一个固定产物（建议）：`docs/deviations/<spec-name>.md`

- 每次偏差必须登记：
  - 偏差点（设计项 -> 实际实现）
  - 原因（性能/复杂度/依赖/时间窗）
  - 影响范围
  - 临时还是永久
  - 谁批准、何时回补
- 没有偏差记录就不允许“按现状增量扩展”。

### 3) 让 spec 与代码形成可检查映射

在每个 spec 的 `tasks.md` 增加两列强制项：

- `Design Anchor`: 对应 `docs/*.md` 的哪个设计点
- `Code Target`: 预期文件结构与关键符号 这样编码前就能检查“是不是在错误目录里实现了正确功能”。

### 4) 明确“目录结构属于契约”的级别

你现在碰到的 `CommonOptions`、文件结构偏差，本质是“目录结构被当成建议”。
应把“关键目录与核心抽象”标记为：

- MUST：必须遵守（如 `BaseGenerator/CommonOptions`）
- SHOULD：优先遵守（允许偏差但要登记）
- MAY：可选项

------

## 是否要改 `CLAUDE.md` / steering？答案：要，而且要“可执行”

### `CLAUDE.md` 建议新增（强约束）

- “文档优先级规则”（例如：`docs/generator.md` > spec design > steering 摘要 > 现有代码形态）
- “实现前核对清单”（必须先引用设计锚点）
- “偏差审批规则”（无记录不得实现）
- “禁止项”（禁止因现状代码而自动降级架构目标）

### `.kiro/steering/generator.md` 建议新增（稳定约束）

- 标记哪些是“冻结架构点”（如 `CommonOptions`、`BaseGenerator`、分层目录）
- 标记哪些是“阶段性可简化点”
- 明确“与权威文档冲突时如何处理 + 必须产出偏差记录”

------

## 执行节奏建议（你希望的“按部就班”）

每次开发固定 5 步：

1. 对齐：读取权威 docs + 当前 spec，输出“实现锚点清单”
2. 核对：检查代码现状是否已偏离；若偏离先写偏差记录
3. 实现：仅做与锚点一致的任务
4. 验证：测试 + 契约检查 + 覆盖映射检查
5. 归档：更新 tasks 状态与偏差文档

------

如果你同意，我下一步可以只做计划层工作（不改代码）：

- 给出可直接粘贴到 `CLAUDE.md` 的“强约束条款草案”
- 给出 `.kiro/steering/generator.md` 的“冻结点模板”
- 给出 `deviation record` 模板和“PR 必填检查清单模板”

非常好，下面给你一套可以直接落地的约束草案 + 模板。
先给 Todo List（本次交付）：

- 1. `CLAUDE.md` 约束条款草案
- 1. `.kiro/steering/generator.md` 冻结点模板
- 1. 偏差记录模板（deviation record）
- 1. PR 必填检查清单模板
- 1. spec 任务项增强模板（Design Anchor / Code Target）

------

## 1) `CLAUDE.md` 约束条款草案

```markdown

**##** **文档驱动开发约束（强制）**

**###** **1. 权威优先级（必须遵守）**

实现时的约束来源优先级如下（从高到低）：

1) `docs/` 下该领域权威设计文档（例如 `docs/generator.md`）
2) 当前 feature 的 spec 文档（`requirements.md` / `design.md` / `tasks.md`）
3) `.kiro/steering/` 长期记忆与稳定约束
4) 现有代码形态（仅用于兼容性参考，不得反向覆盖设计）

若低优先级内容与高优先级冲突，必须以高优先级为准；不得“按现状代码”默默降级设计目标。

**###** **2. 编码前检查（必须通过）**

开始编码前，必须明确并记录：
 - 本次改动对应的 Design Anchors（权威文档章节/条目）
 - 目标代码落点（目录/文件/核心符号）
 - 是否存在已知偏差（Yes/No）

未完成上述检查，不得进入实现阶段。

**###** **3. 偏差处理（强制留痕）**

若实现无法严格遵循权威设计，必须先新增偏差记录（deviation record），包含：
 - 偏差点（设计 -> 实现）
 - 原因
 - 影响范围
 - 临时/永久判定
 - 回补计划与截止时间
 - 审批人

无偏差记录不得合并。

**###** **4. 目录结构与核心抽象等级**

对关键架构元素标记约束等级：
 - MUST：必须遵守（无审批不可变更）
 - SHOULD：优先遵守（偏差需记录）
 - MAY：可选

涉及 MUST 项的改动，必须先更新 docs 与 spec，再编码。

**###** **5. 验收与追溯（合并前）**

合并前必须满足：
 - spec 任务与实现一一映射
 - 测试覆盖关键契约
 - 若有偏差，偏差记录完整且已审批
 - 文档、spec、实现三者一致或已显式说明差异

------

## 2) `.kiro/steering/generator.md` 冻结点模板（建议追加段落）

**##** **生成器架构冻结点（Implementation Freeze Points）**

以下条目用于约束实现阶段的结构一致性。

**###** **MUST（不可绕过）**
 - 统一接口契约必须稳定：`Meta()` / `Generate()` / `GenerateBatch()` / `Reset()`
 - 公共配置与公共处理必须通过 `CommonOptions` + `BaseGenerator`（组合复用）
 - 关键目录分层必须保持（核心框架层、类型子目录层）
 - 生成器配置必须具备稳定序列化协议（含 type 分发）
 - 与 `docs/generator.md` 冲突时，以 `docs/generator.md` 为准

**###** **SHOULD（优先遵守）**
 - 具体生成器按类型子目录组织（如 `int/sequence.go`）
 - 元数据应支持 UI 动态配置渲染（如 schema/示例）
 - 预览链路与执行链路共享同一配置语义

**###** **MAY（可选扩展）**
 - 性能优化策略（缓存、并发实现细节）
 - 非 MVP 的扩展生成器（AI、Python 表达式等）

**##** **偏差治理规则**
 - 若实现与 MUST/SHOULD 不一致，必须先写偏差记录后编码。
 - 偏差未登记视为流程违规。

------

## 3) 偏差记录模板（`docs/deviations/<spec-name>.md`）

**#** **Deviation Record - <spec-name>**

**##** **基本信息**
 - 记录ID: DEV-<yyyyMMdd>-<nn>
 - Spec: <spec-name>
 - 相关任务: <tasks.md 条目>
 - 提交人:
 - 审批人:
 - 状态: Draft | Approved | Rejected | Resolved
 - 日期:

**##** **1. 偏差描述**
 - 设计基线（应当）:
 - 实际实现（当前）:
 - 偏差类型: Architecture | DataModel | APIContract | FileStructure | Other

**##** **2. 偏差原因**
 - 直接原因:
 - 约束条件（工期/依赖/性能/兼容）:
 - 是否有替代方案: Yes/No
 - 若有，为什么未采用:

**##** **3. 影响评估**
 - 影响范围（模块/接口/用户侧）:
 - 风险等级: Low | Medium | High
 - 是否影响向后兼容: Yes/No
 - 是否影响后续 spec: Yes/No（说明）

**##** **4. 处置策略**
 - 临时绕行 / 永久调整:
 - 回补计划（若临时）:
 - 目标版本/截止日期:
 - 验收标准:

**##** **5. 关联变更**
 - 关联文档:
 - 关联代码:
 - 关联测试:
 - 关联 PR/Commit:

------

## 4) PR 必填检查清单模板

**##** **文档与规格一致性检查（必填）**
 - [ ] 我已阅读并引用本次改动对应的权威文档（`docs/...`）
 - [ ] 我已阅读并引用本次 spec 的 `requirements/design/tasks`
 - [ ] 本次实现与设计一致（若不一致，已附 deviation record）

**###** **Design Anchors（必填）**
 - 文档锚点1:
 - 文档锚点2:
 - 文档锚点3:

**###** **Code Targets（必填）**
 - 目标目录/文件:
 - 核心符号/接口:
 - 未触达但受影响区域:

**###** **偏差声明（必填）**
 - [ ] 无偏差
 - [ ] 有偏差（填写 `docs/deviations/...` 路径）:

**###** **测试与验收（必填）**
 - [ ] 单元测试覆盖关键契约
 - [ ] 集成/契约测试已更新
 - [ ] spec 任务状态已同步
 - [ ] 文档已同步更新（如有需要）

------

## 5) spec 任务项增强模板（放到 `tasks.md` 的任务描述中）
 - [ ] <任务编号> <任务标题>
    - Design Anchor: <docs/...#section> + <design.md#section>
    - Code Target: <path/to/file_or_dir>
    - Contract Impact: <API/Model/ErrorCode>
    - Tests Required: <unit/integration/contract>
    - Deviation Allowed: No | Yes(需记录)
    - Requirements: <1.1, 2.3 ...>

```


如果你愿意，我下一步可以再给你一份**“最小改造落地顺序”**（只改文档流程，不碰代码），比如先从 `CLAUDE.md + PR checklist` 两个点开始，1 天内就能把流程硬起来。

---



非常好。先给你这次交付的 Todo List：

- 1. 给出“最小改造落地顺序”（按优先级 + 时间盒）
- 1. 将 `CLAUDE.md` 约束条款草案改为英文版
- 1. 给出每一步的完成判定（DoD）

------

## 改造落地顺序（最小可执行版）

目标：1-2 天内先把流程“硬约束化”，不等大改代码。

### Phase 0（0.5 天）：先立规矩，再动手

1. 更新 `CLAUDE.md`（英文约束条款，见下方）
2. 更新 `.kiro/steering/generator.md`（补 Freeze Points + 偏差治理）
3. 在仓库加 `docs/deviations/README.md`（说明何时必须建偏差记录）

DoD

- 团队能明确回答：文档优先级是什么、何时必须写偏差记录
- 新任务启动时已有统一口径，不再靠口头同步

------

### Phase 1（0.5 天）：把“检查项”塞进日常入口

1. 增加 PR 模板（必填 Design Anchors / Code Targets / Deviation 声明）
2. 在 spec `tasks.md` 新任务统一使用增强模板字段
   （`Design Anchor`、`Code Target`、`Deviation Allowed`）

DoD

- 任意一个新 PR 都必须填写锚点与偏差声明
- 任意一个新 spec 任务都能追溯到 docs 设计点

------

### Phase 2（0.5-1 天）：加轻量自动闸门（CI）

1. 加一个轻量校验脚本（可先用脚本 + CI step）：
   - PR 描述缺少 `Design Anchors` -> fail
   - 声明“有偏差”但无 `docs/deviations/*.md` -> fail
2. 可选：校验 spec 任务中是否含 `Design Anchor` 字段

DoD

- “没对齐文档就编码”在流程上无法直接通过
- 偏差必须显式登记才能合并

------

### Phase 3（持续）：试运行两周后再收紧

1. 收集 2 周内被拦截案例
2. 调整 MUST/SHOULD 边界（避免过严导致阻塞）
3. 将常见偏差沉淀为 FAQ/范例

DoD

- 误拦截率下降
- 偏差记录质量稳定，能指导后续回补



## `CLAUDE.md` 约束条款草案（English）

```markdown
## Documentation-Driven Delivery Constraints (Mandatory)

### 1) Authority Priority (Must Follow)
Implementation decisions must follow this priority order (highest to lowest):
1. Domain authority documents under `docs/` (e.g., `docs/generator.md`)
2. Active spec documents (`requirements.md`, `design.md`, `tasks.md`)
3. Long-term steering docs under `.kiro/steering/`
4. Existing code shape (compatibility reference only; never used to override design)

If lower-priority content conflicts with higher-priority content, follow the higher-priority source.

### 2) Pre-Implementation Alignment Check (Mandatory)
Before coding, the contributor must explicitly record:
- Design Anchors (exact sections/items from authority docs/spec)
- Code Targets (intended directories/files/symbols)
- Deviation status (Yes/No)

No implementation work should start before this check is complete.

### 3) Deviation Governance (Mandatory Traceability)
If implementation cannot fully follow the authority design, a deviation record is required before coding:
- Expected design vs actual implementation
- Reason and constraints
- Impact scope
- Temporary vs permanent decision
- Backfill plan and due date
- Approver

No deviation record, no merge.

### 4) Constraint Levels for Architecture Elements
Mark key architecture elements with explicit levels:
- MUST: non-negotiable; cannot change without prior approval
- SHOULD: preferred; deviations require a record
- MAY: optional

For MUST-level changes, update docs/spec first, then implement.

### 5) Pre-Merge Consistency Requirements
Before merge, all items below must hold:
- Spec tasks are mapped to implementation changes
- Critical contracts are covered by tests
- Deviations (if any) are fully documented and approved
- Docs, spec, and implementation are consistent, or differences are explicitly documented
```



> 对于文档约束与偏差，我有一些想法，征求你的已经：
>
> 1. 为避免 steering 中细节不足导致的偏差，应在 steering/*.md 中增加到 docs/*.md 中链接/锚点。在不增加 steering 体积的情况下，让 Coding Agent 知道何时、何地寻找文档细节。
> 2. 编码前增加“dry run”环节，评估每个 task 的影响，并与原始设计进行对比。这与你提到的：在 CLAUDE.md 增加“编码前检查”的开发约束想匹配。我希望在 CLAUDE 开发刘晨中强调这一点。
> 3. 偏差的确认：1.如果有重大偏差（违法了MUST 约束等级），应该停下来等待确认 2.其它可以接受的偏差允许继续处理。
> 4. 偏差的后期处理：我希望设计与代码永远对齐，因此偏差产生后，应该更新原始设计，避免后续设计参考了过时的设计（遗留了偏差表）。
>
> 你认为如何？



你的 4 点方向都很对，我建议全部采纳，再补两条防走形规则。

先给结论：这套机制能把“文档参考型开发”升级为“文档约束型开发”，正好解决你这次遇到的偏差问题。

## 逐点评估

1. Steering 增加 docs 链接/锚点
   非常好，建议作为必备结构。

- 在每个 `steering/*.md` 增加 `Authority Anchors` 小节：只放“去哪里看细节”的链接，不复制细节。
- 至少包含：`MUST anchors`（必须遵守）+ `Detail anchors`（实现细节入口）。
- 这样不会膨胀 steering，又能强制 agent 定位权威细节。

1. 编码前加 dry run（影响评估 + 设计对比）
   非常关键，建议升为强制门禁。

- 每个 task 编码前输出 3 件事：
  - `Impact Surface`（会影响哪些模块/契约/测试）
  - `Design Delta`（与原始设计是否有差异）
  - `Execution Decision`（按设计实现 / 需偏差审批）
- 这和你说的“在 CLAUDE 开发流程中强调编码前检查”完全一致（你那句“开发刘晨”我理解是“开发流程”）。

1. 偏差确认分级：重大偏差必须停下确认
   完全同意，建议定义“Stop-the-line”触发器。

- 必须停下确认（Major）：触碰 MUST、破坏外部契约、引入安全/数据一致性风险。
- 可继续（Minor）：不触碰 MUST、可逆、无契约破坏；但仍需记录并在 PR 标注。
- 关键是把 Major/Minor 判定标准写死，避免主观漂移。

1. 偏差后应更新原始设计，保持长期对齐
   方向正确，但建议加一个前置判断：

- 不是“发生偏差就直接改设计”，而是先决定：
  - A. 代码回补到设计（设计不变）
  - B. 设计升级接纳实现（设计变更）
- 一旦选择 B，必须同步更新权威文档，确保“当前设计真相”唯一。
- 这样不会长期依赖偏差表，也不会把临时绕行误写成正式设计。

## 我补充的两条防走形规则

- Rule A: 偏差记录有 TTL
  每条偏差必须有“回补截止时间/版本”；过期未处理自动升级为阻断项。
- Rule B: 设计变更必须带版本与变更理由
  在 `docs/*.md` 增加 `Design Change Log`，避免“静默改设计”。

------

如果你认可，我下一步可以给你一版可直接粘贴的文本：

- `steering` 的 `Authority Anchors` 模板（中英双语可选）
- `dry run` 模板（每个 task 一页）
- `Major/Minor` 判定规则模板
- `Design Change Log` 模板

收到，我会先按你的想法制定并执行一个小型落地清单：1) 在 `CLAUDE.md` 增加 SOP 级流程约束（含 dry run 与偏差分级）；2) 在 `steering` 增加 `Authority Anchors` 导航模板；3) 补充可复用模板文档（dry run、偏差分级、设计变更日志）。先读取现有文件再改。



已按你的方向完成落地，并且已一并调整 `CLAUDE.md`。
是的，你理解得对：现在这部分可以作为“开发流程 SOP”的核心约束。

已完成项（对应你前面 4 条想法）：

- 在 `CLAUDE.md` 新增英文 SOP 约束：文档优先级、`Authority Anchors`、编码前 `Dry Run`、Major/Minor 偏差分级、偏差后“设计与代码收敛”机制。
- 在 `steering` 中补“去哪里找细节”的锚点入口，避免因 steering 精简导致实现偏航：
  - `.kiro/steering/generator.md`
  - `.kiro/steering/database-schema.md`
- 新增 3 个可复用模板（可直接用于流程）：
  - `docs/dry-run-template.md`
  - `docs/deviation-record-template.md`
  - `docs/design-change-log-template.md`
- 已检查改动文件，无 linter 问题。

------

如果你愿意，下一步我可以继续把这套 SOP 再“闭环”两步（仍只改文档）：

- 增加 `docs/deviations/README.md`（定义何时必须建偏差记录 + TTL 规则）
- 追加一个 `.github/pull_request_template.md`，把 `Design Anchors` / `Deviation` 变成 PR 必填项（这样执行约束会更稳）

我会继续推进，并先按你的规则给出 Todo List：1) 新增 `docs/deviations/README.md`（偏差触发条件、Major/Minor、TTL 与收敛要求）；2) 新增 `.github/pull_request_template.md`（Design Anchors 与 Deviation 必填）；3) 做一次快速一致性检查。先完成文件补齐。



已继续完成你要的两项“流程闭环”改造：

- 新增 `docs/deviations/README.md`
  - 定义了偏差触发条件
  - 明确 `Major/Minor` 分级和 `Stop-the-line` 规则
  - 增加了偏差收敛策略（代码回补或设计更新二选一）
  - 增加 TTL 机制（到期未关闭升级为阻断）
- 新增 `.github/pull_request_template.md`
  - 强制 `Design Anchors`、`Code Targets`、`Dry Run`、`Deviation Declaration`
  - 要求偏差必须给出记录路径、分级、审批状态、收敛路径与 TTL
  - 补充验证与后续同步检查项

另外你问的理解是对的：
`CLAUDE.md` 现在可以视为“开发流程 SOP”的总规，`steering` 负责“领域约束 + 锚点导航”，PR 模板负责“执行落地与审查闸门”。

如果你愿意，下一步我可以再补一个很小但实用的文档：`docs/sop-checklist.md`（一页纸版本，便于日常执行，不用每次翻多个文件）。
