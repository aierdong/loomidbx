package schema

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
)

// SQLiteSchemaInspector 负责从 SQLite 读取结构元数据并构建统一内存 schema 图。
type SQLiteSchemaInspector struct{}

// NewSQLiteSchemaInspector 创建 SQLite 方言的 SchemaInspector。
func NewSQLiteSchemaInspector() *SQLiteSchemaInspector {
	return &SQLiteSchemaInspector{}
}

// Inspect 执行 SQLite schema 扫描。
func (i *SQLiteSchemaInspector) Inspect(ctx context.Context, db *sql.DB, scope InspectScope) (*SchemaGraph, *UpstreamClassifiedError) {
	tableNames, err := i.listTables(ctx, db, scope)
	if err != nil {
		return nil, ClassifyUpstreamError(ctx, err)
	}

	tables := make([]TableDef, 0, len(tableNames))
	for _, table := range tableNames {
		td, tdErr := i.inspectTable(ctx, db, table)
		if tdErr != nil {
			return nil, ClassifyUpstreamError(ctx, tdErr)
		}
		tables = append(tables, *td)
	}

	// 确保输出确定性顺序
	sort.Slice(tables, func(a, b int) bool {
		return strings.ToLower(tables[a].TableName) < strings.ToLower(tables[b].TableName)
	})

	return &SchemaGraph{Tables: tables}, nil
}

func (i *SQLiteSchemaInspector) listTables(ctx context.Context, db *sql.DB, scope InspectScope) ([]string, error) {
	if scope.Scope == SchemaScanScopeTables {
		// 去重 + 排序（确定性）
		m := make(map[string]struct{}, len(scope.TableNames))
		out := make([]string, 0, len(scope.TableNames))
		for _, t := range scope.TableNames {
			tt := strings.TrimSpace(t)
			if tt == "" {
				continue
			}
			key := strings.ToLower(tt)
			if _, ok := m[key]; ok {
				continue
			}
			m[key] = struct{}{}
			out = append(out, tt)
		}
		sort.Slice(out, func(a, b int) bool { return strings.ToLower(out[a]) < strings.ToLower(out[b]) })
		return out, nil
	}

	rows, err := db.QueryContext(ctx, `
		SELECT name
		FROM sqlite_master
		WHERE type='table' AND name NOT LIKE 'sqlite_%'
		ORDER BY name ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var n string
		if scanErr := rows.Scan(&n); scanErr != nil {
			return nil, scanErr
		}
		names = append(names, n)
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		return nil, rowsErr
	}
	return names, nil
}

func (i *SQLiteSchemaInspector) inspectTable(ctx context.Context, db *sql.DB, table string) (*TableDef, error) {
	cols, pk, err := i.describeColumns(ctx, db, table)
	if err != nil {
		return nil, err
	}

	uniques, err := i.describeUniques(ctx, db, table, cols)
	if err != nil {
		return nil, err
	}

	fks, err := i.describeForeignKeys(ctx, db, table)
	if err != nil {
		return nil, err
	}

	// 统一确定性排序（列名与序号都应稳定）
	sort.Slice(cols, func(a, b int) bool {
		if cols[a].OrdinalPos != cols[b].OrdinalPos {
			if cols[a].OrdinalPos == 0 {
				return false
			}
			if cols[b].OrdinalPos == 0 {
				return true
			}
			return cols[a].OrdinalPos < cols[b].OrdinalPos
		}
		return strings.ToLower(cols[a].Name) < strings.ToLower(cols[b].Name)
	})

	sort.Strings(pk)
	sort.Slice(uniques, func(a, b int) bool {
		la := strings.ToLower(uniques[a].Name)
		lb := strings.ToLower(uniques[b].Name)
		if la != lb {
			return la < lb
		}
		return strings.Join(uniques[a].Columns, ",") < strings.Join(uniques[b].Columns, ",")
	})
	sort.Slice(fks, func(a, b int) bool {
		la := strings.ToLower(fks[a].RefTable) + "|" + strings.Join(fks[a].Columns, ",")
		lb := strings.ToLower(fks[b].RefTable) + "|" + strings.Join(fks[b].Columns, ",")
		return la < lb
	})

	return &TableDef{
		TableName:         table,
		Columns:           cols,
		PrimaryKey:        pk,
		UniqueConstraints: uniques,
		ForeignKeys:       fks,
	}, nil
}

func (i *SQLiteSchemaInspector) describeColumns(ctx context.Context, db *sql.DB, table string) ([]ColumnDef, []string, error) {
	rows, err := db.QueryContext(ctx, fmt.Sprintf("PRAGMA table_info(%s)", quoteSQLiteIdent(table)))
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	type row struct {
		cid        int
		name       string
		typ        string
		notnull    int
		dfltValue  sql.NullString
		pkPosition int
	}

	type pkCol struct {
		name string
		pos  int
	}
	var pkCols []pkCol
	var cols []ColumnDef
	for rows.Next() {
		var r row
		if scanErr := rows.Scan(&r.cid, &r.name, &r.typ, &r.notnull, &r.dfltValue, &r.pkPosition); scanErr != nil {
			return nil, nil, scanErr
		}
		if r.pkPosition > 0 {
			pkCols = append(pkCols, pkCol{name: r.name, pos: r.pkPosition})
		}
		cols = append(cols, ColumnDef{
			Name:            r.name,
			OrdinalPos:      r.cid + 1,
			DataType:        r.typ,
			AbstractType:    ResolveAbstractType(r.typ),
			IsNullable:      r.notnull == 0,
			DefaultValue:    nullStringToString(r.dfltValue),
			IsAutoIncrement: detectSQLiteAutoIncrement(r.typ, r.pkPosition),
		})
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		return nil, nil, rowsErr
	}

	sort.Slice(pkCols, func(a, b int) bool {
		if pkCols[a].pos != pkCols[b].pos {
			return pkCols[a].pos < pkCols[b].pos
		}
		return strings.ToLower(pkCols[a].name) < strings.ToLower(pkCols[b].name)
	})
	outPk := make([]string, 0, len(pkCols))
	for _, c := range pkCols {
		outPk = append(outPk, c.name)
	}
	return cols, outPk, nil
}

func (i *SQLiteSchemaInspector) describeUniques(ctx context.Context, db *sql.DB, table string, cols []ColumnDef) ([]UniqueConstraintDef, error) {
	// 建立列名到 ordinal 的映射，便于按列在表中的顺序稳定排序
	colPos := make(map[string]int, len(cols))
	for _, c := range cols {
		colPos[strings.ToLower(c.Name)] = c.OrdinalPos
	}

	rows, err := db.QueryContext(ctx, fmt.Sprintf("PRAGMA index_list(%s)", quoteSQLiteIdent(table)))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type idxRow struct {
		seq     int
		name    string
		unique  int
		origin  sql.NullString
		partial sql.NullInt64
	}

	var indexes []idxRow
	for rows.Next() {
		var r idxRow
		// pragma index_list: seq, name, unique, origin, partial （版本差异，后两列可能不存在）
		if scanErr := rows.Scan(&r.seq, &r.name, &r.unique, &r.origin, &r.partial); scanErr != nil {
			// 兼容旧 SQLite：只扫描前三列
			var r2 struct {
				seq    int
				name   string
				unique int
			}
			if scanErr2 := rows.Scan(&r2.seq, &r2.name, &r2.unique); scanErr2 != nil {
				return nil, scanErr
			}
			indexes = append(indexes, idxRow{seq: r2.seq, name: r2.name, unique: r2.unique})
			continue
		}
		indexes = append(indexes, r)
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		return nil, rowsErr
	}

	var uniques []UniqueConstraintDef
	for _, idx := range indexes {
		if idx.unique == 0 {
			continue
		}

		cRows, cErr := db.QueryContext(ctx, fmt.Sprintf("PRAGMA index_info(%s)", quoteSQLiteIdent(idx.name)))
		if cErr != nil {
			return nil, cErr
		}

		var colsInIdx []string
		for cRows.Next() {
			var seqno int
			var cid int
			var cname string
			if scanErr := cRows.Scan(&seqno, &cid, &cname); scanErr != nil {
				_ = cRows.Close()
				return nil, scanErr
			}
			if cname != "" {
				colsInIdx = append(colsInIdx, cname)
			}
		}
		_ = cRows.Close()

		sort.Slice(colsInIdx, func(a, b int) bool {
			pa := colPos[strings.ToLower(colsInIdx[a])]
			pb := colPos[strings.ToLower(colsInIdx[b])]
			if pa != pb && pa != 0 && pb != 0 {
				return pa < pb
			}
			return strings.ToLower(colsInIdx[a]) < strings.ToLower(colsInIdx[b])
		})

		uniques = append(uniques, UniqueConstraintDef{
			Name:    idx.name,
			Columns: colsInIdx,
		})
	}

	// 确保确定性
	sort.Slice(uniques, func(a, b int) bool {
		la := strings.ToLower(uniques[a].Name)
		lb := strings.ToLower(uniques[b].Name)
		if la != lb {
			return la < lb
		}
		return strings.Join(uniques[a].Columns, ",") < strings.Join(uniques[b].Columns, ",")
	})

	return uniques, nil
}

func (i *SQLiteSchemaInspector) describeForeignKeys(ctx context.Context, db *sql.DB, table string) ([]ForeignKeyDef, error) {
	rows, err := db.QueryContext(ctx, fmt.Sprintf("PRAGMA foreign_key_list(%s)", quoteSQLiteIdent(table)))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type fkRow struct {
		id       int
		seq      int
		refTable string
		fromCol  string
		toCol    string
		onUpdate sql.NullString
		onDelete sql.NullString
		match    sql.NullString
	}

	// 按 id 聚合复合外键
	type agg struct {
		refTable string
		pairs    []struct {
			seq  int
			from string
			to   string
		}
	}

	m := map[int]*agg{}
	for rows.Next() {
		var r fkRow
		if scanErr := rows.Scan(&r.id, &r.seq, &r.refTable, &r.fromCol, &r.toCol, &r.onUpdate, &r.onDelete, &r.match); scanErr != nil {
			// 兼容旧 SQLite：仅前五列
			var r2 struct {
				id       int
				seq      int
				refTable string
				fromCol  string
				toCol    string
			}
			if scanErr2 := rows.Scan(&r2.id, &r2.seq, &r2.refTable, &r2.fromCol, &r2.toCol); scanErr2 != nil {
				return nil, scanErr
			}
			r.id, r.seq, r.refTable, r.fromCol, r.toCol = r2.id, r2.seq, r2.refTable, r2.fromCol, r2.toCol
		}
		a := m[r.id]
		if a == nil {
			a = &agg{refTable: r.refTable}
			m[r.id] = a
		}
		a.pairs = append(a.pairs, struct {
			seq  int
			from string
			to   string
		}{seq: r.seq, from: r.fromCol, to: r.toCol})
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		return nil, rowsErr
	}

	ids := make([]int, 0, len(m))
	for id := range m {
		ids = append(ids, id)
	}
	sort.Ints(ids)

	out := make([]ForeignKeyDef, 0, len(ids))
	for _, id := range ids {
		a := m[id]
		sort.Slice(a.pairs, func(aIdx, bIdx int) bool { return a.pairs[aIdx].seq < a.pairs[bIdx].seq })
		cols := make([]string, 0, len(a.pairs))
		refCols := make([]string, 0, len(a.pairs))
		for _, p := range a.pairs {
			cols = append(cols, p.from)
			refCols = append(refCols, p.to)
		}
		out = append(out, ForeignKeyDef{
			Columns:    cols,
			RefTable:   a.refTable,
			RefColumns: refCols,
		})
	}

	return out, nil
}

func quoteSQLiteIdent(ident string) string {
	// SQLite 支持使用双引号引用标识符；这里做最小转义。
	return `"` + strings.ReplaceAll(ident, `"`, `""`) + `"`
}

func nullStringToString(ns sql.NullString) string {
	if ns.Valid {
		return ns.String
	}
	return ""
}

func detectSQLiteAutoIncrement(rawType string, pkPosition int) bool {
	// SQLite 的 AUTOINCREMENT 需要解析表 SQL 才能严格判断；
	// 2.1 先按“INTEGER 主键”作为自增候选，为生成器映射做保守提示。
	if pkPosition <= 0 {
		return false
	}
	t := strings.ToLower(strings.TrimSpace(rawType))
	if idx := strings.IndexByte(t, '('); idx >= 0 {
		t = strings.TrimSpace(t[:idx])
	}
	return t == "integer" || t == "int"
}

