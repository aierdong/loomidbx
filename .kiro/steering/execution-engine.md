---
name: Execution Engine
description: LoomiDBX 数据生成执行引擎的 steering 记忆（依赖图、行数计划、写入策略、可观测性、运行历史）
type: reference
---

# 数据生成执行引擎（Steering）

**权威详设**：`docs/execution-engine.md`（完整算法、代码示例、边界情况以该文档为准。）

**updated_at**：2026-04-12（v5）— `CalculateTargetRows` 增加 `wizardTablePlans` 与详设一致；ID 池 `getOrCreateIDPool`、FK 取样单次 RLock；§9.3 Truncate SQL 引用 §4.3 单一实现。

---

## 1. 整体架构

```
Planner → Engine → Recorder
(依赖图+行数) (生成+写入) (运行历史)
```

**核心流程**：用户向导 → 选表 → 设行数 → Planner（依赖图+拓扑） → ExecutionPlan → Engine（批量写入+进度） → Recorder（写历史）

---

## 2. 依赖图与拓扑排序

**依赖来源三方综合**：
- 物理外键：`column_schemas.fk_ref_table`
- 逻辑外键：`column_gen_configs.logic_fk_table`
- 表间数量：`table_relations.from_table_id → to_table_id`

**拓扑排序**：Kahn 算法，O(V+E)，入度 = 尚未满足的前置依赖数（入度为 0 表示所有前置依赖已就绪，可立即生成）。

**环处理策略**：
- 简单环/多表环：**禁止执行**，向导返回错误，提示用户拆批或修改外键配置
- 自引用（A.parent_id → A.id）：**单独处理**，使用 SelfRefGenerator（层级 BFS：每层仅用上一层节点作为父池）

**隐式依赖表**：
- 用户勾选 A，A 依赖 B 且 B 未勾选 → 自动补入 B（标记「隐式依赖」）或阻止生成
- B 无生成配置 → 阻止，提示先配置 B

---

## 3. 行数协调

**统一模型**：`ExecutionPlan.Tables[].TargetRows`

**Planner 入参**：`CalculateTargetRows(..., wizardTablePlans map[string]TablePlan)` — 向导对各表的覆盖（如 1:0-1 的 `OptionalProb`），仅需填充用到的字段；详设见 `docs/execution-engine.md` §3.2。

| 关系类型 | 行数规则 |
|---|---|
| 独立表 | `user_input` 或 `gen_count`（默认） |
| 1:1 | `子表 = 主表`（锁定不可编辑） |
| 1:0-1 | 每主记录 Bernoulli(p) 后求和（期望 ≈ p×主表行数） |
| 1:n | `子表 = 主表 × rand(min, max)` |

**中间表（多对多）**：
- 左主表行数 × 倍数区间 → 中间表行数
- 左右主表**实际 ID 池**随机采样（支持任意主键类型：整数/UUID/雪花 ID）

**自引用（树）**：
- 层级 BFS：每层仅用上一层节点作为父池，避免多层混用
- 根节点比例 + 最大深度 + 子节点区间 → 递归生成

---

## 4. 写入策略

**批量模型**：
- `batchSize`：用户可设，默认 10000
- `txPerBatch`：默认 `true`（每批独立事务）

**事务策略**：
| 策略 | 失败处理 |
|---|---|
| 每批独立（默认） | 失败批次回滚，已提交批次保留，停止后续 |
| 全任务单事务 | 失败回滚全部 |

**Truncate 顺序**：逆拓扑（从表先清空 → 主表后清空），避免 FK 阻止。

**FK 禁用**：
- MySQL/SQLite：进入 Truncate 阶段前一次性禁用，完成后恢复（避免每表重复开关）
- Postgres：默认不加 CASCADE，按逆拓扑逐表执行；可选「级联 Truncate」仅对拓扑序最前的表执行一次

**标识符引用**：所有 DML/DDL 使用 `quoteIdentifier`，防保留字冲突，支持 schema 前缀。

**序列重置**：Postgres/Oracle 需手动 `ALTER SEQUENCE` / 重建序列；MySQL/MSSQL 自动重置。

---

## 5. 运行期可观测性

**进度回调**：流式回调（FFI `SetProgressCallback`），而非一次性阻塞。

**批次流水线机制**（`txPerBatch=true`）：
- generation 和 writing **交替或并发**进行，而非严格串行
- 单线程模式：生成批次 N → 写入批次 N → 生成批次 N+1 → ...
- 多线程模式：写入批次 N 时，同时生成批次 N+1（goroutine 并发）

**Progress 结构（多状态并存）**：
- `phase_flags`: 多状态标记（`["generating", "writing"]`）
- `pipeline.generating_batch` / `writing_batch`: 当前执行中的批次号
- `rows_generated` vs `rows_written`: 区分生成量与写入量
- 进度百分比：保守策略（基于 `rows_written`）避免回退

**UI 展示**：双徽标或合并为「正在生成+写入」，批次列表区分生成中○、写入中●、已完成✓

**取消机制**：`context.Context` + `CancelFunc`，FFI 导出 `CancelGeneration(runID)`。

**超时**：向导高级选项可设，默认无限。

**日志**：`INFO` → UI 可折叠，`WARN` → UI 高亮，`ERROR` → UI + `error_message` 字段。

---

## 6. 运行历史

**与 scan_history 边界**：独立存储，不建外键，避免跨域耦合。

**新增表**：
- `generation_runs`：run 级（配置、状态、总行数、耗时、错误摘要）
- `generation_run_tables`：表级明细（目标/实际行数、拓扑序、耗时、状态）
- `generation_run_logs`：日志级（可选，审计用）

**状态枚举**：`running` / `completed` / `partial` / `failed` / `cancelled`

---

## 7. FFI 接口新增

| 函数 | 用途 |
|---|---|
| `BuildExecutionPlan` | 构建 ExecutionPlan（含拓扑排序、环检测、行数计算） |
| `StartGeneration` | 启动异步生成，返回 run_id |
| `CancelGeneration` | 取消运行 |
| `GetGenerationStatus` | 获取当前状态 |
| `ListGenerationRuns` | 查询历史列表 |
| `GetGenerationRunDetail` | 获取 run + tables + logs |
| `SetProgressCallback` | 注册进度回调 |

---

## 8. 关键决策速查

| 主题 | 方案 |
|---|---|
| 环处理 | 禁止执行 + 提示拆批 |
| 行数继承 | 拓扑序遍历，主表先行数 |
| 1:0-1 | Bernoulli(p) 求和，期望 ≈ p×主表行数 |
| 1:0-1 概率 | TablePlan.OptionalProb（默认 0.5），可由 table_relations 或向导覆盖 |
| 中间表 | 左主 × 倍数，左右主表实际 ID 池采样 |
| 自引用 | 层级 BFS，每层仅用上一层节点 |
| 事务 | 每批独立（默认） |
| 批次流水线 | generation 与 writing 交替/并发，Progress 多状态并存 |
| 失败处理 | 已提交批次保留（默认） |
| Truncate FK | 进入阶段前一次性禁用/恢复 |
| Truncate CASCADE | 默认不加，按逆拓扑；可选级联仅对首表执行一次 |
| 进度 | 流式回调，phase_flags 多状态 |
| 取消 | Go context + CancelFunc |
| 历史 | generation_runs 独立表 |
| ID 池契约 | 父表完成后子表开始 |
| ID 池并发安全 | 多线程模式下需 sync.RWMutex |
| 唯一约束 | 内存池 + 重试，耗尽报错 |
| 标识符 | quoteIdentifier 防保留字 |
| 序列重置 | Postgres/Oracle 手动 |

---

## 9. 关键实现注意事项

**ID 池契约**：父表全部完成后子表才开始生成，避免跨批次协调。

**ID 池初始化**：`AppendToIDPool` / `MarkIDPoolReady` 经 `getOrCreateIDPool` 避免 `globalIDPools` 未注册时 nil panic。

**并发安全**：多线程生成/写入模式下，IDPool 与 UniquePool 需使用 `sync.RWMutex` / `sync.Mutex` 保护；外键从池取样宜在单次读锁内完成长度检查、随机下标与取值。

**唯一约束**：内存池 + 重试机制（默认 100 次），耗尽报错并停止该表。

**标识符引用**：所有 DML/DDL 使用 `quoteIdentifier`（MySQL: ``, Postgres: "", MSSQL: []）。

**序列重置**：Postgres/Oracle 需手动重置，MySQL/MSSQL 自动。

---

## 10. 后续扩展

- 并行生成（无依赖表）
- 断点续传
- 试运行（生成不写入）
- 导出 SQL 脚本
- 约束验证

---
_Document decisions and patterns, not just implementation._