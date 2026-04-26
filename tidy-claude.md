# AI‑DLC / Spec‑Driven Development

## 0. 核心原则

- 本文件是**约束与流程地图**，不是手册。
- 细节一律外置：
  - 结构 → `steering/structure.md`
  - 领域 → `skills/*`
  - 输出格式 → `templates/*`
- 目标：**简单、可追溯、结构正确**

## 1. 目录与角色

- Steering → `.kiro/steering/`（项目规则 + Skills 索引）
- Specs → `.kiro/specs/`（特性定义与任务）
- Skills → `.claude/skills/`（唯一权威设计）
- Templates → `.claude/templates/`（输出骨架）

职责边界：

- Steering：定义规则 + 维护 Skills 映射
- Specs：描述“要做什么”
- Skills：定义“应该怎么做”（权威）
- Templates：定义“怎么写出来”

## 2. 权威顺序（必须遵守）

```
Skills
  > Active Spec
  > Steering
  > Existing Code
```

- 冲突时按此顺序决策
- 必须写入 Deviation Record

## 3. 语言规则

- 思考：英文
- 默认输出：简体中文

表达：

- 短句 + 主动语态
- 少术语（必要时括号解释）
- 新人可读

------

## 4. 标准流程（不可跳）

## Phase 0（可选）

- `/kiro:steering`

## Phase 1（Spec）

- requirements → design → tasks

## Phase 2（Impl）

- `/kiro:spec-impl`
- `/kiro:validate-impl`（可选）

通用约束：

- 固定顺序：需求 → 设计 → 任务 → 实现
- 默认有人审；`-y` = 主动跳过
- 信息不清 → 提问（禁止隐式假设）

------

## 5. Skills 使用规则

- 每次实现必须**显式引用 Skills（Anchors）**
- 所有行为、结构、对外契约 → 以 Skills 为准

------

## 6. 行为约束

## 6.1 先思考

在 design / tasks / impl：

- 写 1–3 条关键假设（需求 / 领域 / Skills）
- 多解时至少列 2 个方案 + 选择理由
- 不清楚 → 明确指出，不得脑补

## 6.2 简单优先

- 只做当前任务的最小实现
- 禁止提前抽象 / 过度设计
- 单点调用 → 不引入复杂结构

## 6.3 精准修改

- 仅修改 Impact Surface 内文件
- 禁止顺手重构 / 美化
- 风格必须一致

例外：

- 若越界修改 → 更新 Impact Surface + 写 Deviation

## 6.4 目标可验证

- 模糊需求 → 转成可测试目标
- 实现前写步骤 + 验证方式
- 每次 impl 定义 2–5 条成功标准

------

## 7. 结构契约（强制）

来源：`steering/structure.md`

- 新文件必须符合：
  - 目录
  - 分层
  - 命名

不符合：

- 记录 Deviation
- 禁止隐式改变结构

------

## 8. 模板规则（强制）

模板位置：`.claude/templates/`

必须使用模板生成：

- Dry Run
- Deviation Record
- Design Change / Conformance

说明：

- 本文件只定义“必须有”
- 字段结构以模板为准

------

## 9. /spec-impl 最小骨架（必须完整）

每次实现必须包含：

1. Skills Anchors
2. Pre‑Coding Dry Run（template）
3. Deviation Record（template，允许 none）
4. Conformance / Design Change（template）

缺一：

- 本次实现无效

------

## 10. 粒度控制

- 单次 impl 必须可：
  - 理解
  - 审查
  - 回滚
- Impact Surface 过大 → 回到 tasks 拆分

------

## 11. 最小示例

```
text
Skills Anchors
- xxx

Dry Run
(template)

Deviation
(template)

Conformance
(template)
```

------

如果你愿意，我可以再帮你做一版「更激进的 60 行版本」（极致地图型，几乎只剩约束），或者反过来帮你把 templates 设计成一套“强约束填空式”，让这个 map 更轻。