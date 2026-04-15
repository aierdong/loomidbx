package schema

import (
	"context"
	"errors"
	"testing"
)

// fakeSchemaSyncRuntimeReader 为单测提供任务上下文读取能力。
type fakeSchemaSyncRuntimeReader struct {
	// ctx 保存 task_id 对应运行时上下文。
	ctx map[string]SchemaScanRuntimeContext
}

// GetRuntimeContext 返回 task_id 对应上下文。
func (f *fakeSchemaSyncRuntimeReader) GetRuntimeContext(taskID string) (SchemaScanRuntimeContext, bool) {
	v, ok := f.ctx[taskID]
	return v, ok
}

// fakeSchemaSyncPreviewStore 为单测提供待同步 schema 快照。
type fakeSchemaSyncPreviewStore struct {
	// bundles 保存 task_id 对应的 schema bundle。
	bundles map[string]*CurrentSchemaBundle
}

// LoadPendingSchemaBundle 根据 task_id 返回待同步 schema。
func (f *fakeSchemaSyncPreviewStore) LoadPendingSchemaBundle(_ context.Context, taskID string) (*CurrentSchemaBundle, error) {
	v, ok := f.bundles[taskID]
	if !ok {
		return nil, errors.New("bundle not found")
	}
	return v, nil
}

// fakeCurrentSchemaRepository 记录 Replace 调用并允许注入错误。
type fakeCurrentSchemaRepository struct {
	// err 为 TransactionalReplaceCurrentSchema 的注入错误。
	err error
}

// LoadCurrentSchema 非本批测试关注点，固定返回空 bundle。
func (f *fakeCurrentSchemaRepository) LoadCurrentSchema(_ context.Context, _ string) (*CurrentSchemaBundle, error) {
	return &CurrentSchemaBundle{}, nil
}

// TransactionalReplaceCurrentSchema 返回预置错误用于验证拒绝路径。
func (f *fakeCurrentSchemaRepository) TransactionalReplaceCurrentSchema(_ context.Context, _ string, _ *CurrentSchemaBundle) error {
	return f.err
}

func TestApplySchemaSync_BlockingRiskUnresolved(t *testing.T) {
	ctx := context.Background()
	repo := newFakeTrustRepo()
	repo.meta["conn-1"] = ConnectionSchemaMeta{
		SchemaTrustState:         SchemaTrustPendingAdjustment,
		SchemaLastBlockingReason: TrustBlockingReasonBlockingRisk,
	}

	svc := NewSchemaSyncService(
		&fakeSchemaSyncRuntimeReader{
			ctx: map[string]SchemaScanRuntimeContext{
				"task-1": {
					TaskID:       "task-1",
					ConnectionID: "conn-1",
					Status:       SchemaScanTaskCompleted,
				},
			},
		},
		&fakeSchemaSyncPreviewStore{
			bundles: map[string]*CurrentSchemaBundle{
				"task-1": {Tables: []TableSchemaPersisted{{ID: "t-1"}}},
			},
		},
		&fakeCurrentSchemaRepository{},
		NewSchemaTrustGate(repo),
	)

	out, err := svc.ApplySchemaSync(ctx, ApplySchemaSyncRequest{
		TaskID:      "task-1",
		AckRiskIDs:  nil,
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Code != "BLOCKING_RISK_UNRESOLVED" {
		t.Fatalf("code: got %s", err.Code)
	}
	if out == nil || out.TrustState != SchemaTrustPendingAdjustment || out.SyncApplied {
		t.Fatalf("unexpected result: %+v", out)
	}
}

func TestApplySchemaSync_RejectOnStorageWriteFailure(t *testing.T) {
	ctx := context.Background()
	repo := newFakeTrustRepo()
	repo.meta["conn-1"] = ConnectionSchemaMeta{SchemaTrustState: SchemaTrustTrusted}

	svc := NewSchemaSyncService(
		&fakeSchemaSyncRuntimeReader{
			ctx: map[string]SchemaScanRuntimeContext{
				"task-1": {
					TaskID:       "task-1",
					ConnectionID: "conn-1",
					Status:       SchemaScanTaskCompleted,
				},
			},
		},
		&fakeSchemaSyncPreviewStore{
			bundles: map[string]*CurrentSchemaBundle{
				"task-1": {Tables: []TableSchemaPersisted{{ID: "t-1"}}},
			},
		},
		&fakeCurrentSchemaRepository{err: errors.New("db down")},
		NewSchemaTrustGate(repo),
	)

	out, err := svc.ApplySchemaSync(ctx, ApplySchemaSyncRequest{
		TaskID:     "task-1",
		AckRiskIDs: []string{"risk-1"},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Code != "STORAGE_ERROR" {
		t.Fatalf("code: got %s", err.Code)
	}
	if out == nil || out.SyncApplied {
		t.Fatalf("unexpected result: %+v", out)
	}
}

func TestApplySchemaSync_RejectOnConcurrentConflict(t *testing.T) {
	ctx := context.Background()
	repo := newFakeTrustRepo()
	repo.meta["conn-1"] = ConnectionSchemaMeta{SchemaTrustState: SchemaTrustTrusted}

	svc := NewSchemaSyncService(
		&fakeSchemaSyncRuntimeReader{
			ctx: map[string]SchemaScanRuntimeContext{
				"task-1": {
					TaskID:       "task-1",
					ConnectionID: "conn-1",
					Status:       SchemaScanTaskCompleted,
				},
			},
		},
		&fakeSchemaSyncPreviewStore{
			bundles: map[string]*CurrentSchemaBundle{
				"task-1": {Tables: []TableSchemaPersisted{{ID: "t-1"}}},
			},
		},
		&fakeCurrentSchemaRepository{err: &SchemaSyncConcurrentConflictError{Message: "version changed"}},
		NewSchemaTrustGate(repo),
	)

	out, err := svc.ApplySchemaSync(ctx, ApplySchemaSyncRequest{
		TaskID:     "task-1",
		AckRiskIDs: []string{"risk-1"},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Code != "FAILED_PRECONDITION" {
		t.Fatalf("code: got %s", err.Code)
	}
	if out == nil || out.SyncApplied {
		t.Fatalf("unexpected result: %+v", out)
	}
}
