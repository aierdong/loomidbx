package schema

import (
	"context"
	"testing"
)

func TestGetGeneratorCompatibilityRisks_noGeneratorConfig_notError(t *testing.T) {
	rt := NewSchemaScanRuntimeStore()
	rt.StartTask(SchemaScanRuntimeStartRequest{
		TaskID:         "t1",
		ConnectionID:   "conn-a",
		Scope:          SchemaScanScopeAll,
		Trigger:        "manual",
		RescanReason:   "",
		RescanStrategy: "",
	})
	rt.MarkCompleted("t1", true)
	res, err := GetGeneratorCompatibilityRisks(context.Background(), "t1", rt, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Mode != GeneratorCompatibilityModeNoGeneratorConfig {
		t.Fatalf("mode: %s", res.Mode)
	}
	if len(res.Risks) != 0 {
		t.Fatalf("risks should be empty: %#v", res.Risks)
	}
}

func TestGetGeneratorCompatibilityRisks_taskNotFound(t *testing.T) {
	rt := NewSchemaScanRuntimeStore()
	_, err := GetGeneratorCompatibilityRisks(context.Background(), "missing", rt, nil, nil, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	se, ok := err.(*SchemaScanStatusError)
	if !ok || se.Code != SchemaScanStatusErrCodeTaskNotFound {
		t.Fatalf("err: %v", err)
	}
}

func TestGetGeneratorCompatibilityRisks_configuredMode(t *testing.T) {
	rt := NewSchemaScanRuntimeStore()
	rt.StartTask(SchemaScanRuntimeStartRequest{
		TaskID:       "t2",
		ConnectionID: "conn-b",
		Scope:        SchemaScanScopeAll,
		Trigger:      "manual",
	})
	rt.MarkCompleted("t2", true)
	diffReader := stubDiffByTaskReader{
		// diffByTask 提供任务对应 Diff 输入。
		diffByTask: map[string]*SchemaDiffResult{
			"t2": {
				TableDiffs: []SchemaTableDiff{
					{
						DatabaseName: "db1",
						TableName:    "users",
						Kind:         SchemaDiffKindModified,
						ColumnDiffs: []SchemaColumnDiff{
							{
								ColumnName: "legacy_code",
								Kind:       SchemaColumnDiffKindRemoved,
								Old: &SchemaColumnSnapshot{
									Name:         "legacy_code",
									AbstractType: "string",
								},
							},
							{
								ColumnName: "nickname",
								Kind:       SchemaColumnDiffKindAdded,
								New: &SchemaColumnSnapshot{
									Name:         "nickname",
									AbstractType: "string",
								},
							},
						},
					},
				},
			},
		},
	}
	genStore := GeneratorConfigSnapshotStoreStub{
		SnapshotsByConnectionID: map[string]*GeneratorConfigSnapshot{
			"conn-b": {
				Columns: []GeneratorColumnConfig{
					{
						ConnectionID: "conn-b",
						DatabaseName: "db1",
						TableName:    "users",
						ColumnName:   "legacy_code",
						ConfigID:     "cfg-legacy",
					},
				},
			},
		},
	}
	res, err := GetGeneratorCompatibilityRisks(context.Background(), "t2", rt, diffReader, genStore, NewGeneratorCompatibilityAnalyzer())
	if err != nil {
		t.Fatal(err)
	}
	if res.Mode != GeneratorCompatibilityModeConfigured {
		t.Fatalf("mode: %s", res.Mode)
	}
	if len(res.Risks) == 0 {
		t.Fatalf("expected configured mode risks, got none")
	}
	assertRiskTypeExists(t, res.Risks, GeneratorCompatibilityRiskTypeColumnMissingOrRenamed)
}

func TestGeneratorCompatibilityAnalyzer_Analyze_blockingRisks(t *testing.T) {
	analyzer := NewGeneratorCompatibilityAnalyzer()
	diff := &SchemaDiffResult{
		TableDiffs: []SchemaTableDiff{
			{
				DatabaseName: "db1",
				TableName:    "users",
				Kind:         SchemaDiffKindModified,
				ColumnDiffs: []SchemaColumnDiff{
					{
						ColumnName: "legacy_code",
						Kind:       SchemaColumnDiffKindRemoved,
						Old: &SchemaColumnSnapshot{
							Name:         "legacy_code",
							AbstractType: "string",
						},
					},
					{
						ColumnName: "nickname",
						Kind:       SchemaColumnDiffKindAdded,
						New: &SchemaColumnSnapshot{
							Name:         "nickname",
							AbstractType: "string",
						},
					},
					{
						ColumnName: "age",
						Kind:       SchemaColumnDiffKindModified,
						AttributeChanges: []string{
							"data_type",
							"abstract_type",
						},
						Old: &SchemaColumnSnapshot{
							Name:         "age",
							DataType:     "INT",
							AbstractType: "int",
						},
						New: &SchemaColumnSnapshot{
							Name:         "age",
							DataType:     "TEXT",
							AbstractType: "string",
						},
					},
				},
			},
			{
				DatabaseName: "db1",
				TableName:    "orders",
				Kind:         SchemaDiffKindRemoved,
			},
		},
	}
	cfg := &GeneratorConfigSnapshot{
		Columns: []GeneratorColumnConfig{
			{
				ConnectionID: "conn-a",
				DatabaseName: "db1",
				TableName:    "users",
				ColumnName:   "legacy_code",
				ConfigID:     "cfg-legacy",
			},
			{
				ConnectionID: "conn-a",
				DatabaseName: "db1",
				TableName:    "users",
				ColumnName:   "age",
				ConfigID:     "cfg-age",
			},
			{
				ConnectionID: "conn-a",
				DatabaseName: "db1",
				TableName:    "orders",
				ColumnName:   "total_amount",
				ConfigID:     "cfg-orders",
			},
		},
	}

	risks := analyzer.Analyze(diff, cfg)
	if len(risks) != 3 {
		t.Fatalf("risk count=%d, risks=%+v", len(risks), risks)
	}
	assertRiskTypeExists(t, risks, GeneratorCompatibilityRiskTypeColumnMissingOrRenamed)
	assertRiskTypeExists(t, risks, GeneratorCompatibilityRiskTypeColumnTypeIncompatible)
	assertRiskTypeExists(t, risks, GeneratorCompatibilityRiskTypeColumnDeleted)
	for _, r := range risks {
		if r.Severity != GeneratorCompatibilityRiskSeverityBlocking {
			t.Fatalf("risk severity should be blocking: %+v", r)
		}
		if r.Object == "" || r.Reason == "" || r.SuggestedAction == "" {
			t.Fatalf("risk fields should not be empty: %+v", r)
		}
	}
}

func TestGeneratorConfigSnapshotStoreStub_LoadByConnectionID(t *testing.T) {
	store := GeneratorConfigSnapshotStoreStub{
		SnapshotsByConnectionID: map[string]*GeneratorConfigSnapshot{
			"conn-a": {
				Columns: []GeneratorColumnConfig{
					{ConnectionID: "conn-a", TableName: "t", ColumnName: "c", ConfigID: "cfg-1"},
				},
			},
		},
	}
	got, err := store.LoadByConnectionID(context.Background(), "conn-a")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || len(got.Columns) != 1 {
		t.Fatalf("unexpected snapshot: %+v", got)
	}
	missing, err := store.LoadByConnectionID(context.Background(), "missing")
	if err != nil {
		t.Fatal(err)
	}
	if missing == nil || len(missing.Columns) != 0 {
		t.Fatalf("missing snapshot should return empty snapshot: %+v", missing)
	}
}

func assertRiskTypeExists(t *testing.T, in []GeneratorCompatibilityRisk, typ GeneratorCompatibilityRiskType) {
	t.Helper()
	for _, r := range in {
		if r.Type == typ {
			return
		}
	}
	t.Fatalf("risk type %s not found, risks=%+v", typ, in)
}

// stubDiffByTaskReader 为测试注入 task_id -> diff 读取结果。
type stubDiffByTaskReader struct {
	// diffByTask 保存任务 Diff 映射。
	diffByTask map[string]*SchemaDiffResult
}

// LoadSchemaDiffByTaskID 返回预置 Diff，缺失时返回未就绪错误。
func (s stubDiffByTaskReader) LoadSchemaDiffByTaskID(taskID string) (*SchemaDiffResult, error) {
	diff, ok := s.diffByTask[taskID]
	if !ok || diff == nil {
		return nil, context.DeadlineExceeded
	}
	return diff, nil
}
