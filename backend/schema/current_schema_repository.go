package schema

import "context"

// TableSchemaPersisted 表示 ldb_table_schemas 中一行的领域读模型（当前 schema 按连接覆盖，不做历史快照）。
type TableSchemaPersisted struct {
	// ID 为表级 schema 行主键。
	ID string

	// ConnectionID 为所属连接 ID。
	ConnectionID string

	// DatabaseName 为目标库名。
	DatabaseName string

	// SchemaName 为逻辑 schema（Oracle/Postgres）；可为空。
	SchemaName string

	// TableName 为表名。
	TableName string

	// TableComment 为表注释文本。
	TableComment string

	// ScanVersion 为表级扫描版本号，每次覆盖同步时按设计递增。
	ScanVersion int

	// ScannedAt 为最近一次写入该表行的时间（Unix 秒）。
	ScannedAt int64
}

// ColumnSchemaPersisted 表示 ldb_column_schemas 中一行的领域读模型。
type ColumnSchemaPersisted struct {
	// ID 为列级 schema 行主键。
	ID string

	// TableSchemaID 为所属 ldb_table_schemas.id。
	TableSchemaID string

	// ColumnName 为列名。
	ColumnName string

	// OrdinalPos 为列顺序（可为 0 表示未知）。
	OrdinalPos int

	// DataType 为方言相关原始类型字符串。
	DataType string

	// AbstractType 为统一抽象类型（int/string/decimal/datetime/boolean）。
	AbstractType string

	// IsPrimaryKey 表示是否主键列。
	IsPrimaryKey bool

	// IsNullable 表示是否可空。
	IsNullable bool

	// IsUnique 表示是否唯一约束列。
	IsUnique bool

	// IsAutoIncrement 表示是否自增列。
	IsAutoIncrement bool

	// DefaultValue 为默认值文本表示（可为空）。
	DefaultValue string

	// ColumnComment 为列注释。
	ColumnComment string

	// FKRefTable 为物理外键引用表（可为空）。
	FKRefTable string

	// FKRefColumn 为物理外键引用列（可为空）。
	FKRefColumn string

	// Extra 为列级扩展 JSON 文本（长度/精度等），存储形态由 StorageDriver 决定。
	Extra string
}

// CurrentSchemaBundle 聚合某连接下当前生效的表/列 schema，用于 Load 与事务性 Replace。
type CurrentSchemaBundle struct {
	// Tables 为表级行集合。
	Tables []TableSchemaPersisted

	// Columns 为列级行集合，引用 Tables 的主键。
	Columns []ColumnSchemaPersisted
}

// CurrentSchemaRepository 仅维护当前 schema（ldb_table_schemas / ldb_column_schemas），按连接维度覆盖更新。
type CurrentSchemaRepository interface {
	// LoadCurrentSchema 读取指定连接的当前持久化 schema；若尚无任何行，返回空 Bundle 或领域 NotFound（由实现约定）。
	LoadCurrentSchema(ctx context.Context, connectionID string) (*CurrentSchemaBundle, error)

	// TransactionalReplaceCurrentSchema 在同一事务内删除该连接旧行并写入新 Bundle，实现覆盖语义。
	TransactionalReplaceCurrentSchema(ctx context.Context, connectionID string, next *CurrentSchemaBundle) error
}
