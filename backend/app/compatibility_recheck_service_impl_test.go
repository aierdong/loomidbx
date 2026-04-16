package app

import (
	"context"
	"testing"

	"loomidbx/generator"
	"loomidbx/schema"
)

// fakeCurrentSchemaRepo 为重判定测试提供内存版 CurrentSchemaRepository。
type fakeCurrentSchemaRepo struct {
	// byConn 保存 connection_id 对应当前 schema。
	byConn map[string]*schema.CurrentSchemaBundle
}

func (r *fakeCurrentSchemaRepo) LoadCurrentSchema(_ context.Context, connectionID string) (*schema.CurrentSchemaBundle, error) {
	if r.byConn == nil {
		return &schema.CurrentSchemaBundle{}, nil
	}
	if v, ok := r.byConn[connectionID]; ok && v != nil {
		return v, nil
	}
	return &schema.CurrentSchemaBundle{}, nil
}

func (r *fakeCurrentSchemaRepo) TransactionalReplaceCurrentSchema(_ context.Context, _ string, _ *schema.CurrentSchemaBundle) error {
	return nil
}

// fakeTrustRepo 为重判定测试提供内存版 TrustConnectionMetaRepository。
type fakeTrustRepo struct {
	// meta 保存 connection_id 对应连接 extra 的 schema 子域元数据。
	meta map[string]schema.ConnectionSchemaMeta
}

func newFakeTrustRepo() *fakeTrustRepo {
	return &fakeTrustRepo{meta: make(map[string]schema.ConnectionSchemaMeta)}
}

func (r *fakeTrustRepo) LoadConnectionSchemaMeta(_ context.Context, connectionID string) (schema.ConnectionSchemaMeta, error) {
	if v, ok := r.meta[connectionID]; ok {
		return v, nil
	}
	return schema.ConnectionSchemaMeta{SchemaTrustState: schema.SchemaTrustTrusted}, nil
}

func (r *fakeTrustRepo) PatchConnectionSchemaMeta(_ context.Context, connectionID string, patch schema.ConnectionSchemaMetaPatch) error {
	existing := r.meta[connectionID]
	if patch.TrustState != nil {
		existing.SchemaTrustState = *patch.TrustState
	}
	if patch.LastBlockingReason != nil {
		existing.SchemaLastBlockingReason = *patch.LastBlockingReason
	}
	if patch.LastSchemaScanUnix != nil {
		existing.LastSchemaScanUnix = *patch.LastSchemaScanUnix
	}
	if patch.LastSchemaSyncUnix != nil {
		existing.LastSchemaSyncUnix = *patch.LastSchemaSyncUnix
	}
	if patch.CompatibilityReport != nil {
		tmp := *patch.CompatibilityReport
		existing.CompatibilityReport = &tmp
	}
	r.meta[connectionID] = existing
	return nil
}

func TestDefaultCompatibilityRecheckService_NoGeneratorConfigWritesEmptySnapshot(t *testing.T) {
	ctx := context.Background()
	connID := "c1"

	reg := generator.NewGeneratorRegistry()
	trustRepo := newFakeTrustRepo()
	gate := schema.NewSchemaTrustGate(trustRepo)

	svc := NewDefaultCompatibilityRecheckService(
		&fakeCurrentSchemaRepo{},
		nil,
		reg,
		gate,
		trustRepo,
	)

	out, err := svc.RevalidateAllConfigs(ctx, connID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != schema.CompatibilityRecheckStatusSkippedNoGeneratorConfig {
		t.Fatalf("unexpected status: %+v", out)
	}
	meta, _ := trustRepo.LoadConnectionSchemaMeta(ctx, connID)
	if meta.CompatibilityReport == nil || meta.CompatibilityReport.Status != schema.CompatibilityRecheckStatusSkippedNoGeneratorConfig {
		t.Fatalf("expected compatibility report snapshot persisted: %+v", meta.CompatibilityReport)
	}
}

func TestDefaultCompatibilityRecheckService_IncompatibleGeneratorMovesTrustToPendingAdjustment(t *testing.T) {
	ctx := context.Background()
	connID := "c1"

	// 当前 schema：users.name 为 string
	currentRepo := &fakeCurrentSchemaRepo{
		byConn: map[string]*schema.CurrentSchemaBundle{
			connID: {
				Tables: []schema.TableSchemaPersisted{
					{ID: "t1", ConnectionID: connID, TableName: "users"},
				},
				Columns: []schema.ColumnSchemaPersisted{
					{ID: "c1", TableSchemaID: "t1", ColumnName: "name", AbstractType: "string", DataType: "varchar(255)"},
				},
			},
		},
	}

	// 配置快照：绑定一个不在候选集内的 generator_type
	genStore := schema.GeneratorConfigSnapshotStoreStub{
		SnapshotsByConnectionID: map[string]*schema.GeneratorConfigSnapshot{
			connID: {
				Columns: []schema.GeneratorColumnConfig{
					{
						ConnectionID:  connID,
						DatabaseName:  "db1",
						SchemaName:    "public",
						TableName:     "users",
						ColumnName:    "name",
						GeneratorType: "int_only_generator",
						ConfigID:      "cfg-1",
					},
				},
			},
		},
	}

	// 注册一个仅支持 string 的生成器，使候选集不包含 int_only_generator
	reg := generator.NewGeneratorRegistry()
	if err := reg.Register(generator.NewStaticGenerator(generator.GeneratorMeta{
		Type:          generator.GeneratorType("string_ok"),
		TypeTags:      []string{"supports:string", "deterministic"},
		Deterministic: true,
	})); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	trustRepo := newFakeTrustRepo()
	gate := schema.NewSchemaTrustGate(trustRepo)
	trustRepo.meta[connID] = schema.ConnectionSchemaMeta{SchemaTrustState: schema.SchemaTrustTrusted}

	svc := NewDefaultCompatibilityRecheckService(
		currentRepo,
		genStore,
		reg,
		gate,
		trustRepo,
	)

	out, err := svc.RevalidateAllConfigs(ctx, connID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != schema.CompatibilityRecheckStatusSuccess {
		t.Fatalf("unexpected status: %+v", out)
	}
	if out.Summary.BlockingRisks == 0 || len(out.Risks) == 0 {
		t.Fatalf("expected blocking risks: %+v", out)
	}
	view, viewErr := gate.GetSchemaTrustState(ctx, connID)
	if viewErr != nil {
		t.Fatalf("get trust state failed: %v", viewErr)
	}
	if view.State != schema.SchemaTrustPendingAdjustment {
		t.Fatalf("expected pending_adjustment, got %q", view.State)
	}
	if view.CompatibilityReport == nil || view.CompatibilityReport.Summary.BlockingRisks == 0 {
		t.Fatalf("expected compatibility report in trust view: %+v", view.CompatibilityReport)
	}
}

