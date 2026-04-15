package schema

import (
	"testing"
)

func TestSchemaDiffEngine_Compare_firstScan_returnsCurrentSchemaNotFound(t *testing.T) {
	eng := NewSchemaDiffEngine()
	scanned := &SchemaGraph{
		Tables: []TableDef{
			{DatabaseName: "db1", SchemaName: "", TableName: "t1", Columns: []ColumnDef{{Name: "id", DataType: "INT", AbstractType: "int"}}},
		},
	}
	_, err := eng.Compare(&CurrentSchemaBundle{}, scanned, SchemaDiffCompareOptions{Scope: SchemaScanScopeAll})
	if err == nil || err.Code != SchemaDiffErrCodeCurrentSchemaNotFound {
		t.Fatalf("got %v want %s", err, SchemaDiffErrCodeCurrentSchemaNotFound)
	}
}

func TestSchemaDiffEngine_Compare_corruptBundle_returnsCurrentSchemaNotFound(t *testing.T) {
	eng := NewSchemaDiffEngine()
	bundle := &CurrentSchemaBundle{
		Tables: []TableSchemaPersisted{{ID: "tbl1", ConnectionID: "c1", DatabaseName: "db1", TableName: "t1"}},
		Columns: []ColumnSchemaPersisted{{
			ID: "col1", TableSchemaID: "missing", ColumnName: "id", DataType: "INT", AbstractType: "int",
		}},
	}
	scanned := &SchemaGraph{Tables: []TableDef{{DatabaseName: "db1", TableName: "t1"}}}
	_, err := eng.Compare(bundle, scanned, SchemaDiffCompareOptions{Scope: SchemaScanScopeAll})
	if err == nil || err.Code != SchemaDiffErrCodeCurrentSchemaNotFound {
		t.Fatalf("got %v want %s", err, SchemaDiffErrCodeCurrentSchemaNotFound)
	}
}

func TestSchemaDiffEngine_Compare_tableScope_scannedSetMismatch_returnsDiffScopeMismatch(t *testing.T) {
	eng := NewSchemaDiffEngine()
	cur := &CurrentSchemaBundle{
		Tables: []TableSchemaPersisted{{ID: "tbl1", ConnectionID: "c1", DatabaseName: "db1", TableName: "a"}},
		Columns: []ColumnSchemaPersisted{{
			ID: "c1", TableSchemaID: "tbl1", ColumnName: "id", DataType: "INT", AbstractType: "int",
		}},
	}
	scanned := &SchemaGraph{
		Tables: []TableDef{
			{DatabaseName: "db1", TableName: "a"},
			{DatabaseName: "db1", TableName: "b"},
		},
	}
	_, err := eng.Compare(cur, scanned, SchemaDiffCompareOptions{Scope: SchemaScanScopeTables, TableNames: []string{"a"}})
	if err == nil || err.Code != SchemaDiffErrCodeDiffScopeMismatch {
		t.Fatalf("got %v want %s", err, SchemaDiffErrCodeDiffScopeMismatch)
	}
}

func TestSchemaDiffEngine_Compare_happyPath_classifiesAndColumnDetails(t *testing.T) {
	eng := NewSchemaDiffEngine()
	cur := &CurrentSchemaBundle{
		Tables: []TableSchemaPersisted{
			{ID: "ta", ConnectionID: "c1", DatabaseName: "db1", TableName: "stay"},
			{ID: "tr", ConnectionID: "c1", DatabaseName: "db1", TableName: "remove_me"},
			{ID: "tm", ConnectionID: "c1", DatabaseName: "db1", TableName: "modify_me"},
		},
		Columns: []ColumnSchemaPersisted{
			{ID: "c1", TableSchemaID: "ta", ColumnName: "id", DataType: "INT", AbstractType: "int", IsPrimaryKey: true},
			{ID: "c2", TableSchemaID: "tr", ColumnName: "x", DataType: "TEXT", AbstractType: "string"},
			{ID: "c3", TableSchemaID: "tm", ColumnName: "n", DataType: "INT", AbstractType: "int", IsNullable: true, DefaultValue: "0"},
		},
	}
	scanned := &SchemaGraph{
		Tables: []TableDef{
			{
				DatabaseName: "db1", TableName: "stay",
				Columns:      []ColumnDef{{Name: "id", DataType: "INT", AbstractType: "int"}},
				PrimaryKey:   []string{"id"},
			},
			{
				DatabaseName: "db1", TableName: "new_tbl",
				Columns:      []ColumnDef{{Name: "k", DataType: "BIGINT", AbstractType: "int"}},
				PrimaryKey:   []string{"k"},
			},
			{
				DatabaseName: "db1", TableName: "modify_me",
				Columns: []ColumnDef{
					{Name: "n", DataType: "BIGINT", AbstractType: "int", IsNullable: false, DefaultValue: "1"},
				},
				PrimaryKey: []string{"n"},
			},
		},
	}
	res, err := eng.Compare(cur, scanned, SchemaDiffCompareOptions{Scope: SchemaScanScopeAll})
	if err != nil {
		t.Fatal(err)
	}
	if res.Summary.AddedTables != 1 || res.Summary.RemovedTables != 1 || res.Summary.ModifiedTables != 1 {
		t.Fatalf("summary: %+v", res.Summary)
	}
	var sawNew, sawRemoved, sawMod bool
	for _, td := range res.TableDiffs {
		switch td.TableName {
		case "new_tbl":
			sawNew = td.Kind == SchemaDiffKindAdded
		case "remove_me":
			sawRemoved = td.Kind == SchemaDiffKindRemoved
		case "modify_me":
			sawMod = td.Kind == SchemaDiffKindModified
		}
	}
	if !sawNew || !sawRemoved || !sawMod {
		t.Fatalf("classification flags: new=%v removed=%v mod=%v", sawNew, sawRemoved, sawMod)
	}
	var colMod *SchemaColumnDiff
	for _, td := range res.TableDiffs {
		if td.TableName != "modify_me" {
			continue
		}
		for i := range td.ColumnDiffs {
			if td.ColumnDiffs[i].ColumnName == "n" {
				colMod = &td.ColumnDiffs[i]
				break
			}
		}
	}
	if colMod == nil || colMod.Kind != SchemaColumnDiffKindModified {
		t.Fatalf("column modify: %+v", colMod)
	}
	wantAttrs := map[string]struct{}{
		"data_type": {}, "is_nullable": {}, "default_value": {}, "primary_key": {},
	}
	for _, a := range colMod.AttributeChanges {
		delete(wantAttrs, a)
	}
	if len(wantAttrs) != 0 {
		t.Fatalf("missing attrs, remainder=%v changes=%v", wantAttrs, colMod.AttributeChanges)
	}
}

func TestSchemaDiffEngine_Compare_tableScope_ignoresOutOfScopeCurrentTables(t *testing.T) {
	eng := NewSchemaDiffEngine()
	cur := &CurrentSchemaBundle{
		Tables: []TableSchemaPersisted{
			{ID: "t1", ConnectionID: "c1", DatabaseName: "db1", TableName: "in_scope"},
			{ID: "t2", ConnectionID: "c1", DatabaseName: "db1", TableName: "other"},
		},
		Columns: []ColumnSchemaPersisted{
			{ID: "c1", TableSchemaID: "t1", ColumnName: "a", DataType: "INT", AbstractType: "int"},
			{ID: "c2", TableSchemaID: "t2", ColumnName: "b", DataType: "INT", AbstractType: "int"},
		},
	}
	scanned := &SchemaGraph{
		Tables: []TableDef{
			{DatabaseName: "db1", TableName: "in_scope", Columns: []ColumnDef{{Name: "a", DataType: "INT", AbstractType: "int"}}},
		},
	}
	res, derr := eng.Compare(cur, scanned, SchemaDiffCompareOptions{Scope: SchemaScanScopeTables, TableNames: []string{"in_scope"}})
	if derr != nil {
		t.Fatal(derr)
	}
	for _, td := range res.TableDiffs {
		if td.TableName == "other" {
			t.Fatalf("out-of-scope table leaked into diff: %+v", td)
		}
	}
}

func TestSchemaDiffEngine_Compare_nilScanned_returnsFailedPrecondition(t *testing.T) {
	eng := NewSchemaDiffEngine()
	cur := &CurrentSchemaBundle{
		Tables: []TableSchemaPersisted{{ID: "t1", ConnectionID: "c1", DatabaseName: "db1", TableName: "t"}},
		Columns: []ColumnSchemaPersisted{
			{ID: "c1", TableSchemaID: "t1", ColumnName: "a", DataType: "INT", AbstractType: "int"},
		},
	}
	_, err := eng.Compare(cur, nil, SchemaDiffCompareOptions{Scope: SchemaScanScopeAll})
	if err == nil || err.Code != SchemaDiffErrCodeFailedPrecondition {
		t.Fatalf("got %v want %s", err, SchemaDiffErrCodeFailedPrecondition)
	}
}
