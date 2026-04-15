package schema

import (
	"fmt"
	"sort"
	"strings"
)

const (
	// SchemaDiffErrCodeCurrentSchemaNotFound 表示当前持久化 schema 缺失或无法作为 Diff 基线（含损坏、首扫无基线）。
	SchemaDiffErrCodeCurrentSchemaNotFound = "CURRENT_SCHEMA_NOT_FOUND"

	// SchemaDiffErrCodeDiffScopeMismatch 表示单表/按表范围扫描结果与 Compare 期望的表集合不一致，无法做与 PreviewSchemaDiff 一致的安全对比。
	SchemaDiffErrCodeDiffScopeMismatch = "DIFF_SCOPE_MISMATCH"

	// SchemaDiffErrCodeFailedPrecondition 表示缺少内存快照等前置条件（对齐 design.md 的 FAILED_PRECONDITION）。
	SchemaDiffErrCodeFailedPrecondition = "FAILED_PRECONDITION"

	// SchemaDiffErrCodeInvalidArgument 表示 Compare 入参组合不合法（如 scope=all 却携带 table_names）。
	SchemaDiffErrCodeInvalidArgument = "INVALID_ARGUMENT"
)

// SchemaDiffKind 表示表级变化分类（新增/删除/修改）。
type SchemaDiffKind string

const (
	// SchemaDiffKindAdded 表示目标库扫描结果相对当前持久化 schema 新增的表。
	SchemaDiffKindAdded SchemaDiffKind = "added"

	// SchemaDiffKindRemoved 表示当前持久化 schema 中存在但扫描结果中缺失的表。
	SchemaDiffKindRemoved SchemaDiffKind = "removed"

	// SchemaDiffKindModified 表示表仍存在但列或表级约束/索引语义发生变化。
	SchemaDiffKindModified SchemaDiffKind = "modified"
)

// SchemaColumnDiffKind 表示列级变化分类。
type SchemaColumnDiffKind string

const (
	// SchemaColumnDiffKindAdded 表示新增列。
	SchemaColumnDiffKindAdded SchemaColumnDiffKind = "added"

	// SchemaColumnDiffKindRemoved 表示删除列。
	SchemaColumnDiffKindRemoved SchemaColumnDiffKind = "removed"

	// SchemaColumnDiffKindModified 表示列仍存在但属性变化。
	SchemaColumnDiffKindModified SchemaColumnDiffKind = "modified"
)

// SchemaDiffError 为 Diff 引擎返回的结构化错误（稳定 Code，不含敏感信息）。
type SchemaDiffError struct {
	// Code 为稳定错误码（如 CURRENT_SCHEMA_NOT_FOUND）。
	Code string

	// Message 为人类可读说明。
	Message string
}

// Error 实现 error 接口。
func (e *SchemaDiffError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// SchemaColumnSnapshot 为列级属性快照，供列 Diff 与 UI/下游消费。
type SchemaColumnSnapshot struct {
	// Name 为列名。
	Name string

	// OrdinalPos 为列顺序（未知时可为 0）。
	OrdinalPos int

	// DataType 为方言相关原始类型字符串。
	DataType string

	// AbstractType 为统一抽象类型。
	AbstractType string

	// IsNullable 表示是否可空。
	IsNullable bool

	// DefaultValue 为默认值文本（空表示无或未知）。
	DefaultValue string

	// IsAutoIncrement 表示是否自增。
	IsAutoIncrement bool

	// InPrimaryKey 表示是否属于主键列集合。
	InPrimaryKey bool

	// InUniqueConstraint 表示是否出现在任一唯一约束/唯一索引列集合中。
	InUniqueConstraint bool

	// FKRefTable 为外键引用表（空表示无外键）。
	FKRefTable string

	// FKRefColumn 为外键引用列（空表示无外键或未解析）。
	FKRefColumn string
}

// SchemaColumnDiff 描述单列的新增/删除/修改及修改维度。
type SchemaColumnDiff struct {
	// ColumnName 为列名。
	ColumnName string

	// Kind 为列级分类。
	Kind SchemaColumnDiffKind

	// AttributeChanges 在 Kind=modified 时列出变化维度（如 data_type、primary_key）。
	AttributeChanges []string

	// Old 为持久化侧列快照；新增列时为空。
	Old *SchemaColumnSnapshot

	// New 为内存扫描侧列快照；删除列时为空。
	New *SchemaColumnSnapshot
}

// SchemaTableDiff 描述单张表的新增/删除/修改。
type SchemaTableDiff struct {
	// DatabaseName 为数据库名。
	DatabaseName string

	// SchemaName 为逻辑 schema 名（可为空）。
	SchemaName string

	// TableName 为表名。
	TableName string

	// Kind 为表级分类。
	Kind SchemaDiffKind

	// ColumnDiffs 为列级变化列表（确定性排序）。
	ColumnDiffs []SchemaColumnDiff

	// TableLevelChanges 描述无法仅归属单列的变化（如复合唯一/外键集合变化、主键列序变化）。
	TableLevelChanges []string
}

// SchemaDiffSummary 为变更计数摘要。
type SchemaDiffSummary struct {
	// AddedTables 为新增表数量。
	AddedTables int

	// RemovedTables 为删除表数量。
	RemovedTables int

	// ModifiedTables 为修改表数量。
	ModifiedTables int

	// AddedColumns 为新增列数量（跨表累计）。
	AddedColumns int

	// RemovedColumns 为删除列数量（跨表累计）。
	RemovedColumns int

	// ModifiedColumns 为修改列数量（跨表累计）。
	ModifiedColumns int
}

// SchemaDiffResult 为 Compare 成功时的输出。
type SchemaDiffResult struct {
	// TableDiffs 为表级变化列表（确定性排序）。
	TableDiffs []SchemaTableDiff

	// Summary 为汇总统计。
	Summary SchemaDiffSummary
}

// SchemaDiffCompareOptions 描述 Compare 的范围语义（需与扫描任务 scope 一致）。
type SchemaDiffCompareOptions struct {
	// Scope 为 all 或 table。
	Scope SchemaScanScope

	// TableNames 在 Scope=table 时必须非空（经 StartSchemaScan 校验的表名列表）。
	TableNames []string
}

// SchemaDiffEngine 对比当前持久化 schema 与内存扫描快照（领域服务，无外部 I/O）。
type SchemaDiffEngine struct{}

// NewSchemaDiffEngine 构造 Diff 引擎实例。
func NewSchemaDiffEngine() *SchemaDiffEngine {
	return &SchemaDiffEngine{}
}

// Compare 将 current 持久化 bundle 与 scanned 内存图对比，输出三级分类与列级详情。
//
// 输入参数：current 为当前 schema；scanned 为扫描得到的内存快照；opts 为范围选项。
// 返回值：成功返回 Diff 结果；失败返回 SchemaDiffError（首扫/损坏/范围不兼容不返回半成品 Diff）。
func (e *SchemaDiffEngine) Compare(current *CurrentSchemaBundle, scanned *SchemaGraph, opts SchemaDiffCompareOptions) (*SchemaDiffResult, *SchemaDiffError) {
	_ = e
	if scanned == nil {
		return nil, &SchemaDiffError{Code: SchemaDiffErrCodeFailedPrecondition, Message: "in-memory schema snapshot is required"}
	}
	if err := validateDiffCompareOptions(opts); err != nil {
		return nil, err
	}
	if current == nil || len(current.Tables) == 0 {
		return nil, &SchemaDiffError{Code: SchemaDiffErrCodeCurrentSchemaNotFound, Message: "current persisted schema is missing"}
	}
	if err := validateCurrentSchemaBundleIntegrity(current); err != nil {
		return nil, &SchemaDiffError{Code: SchemaDiffErrCodeCurrentSchemaNotFound, Message: "current persisted schema is invalid or corrupted"}
	}
	if opts.Scope == SchemaScanScopeTables {
		want := dedupeAndSort(opts.TableNames)
		got := tableNamesFromGraph(scanned)
		if !stringSliceEqual(want, got) {
			return nil, &SchemaDiffError{Code: SchemaDiffErrCodeDiffScopeMismatch, Message: "scanned table set does not match compare scope"}
		}
	}

	curIdx, err := indexCurrentBundle(current)
	if err != nil {
		return nil, &SchemaDiffError{Code: SchemaDiffErrCodeCurrentSchemaNotFound, Message: "current persisted schema is invalid or corrupted"}
	}
	memIdx := indexScannedGraph(scanned)

	var result SchemaDiffResult
	switch opts.Scope {
	case SchemaScanScopeAll:
		result = compareAllScope(curIdx, memIdx)
	case SchemaScanScopeTables:
		result = compareTableScope(curIdx, memIdx, dedupeAndSort(opts.TableNames))
	default:
		return nil, &SchemaDiffError{Code: SchemaDiffErrCodeInvalidArgument, Message: "invalid scope"}
	}
	return &result, nil
}

func validateDiffCompareOptions(opts SchemaDiffCompareOptions) *SchemaDiffError {
	switch opts.Scope {
	case SchemaScanScopeAll:
		if len(dedupeAndSort(opts.TableNames)) > 0 {
			return &SchemaDiffError{Code: SchemaDiffErrCodeInvalidArgument, Message: "table_names must be empty when scope=all"}
		}
	case SchemaScanScopeTables:
		if len(dedupeAndSort(opts.TableNames)) == 0 {
			return &SchemaDiffError{Code: SchemaDiffErrCodeInvalidArgument, Message: "table_names required when scope=table"}
		}
	default:
		return &SchemaDiffError{Code: SchemaDiffErrCodeInvalidArgument, Message: "invalid scope"}
	}
	return nil
}

func validateCurrentSchemaBundleIntegrity(bundle *CurrentSchemaBundle) error {
	ids := make(map[string]struct{}, len(bundle.Tables))
	for _, t := range bundle.Tables {
		if t.ID == "" {
			return fmt.Errorf("empty table id")
		}
		ids[t.ID] = struct{}{}
	}
	for _, c := range bundle.Columns {
		if _, ok := ids[c.TableSchemaID]; !ok {
			return fmt.Errorf("orphan column")
		}
	}
	return nil
}

func indexCurrentBundle(bundle *CurrentSchemaBundle) (map[string]*indexedTable, error) {
	out := make(map[string]*indexedTable, len(bundle.Tables))
	for _, t := range bundle.Tables {
		k := tableKey(t.DatabaseName, t.SchemaName, t.TableName)
		if _, dup := out[k]; dup {
			return nil, fmt.Errorf("duplicate table key")
		}
		out[k] = &indexedTable{
			key:     k,
			table:   t,
			columns: make(map[string]ColumnSchemaPersisted),
		}
	}
	for _, c := range bundle.Columns {
		k := findTableKeyForID(out, c.TableSchemaID)
		if k == "" {
			return nil, fmt.Errorf("unknown table for column")
		}
		out[k].columns[strings.ToLower(strings.TrimSpace(c.ColumnName))] = c
	}
	return out, nil
}

func findTableKeyForID(idx map[string]*indexedTable, tableID string) string {
	for k, v := range idx {
		if v.table.ID == tableID {
			return k
		}
	}
	return ""
}

type indexedTable struct {
	key     string
	table   TableSchemaPersisted
	columns map[string]ColumnSchemaPersisted
}

func indexScannedGraph(g *SchemaGraph) map[string]*TableDef {
	out := make(map[string]*TableDef, len(g.Tables))
	for i := range g.Tables {
		t := &g.Tables[i]
		k := tableKey(t.DatabaseName, t.SchemaName, t.TableName)
		out[k] = t
	}
	return out
}

func tableKey(db, schema, tbl string) string {
	return strings.ToLower(strings.TrimSpace(db)) + "\x00" + strings.ToLower(strings.TrimSpace(schema)) + "\x00" + strings.ToLower(strings.TrimSpace(tbl))
}

func tableNamesFromGraph(g *SchemaGraph) []string {
	names := make([]string, 0, len(g.Tables))
	for i := range g.Tables {
		names = append(names, g.Tables[i].TableName)
	}
	return dedupeAndSort(names)
}

func stringSliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func compareAllScope(cur map[string]*indexedTable, mem map[string]*TableDef) SchemaDiffResult {
	keys := make(map[string]struct{})
	for k := range cur {
		keys[k] = struct{}{}
	}
	for k := range mem {
		keys[k] = struct{}{}
	}
	sorted := make([]string, 0, len(keys))
	for k := range keys {
		sorted = append(sorted, k)
	}
	sort.Strings(sorted)

	var out SchemaDiffResult
	for _, k := range sorted {
		ct, okCur := cur[k]
		mt, okMem := mem[k]
		switch {
		case okCur && !okMem:
			td := buildRemovedTableDiff(ct)
			out.TableDiffs = append(out.TableDiffs, td)
			out.Summary.RemovedTables++
			out.Summary.RemovedColumns += len(td.ColumnDiffs)
		case !okCur && okMem:
			td := buildAddedTableDiff(mt)
			out.TableDiffs = append(out.TableDiffs, td)
			out.Summary.AddedTables++
			out.Summary.AddedColumns += len(td.ColumnDiffs)
		default:
			td := buildModifiedTableDiff(ct, mt)
			if len(td.ColumnDiffs) == 0 && len(td.TableLevelChanges) == 0 {
				continue
			}
			out.TableDiffs = append(out.TableDiffs, td)
			out.Summary.ModifiedTables++
			for _, cd := range td.ColumnDiffs {
				switch cd.Kind {
				case SchemaColumnDiffKindAdded:
					out.Summary.AddedColumns++
				case SchemaColumnDiffKindRemoved:
					out.Summary.RemovedColumns++
				case SchemaColumnDiffKindModified:
					out.Summary.ModifiedColumns++
				}
			}
		}
	}
	return out
}

func compareTableScope(cur map[string]*indexedTable, mem map[string]*TableDef, wantSorted []string) SchemaDiffResult {
	var out SchemaDiffResult
	for _, w := range wantSorted {
		k := lookupTableKeyForScope(cur, mem, w)
		if k == "" {
			continue
		}
		ct, okCur := cur[k]
		mt, okMem := mem[k]
		switch {
		case okCur && !okMem:
			td := buildRemovedTableDiff(ct)
			out.TableDiffs = append(out.TableDiffs, td)
			out.Summary.RemovedTables++
			out.Summary.RemovedColumns += len(td.ColumnDiffs)
		case !okCur && okMem:
			td := buildAddedTableDiff(mt)
			out.TableDiffs = append(out.TableDiffs, td)
			out.Summary.AddedTables++
			out.Summary.AddedColumns += len(td.ColumnDiffs)
		default:
			td := buildModifiedTableDiff(ct, mt)
			if len(td.ColumnDiffs) == 0 && len(td.TableLevelChanges) == 0 {
				continue
			}
			out.TableDiffs = append(out.TableDiffs, td)
			out.Summary.ModifiedTables++
			for _, cd := range td.ColumnDiffs {
				switch cd.Kind {
				case SchemaColumnDiffKindAdded:
					out.Summary.AddedColumns++
				case SchemaColumnDiffKindRemoved:
					out.Summary.RemovedColumns++
				case SchemaColumnDiffKindModified:
					out.Summary.ModifiedColumns++
				}
			}
		}
	}
	return out
}

// lookupTableKeyForScope 在按表名扫描场景下解析表键（优先内存快照侧，其次当前持久化侧）。
func lookupTableKeyForScope(cur map[string]*indexedTable, mem map[string]*TableDef, tableName string) string {
	for k, td := range mem {
		if strings.EqualFold(strings.TrimSpace(td.TableName), strings.TrimSpace(tableName)) {
			return k
		}
	}
	for k, it := range cur {
		if strings.EqualFold(strings.TrimSpace(it.table.TableName), strings.TrimSpace(tableName)) {
			return k
		}
	}
	return ""
}

func buildAddedTableDiff(t *TableDef) SchemaTableDiff {
	var cds []SchemaColumnDiff
	for _, col := range t.Columns {
		snap := columnSnapshotFromMemory(t, col)
		cds = append(cds, SchemaColumnDiff{
			ColumnName: col.Name,
			Kind:       SchemaColumnDiffKindAdded,
			New:        snap,
		})
	}
	sort.Slice(cds, func(i, j int) bool { return strings.ToLower(cds[i].ColumnName) < strings.ToLower(cds[j].ColumnName) })
	return SchemaTableDiff{
		DatabaseName: t.DatabaseName,
		SchemaName:   t.SchemaName,
		TableName:    t.TableName,
		Kind:         SchemaDiffKindAdded,
		ColumnDiffs:  cds,
	}
}

func buildRemovedTableDiff(t *indexedTable) SchemaTableDiff {
	var cds []SchemaColumnDiff
	pk := pkSetFromPersisted(t)
	for _, c := range t.columns {
		snap := columnSnapshotFromPersisted(c, pk)
		cds = append(cds, SchemaColumnDiff{
			ColumnName: c.ColumnName,
			Kind:       SchemaColumnDiffKindRemoved,
			Old:        snap,
		})
	}
	sort.Slice(cds, func(i, j int) bool { return strings.ToLower(cds[i].ColumnName) < strings.ToLower(cds[j].ColumnName) })
	return SchemaTableDiff{
		DatabaseName: t.table.DatabaseName,
		SchemaName:   t.table.SchemaName,
		TableName:    t.table.TableName,
		Kind:         SchemaDiffKindRemoved,
		ColumnDiffs:  cds,
	}
}

func buildModifiedTableDiff(cur *indexedTable, mem *TableDef) SchemaTableDiff {
	pkP := pkSetFromPersisted(cur)
	memPK := pkSetFromMemory(mem)

	colNames := make(map[string]struct{})
	for n := range cur.columns {
		colNames[n] = struct{}{}
	}
	for _, c := range mem.Columns {
		colNames[strings.ToLower(strings.TrimSpace(c.Name))] = struct{}{}
	}
	var names []string
	for n := range colNames {
		names = append(names, n)
	}
	sort.Strings(names)

	var cds []SchemaColumnDiff
	for _, ln := range names {
		pc, okP := cur.columns[ln]
		var mc *ColumnDef
		for i := range mem.Columns {
			if strings.EqualFold(mem.Columns[i].Name, ln) {
				mc = &mem.Columns[i]
				break
			}
		}
		switch {
		case okP && mc == nil:
			snap := columnSnapshotFromPersisted(pc, pkP)
			cds = append(cds, SchemaColumnDiff{ColumnName: pc.ColumnName, Kind: SchemaColumnDiffKindRemoved, Old: snap})
		case !okP && mc != nil:
			snap := columnSnapshotFromMemory(mem, *mc)
			cds = append(cds, SchemaColumnDiff{ColumnName: mc.Name, Kind: SchemaColumnDiffKindAdded, New: snap})
		default:
			oldS := columnSnapshotFromPersisted(pc, pkP)
			newS := columnSnapshotFromMemory(mem, *mc)
			attrs := diffColumnAttributes(oldS, newS)
			if len(attrs) > 0 {
				cds = append(cds, SchemaColumnDiff{
					ColumnName:       pc.ColumnName,
					Kind:             SchemaColumnDiffKindModified,
					AttributeChanges: attrs,
					Old:              oldS,
					New:              newS,
				})
			}
		}
	}

	tl := diffTableLevel(cur, mem, pkP, memPK)
	sort.Strings(tl)

	sort.Slice(cds, func(i, j int) bool { return strings.ToLower(cds[i].ColumnName) < strings.ToLower(cds[j].ColumnName) })

	return SchemaTableDiff{
		DatabaseName:      mem.DatabaseName,
		SchemaName:        mem.SchemaName,
		TableName:         mem.TableName,
		Kind:              SchemaDiffKindModified,
		ColumnDiffs:       cds,
		TableLevelChanges: tl,
	}
}

func pkSetFromPersisted(t *indexedTable) map[string]struct{} {
	out := make(map[string]struct{})
	for ln, c := range t.columns {
		if c.IsPrimaryKey {
			out[ln] = struct{}{}
		}
	}
	return out
}

func pkSetFromMemory(t *TableDef) map[string]struct{} {
	out := make(map[string]struct{})
	for _, p := range t.PrimaryKey {
		out[strings.ToLower(strings.TrimSpace(p))] = struct{}{}
	}
	return out
}

func columnSnapshotFromPersisted(c ColumnSchemaPersisted, pk map[string]struct{}) *SchemaColumnSnapshot {
	ln := strings.ToLower(strings.TrimSpace(c.ColumnName))
	_, inPK := pk[ln]
	return &SchemaColumnSnapshot{
		Name:               c.ColumnName,
		OrdinalPos:         c.OrdinalPos,
		DataType:           c.DataType,
		AbstractType:       c.AbstractType,
		IsNullable:         c.IsNullable,
		DefaultValue:       strings.TrimSpace(c.DefaultValue),
		IsAutoIncrement:    c.IsAutoIncrement,
		InPrimaryKey:       inPK,
		InUniqueConstraint: c.IsUnique,
		FKRefTable:         strings.TrimSpace(c.FKRefTable),
		FKRefColumn:        strings.TrimSpace(c.FKRefColumn),
	}
}

func columnSnapshotFromMemory(t *TableDef, col ColumnDef) *SchemaColumnSnapshot {
	pk := pkSetFromMemory(t)
	ln := strings.ToLower(strings.TrimSpace(col.Name))
	_, inPK := pk[ln]
	inU := columnInUniqueConstraints(t, col.Name)
	rt, rc := fkRefForColumn(t, col.Name)
	return &SchemaColumnSnapshot{
		Name:               col.Name,
		OrdinalPos:         col.OrdinalPos,
		DataType:           col.DataType,
		AbstractType:       col.AbstractType,
		IsNullable:         col.IsNullable,
		DefaultValue:       strings.TrimSpace(col.DefaultValue),
		IsAutoIncrement:    col.IsAutoIncrement,
		InPrimaryKey:       inPK,
		InUniqueConstraint: inU,
		FKRefTable:         rt,
		FKRefColumn:        rc,
	}
}

func columnInUniqueConstraints(t *TableDef, colName string) bool {
	for _, u := range t.UniqueConstraints {
		for _, c := range u.Columns {
			if strings.EqualFold(c, colName) {
				return true
			}
		}
	}
	return false
}

func fkRefForColumn(t *TableDef, colName string) (string, string) {
	for _, fk := range t.ForeignKeys {
		for i, c := range fk.Columns {
			if strings.EqualFold(c, colName) && i < len(fk.RefColumns) {
				return strings.TrimSpace(fk.RefTable), strings.TrimSpace(fk.RefColumns[i])
			}
		}
	}
	return "", ""
}

func diffColumnAttributes(a, b *SchemaColumnSnapshot) []string {
	if a == nil || b == nil {
		return nil
	}
	var attrs []string
	if !strings.EqualFold(strings.TrimSpace(a.DataType), strings.TrimSpace(b.DataType)) {
		attrs = append(attrs, "data_type")
	}
	if !strings.EqualFold(strings.TrimSpace(a.AbstractType), strings.TrimSpace(b.AbstractType)) {
		attrs = append(attrs, "abstract_type")
	}
	if a.IsNullable != b.IsNullable {
		attrs = append(attrs, "is_nullable")
	}
	if a.DefaultValue != b.DefaultValue {
		attrs = append(attrs, "default_value")
	}
	if a.IsAutoIncrement != b.IsAutoIncrement {
		attrs = append(attrs, "auto_increment")
	}
	if a.InPrimaryKey != b.InPrimaryKey {
		attrs = append(attrs, "primary_key")
	}
	if a.InUniqueConstraint != b.InUniqueConstraint {
		attrs = append(attrs, "unique_constraint")
	}
	if !strings.EqualFold(a.FKRefTable, b.FKRefTable) || !strings.EqualFold(a.FKRefColumn, b.FKRefColumn) {
		attrs = append(attrs, "foreign_key")
	}
	sort.Strings(attrs)
	return attrs
}

func diffTableLevel(cur *indexedTable, mem *TableDef, pkP, pkM map[string]struct{}) []string {
	var out []string
	pkOrderP := sortedPKNamesFromPersisted(cur)
	pkOrderM := append([]string(nil), mem.PrimaryKey...)
	if !pkSetEqual(pkP, pkM) {
		out = append(out, "primary_key")
	} else if !stringsEqualSlice(pkOrderP, pkOrderM) {
		out = append(out, "primary_key_order")
	}

	if !uniqueConstraintSetEqual(cur, mem) {
		out = append(out, "unique_index")
	}
	if !foreignKeySetEqual(cur, mem) {
		out = append(out, "foreign_key_constraint")
	}
	return out
}

func sortedPKNamesFromPersisted(cur *indexedTable) []string {
	var cols []ColumnSchemaPersisted
	for _, c := range cur.columns {
		if c.IsPrimaryKey {
			cols = append(cols, c)
		}
	}
	sort.Slice(cols, func(i, j int) bool {
		if cols[i].OrdinalPos != cols[j].OrdinalPos {
			return cols[i].OrdinalPos < cols[j].OrdinalPos
		}
		return strings.ToLower(cols[i].ColumnName) < strings.ToLower(cols[j].ColumnName)
	})
	out := make([]string, len(cols))
	for i := range cols {
		out[i] = strings.TrimSpace(cols[i].ColumnName)
	}
	return out
}

func pkSetEqual(a, b map[string]struct{}) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if _, ok := b[k]; !ok {
			return false
		}
	}
	return true
}

func stringsEqualSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !strings.EqualFold(strings.TrimSpace(a[i]), strings.TrimSpace(b[i])) {
			return false
		}
	}
	return true
}

func uniqueConstraintSetEqual(cur *indexedTable, mem *TableDef) bool {
	a := normalizeUniqueFromPersisted(cur)
	b := normalizeUniqueFromMemory(mem)
	return stringSetEqual(a, b)
}

func normalizeUniqueFromPersisted(cur *indexedTable) map[string]struct{} {
	out := make(map[string]struct{})
	for _, c := range cur.columns {
		if !c.IsUnique {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(c.ColumnName))
		out[key] = struct{}{}
	}
	return out
}

func normalizeUniqueFromMemory(mem *TableDef) map[string]struct{} {
	out := make(map[string]struct{})
	for _, u := range mem.UniqueConstraints {
		parts := append([]string{}, u.Columns...)
		for i := range parts {
			parts[i] = strings.ToLower(strings.TrimSpace(parts[i]))
		}
		sort.Strings(parts)
		out[strings.Join(parts, ",")] = struct{}{}
	}
	return out
}

func stringSetEqual(a, b map[string]struct{}) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if _, ok := b[k]; !ok {
			return false
		}
	}
	return true
}

func foreignKeySetEqual(cur *indexedTable, mem *TableDef) bool {
	a := normalizeFKFromPersisted(cur)
	b := normalizeFKFromMemory(mem)
	return stringSetEqual(a, b)
}

func normalizeFKFromPersisted(cur *indexedTable) map[string]struct{} {
	out := make(map[string]struct{})
	for _, c := range cur.columns {
		if strings.TrimSpace(c.FKRefTable) == "" {
			continue
		}
		key := strings.Join([]string{
			strings.ToLower(strings.TrimSpace(c.ColumnName)),
			strings.ToLower(strings.TrimSpace(c.FKRefTable)),
			strings.ToLower(strings.TrimSpace(c.FKRefColumn)),
		}, "\x00")
		out[key] = struct{}{}
	}
	return out
}

func normalizeFKFromMemory(mem *TableDef) map[string]struct{} {
	out := make(map[string]struct{})
	for _, fk := range mem.ForeignKeys {
		cols := append([]string{}, fk.Columns...)
		rc := append([]string{}, fk.RefColumns...)
		for i := range cols {
			cols[i] = strings.ToLower(strings.TrimSpace(cols[i]))
		}
		for i := range rc {
			rc[i] = strings.ToLower(strings.TrimSpace(rc[i]))
		}
		key := strings.Join(cols, ",") + "=>" + strings.ToLower(strings.TrimSpace(fk.RefTable)) + "(" + strings.Join(rc, ",") + ")"
		out[key] = struct{}{}
	}
	return out
}
