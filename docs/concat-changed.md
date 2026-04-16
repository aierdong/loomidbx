## 契约变化清单（面向 Flutter）

### `ApplySchemaSync` 成功响应新增字段

- 旧：直接返回 `ApplySchemaSyncResult`（主要是 `SyncApplied`、`TrustState`）
- 新：FFI 成功响应 data 采用稳定 snake_case 包装，包含：
  - `sync_applied: bool`
  - `trust_state: string`
  - `compatibility_recheck: object`

`compatibility_recheck` 结构：

- `status: "success" | "failed" | "skipped_no_generator_config"`
- `generated_at_unix: number`
- `error_code?: string`（仅 `status=failed` 时有值，当前稳定值：`COMPATIBILITY_RECHECK_FAILED`）
- summary:
  - `mode: "configured" | "no_generator_config"`
  - `total_risks: number`
  - `blocking_risks: number`
- `risks: GeneratorCompatibilityRisk[]`（保证返回数组，空则 `[]`）

------

### `GetSchemaTrustState` 成功响应新增字段

- 旧字段保持不变：
  - `trust_state`
  - `reason`
  - `last_schema_scan_unix`
  - `last_schema_sync_unix`
- 新增：
  - `compatibility_report: object | null`

`compatibility_report` 与 `compatibility_recheck` 同结构（latest-only 快照）。

------

## 语义变化（重要）

- `ApplySchemaSync` 的“成功”语义现在以 schema 落库 + trust state 更新 为准。
- 若重判定失败：
  - 接口仍返回 `ok=true`（同步成功）
  - 通过 `compatibility_recheck.status=failed` + `error_code` 反映失败
- 这意味着 Flutter 侧不应再把“重判定失败”等同于“同步失败”。

------

## 向后兼容说明

- 旧调用方如果只看 `sync_applied/trust_state`，可无缝工作。
- 新字段均为增量扩展，不会破坏已有 JSON 解析（前提是前端允许忽略未知字段）。
- `compatibility_report` 在历史连接上可能为 `null`（未产生过快照），需要前端兜底展示“暂无重判定结果”。

------

## Flutter 侧最小改造建议

- ApplySchemaSync 后：
  - 继续按 `sync_applied` 走原流程
  - 额外读取 `compatibility_recheck`，展示重判定状态 badge
- TrustState 页面：
  - 优先读取 `compatibility_report.summary.blocking_risks`
  - 若 `compatibility_report == null` 显示“尚未生成报告”
- 错误处理：
  - 不把 `compatibility_recheck.status=failed` 当接口失败 toast
  - 改为 warning + “稍后重试/刷新状态”提示

可以，给你一份可直接给前端同学使用的“字段级清单”。

## 1) `ApplySchemaSync` 响应变化

### 请求（不变）

`task_id`, `ack_risk_ids`

### 成功响应 `data`（变更后）

- `sync_applied: bool`
- `trust_state: string`
- `compatibility_recheck: CompatibilityReportSnapshot`

### `compatibility_recheck` 结构

- `status: "success" | "failed" | "skipped_no_generator_config"`
- `generated_at_unix: number`
- `error_code?: string`（仅失败时，当前为 `COMPATIBILITY_RECHECK_FAILED`）
-  `summary`
  - `mode: "configured" | "no_generator_config"`
  - `total_risks: number`
  - `blocking_risks: number`
- `risks: GeneratorCompatibilityRisk[]`（始终数组，空为 `[]`）

------

## 2) `GetSchemaTrustState` 响应变化

### 既有字段（保留）

- `trust_state`
- `reason`
- `last_schema_scan_unix`
- `last_schema_sync_unix`

### 新增字段

- `compatibility_report: CompatibilityReportSnapshot | null`

> 该字段是 latest-only 快照，来自 `ldb_connections.extra.compatibility_report`。

------

## 3) 关键行为变化

- `ApplySchemaSync` 成功判定改为：schema 落库成功 + trust state 更新成功。
- 即使重判定失败，也会：
  - 返回 `ok=true`
  - 在 `compatibility_recheck.status=failed` 暴露失败状态
- 前端不要再把“重判定失败”当成“同步失败”。

------

## 4) 兼容性说明

- 这是向后兼容的增量字段扩展。
- 老前端只读取 `sync_applied/trust_state` 仍可工作。
- 老数据可能没有 `compatibility_report`，前端需允许 `null`。

------

## 5) 前端最小改造建议

- 同步成功弹窗/状态区增加一行：
  - `compatibility_recheck.status`
  - 若 `failed`，显示 warning（非 error）
- 信任状态页增加“兼容性报告摘要”：
  - `blocking_risks`, `total_risks`, `generated_at_unix`
- 空态：
  - `compatibility_report == null` -> “暂无重判定报告”

如果你要，我可以再补一版 Dart 数据类定义（json_serializable），直接贴到 Flutter 工程里用。