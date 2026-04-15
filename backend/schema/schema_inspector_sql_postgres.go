package schema

import (
	"context"
	"database/sql"
	"sort"
	"strings"
)

func inspectColumnsPostgres(ctx context.Context, db *sql.DB, schemaName, tableName string) ([]ColumnDef, []string, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT
			c.column_name,
			c.ordinal_position,
			c.udt_name,
			c.is_nullable,
			c.column_default,
			COALESCE(
				EXISTS (
					SELECT 1
					FROM information_schema.key_column_usage k
					JOIN information_schema.table_constraints tc
					  ON tc.constraint_name = k.constraint_name
					 AND tc.table_schema = k.table_schema
					 AND tc.table_name = k.table_name
					WHERE tc.constraint_type = 'PRIMARY KEY'
					  AND k.table_schema = c.table_schema
					  AND k.table_name = c.table_name
					  AND k.column_name = c.column_name
				), false
			) AS is_pk
		FROM information_schema.columns c
		WHERE c.table_schema = $1 AND c.table_name = $2
		ORDER BY c.ordinal_position ASC
	`, schemaName, tableName)
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
			udtName  string
			nullable string
			defVal   sql.NullString
			isPK     bool
		)
		if scanErr := rows.Scan(&name, &pos, &udtName, &nullable, &defVal, &isPK); scanErr != nil {
			return nil, nil, scanErr
		}
		if isPK {
			pk = append(pk, name)
		}
		cols = append(cols, ColumnDef{
			Name:            name,
			OrdinalPos:      pos,
			DataType:        udtName,
			AbstractType:    ResolveAbstractType(udtName),
			IsNullable:      strings.EqualFold(nullable, "YES"),
			DefaultValue:    nullStringToString(defVal),
			IsAutoIncrement: strings.Contains(strings.ToLower(defVal.String), "nextval"),
		})
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		return nil, nil, rowsErr
	}
	sort.Slice(pk, func(a, b int) bool { return strings.ToLower(pk[a]) < strings.ToLower(pk[b]) })
	return cols, pk, nil
}

func inspectUniquePostgres(ctx context.Context, db *sql.DB, schemaName, tableName string) ([]UniqueConstraintDef, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT
			tc.constraint_name,
			kcu.column_name,
			kcu.ordinal_position
		FROM information_schema.table_constraints tc
		JOIN information_schema.key_column_usage kcu
		  ON tc.constraint_name = kcu.constraint_name
		 AND tc.table_schema = kcu.table_schema
		 AND tc.table_name = kcu.table_name
		WHERE tc.constraint_type = 'UNIQUE'
		  AND tc.table_schema = $1
		  AND tc.table_name = $2
		ORDER BY tc.constraint_name ASC, kcu.ordinal_position ASC
	`, schemaName, tableName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type item struct {
		seq int
		col string
	}
	m := map[string][]item{}
	var names []string
	seen := map[string]struct{}{}
	for rows.Next() {
		var name, col string
		var seq int
		if scanErr := rows.Scan(&name, &col, &seq); scanErr != nil {
			return nil, scanErr
		}
		m[name] = append(m[name], item{seq: seq, col: col})
		if _, ok := seen[name]; !ok {
			seen[name] = struct{}{}
			names = append(names, name)
		}
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		return nil, rowsErr
	}

	sort.Slice(names, func(a, b int) bool { return strings.ToLower(names[a]) < strings.ToLower(names[b]) })
	out := make([]UniqueConstraintDef, 0, len(names))
	for _, n := range names {
		items := m[n]
		sort.Slice(items, func(a, b int) bool { return items[a].seq < items[b].seq })
		cols := make([]string, 0, len(items))
		for _, it := range items {
			cols = append(cols, it.col)
		}
		out = append(out, UniqueConstraintDef{Name: n, Columns: cols})
	}
	return out, nil
}

func inspectForeignKeysPostgres(ctx context.Context, db *sql.DB, schemaName, tableName string) ([]ForeignKeyDef, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT
			tc.constraint_name,
			kcu.column_name,
			ccu.table_name AS ref_table_name,
			ccu.column_name AS ref_column_name,
			kcu.ordinal_position
		FROM information_schema.table_constraints tc
		JOIN information_schema.key_column_usage kcu
		  ON tc.constraint_name = kcu.constraint_name
		 AND tc.table_schema = kcu.table_schema
		 AND tc.table_name = kcu.table_name
		JOIN information_schema.constraint_column_usage ccu
		  ON tc.constraint_name = ccu.constraint_name
		 AND tc.table_schema = ccu.table_schema
		WHERE tc.constraint_type = 'FOREIGN KEY'
		  AND tc.table_schema = $1
		  AND tc.table_name = $2
		ORDER BY tc.constraint_name ASC, kcu.ordinal_position ASC
	`, schemaName, tableName)
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
		var name, col, refTable, refCol string
		var seq int
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

