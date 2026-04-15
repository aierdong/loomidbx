package schema

import (
	"context"
	"fmt"
	"testing"
)

// fakeTrustRepo 为单测提供内存 TrustConnectionMetaRepository。
type fakeTrustRepo struct {
	// meta 为 connection_id 到 ConnectionSchemaMeta 的内存表。
	meta map[string]ConnectionSchemaMeta
}

func newFakeTrustRepo() *fakeTrustRepo {
	return &fakeTrustRepo{meta: make(map[string]ConnectionSchemaMeta)}
}

// LoadConnectionSchemaMeta 从内存读取连接元数据。
func (f *fakeTrustRepo) LoadConnectionSchemaMeta(_ context.Context, connectionID string) (ConnectionSchemaMeta, error) {
	m, ok := f.meta[connectionID]
	if !ok {
		return ConnectionSchemaMeta{}, fmt.Errorf("connection not found")
	}
	return m, nil
}

// PatchConnectionSchemaMeta 将 patch 合并进内存模型。
func (f *fakeTrustRepo) PatchConnectionSchemaMeta(_ context.Context, connectionID string, patch ConnectionSchemaMetaPatch) error {
	cur, ok := f.meta[connectionID]
	if !ok {
		return fmt.Errorf("connection not found")
	}
	if patch.TrustState != nil {
		cur.SchemaTrustState = *patch.TrustState
	}
	if patch.LastBlockingReason != nil {
		cur.SchemaLastBlockingReason = *patch.LastBlockingReason
	}
	if patch.LastSchemaScanUnix != nil {
		cur.LastSchemaScanUnix = *patch.LastSchemaScanUnix
	}
	if patch.LastSchemaSyncUnix != nil {
		cur.LastSchemaSyncUnix = *patch.LastSchemaSyncUnix
	}
	f.meta[connectionID] = cur
	return nil
}

func TestSchemaTrustGate_UpdateTrustState_persists(t *testing.T) {
	ctx := context.Background()
	repo := newFakeTrustRepo()
	repo.meta["c1"] = ConnectionSchemaMeta{SchemaTrustState: SchemaTrustTrusted}

	gate := NewSchemaTrustGate(repo)
	next, err := gate.UpdateTrustState(ctx, "c1", TrustStateUpdateInput{HasBlockingRisk: true})
	if err != nil {
		t.Fatal(err)
	}
	if next != SchemaTrustPendingAdjustment {
		t.Fatalf("next: %s", next)
	}
	v, err := gate.GetSchemaTrustState(ctx, "c1")
	if err != nil {
		t.Fatal(err)
	}
	if v.State != SchemaTrustPendingAdjustment || v.LastBlockingReason != TrustBlockingReasonBlockingRisk {
		t.Fatalf("view: %+v", v)
	}
}

func TestSchemaTrustGate_CheckBlockingRisksHandled(t *testing.T) {
	ctx := context.Background()
	repo := newFakeTrustRepo()
	repo.meta["c1"] = ConnectionSchemaMeta{
		SchemaTrustState:         SchemaTrustPendingAdjustment,
		SchemaLastBlockingReason: TrustBlockingReasonBlockingRisk,
	}
	gate := NewSchemaTrustGate(repo)
	if err := gate.CheckBlockingRisksHandled(ctx, "c1", nil); err == nil {
		t.Fatal("expected error")
	}
	if err := gate.CheckBlockingRisksHandled(ctx, "c1", []string{"r1"}); err != nil {
		t.Fatal(err)
	}
}
