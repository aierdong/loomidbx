package schema

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
)

// SQLDialect 表示基于 information_schema 的 SQL 方言类型。
type SQLDialect string

const (
	// SQLDialectMySQL 表示 MySQL 方言。
	SQLDialectMySQL SQLDialect = "mysql"
	
	// SQLDialectPostgres 表示 PostgreSQL 方言。
	SQLDialectPostgres SQLDialect = "postgres"
)

// SQLSchemaInspector 负责 MySQL/Postgres 的 schema 扫描与统一结构构建。
type SQLSchemaInspector struct {
	// Dialect 指定方言类型。
	Dialect SQLDialect
}

// NewMySQLSchemaInspector 创建 MySQL 方言 inspector。
func NewMySQLSchemaInspector() *SQLSchemaInspector {
	return &SQLSchemaInspector{Dialect: SQLDialectMySQL}
}

// NewPostgresSchemaInspector 创建 Postgres 方言 inspector。
func NewPostgresSchemaInspector() *SQLSchemaInspector {
	return &SQLSchemaInspector{Dialect: SQLDialectPostgres}
}

// Inspect 执行 schema 扫描。
func (i *SQLSchemaInspector) Inspect(ctx context.Context, db *sql.DB, scope InspectScope) (*SchemaGraph, *UpstreamClassifiedError) {
	tables, err := i.listTables(ctx, db, scope)
	if err != nil {
		return nil, ClassifyUpstreamError(ctx, err)
	}

	out := make([]TableDef, 0, len(tables))
	for _, t := range tables {
		table, tErr := i.inspectTable(ctx, db, t.DatabaseName, t.SchemaName, t.TableName)
		if tErr != nil {
			return nil, ClassifyUpstreamError(ctx, tErr)
		}
		out = append(out, *table)
	}

	sort.Slice(out, func(a, b int) bool {
		ka := strings.ToLower(out[a].DatabaseName) + "|" + strings.ToLower(out[a].SchemaName) + "|" + strings.ToLower(out[a].TableName)
		kb := strings.ToLower(out[b].DatabaseName) + "|" + strings.ToLower(out[b].SchemaName) + "|" + strings.ToLower(out[b].TableName)
		return ka < kb
	})
	return &SchemaGraph{Tables: out}, nil
}

type sqlTableRef struct {
	DatabaseName string
	SchemaName   string
	TableName    string
}

func (i *SQLSchemaInspector) listTables(ctx context.Context, db *sql.DB, scope InspectScope) ([]sqlTableRef, error) {
	switch i.Dialect {
	case SQLDialectMySQL:
		return i.listTablesMySQL(ctx, db, scope)
	case SQLDialectPostgres:
		return i.listTablesPostgres(ctx, db, scope)
	default:
		return nil, fmt.Errorf("unsupported dialect: %s", i.Dialect)
	}
}

func (i *SQLSchemaInspector) listTablesMySQL(ctx context.Context, db *sql.DB, scope InspectScope) ([]sqlTableRef, error) {
	dbName := strings.TrimSpace(scope.DatabaseName)
	if dbName == "" {
		return nil, fmt.Errorf("database_name is required for mysql inspector")
	}

	if scope.Scope == SchemaScanScopeTables {
		names := dedupeAndSort(scope.TableNames)
		out := make([]sqlTableRef, 0, len(names))
		for _, n := range names {
			out = append(out, sqlTableRef{DatabaseName: dbName, TableName: n})
		}
		return out, nil
	}

	rows, err := db.QueryContext(ctx, `
		SELECT table_name
		FROM information_schema.tables
		WHERE table_schema = ? AND table_type = 'BASE TABLE'
		ORDER BY table_name ASC
	`, dbName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []sqlTableRef
	for rows.Next() {
		var n string
		if scanErr := rows.Scan(&n); scanErr != nil {
			return nil, scanErr
		}
		out = append(out, sqlTableRef{DatabaseName: dbName, TableName: n})
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		return nil, rowsErr
	}
	return out, nil
}

func (i *SQLSchemaInspector) listTablesPostgres(ctx context.Context, db *sql.DB, scope InspectScope) ([]sqlTableRef, error) {
	schemaName := strings.TrimSpace(scope.SchemaName)
	if schemaName == "" {
		schemaName = "public"
	}

	if scope.Scope == SchemaScanScopeTables {
		names := dedupeAndSort(scope.TableNames)
		out := make([]sqlTableRef, 0, len(names))
		for _, n := range names {
			out = append(out, sqlTableRef{SchemaName: schemaName, TableName: n})
		}
		return out, nil
	}

	rows, err := db.QueryContext(ctx, `
		SELECT table_name
		FROM information_schema.tables
		WHERE table_schema = $1 AND table_type = 'BASE TABLE'
		ORDER BY table_name ASC
	`, schemaName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []sqlTableRef
	for rows.Next() {
		var n string
		if scanErr := rows.Scan(&n); scanErr != nil {
			return nil, scanErr
		}
		out = append(out, sqlTableRef{SchemaName: schemaName, TableName: n})
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		return nil, rowsErr
	}
	return out, nil
}

func (i *SQLSchemaInspector) inspectTable(ctx context.Context, db *sql.DB, databaseName, schemaName, tableName string) (*TableDef, error) {
	var (
		cols []ColumnDef
		pk   []string
		ucs  []UniqueConstraintDef
		fks  []ForeignKeyDef
		err  error
	)

	switch i.Dialect {
	case SQLDialectMySQL:
		cols, pk, err = inspectColumnsMySQL(ctx, db, databaseName, tableName)
		if err != nil {
			return nil, err
		}
		ucs, err = inspectUniqueMySQL(ctx, db, databaseName, tableName)
		if err != nil {
			return nil, err
		}
		fks, err = inspectForeignKeysMySQL(ctx, db, databaseName, tableName)
		if err != nil {
			return nil, err
		}
	case SQLDialectPostgres:
		cols, pk, err = inspectColumnsPostgres(ctx, db, schemaName, tableName)
		if err != nil {
			return nil, err
		}
		ucs, err = inspectUniquePostgres(ctx, db, schemaName, tableName)
		if err != nil {
			return nil, err
		}
		fks, err = inspectForeignKeysPostgres(ctx, db, schemaName, tableName)
		if err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unsupported dialect: %s", i.Dialect)
	}

	sort.Slice(cols, func(a, b int) bool {
		if cols[a].OrdinalPos != cols[b].OrdinalPos {
			return cols[a].OrdinalPos < cols[b].OrdinalPos
		}
		return strings.ToLower(cols[a].Name) < strings.ToLower(cols[b].Name)
	})
	sort.Slice(ucs, func(a, b int) bool {
		ka := strings.ToLower(ucs[a].Name) + "|" + strings.Join(ucs[a].Columns, ",")
		kb := strings.ToLower(ucs[b].Name) + "|" + strings.Join(ucs[b].Columns, ",")
		return ka < kb
	})
	sort.Slice(fks, func(a, b int) bool {
		ka := strings.ToLower(fks[a].Name) + "|" + strings.ToLower(fks[a].RefTable) + "|" + strings.Join(fks[a].Columns, ",")
		kb := strings.ToLower(fks[b].Name) + "|" + strings.ToLower(fks[b].RefTable) + "|" + strings.Join(fks[b].Columns, ",")
		return ka < kb
	})

	return &TableDef{
		DatabaseName:      databaseName,
		SchemaName:        schemaName,
		TableName:         tableName,
		Columns:           cols,
		PrimaryKey:        pk,
		UniqueConstraints: ucs,
		ForeignKeys:       fks,
	}, nil
}

func dedupeAndSort(names []string) []string {
	m := make(map[string]struct{}, len(names))
	out := make([]string, 0, len(names))
	for _, n := range names {
		t := strings.TrimSpace(n)
		if t == "" {
			continue
		}
		key := strings.ToLower(t)
		if _, ok := m[key]; ok {
			continue
		}
		m[key] = struct{}{}
		out = append(out, t)
	}
	sort.Slice(out, func(a, b int) bool { return strings.ToLower(out[a]) < strings.ToLower(out[b]) })
	return out
}

