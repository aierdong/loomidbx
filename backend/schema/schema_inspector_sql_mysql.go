package schema

import (
	"context"
	"database/sql"
	"sort"
	"strings"
)

func inspectColumnsMySQL(ctx context.Context, db *sql.DB, dbName, tableName string) ([]ColumnDef, []string, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT
			column_name,
			ordinal_position,
			column_type,
			is_nullable,
			column_default,
			extra,
			column_key
		FROM information_schema.columns
		WHERE table_schema = ? AND table_name = ?
		ORDER BY ordinal_position ASC
	`, dbName, tableName)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	var cols []ColumnDef
	var pk []string
	for rows.Next() {
		var (
			name     string
			pos      int
			colType  string
			nullable string
			defVal   sql.NullString
			extra    sql.NullString
			colKey   sql.NullString
		)
		if scanErr := rows.Scan(&name, &pos, &colType, &nullable, &defVal, &extra, &colKey); scanErr != nil {
			return nil, nil, scanErr
		}
		if strings.EqualFold(colKey.String, "PRI") {
			pk = append(pk, name)
		}
		cols = append(cols, ColumnDef{
			Name:            name,
			OrdinalPos:      pos,
			DataType:        colType,
			AbstractType:    ResolveAbstractType(colType),
			IsNullable:      strings.EqualFold(nullable, "YES"),
			DefaultValue:    nullStringToString(defVal),
			IsAutoIncrement: strings.Contains(strings.ToLower(extra.String), "auto_increment"),
		})
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		return nil, nil, rowsErr
	}
	sort.Slice(pk, func(a, b int) bool { return strings.ToLower(pk[a]) < strings.ToLower(pk[b]) })
	return cols, pk, nil
}

func inspectUniqueMySQL(ctx context.Context, db *sql.DB, dbName, tableName string) ([]UniqueConstraintDef, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT index_name, column_name, seq_in_index
		FROM information_schema.statistics
		WHERE table_schema = ? AND table_name = ? AND non_unique = 0
		ORDER BY index_name ASC, seq_in_index ASC
	`, dbName, tableName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type item struct {
		seq int
		col string
	}
	idxMap := map[string][]item{}
	var names []string
	seen := map[string]struct{}{}
	for rows.Next() {
		var idxName, col string
		var seq int
		if scanErr := rows.Scan(&idxName, &col, &seq); scanErr != nil {
			return nil, scanErr
		}
		idxMap[idxName] = append(idxMap[idxName], item{seq: seq, col: col})
		if _, ok := seen[idxName]; !ok {
			seen[idxName] = struct{}{}
			names = append(names, idxName)
		}
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		return nil, rowsErr
	}

	sort.Slice(names, func(a, b int) bool { return strings.ToLower(names[a]) < strings.ToLower(names[b]) })
	out := make([]UniqueConstraintDef, 0, len(names))
	for _, n := range names {
		items := idxMap[n]
		sort.Slice(items, func(a, b int) bool { return items[a].seq < items[b].seq })
		cols := make([]string, 0, len(items))
		for _, it := range items {
			cols = append(cols, it.col)
		}
		out = append(out, UniqueConstraintDef{Name: n, Columns: cols})
	}
	return out, nil
}

func inspectForeignKeysMySQL(ctx context.Context, db *sql.DB, dbName, tableName string) ([]ForeignKeyDef, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT
			constraint_name,
			column_name,
			referenced_table_name,
			referenced_column_name,
			ordinal_position
		FROM information_schema.key_column_usage
		WHERE table_schema = ? AND table_name = ? AND referenced_table_name IS NOT NULL
		ORDER BY constraint_name ASC, ordinal_position ASC
	`, dbName, tableName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type item struct {
		seq    int
		col    string
		refCol string
	}
	type group struct {
		refTable string
		items    []item
	}
	m := map[string]*group{}
	var names []string
	seen := map[string]struct{}{}
	for rows.Next() {
		var (
			name, col, refTable, refCol string
			seq                         int
		)
		if scanErr := rows.Scan(&name, &col, &refTable, &refCol, &seq); scanErr != nil {
			return nil, scanErr
		}
		g := m[name]
		if g == nil {
			g = &group{refTable: refTable}
			m[name] = g
		}
		g.items = append(g.items, item{seq: seq, col: col, refCol: refCol})
		if _, ok := seen[name]; !ok {
			seen[name] = struct{}{}
			names = append(names, name)
		}
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		return nil, rowsErr
	}

	sort.Slice(names, func(a, b int) bool { return strings.ToLower(names[a]) < strings.ToLower(names[b]) })
	out := make([]ForeignKeyDef, 0, len(names))
	for _, n := range names {
		g := m[n]
		sort.Slice(g.items, func(a, b int) bool { return g.items[a].seq < g.items[b].seq })
		cols := make([]string, 0, len(g.items))
		refCols := make([]string, 0, len(g.items))
		for _, it := range g.items {
			cols = append(cols, it.col)
			refCols = append(refCols, it.refCol)
		}
		out = append(out, ForeignKeyDef{
			Name:       n,
			Columns:    cols,
			RefTable:   g.refTable,
			RefColumns: refCols,
		})
	}
	return out, nil
}

