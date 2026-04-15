package schema

// SchemaGraph 是一次扫描在内存中构建的统一 schema 图。
//
// 设计约束：
// - 输出必须按确定性顺序稳定排序，便于 Diff 与 UI 展示；
// - 该结构不包含任何连接凭据或敏感信息。
type SchemaGraph struct {
	// Tables 为扫描到的表定义集合，必须已按确定性顺序排序。
	Tables []TableDef
}

// TableDef 表示内存中的表结构定义。
type TableDef struct {
	// DatabaseName 为数据库名（MySQL/PG）；SQLite 可为空。
	DatabaseName string

	// SchemaName 为逻辑 schema 名（PG/Oracle）；可为空。
	SchemaName string

	// TableName 为表名。
	TableName string

	// Columns 为列定义集合，必须已按确定性顺序排序。
	Columns []ColumnDef

	// PrimaryKey 为主键列名列表，必须按主键列顺序排序。
	PrimaryKey []string

	// UniqueConstraints 为唯一约束集合，必须已按确定性顺序排序。
	UniqueConstraints []UniqueConstraintDef

	// ForeignKeys 为外键集合，必须已按确定性顺序排序。
	ForeignKeys []ForeignKeyDef
}

// ColumnDef 表示内存中的列结构定义。
type ColumnDef struct {
	// Name 为列名。
	Name string

	// OrdinalPos 为列顺序（从 1 开始）；未知时为 0。
	OrdinalPos int

	// DataType 为方言相关原始类型字符串。
	DataType string

	// AbstractType 为统一抽象类型枚举：int/string/decimal/datetime/boolean。
	AbstractType string

	// IsNullable 表示是否可空。
	IsNullable bool

	// DefaultValue 为默认值文本表示（为空表示无默认值或未知）。
	DefaultValue string

	// IsAutoIncrement 表示是否自增列。
	IsAutoIncrement bool
}

// UniqueConstraintDef 表示唯一约束或唯一索引在内存中的标准化定义。
type UniqueConstraintDef struct {
	// Name 为约束/索引名称；方言可能为系统生成名。
	Name string

	// Columns 为参与唯一约束的列名列表，必须按列在表中的顺序排序。
	Columns []string
}

// ForeignKeyDef 表示外键约束在内存中的标准化定义。
type ForeignKeyDef struct {
	// Name 为外键约束名；SQLite 可能为空。
	Name string

	// Columns 为本表外键列名列表，必须按约束列顺序排序。
	Columns []string

	// RefTable 为引用表名。
	RefTable string

	// RefColumns 为引用列名列表，必须与 Columns 一一对应并按顺序排序。
	RefColumns []string
}

