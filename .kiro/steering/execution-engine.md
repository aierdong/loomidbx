---
name: Execution Engine Memory
description: 数据生成执行引擎的长期记忆要点（详设请查 docs/execution-engine.md）
type: reference
---

# 数据生成执行引擎（长期记忆）

## 权威文档

- **唯一权威详设**：`docs/execution-engine.md`
- 本文件仅保留稳定决策与高频约束；若冲突，**以权威详设为准**。

## 核心流程与边界

- 固定主链路：`Planner -> Engine -> Recorder`（计划、执行、历史分层）。
- `ExecutionPlan.Tables[].TargetRows` 是唯一行数计划出口。
- 执行引擎负责“生成+写入+进度+取消+历史”，不承载扫描域（`ldb_scan_history`）耦合。
- 运行历史独立存储：`generation_runs`、`generation_run_tables`、`generation_run_logs`。

## 依赖与行数规则（必须遵守）

- 依赖图综合三源：物理 FK、逻辑 FK、表间关系；拓扑排序采用 Kahn。
- 普通环依赖禁止执行并提示拆批；自引用按树模型单独处理（层级 BFS）。
- 1:1 子表行数继承主表；1:0-1 采用 Bernoulli(p) 求和；1:n 使用区间倍数。
- 多对多中间表基于主表规模与倍数计算，并从主表实际 ID 池采样。
- 子表依赖父表 ID 池，父表完成后子表开始，避免跨批协调。

## 写入与事务策略

- 默认批量写入：`batchSize=10000`，`txPerBatch=true`（每批独立事务）。
- 每批事务模式下，失败批回滚、已提交批保留，并停止后续批次。
- 全任务单事务为可选策略，失败时全量回滚。
- Truncate 顺序固定为逆拓扑；标识符统一 `quoteIdentifier`。
- FK 开关与序列重置遵循数据库方言差异，具体 SQL 以权威详设为准。
- 性能口径基线：不包含外部数据源调用、不包含计算字段、包含唯一性约束检查、每批事务。

## 可复现与审计

- MVP 支持全局 seed，并在运行历史中可追溯 seed 以便复盘。

## 可观测性与运行控制

- 进度通过流式回调上报（`SetProgressCallback`），非一次性阻塞返回。
- 批次流水线允许 generating/writing 交替或并发，进度可多状态并存。
- 进度百分比以 `rows_written` 为保守口径，避免 UI 进度回退。
- 取消机制统一 `context + CancelGeneration(runID)`；超时由向导高级参数控制。

## FFI 与契约要点

- 核心接口集：`BuildExecutionPlan`、`StartGeneration`、`CancelGeneration`、`GetGenerationStatus`、`ListGenerationRuns`、`GetGenerationRunDetail`、`SetProgressCallback`。
- 日志等级语义固定：`INFO` 可折叠、`WARN` 高亮、`ERROR` 必须进入错误摘要。

## 维护规则

- 当依赖建图规则、行数规则、事务失败语义、进度口径、历史模型或 FFI 合同变化时，必须更新本文件。
- 仅实现重构且不改变以上决策时，不更新本文件。