---

## name: Database Schema & Connection
description: LoomiDBX 数据库连接与 Schema 扫描的 steering 记忆（元数据层、Diff、AutoMap、FFI）
type: reference

# 数据库连接与 Schema 扫描（Steering）

**权威详设**：`docs/schema.md`（DDL、完整接口签名、语义规则表以该文档为准。）

**updated_at**：2026-04-11 — 从 `docs/schema.md` 提炼为项目记忆，便于实现与评审时对齐架构。

---

## 1. 分层与职责

```
Flutter UI  ← FFI(JSON) →  libloomidbx (Go)
                    Connector │ Scanner │ Mapper
                              ↓
                    Storage Driver（应用元数据持久化）
                    ldb_schema_migrations
```

- **Connector**：各目标库的建连、`List`*、`DescribeTable`、`GetForeignKeys`；隔离驱动差异。
- **Scanner**：`RawColumn` → 抽象类型、自增识别、与快照表同步、**Diff**。
- **Mapper**：扫描结果 → `ldb_column_gen_configs` / `ldb_table_gen_configs` / `ldb_table_relations` 的自动映射与优先级。
- **StorageDriver**：LoomiDBX **自身**元数据 DB（非业务库）的 DDL 方言抽象与 Migration；与「用户要连的业务库」区分。

---

## 2. 元数据持久化（Storage Driver）

- **切换**：`LOOMIDBX_STORAGE` — 空/`sqlite`（默认）、`mysql`、`postgres`；DSN 用 `LOOMIDBX_STORAGE_DSN`，SQLite 路径用 `LOOMIDBX_STORAGE_PATH`（默认 `./loomidbx.db`）。
- **接口要点**：`DB()`、`DriverName()`、`DSN()`，以及 `AutoIncrementDDL` / `BooleanType` / `JSONType` / `UpsertSQL` 等 DDL 差异方法。
- **Migration**：顺序版本 `Migrations`，先 `ensureMetaTable`，再 `Up` + 记录版本；业务 DDL 中 JSON 列用 `d.JSONType()` 等适配多库。

---

## 3. 核心表职责（实现时勿混用）


| 领域   | 表                                                                               | 要点                                                                                          |
| ---- | ------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------- |
| 连接   | `ldb_connections`                                                               | `db_type`、`extra` JSON；密码 **AES-256 落盘**，禁止明文。                                              |
| 快照   | `ldb_table_schemas` / `ldb_column_schemas`                                      | 按连接+库+（schema）+表/列存扫描结果；`scan_version` **按表**递增。                                            |
| 生成配置 | `ldb_table_gen_configs` / `ldb_column_gen_configs`                              | `confirmed_at`：`NULL` = 待确认（UI 橙色），非空 = 已确认；自增列 `is_enabled=0`。                             |
| 表间数量 | `ldb_table_relations`                                                           | 与「列值从哪来」解耦；`relation_type`：`1:1` / `1:0-1` / `1:n` + `multiplier_`*。                        |
| 扫描审计 | `ldb_scan_history` / `ldb_scan_diffs`                                           | `scan_scope`：`full_db` vs `single_table` + `scope_target`；diff 未确认前 `confirmed_at IS NULL`。 |
| 运行历史 | `ldb_generation_runs` / `ldb_generation_run_tables` / `ldb_generation_run_logs` | 数据生成执行记录。与 `ldb_scan_history` 独立存储，避免跨域耦合。见 `steering/execution-engine.md`。                 |


抽象类型枚举：`int` / `string` / `decimal` / `datetime` / `boolean`（映射与 Fallback 以此为准）。

---

## 4. Connector 与扫描源

- `DBType`：`mysql`、`postgres`、`oracle`、`mssql`、`sqlite`、`clickhouse`、`hive`（与 `ldb_connections.db_type` 一致）。
- 各库实现从系统表/PRAGMA/`DESCRIBE` 等取列与外键（详设见 `docs/schema.md` 对照表）。
- **自增识别**：综合 `extra`、`rawType`、`default`（如 `auto_increment`、`identity`、`serial`、`nextval`、SQLite 组合规则），统一进入 `DetectAutoIncrement`。

---

## 5. 扫描范围与版本

- **全库扫描**：库下各表快照更新；相关表 `scan_version` 一并推进（语义以详设为准）。
- **单表扫描**：仅目标表 `scan_version` +1，不扰动同库其他表。
- **展示 diff**：以该表**最新一次**扫描记录为准（全库/单表共存时的冲突规则见详设 §八）。

---

## 6. Diff 与生成器联动（原则）

- `DiffType`：`added` / `removed` / `modified`（及内部 `unchanged`）。
- **新增列**：新建生成器，`confirmed_at = NULL`。
- **删除列**：生成器禁用等处理 + `ldb_scan_diffs`，待用户确认。
- **类型/约束变化**：可能重置待确认；**名称/注释-only**：静默更新，**不写** `ldb_scan_diffs`。

---

## 7. AutoMap 优先级（列级）

1. **自增** → `none`、`is_enabled=false`，`confirmed_at` 直接写入（无需用户确认）。
2. **物理外键** → `ForeignKeyGenerator`；`confirmed_at` 仍为待确认。
3. **整型主键** → `SequenceGenerator`。
4. **列名语义规则** → `mapper/semantic` 关键词表。
5. **抽象类型 Fallback** → `generator_opts` 常设为 `"{}"`，默认值由生成器 `DefaultOpts()` 提供（便于升级默认行为而不迁库）。

逻辑外键仅存 `ldb_column_gen_configs.logic_fk_`*，由用户确认写入。

---

## 8. FFI 约定

- 入参/出参均为 **JSON**；统一 `{"ok", "data", "error"}`。
- 关键导出：`TestConnection`、`SaveConnection`、`ListConnections`、`DeleteConnection`、`ListDatabases`、`ScanSchema`（`tableName` 空=全库）、`GetTableConfig`、`Save*GenConfig`、`SaveTableRelation`、`GetScanDiffs`、`ConfirmDiff` / `ConfirmAllDiffs`、`FreeString`。
- 实现新接口时保持与 `docs/schema.md` §六 清单一致，避免 Flutter 与 Go 两端漂移。

---

## 9. Flutter 侧要点

- 连接树 → `ListDatabases` → `ScanSchema`；非首次扫描有 diff 时 **DiffDialog**，确认走 `ConfirmDiff` / `ConfirmAllDiffs`。
- 表配置页：表级配置 + 列列表 + 生成器面板；徽标：**自增灰**、**待确认橙**、**外键蓝**、表头待确认计数。

---

## 10. 关键决策速查


| 主题          | 方案                                                   |
| ----------- | ---------------------------------------------------- |
| 元数据多后端      | 环境变量 + `StorageDriver` + Migration                   |
| Schema 版本   | `scan_version` 表粒度，全库/单表可并存                          |
| 用户确认状态      | 列级 `confirmed_at`，不用单独 `is_auto_mapped`              |
| 表间数量 vs 外键值 | `ldb_table_relations` vs `ldb_column_gen_configs` 分工 |
| FFI         | JSON 优先于手写 C 结构体                                     |


---

## 11. 安全

- 连接密码：**加密存储**；steering 与代码审查中不得写入密钥、示例密码或真实 DSN。

