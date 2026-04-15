package schema

import (
	"context"
	"database/sql"
)

// InspectScope 描述一次扫描的目标范围。
type InspectScope struct {
	// Scope 表示扫描范围（all/table）。
	Scope SchemaScanScope

	// TableNames 在 Scope=table 时为目标表名列表；全库时忽略。
	TableNames []string

	// DatabaseName 为目标数据库名（MySQL/PG）；SQLite 可为空。
	DatabaseName string

	// SchemaName 为目标 schema 名（PG/Oracle）；可为空。
	SchemaName string
}

// SchemaInspector 负责从目标数据库读取结构元数据并构建统一内存 schema 图。
//
// 约束：
// - 必须按确定性顺序读取并输出 tables/columns/pk/unique/fk；
// - 必须将方言差异收敛为同一套内存结构；
// - 错误必须映射为稳定上游错误码，且不泄漏敏感信息。
type SchemaInspector interface {
	// Inspect 执行一次 schema 扫描并返回内存 schema 图。
	Inspect(ctx context.Context, db *sql.DB, scope InspectScope) (*SchemaGraph, *UpstreamClassifiedError)
}

