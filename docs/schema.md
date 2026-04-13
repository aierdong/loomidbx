# LoomiDBX — 数据库连接与 Schema 扫描方案

---

## 一、整体架构

```
Flutter UI  <──FFI JSON──>  Go 动态库 (libloomidbx)
                                  │
                    ┌─────────────┼─────────────────┐
                 Connector     Scanner           Mapper
                 (连接管理)    (Schema扫描/Diff)  (生成器映射)
                                  │
                          Storage Driver（可配置）
                          ├── SQLite（默认）
                          ├── MySQL
                          └── Postgres / ...
                                  │
                          ldb_schema_migrations（版本管理）
```

核心分三层：**连接层**（抽象不同数据库的连接差异）、**扫描层**（统一提取 Schema 元数据与 Diff）、**映射层**（根据字段信息自动推断生成器）。持久化层通过 Migration 机制动态适配不同存储数据库。

---

## 二、持久化层：动态适配多种数据库

### 2.1 Storage Driver 抽象

持久化存储后端通过环境变量决定，默认使用内嵌 SQLite。所有 DDL 通过 `StorageDriver` 接口抽象，屏蔽各数据库语法差异。

```go
// storage/driver.go

type StorageDriver interface {
    DB() *sql.DB
    DriverName() string           // sqlite / mysql / postgres / ...
    DSN() string
    // DDL 差异方法
    AutoIncrementDDL() string     // "INTEGER PRIMARY KEY AUTOINCREMENT" vs "SERIAL PRIMARY KEY"
    BooleanType() string          // "INTEGER" vs "BOOLEAN" vs "TINYINT(1)"
    JSONType() string             // "TEXT" vs "JSON" vs "JSONB"
    UpsertSQL(table string, conflictCol string, setCols []string) string
}
```

**通过环境变量初始化 Driver：**

```go
// storage/init.go

func NewStorageDriver() (StorageDriver, error) {
    backend := os.Getenv("LOOMIDBX_STORAGE")  // 默认空 = sqlite
    switch backend {
    case "mysql":
        return NewMySQLDriver(os.Getenv("LOOMIDBX_STORAGE_DSN"))
    case "postgres":
        return NewPostgresDriver(os.Getenv("LOOMIDBX_STORAGE_DSN"))
    default:
        dbPath := os.Getenv("LOOMIDBX_STORAGE_PATH")
        if dbPath == "" { dbPath = "./loomidbx.db" }
        return NewSQLiteDriver(dbPath)
    }
}
```

### 2.2 Migration 机制

所有建表 DDL 通过 Migration 版本化管理，支持动态创建和修改表结构：

```go
// storage/migrate.go

type Migration struct {
    Version int
    Up      func(d StorageDriver, db *sql.DB) error
    Down    func(d StorageDriver, db *sql.DB) error
}

var Migrations = []Migration{
    {Version: 1, Up: createConnectionsTable},
    {Version: 2, Up: createTableSchemasTable},
    {Version: 3, Up: createColumnSchemasTable},
    {Version: 4, Up: createTableGenConfigsTable},
    {Version: 5, Up: createColumnGenConfigsTable},
    {Version: 6, Up: createTableRelationsTable},
    {Version: 7, Up: createScanHistoryTable},
    {Version: 8, Up: createScanDiffsTable},
}

func RunMigrations(d StorageDriver) error {
    ensureMetaTable(d)   // 创建 ldb_schema_migrations 版本记录表
    current := getCurrentVersion(d)
    for _, m := range Migrations {
        if m.Version > current {
            if err := m.Up(d, d.DB()); err != nil {
                return fmt.Errorf("migration v%d failed: %w", m.Version, err)
            }
            recordVersion(d, m.Version)
        }
    }
    return nil
}
```

**Migration 示例（驱动差异体现）：**

```go
func createConnectionsTable(d StorageDriver, db *sql.DB) error {
    _, err := db.Exec(fmt.Sprintf(`
        CREATE TABLE IF NOT EXISTS ldb_connections (
            id          TEXT PRIMARY KEY,
            name        TEXT NOT NULL,
            db_type     TEXT NOT NULL,
            host        TEXT,
            port        INTEGER,
            username    TEXT,
            password    TEXT,
            database    TEXT,
            extra       %s,
            created_at  INTEGER,
            updated_at  INTEGER
        )`, d.JSONType()))   // SQLite: TEXT；Postgres: JSONB；MySQL: JSON
    return err
}
```

---

## 三、数据模型

### 3.1 完整表结构

```sql
-- ① 数据库连接配置
CREATE TABLE ldb_connections (
    id          TEXT PRIMARY KEY,   -- UUID
    name        TEXT NOT NULL,
    db_type     TEXT NOT NULL,      -- mysql/postgres/oracle/mssql/sqlite/clickhouse/hive
    host        TEXT,
    port        INTEGER,
    username    TEXT,
    password    TEXT,               -- AES-256 加密存储
    database    TEXT,
    extra       TEXT,               -- JSON: 额外连接参数（charset, sslmode 等）
    created_at  INTEGER,
    updated_at  INTEGER
);

-- ② 表 Schema 快照
CREATE TABLE ldb_table_schemas (
    id              TEXT PRIMARY KEY,
    connection_id   TEXT NOT NULL,
    database_name   TEXT NOT NULL,
    schema_name     TEXT,           -- 仅 Oracle/Postgres 有 schema
    table_name      TEXT NOT NULL,
    table_comment   TEXT,
    scan_version    INTEGER NOT NULL DEFAULT 1,   -- 每次扫描（全库或单表）+1，以表为粒度独立推进
    scanned_at      INTEGER NOT NULL,
    FOREIGN KEY (connection_id) REFERENCES ldb_connections(id)
);

-- ③ 字段 Schema 快照
CREATE TABLE ldb_column_schemas (
    id              TEXT PRIMARY KEY,
    table_schema_id TEXT NOT NULL,
    column_name     TEXT NOT NULL,
    ordinal_pos     INTEGER,
    data_type       TEXT NOT NULL,      -- 原始数据库类型，如 varchar(255)
    abstract_type   TEXT NOT NULL,      -- 抽象类型: int/string/decimal/datetime/boolean
    is_primary_key  INTEGER,
    is_nullable     INTEGER,
    is_unique       INTEGER,
    is_auto_increment INTEGER,          -- 自增字段标记，映射时自动 is_enabled=0
    default_value   TEXT,
    column_comment  TEXT,
    fk_ref_table    TEXT,               -- 物理外键：引用的表
    fk_ref_column   TEXT,               -- 物理外键：引用的列
    extra           TEXT,               -- JSON: 额外信息（长度、精度、原始 extra 值等）
    FOREIGN KEY (table_schema_id) REFERENCES ldb_table_schemas(id)
);

-- ④ 表级生成器配置
CREATE TABLE ldb_table_gen_configs (
    id              TEXT PRIMARY KEY,
    table_schema_id TEXT NOT NULL UNIQUE,
    gen_count       INTEGER DEFAULT 100,
    truncate_before INTEGER DEFAULT 0,  -- bool
    order_index     INTEGER,            -- 生成顺序（处理外键依赖拓扑排序结果）
    is_enabled      INTEGER DEFAULT 1,
    FOREIGN KEY (table_schema_id) REFERENCES ldb_table_schemas(id)
);

-- ⑤ 字段级生成器配置
--    confirmed_at IS NULL  → 系统自动推断，待用户确认（UI 显示橙色 "待确认" 标签）
--    confirmed_at NOT NULL → 用户已确认（含逻辑外键，由用户操作直接写入带时间戳）
CREATE TABLE ldb_column_gen_configs (
    id                TEXT PRIMARY KEY,
    column_schema_id  TEXT NOT NULL UNIQUE,
    generator_type    TEXT NOT NULL,        -- 见§五 生成器类型枚举
    generator_opts    TEXT NOT NULL DEFAULT '{}',  -- JSON，空时生成器使用内置默认值
    is_enabled        INTEGER NOT NULL DEFAULT 1,  -- 自增字段自动为 0
    logic_fk_table    TEXT,                 -- 逻辑外键：关联表（用户手动指定）
    logic_fk_column   TEXT,                 -- 逻辑外键：关联列（用户手动指定）
    confirmed_at      INTEGER,              -- NULL=待确认；非NULL=已确认时间戳
    FOREIGN KEY (column_schema_id) REFERENCES ldb_column_schemas(id)
);

-- ⑥ 表间关系及数量倍数（独立于 ldb_column_gen_configs，描述两表间生成数量约定）
CREATE TABLE ldb_table_relations (
    id                  TEXT PRIMARY KEY,
    from_table_id       TEXT NOT NULL,      -- 主表（"一"侧）
    from_column_id      TEXT NOT NULL,      -- 主表关联列
    to_table_id         TEXT NOT NULL,      -- 从表（"多"侧或子表）
    to_column_id        TEXT NOT NULL,      -- 从表关联列
    relation_type       TEXT NOT NULL,      -- "1:1" | "1:0-1" | "1:n"
    multiplier_min      INTEGER,            -- 仅 1:n 有效，倍数下限（如 1）
    multiplier_max      INTEGER,            -- 仅 1:n 有效，倍数上限（如 30）
    source              TEXT NOT NULL,      -- "physical_fk" | "logical"
    FOREIGN KEY (from_table_id) REFERENCES ldb_table_schemas(id),
    FOREIGN KEY (to_table_id)   REFERENCES ldb_table_schemas(id)
);

-- ⑦ 扫描历史
--    scan_scope = "full_db"      : 全库扫描，scope_target = NULL
--    scan_scope = "single_table" : 单表扫描，scope_target = table_name
CREATE TABLE ldb_scan_history (
    id              TEXT PRIMARY KEY,
    connection_id   TEXT NOT NULL,
    database_name   TEXT NOT NULL,
    scan_version    INTEGER NOT NULL,   -- 本次扫描后的版本号
    scan_scope      TEXT NOT NULL DEFAULT 'full_db',
    scope_target    TEXT,               -- 单表扫描时为 table_name
    scanned_at      INTEGER NOT NULL,
    FOREIGN KEY (connection_id) REFERENCES ldb_connections(id)
);

-- ⑧ 差异记录及确认状态
--    confirmed_at IS NULL  → 用户尚未处理该差异（UI 展示）
--    confirmed_at NOT NULL → 已确认，UI 不再展示
CREATE TABLE ldb_scan_diffs (
    id              TEXT PRIMARY KEY,
    scan_history_id TEXT NOT NULL,
    target_id       TEXT NOT NULL,      -- table_schema_id 或 column_schema_id
    target_type     TEXT NOT NULL,      -- "table" | "column"
    diff_type       TEXT NOT NULL,      -- "added" | "removed" | "modified"
    diff_detail     TEXT,               -- JSON: 变化前后的具体属性快照
    confirmed_at    INTEGER,            -- NULL=未确认；非NULL=确认时间戳
    FOREIGN KEY (scan_history_id) REFERENCES ldb_scan_history(id)
);

-- ⑨ 持久化层版本管理（由 Migration 机制维护，不手动操作）
CREATE TABLE ldb_schema_migrations (
    version     INTEGER PRIMARY KEY,
    applied_at  INTEGER NOT NULL
);
```

### 3.2 三种关系类型的生成数量语义

| relation_type | 语义 | 生成数量规则 |
|---|---|---|
| `1:1` | 每条父记录恰好对应一条子记录 | `count(子表) = count(父表)` |
| `1:0-1` | 每条父记录对应零或一条子记录 | `count(子表) ≤ count(父表)`，按概率决定是否生成 |
| `1:n` | 每条父记录对应若干条子记录 | `count(子表) = count(父表) × rand(min, max)` |

---

## 四、Go 后端实现

### 4.1 Connector 接口抽象

为不同数据库统一一个 `Connector` 接口，隔离各数据库的驱动差异：

```go
// connector/interface.go

type DBType string
const (
    MySQL      DBType = "mysql"
    Postgres   DBType = "postgres"
    Oracle     DBType = "oracle"
    MSSQL      DBType = "mssql"
    SQLite     DBType = "sqlite"
    ClickHouse DBType = "clickhouse"
    Hive       DBType = "hive"
)

type ConnectParams struct {
    ID       string
    Name     string
    DBType   DBType
    Host     string
    Port     int
    Username string
    Password string
    Database string
    Extra    map[string]string // sslmode, charset, etc.
}

type Connector interface {
    // 建立连接 / 测试连通性
    Connect(params ConnectParams) error
    Ping() error
    Close() error

    // 列举数据库、Schema、表
    ListDatabases() ([]string, error)
    ListSchemas(database string) ([]string, error)   // Oracle/Postgres 专用
    ListTables(database, schema string) ([]RawTable, error)

    // 扫描字段元数据
    DescribeTable(database, schema, table string) ([]RawColumn, error)

    // 获取物理外键
    GetForeignKeys(database, schema, table string) ([]ForeignKey, error)
}
```

**各数据库 Connector 实现要点：**

| 数据库 | 驱动 | Schema 查询来源 |
|---|---|---|
| MySQL | `go-sql-driver/mysql` | `information_schema.COLUMNS` + `KEY_COLUMN_USAGE` |
| Postgres | `lib/pq` | `information_schema.COLUMNS` + `pg_constraint` |
| Oracle | `godror` | `ALL_TAB_COLUMNS` + `ALL_CONSTRAINTS` |
| MSSQL | `go-mssqldb` | `INFORMATION_SCHEMA.COLUMNS` + `sys.foreign_keys` |
| SQLite | `mattn/go-sqlite3` | `PRAGMA table_info()` + `PRAGMA foreign_key_list()` |
| ClickHouse | `ClickHouse/clickhouse-go` | `system.columns` |
| Hive | `beltran/gohive` | `DESCRIBE FORMATTED` |

### 4.2 Scanner 与抽象类型映射

```go
// scanner/scanner.go

type RawColumn struct {
    Name            string
    OrdinalPos      int
    RawType         string   // 原始类型，如 "varchar(255)", "int(11) unsigned"
    IsNullable      bool
    IsPrimaryKey    bool
    IsUnique        bool
    IsAutoIncrement bool     // 见各数据库识别规则
    DefaultValue    *string
    Comment         string
    MaxLength       *int
    Precision       *int
    Scale           *int
    Extra           string   // 原始 extra 字段值
}

type AbstractType string
const (
    TypeInt      AbstractType = "int"
    TypeString   AbstractType = "string"
    TypeDecimal  AbstractType = "decimal"
    TypeDatetime AbstractType = "datetime"
    TypeBoolean  AbstractType = "boolean"
)

// 原始类型 → 抽象类型（按前缀/关键字匹配，去括号后匹配）
var typeMapping = map[string]AbstractType{
    // int 族
    "int": TypeInt, "tinyint": TypeInt, "smallint": TypeInt,
    "mediumint": TypeInt, "bigint": TypeInt, "serial": TypeInt,
    "integer": TypeInt, "number": TypeInt,
    // decimal 族
    "decimal": TypeDecimal, "numeric": TypeDecimal, "float": TypeDecimal,
    "double": TypeDecimal, "real": TypeDecimal, "money": TypeDecimal,
    // string 族
    "char": TypeString, "varchar": TypeString, "text": TypeString,
    "nchar": TypeString, "nvarchar": TypeString, "clob": TypeString,
    "string": TypeString,
    // datetime 族
    "date": TypeDatetime, "time": TypeDatetime, "datetime": TypeDatetime,
    "timestamp": TypeDatetime, "year": TypeDatetime,
    // boolean 族
    "bool": TypeBoolean, "boolean": TypeBoolean, "bit": TypeBoolean,
}

func ResolveAbstractType(rawType string) AbstractType {
    base := strings.Split(strings.ToLower(rawType), "(")[0]
    if t, ok := typeMapping[base]; ok {
        return t
    }
    return TypeString // fallback
}
```

**各数据库自增字段识别规则：**

```go
// scanner/autoincrement.go

func DetectAutoIncrement(col RawColumn) bool {
    extra := strings.ToLower(col.Extra)
    rawType := strings.ToLower(col.RawType)
    defVal := ""
    if col.DefaultValue != nil { defVal = strings.ToLower(*col.DefaultValue) }

    return strings.Contains(extra, "auto_increment") ||     // MySQL
           strings.Contains(extra, "identity") ||           // MSSQL
           strings.Contains(rawType, "serial") ||           // Postgres: serial/bigserial
           strings.Contains(defVal, "nextval") ||           // Postgres: sequence
           (col.IsPrimaryKey && rawType == "integer" &&     // SQLite: implicit rowid
            strings.Contains(extra, "autoincrement"))
}
```

### 4.3 重新扫描与 Diff

#### 扫描范围说明

- **全库扫描**：遍历指定连接/库下所有表，更新全部 `ldb_table_schemas`/`ldb_column_schemas`，`scan_version` 统一 +1
- **单表扫描**：只更新指定表，仅该表 `scan_version` +1，不影响同库其他表

```go
// scanner/diff.go

type DiffType string
const (
    DiffAdded     DiffType = "added"
    DiffRemoved   DiffType = "removed"
    DiffModified  DiffType = "modified"
    DiffUnchanged DiffType = "unchanged"
)

type ColumnDiff struct {
    DiffType   DiffType
    ColumnName string
    OldColumn  *ColumnSchema // nil if added
    NewColumn  *ColumnSchema // nil if removed
    Changes    []string      // 发生变化的属性，如 ["data_type", "is_nullable"]
}

type TableDiff struct {
    DiffType    DiffType
    TableName   string
    ColumnDiffs []ColumnDiff
}

type SchemaDiff struct {
    TableDiffs         []TableDiff
    HasChanges         bool
    AffectedGenerators []AffectedGen  // 影响已有生成器设置的字段，需提醒用户
}

type AffectedGen struct {
    TableName  string
    ColumnName string
    Reason     string // "type_changed" | "column_removed" | "constraint_changed"
}
```

**Diff 处理规则：**

| 变化类型 | 对已有生成器的影响 |
|---|---|
| 新增字段 | 自动创建新生成器，`confirmed_at = NULL`（待确认） |
| 删除字段 | 标记生成器 `is_enabled = 0`，`ldb_scan_diffs` 记录，待用户确认处理方式 |
| 类型变化 | 原生成器可能失效，`confirmed_at = NULL`（重置为待确认） |
| 约束变化（nullable/unique）| 更新约束参数，`confirmed_at = NULL`（重置为待确认） |
| 名称/注释变化 | 静默更新，不影响生成器，不写入 `ldb_scan_diffs` |

---

## 五、自动生成器映射

### 5.1 映射优先级

```
优先级 0（最高）：自增字段  →  is_enabled = false，generator_type = "none"，直接写入 confirmed_at
优先级 1：物理外键           →  ForeignKeyGenerator（值从引用表取，不可覆盖）
优先级 2：主键 + 整数类型    →  SequenceGenerator（起始 1，步长 1）
优先级 3：列名关键词语义匹配 →  SemanticGenerator（见§5.2）
优先级 4：抽象类型 Fallback  →  通用生成器，opts = "{}"（生成器内置默认值填充）
```

### 5.2 列名关键词 → 语义生成器映射表

```go
// mapper/semantic.go

var semanticRules = []SemanticRule{
    // 身份信息
    {Keywords: []string{"name", "username", "full_name", "姓名", "用户名"},
        Generator: "ChineseNameGenerator"},
    {Keywords: []string{"email", "mail", "邮箱"},
        Generator: "EmailGenerator"},
    {Keywords: []string{"phone", "mobile", "tel", "手机", "电话"},
        Generator: "PhoneGenerator", Opts: `{"pattern": "1[3-9]\\d{9}"}`},
    {Keywords: []string{"id_card", "identity", "身份证"},
        Generator: "IDCardGenerator"},
    {Keywords: []string{"address", "addr", "地址"},
        Generator: "ChineseAddressGenerator"},

    // 业务编号
    {Keywords: []string{"order_no", "order_num", "订单号"},
        Generator: "PatternGenerator",
        Opts: `{"pattern": "ORD{date:yyyyMMdd}{seq:6}"}`},
    {Keywords: []string{"sn", "serial_no", "序列号"},
        Generator: "PatternGenerator",
        Opts: `{"pattern": "{prefix:SN}{seq:8}"}`},

    // 时间
    {Keywords: []string{"created_at", "updated_at", "create_time", "update_time"},
        Generator: "DatetimeGenerator",
        Opts: `{"range_start": "-1y", "range_end": "now"}`},
    {Keywords: []string{"birthday", "birth_date", "出生日期"},
        Generator: "DatetimeGenerator",
        Opts: `{"range_start": "-60y", "range_end": "-18y"}`},

    // 金额/数值
    {Keywords: []string{"price", "amount", "salary", "wage", "金额", "价格", "薪资"},
        Generator: "DistributionGenerator",
        Opts: `{"distribution": "lognormal", "mean": 8.5, "stddev": 0.8}`},
    {Keywords: []string{"age", "年龄"},
        Generator: "RangeGenerator", Opts: `{"min": 18, "max": 65}`},
    {Keywords: []string{"score", "rating", "评分"},
        Generator: "RangeGenerator", Opts: `{"min": 1, "max": 100}`},

    // 状态/枚举
    {Keywords: []string{"status", "state", "type", "gender", "状态", "类型", "性别"},
        Generator: "EnumGenerator",
        Opts: `{"values": ["A", "B", "C"], "weights": [1, 1, 1]}`},
    {Keywords: []string{"is_", "enable", "active", "flag"},
        Generator: "BooleanGenerator", Opts: `{"true_ratio": 0.7}`},

    // 文本
    {Keywords: []string{"remark", "note", "comment", "description", "备注", "描述"},
        Generator: "AITextGenerator", Opts: `{"max_length": 200}`},
    {Keywords: []string{"url", "link", "avatar", "image"},
        Generator: "URLGenerator"},
}
```

### 5.3 抽象类型 Fallback 生成器

`opts` 统一为 `"{}"`，由生成器运行时通过 `DefaultOpts()` 提供内置默认值。升级生成器默认值时，未经用户修改的字段自动受益，无需数据迁移。

```go
var typeFallback = map[AbstractType]GeneratorConfig{
    TypeInt:      {Type: "RangeGenerator",        Opts: "{}"},
    TypeString:   {Type: "RandomStringGenerator", Opts: "{}"},
    TypeDecimal:  {Type: "RangeGenerator",        Opts: "{}"},
    TypeDatetime: {Type: "DatetimeGenerator",     Opts: "{}"},
    TypeBoolean:  {Type: "BooleanGenerator",      Opts: "{}"},
}
```

### 5.4 完整 AutoMap 逻辑

```go
// mapper/auto_mapper.go

func AutoMap(col ColumnSchema) ColumnGenConfig {

    // 优先级 0：自增字段 → 禁用生成，直接标记已确认（无需用户介入）
    if col.IsAutoIncrement {
        return ColumnGenConfig{
            ColumnSchemaID: col.ID,
            GeneratorType:  "none",
            GeneratorOpts:  "{}",
            IsEnabled:      false,
            ConfirmedAt:    ptr(time.Now().Unix()),
        }
    }

    // 优先级 1：物理外键 → ForeignKeyGenerator
    if col.FKRefTable != "" {
        return ColumnGenConfig{
            ColumnSchemaID: col.ID,
            GeneratorType:  "ForeignKeyGenerator",
            GeneratorOpts:  fmt.Sprintf(`{"ref_table":"%s","ref_column":"%s"}`,
                                col.FKRefTable, col.FKRefColumn),
            IsEnabled:      true,
            ConfirmedAt:    nil, // 仍需用户确认关联关系
        }
    }

    // 优先级 2：整数主键 → SequenceGenerator
    if col.IsPrimaryKey && col.AbstractType == TypeInt {
        return ColumnGenConfig{
            ColumnSchemaID: col.ID,
            GeneratorType:  "SequenceGenerator",
            GeneratorOpts:  `{"start": 1, "step": 1}`,
            IsEnabled:      true,
            ConfirmedAt:    nil,
        }
    }

    // 优先级 3：语义匹配
    if rule := matchSemantic(col.ColumnName); rule != nil {
        return ColumnGenConfig{
            ColumnSchemaID: col.ID,
            GeneratorType:  rule.Generator,
            GeneratorOpts:  rule.Opts,
            IsEnabled:      true,
            ConfirmedAt:    nil,
        }
    }

    // 优先级 4：抽象类型 Fallback
    fb := typeFallback[col.AbstractType]
    return ColumnGenConfig{
        ColumnSchemaID: col.ID,
        GeneratorType:  fb.Type,
        GeneratorOpts:  "{}",
        IsEnabled:      true,
        ConfirmedAt:    nil,
    }
}
```

---

## 六、FFI 接口设计（Go → Flutter）

所有接口统一通过 JSON 序列化传递，Go 端导出 C 函数。

### 6.1 导出函数清单

```go
// ffi/exports.go

//export TestConnection
func TestConnection(paramsJSON *C.char) *C.char
// 入参: ConnectParams JSON
// 返回: {"ok": true} 或 {"ok": false, "error": "..."}

//export SaveConnection
func SaveConnection(paramsJSON *C.char) *C.char
// 入参: ConnectParams JSON
// 返回: {"ok": true, "data": {"id": "..."}}

//export ListConnections
func ListConnections() *C.char
// 返回: {"ok": true, "data": [Connection...]}

//export DeleteConnection
func DeleteConnection(connID *C.char) *C.char

//export ListDatabases
func ListDatabases(connID *C.char) *C.char
// 返回: {"ok": true, "data": ["db1", "db2"]}

//export ScanSchema
func ScanSchema(connID *C.char, dbName *C.char, tableName *C.char) *C.char
// tableName 为空字符串时执行全库扫描，非空时执行单表扫描
// 返回: {"ok": true, "data": ScanResult}
// ScanResult 包含: scan_history_id, tables[], diff(若非首次扫描)

//export GetTableConfig
func GetTableConfig(tableSchemaID *C.char) *C.char
// 返回表配置 + 所有字段的生成器配置 + 表间关系

//export SaveTableGenConfig
func SaveTableGenConfig(configJSON *C.char) *C.char

//export SaveColumnGenConfig
func SaveColumnGenConfig(configJSON *C.char) *C.char
// 保存字段生成器配置，同时写入 confirmed_at = now

//export SaveTableRelation
func SaveTableRelation(relationJSON *C.char) *C.char
// 保存表间关系（含 relation_type, multiplier_min/max）

//export GetScanDiffs
func GetScanDiffs(scanHistoryID *C.char) *C.char
// 返回指定扫描历史下所有未确认的 diff

//export ConfirmDiff
func ConfirmDiff(diffID *C.char) *C.char
// 确认单条 diff，写入 ldb_scan_diffs.confirmed_at = now

//export ConfirmAllDiffs
func ConfirmAllDiffs(scanHistoryID *C.char) *C.char
// 批量确认同一次扫描下所有未确认 diff

//export FreeString
func FreeString(ptr *C.char)
// 释放 Go 分配的字符串内存
```

### 6.2 统一响应格式

```json
{
  "ok": true,
  "data": { ... },
  "error": null
}
```

---

## 七、Flutter 前端数据流

```
ConnectionTreeWidget
    │  双击连接节点
    ▼
ListDatabases(connId) ──FFI──► 展示数据库列表
    │  双击数据库
    ▼
ScanSchema(connId, dbName, "") ──FFI──► Go: 全库扫描
    │  返回 ScanResult
    ├── 首次扫描：直接更新 ConnectionProvider
    └── 非首次扫描：弹出 DiffDialog
            │  用户逐条或批量确认
            ▼
        ConfirmDiff / ConfirmAllDiffs ──FFI──► Go: 写 confirmed_at
    │
    ▼
更新 ConnectionProvider（Riverpod）
    │
    ▼
TableConfigScreen（右侧 Tab，双击表节点打开）
    ├── TableHeaderSection
    │       ├── 表名、注释
    │       ├── 生成数量、Truncate 选项
    │       └── 关联关系（ldb_table_relations）
    └── ColumnListSection（字段列表）
            │  点击字段行
            ▼
        GeneratorConfigPanel（右侧面板）
            ├── 字段信息（类型、约束）
            ├── 自增字段：灰色显示，"AUTO-INCREMENT" 徽标，is_enabled=false
            ├── 待确认字段：橙色 "待确认" 徽标（confirmed_at IS NULL）
            ├── 生成器类型选择下拉
            ├── 参数表单（按 generator_type 动态渲染）
            ├── 逻辑外键配置入口
            └── "预览 / 确认" 按钮
                    │ 点击确认
                    ▼
            SaveColumnGenConfig ──FFI──► Go: 写 confirmed_at
```

**UI 状态标识说明：**

| 标识 | 颜色 | 含义 |
|---|---|---|
| `AUTO-INCREMENT` | 灰色 | 自增字段，已自动禁用生成 |
| `待确认` | 橙色 | 系统自动推断，需用户检查并确认 |
| `外键` | 蓝色 | 物理外键或逻辑外键 |
| `⚠ N fields need review` | 表头警告 | 本表存在 N 个待确认字段 |

---

## 八、重新扫描完整流程

```
用户右键表/数据库 → 选择"重新扫描"
    │
    ▼
ScanSchema(connId, dbName, tableName?) ──FFI──►
    │
    Go 内部流程：
    ├── 1. 执行新一轮扫描，获取最新 Schema 快照
    ├── 2. DiffSchema(旧快照, 新快照)
    ├── 3. 按规则处理各 diff 类型（见§4.3 处理规则表）
    ├── 4. 写入 ldb_scan_history（scan_scope + scope_target）
    ├── 5. 写入 ldb_scan_diffs（confirmed_at = NULL）
    ├── 6. 更新 ldb_table_schemas/ldb_column_schemas（scan_version +1）
    └── 7. 返回 ScanDiffResult
    │
    ▼
Flutter: 有未确认 diff？
    ├── 是 → 弹出 DiffDialog
    │       ├── 新增表/字段：绿色 [+]，显示自动推断的生成器，可编辑
    │       ├── 删除表/字段：红色 [-]，提示处理方式（禁用/删除生成器）
    │       ├── 修改字段：黄色 [~]，左右对比变化前后属性
    │       ├── 逐条"确认"按钮 → ConfirmDiff
    │       └── "全部接受" 按钮 → ConfirmAllDiffs
    └── 否 → 静默刷新树节点（无实质变化）
```

**扫描历史面板（按时间倒序展示，全库与单表共存）：**

```
扫描历史

  2024-01-15 10:32  全库扫描    mydb           [展开] 查看各表 diff
  2024-01-14 16:20  单表扫描    orders         [展开] 查看该表 diff
  2024-01-13 09:10  全库扫描    mydb           [展开] 查看各表 diff

版本冲突规则：
  - 单表扫描只影响该表的 scan_version，不影响同库其他表
  - 某张表展示 diff 时，以该表最新一条扫描记录（无论全库/单表）为准
  - 全库扫描不会覆盖单表扫描产生的更高 scan_version
```

---

## 九、关键设计决策汇总

| 决策点 | 方案选择 | 理由 |
|---|---|---|
| 持久化后端 | 环境变量切换，Migration 动态建表 | 支持 SQLite/MySQL/Postgres，零硬编码 DDL |
| Schema 版本化 | `scan_version` 以表为粒度独立推进 | 全库/单表扫描互不干扰 |
| 生成器确认状态 | 单字段 `confirmed_at`（NULL=待确认） | 消除冗余 `is_auto_mapped` 字段，逻辑更清晰 |
| 表间数量关系 | 独立 `ldb_table_relations` 表 | 与"值从哪来"的 ldb_column_gen_configs 职责分离 |
| 外键数量类型 | `1:1` / `1:0-1` / `1:n` + 倍数范围 | 覆盖业务中常见的三类数量约定 |
| 自增字段处理 | 自动推断 `is_auto_increment`，`is_enabled=0` | 避免插入时与数据库序列冲突，直接确认无需用户操作 |
| Fallback 生成器参数 | `generator_opts = "{}"` | 生成器内置默认值，升级时自动受益 |
| Diff 确认持久化 | `ldb_scan_diffs.confirmed_at` | 确认后 UI 自动消除，历史可查 |
| FFI 传输格式 | JSON 序列化 | 与产品设计一致，牺牲少量性能换开发便利性 |
| 密码存储 | AES-256 加密后存储 | 避免连接密码明文落盘 |
```