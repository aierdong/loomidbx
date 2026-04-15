---

## name: Database Schema & Connection

description: LoomiDBX 数据库连接与 Schema 扫描的 steering 记忆（当前 schema、内存 Diff、AutoMap、FFI）
type: reference

# 数据库连接与 Schema 扫描（Steering）

**权威详设**：`docs/schema.md`（DDL、完整接口签名、语义规则表以该文档为准。）

**updated_at**：2026-04-14 — 对齐 spec-02：仅维护当前 schema，扫描快照仅内存态，Diff 必须经 UI 呈现。

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
- **Scanner**：`RawColumn` → 抽象类型、自增识别、内存快照构建、**Diff**。
- **Mapper**：扫描结果 → `ldb_column_gen_configs` / `ldb_table_gen_configs` / `ldb_table_relations` 的自动映射与优先级。
- **StorageDriver**：LoomiDBX **自身**元数据 DB（非业务库）的 DDL 方言抽象与 Migration；与「用户要连的业务库」区分。

---

## 2. 元数据持久化（Storage Driver）

- **切换**：`LOOMIDBX_STORAGE` — 空/`sqlite`（默认）、`mysql`、`postgres`；DSN 用 `LOOMIDBX_STORAGE_DSN`，SQLite 路径用 `LOOMIDBX_STORAGE_PATH`（默认 `./loomidbx.db`）。
- **接口要点**：`DB()`、`DriverName()`、`DSN()`，以及 `AutoIncrementDDL` / `BooleanType` / `JSONType` / `UpsertSQL` 等 DDL 差异方法。
- **Migration**：顺序版本 `Migrations`，先 `ensureMetaTable`，再 `Up` + 记录版本；业务 DDL 中 JSON 列用 `d.JSONType()` 等适配多库。

---

## 3. 核心表职责（实现时勿混用）


| 领域        | 表                                                                               | 要点                                                                                     |
| --------- | ------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------- |
| 连接        | `ldb_connections`                                                               | `db_type`、`extra` JSON；密码 **AES-256 落盘**，禁止明文。`extra` 中可维护 `schema_trust_state` 与阻断原因。 |
| 当前 Schema | `ldb_table_schemas` / `ldb_column_schemas`                                      | 当前生效 schema 的唯一持久化来源；扫描完成后基于兼容性结果决定是否覆盖更新。                                             |
| 生成配置      | `ldb_table_gen_configs` / `ldb_column_gen_configs`                              | 由 Diff 兼容性分析读取；不兼容时要求用户调整后再允许 schema 同步。                                               |
| 表间数量      | `ldb_table_relations`                                                           | 与「列值从哪来」解耦；`relation_type`：`1:1` / `1:0-1` / `1:n` + `multiplier_`*。                   |
| 运行历史      | `ldb_generation_runs` / `ldb_generation_run_tables` / `ldb_generation_run_logs` | 数据生成执行记录，与扫描任务域解耦。见 `steering/execution-engine.md`。                                    |


抽象类型枚举：`int` / `string` / `decimal` / `datetime` / `boolean`（映射与 Fallback 以此为准）。

---

## 4. Connector 与扫描源

- `DBType`：`mysql`、`postgres`、`oracle`、`mssql`、`sqlite`、`clickhouse`、`hive`（与 `ldb_connections.db_type` 一致）。
- 各库实现从系统表/PRAGMA/`DESCRIBE` 等取列与外键（详设见 `docs/schema.md` 对照表）。
- **自增识别**：综合 `extra`、`rawType`、`default`（如 `auto_increment`、`identity`、`serial`、`nextval`、SQLite 组合规则），统一进入 `DetectAutoIncrement`。

---

## 5. 扫描范围、Diff 与同步

- **全库扫描**：在内存中构建全库 schema，与当前持久化 schema 对比。
- **单表扫描**：只比较目标表结构，生成该范围的 Diff。
- **Diff 呈现约束**：每次扫描产生的 Diff 都必须经 UI 呈现给用户（无论是否阻断）。
- **同步约束**：无阻断风险可直接覆盖更新当前 schema；有阻断风险必须先调整生成器配置后再同步。

---

## 6. Diff 与生成器联动（原则）

- `DiffType`：`added` / `removed` / `modified`（及内部 `unchanged`）。
- **新增列**：允许自动建议生成器，进入待确认状态。
- **删除列**：关联生成器进入风险清单；未处理前可阻断同步。
- **类型/约束变化**：触发兼容性分析；阻断级风险要求先调整再同步。
- **无生成器配置场景**：返回 `no_generator_config`（空风险），不作为错误。

### 可信度状态机转换规则（必须遵守）


| 当前状态                 | 触发条件                       | 下一状态                 | 说明                            |
| -------------------- | -------------------------- | -------------------- | ----------------------------- |
| `trusted`            | 连接配置变更（驱动、DSN、凭据、目标库切换）    | `pending_rescan`     | 当前 schema 的可信度下降，必须重扫后才能恢复可信。 |
| `trusted`            | 新扫描 Diff 存在阻断级风险           | `pending_adjustment` | 先调整生成器配置，再允许同步与执行。            |
| `pending_rescan`     | 重扫完成且无阻断级风险，并成功同步当前 schema | `trusted`            | 重建“当前 schema 单一真相”。           |
| `pending_rescan`     | 重扫完成但仍有阻断级风险               | `pending_adjustment` | 需要用户先处理风险，不可直接恢复可信。           |
| `pending_adjustment` | 风险已确认处理 + 同步成功             | `trusted`            | 解除阻断，恢复执行准入。                  |
| `pending_adjustment` | 用户再次修改连接配置                 | `pending_rescan`     | 连接变化优先触发重扫要求。                 |


补充约束：

- 未处于 `trusted` 时，必须向下游返回稳定的前置条件错误，阻断执行链路。
- 不再维护扫描任务持久化表；扫描过程中的任务状态仅作为运行时上下文，不落库为独立历史实体。

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
- 关键导出：`TestConnection`、`SaveConnection`、`ListConnections`、`DeleteConnection`、`ListDatabases`、`ScanSchema`（`tableName` 空=全库）、`GetTableConfig`、`Save*GenConfig`、`SaveTableRelation`、`PreviewSchemaDiff`、`GetGeneratorCompatibilityRisks`、`ApplySchemaSync`、`GetSchemaTrustState`、`FreeString`。
- 实现新接口时保持与 `docs/schema.md` §六 清单一致，避免 Flutter 与 Go 两端漂移。

---

## 9. Flutter 侧要点

- 连接树 → `ListDatabases` → `ScanSchema`；每次扫描后都展示 **DiffDialog**（含风险清单与同步动作）。
- 无阻断风险时允许直接“同步当前 schema”；有阻断风险时引导用户修复配置后再同步。
- 表配置页：表级配置 + 列列表 + 生成器面板；徽标：**自增灰**、**待确认橙**、**外键蓝**、表头待确认计数。

---

## 10. 关键决策速查


| 主题         | 方案                                                               |
| ---------- | ---------------------------------------------------------------- |
| 元数据多后端     | 环境变量 + `StorageDriver` + Migration                               |
| Schema 持久化 | 仅维护当前 schema（`ldb_table_schemas`/`ldb_column_schemas`），不保留扫描快照历史 |
| 同步策略       | 无阻断风险可直接覆盖；阻断风险必须先调整生成器                                          |
| 可信度状态      | `trusted` / `pending_rescan` / `pending_adjustment`              |
| FFI        | JSON 优先于手写 C 结构体                                                 |


---

## 11. 安全

- 连接密码：**加密存储**；steering 与代码审查中不得写入密钥、示例密码或真实 DSN。

