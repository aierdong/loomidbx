## 目标

让 Agent 在 `/spec-impl` 时：

- 不思考“用哪些 Skills”
- 只执行：**匹配 → 选定 → 声明 Anchors**

------

## 1. Steering 中的 Skills Router（核心）

在 `.kiro/steering/skills-router.md` 定义一份“路由表”：

```
text
# Skills Router

## By Feature Tag

auth:
  - auth/login-flow
  - auth/session-management

billing:
  - billing/reconciliation
  - billing/invoice-generation


## By Layer

http-api:
  - api/http-handler
  - api/error-mapping

domain:
  - domain/aggregate-pattern
  - domain/validation


## By Keyword (fallback)

login:
  - auth/login-flow

payment:
  - billing/payment-processing
```

------

## 2. 选择算法（固定规则）

Agent 必须按顺序执行：

1. **Feature Tag 匹配（最高优先）**
   - 来自：spec metadata（如 `feature: auth/login`）
   - 命中 → 直接选
2. **Task / Design 中的 Layer 匹配**
   - 如：HTTP / domain / infra
   - 补充对应 Skills
3. **Keyword fallback**
   - 从需求 / task 文本提取关键词
   - 仅用于补充，不作为主来源
4. **去重 + 排序**
   - 保持稳定顺序（feature → layer → keyword）

------

## 3. 强约束（避免失控）

- 最多选择 **3–5 个 Skills**
- 不允许：
  - 全量加载某个目录
  - 使用未在 router 注册的 Skills
- 若匹配不到：
  - 必须显式写：`Skills Anchors: UNKNOWN`
  - 并请求补充（而不是自己猜）

------

## 4. Spec 侧的最小配合（关键优化）

在 `spec.json` 或 spec 文件中加：

```
json
{
  "feature": "auth/login",
  "layers": ["http-api", "domain"]
}
```

这样 Agent 可以**零推理直接命中**：

- feature → auth/*
- layer → http-api + domain

------

## 5. 决策示例

输入：

- feature: `auth/login`
- layers: `http-api`
- task: "implement login endpoint"

输出：

```
text
Skills Anchors
- auth/login-flow          (feature)
- api/http-handler         (layer)
- api/error-mapping        (layer)
```

不需要任何解释或推理过程 ✅

------

## 6. 进阶（可选但很有用）

## 6.1 Skill 依赖声明（自动补全）

在 skill 文件头部：

```
text
depends:
  - domain/validation
```

→ Agent 自动补齐，不用 router 写全

------

## 6.2 禁止组合（防冲突）

```
text
conflicts:
  - auth/session-cookie
```

→ 防止选到互斥实现

------

## 6.3 默认 Skill（兜底）

```
text
default:
  - core/error-handling
```

→ 永远附带，减少遗漏

------

## 7. 一句话版本（给 CLAUDE.md 用）

你可以把规则压成这样一行塞回 CLAUDE.md：

```
text
Skills = feature match → layer补充 → keyword补充（≤5个，未命中则标记UNKNOWN，不得猜测）
```

------

如果你想再极端一点，还可以把 router 改成**完全声明式（无算法）**：

- 每个 feature 直接绑定完整 Skills 集合
  → Agent 连“组合”都不用做

我也可以帮你设计那个“零决策版 router”（适合更大团队或更强约束环境）。