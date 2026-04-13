---
name: External Feeds Memory
description: 外部数据源能力的长期记忆要点（详设请查 docs/external-data-and-ai-research.md）
type: reference
---

# 外部数据源（长期记忆）

## 权威文档

- **唯一权威详设**：`docs/external-data-and-ai-research.md`
- 相关实现落点：`docs/generator.md`、`docs/execution-engine.md`
- 本文件仅保留稳定决策与关键约束；若冲突，**以权威详设为准**。

## 能力边界（必须遵守）

- 统一抽象为 `ExternalFeed`，用于提供“单字段候选值”。
- 全部数据源最终都要归一为“单列值数组”。
- MVP 启用：文件（内嵌/上传）+ HTTP + SQL；LLM 仅预留，不进入 MVP 默认流程。
- 任务开始时一次全量拉取，任务内不刷新；默认行数上限 `10000`（可环境变量覆盖）。
- 超限行为为“截断并告警”；不做分页拼接与多请求合并。

## 认证与凭据策略

- HTTP 支持：`none`、`api_key`、`bearer/oauth2`、`basic`、`hmac`、`digest`。
- SQL 支持：用户名密码、DSN 内嵌凭据、环境变量注入。
- OAuth 不处理 refresh token 生命周期，仅支持动态取 token。
- 允许 query 携带密钥但必须风险提示；日志与历史中必须脱敏。
- 密钥管理策略：系统密钥环 + 环境变量注入。
- 直接输入密钥时需写入系统密钥环；配置库仅保存引用信息，不保存可直接复用的明文密钥。

## 提取与失败语义（硬约束）

- 提取失败条件包含：路径/列未命中、访问失败、HTTP 非 200、结果为空、结果含 `null`。
- 失败策略固定为 `hard_fail`：立即终止、返回错误、回滚事务。
- 不允许列级降级、占位值回填或回退词表。
- CSV 同时配置列名与列序号时，优先列名。

## 安全与运行可观测

- URL 仅记录不含 query 的部分；SQL 端点与凭据信息必须脱敏。
- 记录 HTTP 状态码，必要时可记录公开数据集校验哈希。
- 运行历史聚合字段使用 `external_calls`（总数/成功/失败/按类型/耗时/脱敏 endpoint）。

## 跨模块契约

- 与 Generator：`ExternalFeed` 必须显式包含 `kind/auth/request/extract/row_limit/failure_policy`。
- 与 Execution Engine：执行进度与历史需暴露 `external_calls` 聚合信息并遵守脱敏规则。
- 与 UI：执行任务前必须进行出网确认；确认框允许“对此连接不再提示”。

## 维护规则

- 新增或删除 `ExternalFeed.kind`、认证边界变化、失败语义变化、脱敏口径变化时，必须更新本文件。
- 纯实现重构且不影响上述决策时，不更新本文件。