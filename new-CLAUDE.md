# AI-DLC and Spec-Driven Development

## 1. 目标与范围

本文件定义：在本项目中，Coding Agent 必须如何使用：

- 项目级规则（Steering）  
- 特性规格（Specs）  
- Skills 文档（领域权威）  
- 模板文件（固定输出骨架）

来产出**简单、可追溯、结构正确**的代码。

***

## 2. 路径与角色

### 2.1 目录约定

- Steering（项目级规则与上下文）：`.kiro/steering/`  
- Specs（特性规格）：`.kiro/specs/`  
- Skills（唯一权威领域文档）：`.claude/skills/`  
- Templates（固定输出模板）：`.claude/templates/`

### 2.2 各自职责

- **Steering**  
  - 定义项目级规则、技术栈、结构契约。  
  - 维护 Skills 索引，将特性 / 任务映射到具体 Skills。  

- **Specs**  
  - 针对特性，描述需求、设计、任务列表、实现状态。  

- **Skills（`.claude/skills/`）**  
  - 每个文件聚焦一个子领域或模式（如 login flow、账单对账）。  
  - 定义领域行为、业务流程、对外契约、结构约束。  
  - 所有实现以 Skills 为最终权威，不再使用 `docs/*.md`。  

- **Templates（`.claude/templates/`）**  
  - 定义固定输出结构（Dry Run、偏差记录、设计变更日志等）的字段与格式。  
  - CLAUDE.md 只规定“必须使用哪些模板”，具体字段以模板文件为准。

***

## 3. 语言与风格

- 默认：用英文思考，输出统一使用**简体中文**（除非 Spec 明确要求其他目标语言）。  
- 写入项目文件的 Markdown 内容（requirements、design、tasks、research、验证报告等）必须使用 `spec.json.language` 指定的语言。  
- 表达要求：  
  - 短句、主动语态；  
  - 尽量少用行话，技术术语出现时用括号做一句简短解释；  
  - 任何文档都要让“不熟悉项目的人”能快速看懂意图。

***

## 4. AI‑DLC 流程（压缩版）

### 4.1 流程阶段

- Phase 0（可选，加载项目规则）：  
  - `/kiro:steering`  
  - `/kiro:steering-custom`  

- Phase 1（规格阶段）：  
  - `/kiro:spec-init`  
  - `/kiro:spec-requirements`  
  - `/kiro:validate-gap`（可选）  
  - `/kiro:spec-design`  
  - `/kiro:validate-design`（可选）  
  - `/kiro:spec-tasks`  

- Phase 2（实现阶段）：  
  - `/kiro:spec-impl {feature} [tasks]`  
  - `/kiro:validate-impl {feature}`（可选）  

- 随时：`/kiro:spec-status {feature}` 查询特性进度。

### 4.2 通用规则

- 固定流程：需求 → 设计 → 任务 → 实现。  
- 建议每一阶段都有人工审查；使用 `-y` 时视为“在充分知情下主动加速”。  
- 在用户指令范围内，尽量一次完成：拉取上下文 → 规划 → 实现 → 自检。  
- 信息缺失或存在严重歧义时再提问，避免无声假设。

***

## 5. Steering 与 Skills 使用

### 5.1 Steering 配置

- 推荐 Steering 文件：  
  - `product.md`：产品目标、关键场景。  
  - `tech.md`：技术栈、性能 / 安全 / 合规约束。  
  - `structure.md`：**项目结构契约**（目录、分层、命名规则）。  

- 在 Steering 中维护 **Skills 索引**，示例：

  ```markdown
  Skills Index:
  - auth/login-flow           -> .claude/skills/auth/login-flow.md
  - auth/http-api-login       -> .claude/skills/auth/http-api-login.md
  - billing/reconciliation    -> .claude/skills/billing/reconciliation.md
  ```

### 5.2 权威优先级

本项目只保留一套权威文档（Skills），优先级从高到低：

```text
Skills（.claude/skills/*，权威设计）
  > 当前激活的 Spec
  > Steering 总结
  > 现有代码形状
```

当各层内容冲突时，必须遵从上述顺序，并在偏差记录中说明原因。

***

## 6. 行为准则（精简 Skills）

### 6.1 先思考再编码

在 `/kiro:spec-design`、`/kiro:spec-tasks`、`/kiro:spec-impl` 中：

- 写出 1–3 条关键假设，覆盖：  
  - 需求理解、  
  - 领域行为、  
  - 所引用 Skills 的关键约束。  
- 若存在多种理解或实现方案：  
  - 简要列出至少 2 种，并说明当前选择的理由。  
- 若 Skills 或 Spec 有不清晰之处：  
  - 指明“哪里不清楚”，必要时请求澄清，而不是自己乱补设定。

### 6.2 简单优先

在不违反 Skills 的前提下：

- 只实现当前 Spec/任务所需的**最小代码**。  
- 不做“未来可能用得上”的扩展、配置或抽象。  
- 不给单一调用点设计复杂抽象。  
- 不为被明确认定为“不可能发生”的场景写过度防御代码。  
- 发现可明显简化的实现时，应优先选择更简单的方案，并在设计或 Dry Run 中简单说明。

### 6.3 精准修改

在 `/kiro:spec-impl` 中修改现有代码时：

- 仅修改：  
  - 当前 Spec 任务要求的内容，且  
  - 已在 Dry Run 的 `Impact Surface` 中列出的文件 / 模块 / 测试。  
- 禁止：  
  - 顺手美化无关代码、注释或格式；  
  - 未经请求重构其他模块。  
- 保持与已有代码风格一致。  
- 删除孤儿（未使用的 import / 变量 / 函数）时：  
  - 只删除由本次修改产生的孤儿；  
  - 历史死代码除非有明确指令，不主动清理。  
- 如必须修改 `Impact Surface` 之外的文件：  
  - 更新 `Impact Surface`；  
  - 在偏差记录中说明原因。

### 6.4 目标驱动

- 将模糊指令改写为可验证目标：  
  - “加校验” → “先写无效输入的测试，再实现使其通过”；  
  - “修 bug” → “先写复现测试，再修复使测试通过”。  
- 在实现前写出简短步骤计划，每步附带“验证方式”。  
- `/kiro:spec-impl` 每次设定 2–5 条成功标准，在 `/kiro:validate-impl` 中逐条确认是否达成。

***

## 7. 开发 SOP（Skills‑Only + 模板）

以下为强制流程，不符合即视为错误，需要重跑。

### 7.1 项目结构契约（由 `structure.md` 定义）

- 明确：  
  - 顶层目录（如 `src/`、`tests/`、`.claude/skills/` 等）及各自职责；  
  - 分层规则（接口层、应用层、领域层、基础设施层等的文件位置）；  
  - 命名约定（Service、UseCase、DTO、测试文件后缀等）。  
- 新建文件 / 目录前必须对照结构契约自检：  
  - 若不符合，必须作为偏差记录，不得悄然更改结构。

### 7.2 模板引用约定

- 模板文件统一放在 `.claude/templates/` 下，例如：  
  - Dry Run 模板：`.claude/templates/dry-run.md`  
  - Deviation Record 模板：`.claude/templates/deviation-record.md`  
  - Design Change Log / Conformance 模板：`.claude/templates/design-change-log.md`  

- 生成以下内容时，必须使用对应模板的字段结构：  
  - Pre‑Coding Dry Run  
  - Deviation Record（偏差记录）  
  - 设计变更 / 事后符合性检查（Design Change Log / Conformance）

> CLAUDE.md 只定义“必须有哪些产物”，  
> 具体字段与写法以模板文件为准，模板更新后以模板为最终规范。

### 7.3 `/kiro:spec-impl` 必备输出（骨架）

每次 `/kiro:spec-impl` 必须包含以下 4 块内容（可用模板组合实现）：

1. **Skills Anchors**

   - 列出本次实现所依据的 Skills：  
     - `.claude/skills/auth/login-flow.md`  
     - `.claude/skills/auth/http-api-login.md`  

2. **Pre‑Coding Dry Run**（结构来自 `dry-run.md` 模板）

   - 必含字段：Assumptions、Impact Surface、Design Delta、Execution Decision 等。

3. **Deviation Record**（结构来自 `deviation-record.md` 模板）

   - 必含字段：Type（major/minor/none）、Description、Reason、Impact Surface Changes 等。  
   - 即使没有偏差，也必须显式写 `Type: none`。

4. **Conformance / Design Change Log**（结构来自 `design-change-log.md` 模板）

   - 用简短 checklist 对照：  
     - 是否遵守项目结构契约；  
     - 对外契约是否与 Skills 一致；  
     - 预设成功标准是否达成；  
     - 是否有设计真实变化需要写入 Skills。  

> 若缺失上述任意一块（尤其是 Skills Anchors、Dry Run、Deviation、Conformance），  
> 则视为本次实现无效，不应合并入代码库。

### 7.4 任务粒度控制

- 单次 `/kiro:spec-impl` 不应覆盖过多文件或对外接口。  
- 若预估 `Impact Surface` 过大，应在 `/kiro:spec-tasks` 阶段先拆分为多个小任务。  
- 目标：每次实现都可以在一次对话上下文中被完整理解、审查和回滚。

***

## 8. 极简示例（逻辑骨架）

```markdown
Skills Anchors
- .claude/skills/auth/login-flow.md
- .claude/skills/auth/http-api-login.md

Pre-Coding Dry Run
（使用 .claude/templates/dry-run.md 的字段结构）

Deviation Record
（使用 .claude/templates/deviation-record.md 的字段结构）

Conformance / Design Change Log
（使用 .claude/templates/design-change-log.md 的字段结构）
```


下面先给出英文版 CLAUDE.md（与当前中文版结构一一对应），然后给出三份模板文件的内容骨架，方便你直接放进 `.claude/templates/` 使用。

***

# CLAUDE.md (Concise, Skills‑Only + Templates)

## 1. Purpose & Scope

This document defines how the coding agent must use:

- Project‑level rules (Steering)  
- Feature specifications (Specs)  
- Skill documents (authoritative domain knowledge)  
- Template files (fixed output skeletons)

to produce **simple, traceable, and structurally correct** code.

***

## 2. Paths & Roles

### 2.1 Directories

- Steering (project rules & context): `.kiro/steering/`  
- Specs (feature specifications): `.kiro/specs/`  
- Skills (single source of domain truth): `.claude/skills/`  
- Templates (fixed output layouts): `.claude/templates/`

### 2.2 Responsibilities

- **Steering**  
  - Defines project‑level rules, tech stack, and structure contract.  
  - Maintains the Skills index and maps features/tasks to specific Skills.

- **Specs**  
  - Describe requirements, design, task breakdown, and implementation status for each feature.

- **Skills** (`.claude/skills/`)  
  - Each file focuses on one sub‑domain or pattern (e.g. login flow, billing reconciliation).  
  - Define domain behavior, business flows, external contracts, and structural constraints.  
  - All implementation work must treat Skills as the final authority; do not use `docs/*.md` as authority.

- **Templates** (`.claude/templates/`)  
  - Define the structure and fields of fixed outputs (Dry Run, Deviation Record, Design Change Log / Conformance).  
  - CLAUDE.md only specifies **which templates must be used**; concrete fields are defined in the template files.

***

## 3. Language & Style

- Default: think in English, **respond in Simplified Chinese**, unless a spec explicitly configures another target language.  
- All Markdown artifacts written into the project (requirements, design, tasks, research, validation reports, etc.) MUST use the language configured in `spec.json.language`.  
- Style guidelines:  
  - Short sentences, active voice.  
  - Minimize jargon; when a technical term is needed, add a short plain‑language explanation in parentheses.  
  - Every document should be understandable by someone unfamiliar with the project within a short reading time.

***

## 4. AI‑DLC Flow (Compact)

### 4.1 Phases

- Phase 0 (optional, load project rules):  
  - `/kiro:steering`  
  - `/kiro:steering-custom`  

- Phase 1 (Specification):  
  - `/kiro:spec-init`  
  - `/kiro:spec-requirements`  
  - `/kiro:validate-gap` (optional)  
  - `/kiro:spec-design`  
  - `/kiro:validate-design` (optional)  
  - `/kiro:spec-tasks`  

- Phase 2 (Implementation):  
  - `/kiro:spec-impl {feature} [tasks]`  
  - `/kiro:validate-impl {feature}` (optional)  

- At any time:  
  - `/kiro:spec-status {feature}` to inspect feature status.

### 4.2 General Rules

- Follow the pipeline: Requirements → Design → Tasks → Implementation.  
- Human review is recommended for each phase; using `-y` means “fast‑track with explicit awareness of the risk”.  
- Within the user’s instruction scope, prefer to complete in one run: gather context → plan → implement → self‑check.  
- Ask clarifying questions only when information is missing or instructions are critically ambiguous.

***

## 5. Steering & Skills Usage

### 5.1 Steering Configuration

- Recommended Steering files:  
  - `product.md`: product goals and key user scenarios.  
  - `tech.md`: tech stack, performance/security/compliance constraints.  
  - `structure.md`: **Project Structure Contract** (directories, layering, naming rules).

- Maintain a **Skills Index** in Steering, for example:

  ```markdown
  Skills Index:
  - auth/login-flow           -> .claude/skills/auth/login-flow.md
  - auth/http-api-login       -> .claude/skills/auth/http-api-login.md
  - billing/reconciliation    -> .claude/skills/billing/reconciliation.md
  ```

### 5.2 Authority Priority

There is exactly one authoritative documentation layer (Skills). Priority from high to low:

```text
Skills (.claude/skills/*, authoritative design)
  > Active Spec for this feature
  > Steering summaries
  > Existing code shape
```

When conflicts exist, follow this order and explain deviations in the Deviation Record.

***

## 6. Behavioral Guidelines (Concise Skills)

### 6.1 Think Before Coding

During `/kiro:spec-design`, `/kiro:spec-tasks`, and `/kiro:spec-impl`:

- Write 1–3 key assumptions that cover:  
  - Requirement understanding,  
  - Domain behavior,  
  - Critical constraints from the referenced Skills.  
- If there are multiple interpretations or implementation paths:  
  - List at least two options briefly,  
  - State why you choose the current one.  
- If something in Skills or the Spec is unclear:  
  - Explicitly name what is unclear,  
  - Ask for clarification when needed instead of silently guessing.

### 6.2 Simplicity First

Within the boundaries of Skills:

- Implement the **minimal code** required for the current Spec/tasks.  
- Do not add future‑feature hooks, unused configurability, or premature abstractions.  
- Do not build complex abstractions for a single call site.  
- Do not implement heavy error handling for cases that Skills declare as impossible.  
- When a clearly simpler implementation exists, prefer it and briefly note the reasoning in the Design or Dry Run.

### 6.3 Surgical Changes

During `/kiro:spec-impl` when modifying existing code:

- Only modify:  
  - What current Spec tasks require, and  
  - Files/modules/tests listed in the `Impact Surface` of the Dry Run.  
- Do NOT:  
  - “Clean up” unrelated code/comments/formatting,  
  - Refactor unrelated modules without explicit request.  
- Match the existing style of the touched code.  
- When removing orphans (unused imports/variables/functions):  
  - Only remove those introduced by this change,  
  - Do not delete old dead code unless explicitly requested.  
- If you must touch files beyond the original `Impact Surface`:  
  - Update `Impact Surface`,  
  - Record the reason in the Deviation Record.

### 6.4 Goal‑Driven Execution

- Turn vague instructions into verifiable goals:  
  - “Add validation” → “Write tests for invalid inputs, then implement until they pass”.  
  - “Fix the bug” → “Write a test that reproduces the bug, then fix it to make the test pass”.  
- Before implementation, write a short step‑by‑step plan with “how to verify” for each step.  
- For each `/kiro:spec-impl`, define 2–5 success criteria and, in `/kiro:validate-impl`, explicitly confirm which are met and which are not.

***

## 7. Development SOP (Skills‑Only + Templates)

The following SOP is **mandatory**. Non‑compliant runs are considered invalid and should be re‑run.

### 7.1 Project Structure Contract (defined in `structure.md`)

- Define clearly:  
  - Top‑level directories (e.g. `src/`, `tests/`, `.claude/skills/`, etc.) and their responsibilities,  
  - Layering rules (where interface layer, application layer, domain, infrastructure live),  
  - Naming conventions (Service, UseCase, DTO, test files, etc.).  
- Before creating any new file/directory:  
  - Check it against the structure contract.  
  - If it violates the contract, treat it as a deviation and record it; never introduce structural changes silently.

### 7.2 Template Usage

- Template files are stored under `.claude/templates/`, for example:  
  - Dry Run template: `.claude/templates/dry-run.md`  
  - Deviation Record template: `.claude/templates/deviation-record.md`  
  - Design Change Log / Conformance template: `.claude/templates/design-change-log.md`  

- When generating the following content, you MUST follow the corresponding template structure:  
  - Pre‑Coding Dry Run  
  - Deviation Record  
  - Design Change Log / Conformance

> CLAUDE.md only defines **what artifacts must exist**.  
> The concrete fields and format are defined in template files.  
> When template versions change, the templates are the source of truth.

### 7.3 Required Output for `/kiro:spec-impl` (Skeleton)

Each `/kiro:spec-impl` run MUST include these four parts (built using templates):

1. **Skills Anchors**

   - List the Skills used as authority for this implementation, for example:  
     - `.claude/skills/auth/login-flow.md`  
     - `.claude/skills/auth/http-api-login.md`

2. **Pre‑Coding Dry Run** (structure from `dry-run.md` template)

   - Must include at least: Assumptions, Impact Surface, Design Delta, Execution Decision.

3. **Deviation Record** (structure from `deviation-record.md` template)

   - Must include at least: Type (major/minor/none), Description, Reason, Impact Surface Changes.  
   - Even when there is no deviation, explicitly output `Type: none`.

4. **Conformance / Design Change Log** (structure from `design-change-log.md` template)

   - Use a short checklist to confirm:  
     - Structure contract compliance,  
     - External contracts vs Skills,  
     - Success criteria status,  
     - Any real design changes that should be written back into Skills.

> If any of these four parts is missing  
> (especially Skills Anchors, Dry Run, Deviation, Conformance),  
> the implementation is considered invalid and must not be merged.

### 7.4 Task Granularity Control

- A single `/kiro:spec-impl` should not touch too many files or public entry points.  
- If the estimated `Impact Surface` is large, split the work into several smaller tasks during `/kiro:spec-tasks`.  
- Goal: every implementation run should fit comfortably into one conversation context and be easy to review and roll back.

***

## 8. Minimal Example (Logical Skeleton)

```markdown
Skills Anchors
- .claude/skills/auth/login-flow.md
- .claude/skills/auth/http-api-login.md

Pre-Coding Dry Run
(Use fields from .claude/templates/dry-run.md)

Deviation Record
(Use fields from .claude/templates/deviation-record.md)

Conformance / Design Change Log
(Use fields from .claude/templates/design-change-log.md)
```

***

## Template File Skeletons

下面是三份模板的“字段骨架”，你可以按需增减字段，直接保存为对应文件。

### `.claude/templates/dry-run.md`

```markdown
# Pre-Coding Dry Run

## Context
- Feature / Spec:
- Tasks in scope:
- Related Skills:
  - .claude/skills/...

## Assumptions
- [Assumption 1]
- [Assumption 2]
- [Assumption 3]

## Impact Surface
- Files to create:
  - path/to/file1
- Files to modify:
  - path/to/file2
- Tests to add/modify:
  - path/to/test1

## Design Delta
- Summary:
  - [Do we diverge from Skills? Where and why?]
- Details:
  - [Delta 1]
  - [Delta 2]

## Execution Decision
- Strategy:
  - [Follow Skills exactly | Proceed with deviations]
- Notes:
  - [Anything that needs reviewer attention before coding]
```

### `.claude/templates/deviation-record.md`

```markdown
# Deviation Record

## Overview
- Feature / Spec:
- Date:
- Author / Agent run id:

## Deviation Type
- Type: [major | minor | none]

## Description
- What differs from Skills / Spec / structure contract:
  - [Item 1]
  - [Item 2]

## Reason
- Why this deviation is necessary or chosen:
  - [Reason 1]
  - [Reason 2]

## Impact Surface Changes
- Files added beyond original Impact Surface:
  - [file paths or "none"]
- Files modified beyond original Impact Surface:
  - [file paths or "none"]

## Risk & Mitigation
- Potential risks:
  - [Risk 1]
- Mitigations:
  - [Mitigation 1]

## Review Status
- Status: [pending | approved | rejected]
- Reviewer:
- Notes:
```

### `.claude/templates/design-change-log.md`

```markdown
# Design Change Log / Conformance Check

## Context
- Feature / Spec:
- Related Skills:
  - .claude/skills/...

## Conformance Checklist
- Project Structure Contract:
  - [pass | fail] – details if fail:
- External contracts vs Skills:
  - [pass | fail] – details if fail:
- Control flow vs Skills (main & error paths):
  - [pass | fail] – details if fail:
- Pre-defined success criteria:
  - Criterion 1: [pass | fail] – note:
  - Criterion 2: [pass | fail] – note:
  - Criterion 3: [pass | fail] – note:

## Design Changes to Record
- Do we need to update any Skill files? [yes | no]
- If yes:
  - Target Skills:
    - .claude/skills/...
  - Proposed updates:
    - [Short description of the new truth]

## Follow-ups / TODOs
- [Item 1]
- [Item 2]
```

你可以先把这四个文件（英文 CLAUDE.md + 3 个模板）落到仓库里试跑一个小 feature，看实际交互中还有哪些多余或不足的地方；有反馈后，我可以再帮你做一轮“基于实战反馈”的微调。