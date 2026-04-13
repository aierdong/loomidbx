---

## name: External Feeds
description: 外部数据源（文件/HTTP/SQL/LLM）长期决策记忆：边界、认证、提取、失败语义与跨模块契约
type: reference

# 外部数据源（ExternalFeed）Steering

**权威详设**：`docs/external-data-and-ai-research.md`  
**相关实现文档**：`docs/generator.md`、`docs/execution-engine.md`

**updated_at**：2026-04-13（由研究草案收敛为已决策基线）

---

## 1. 目标与定位

ExternalFeed 用于拉取外部“维度数据”，为单字段填充值提供候选集。

稳定特征：

- 数据规模通常在数十到数万条。
- 数据是关键输入，不接受降级补位。
- 无外部变更订阅，任务内视图一次性获取。
- 流程内只保留目标字段单列值，其他列丢弃。

---

## 2. 数据源分类（统一模型）

统一抽象：`ExternalFeed`，按 `kind` 区分：

1. `embedded_json` / `embedded_csv`：内嵌静态文件（不出网）
2. `uploaded_json` / `uploaded_csv`：用户上传文件（不出网）
3. `http`：HTTP(S) API 或公开资源（出网）
4. `sql`：外部数据库查询（网络取决于部署环境）
5. `llm`：OpenAI 协议兼容模型（MVP 外，预留）

MVP 启用范围：文件 + HTTP + SQL。  
LLM 保留接口，不纳入 MVP 默认流程。

---

## 3. 认证与凭据策略

### HTTP 认证

支持：

- `none`
- `api_key`（Header / Query）
- `bearer` / `oauth2`（支持动态 token 获取）
- `basic`
- `hmac`
- `digest`

约束：

- 不做 OAuth refresh token 生命周期管理。
- 允许 URL query 携带密钥，但需风险告警。

### SQL 认证

支持：

- 用户名/密码
- DSN 内嵌凭据
- 环境变量注入凭据

约束：

- SQL 外部源以“连接凭证 + SQL 语句”为最小单元。
- 不纳入 NTLM/Kerberos 专项适配。

### 凭据存储

- 允许直接输入并落库（强调非生产便捷性）。
- 允许环境变量注入；若使用环境变量，配置可落库变量名而非明文。

---

## 4. 返回格式与单列提取契约

所有数据源必须归一为“单列值数组”。


| 数据源      | 输入格式要求   | 单列提取                           |
| -------- | -------- | ------------------------------ |
| HTTP API | JSON 数组  | `field_path`                   |
| SQL      | 单列或多列结果集 | `target_column`                |
| JSON 文件  | JSON 数组  | `field_path`                   |
| CSV 文件   | UTF-8    | `column_index` 或 `column_name` |


规则：

- `field_path` 未命中、`target_column` 不存在、或提取值为 `null` -> 失败。
- 结果为空数组 -> 失败。
- CSV 同时配置列序号与列名时，优先列名。

---

## 5. 执行与失败语义（硬约束）

### 拉取策略

- 任务开始前一次性全量拉取（任务内不刷新）。
- 默认上限 `10000` 行，可由环境变量覆盖。
- 超限行为：截断并告警（不分页、不合并请求）。

### 失败定义

以下任一条件成立即失败：

- 超时、连接失败、路径失效等基础访问失败
- HTTP 状态码非 `200`
- 提取后结果为空数组
- 结果包含 `null`

### 失败处理

- 立即终止当前处理流程
- 返回错误提示
- 回滚事务
- 不允许列级降级、占位、回退词表

---

## 6. 产品边界与网络策略

- 不要求离线优先能力。
- 默认允许出网；执行生成时需用户确认“将访问外网”。
- 不提供代理/IPv6/内网/TLS 信任专项能力；依赖用户运行环境。
- AI 生成器不纳入当前性能承诺。

---

## 7. 日志与安全基线

- 日志记录 HTTP 状态码。
- URL 仅记录不含 query 的部分。
- SQL 端点记录需脱敏，不记录用户名/密码。
- 可选记录公开数据集校验哈希（不强制 HTTPS 信任链能力扩展）。

运行历史建议聚合字段：

- `external_calls`（总数、成功数、失败数、按类型统计、脱敏 endpoint、耗时）

---

## 8. 跨模块契约

### 与 Generator 的契约

- `ExternalFeed` schema 必须显式包含：`kind`、`auth`、`request`、`extract`、`row_limit`、`failure_policy`。
- `failure_policy` 当前固定为 `hard_fail`。
- 生成器只消费“已归一化单列值数组”，不关心原始源格式。

### 与 Execution Engine 的契约

- 执行期需在进度/历史中暴露 `external_calls` 聚合信息。
- `generation_runs` 与 `generation_run_tables` 可记录 `external_calls` JSON。
- 历史日志必须遵守 URL 与凭据脱敏规则。

---

## 9. 变更守则（维护本 Steering）

以下变化需要更新本文件：

- 新增/删除 `ExternalFeed.kind`
- 认证方式边界变化（新增 OAuth 刷新、代理能力等）
- 失败语义从硬失败改为可降级
- 日志脱敏口径变化

若仅是代码实现重构，但上述决策不变，不需要更新本文件。