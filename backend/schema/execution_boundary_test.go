package schema

import "testing"

func TestEnsureExecutionPrecondition_AllowsTrusted(t *testing.T) {
	if err := EnsureExecutionPrecondition(SchemaTrustTrusted); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEnsureExecutionPrecondition_BlocksPendingAdjustmentWithStableCode(t *testing.T) {
	err := EnsureExecutionPrecondition(SchemaTrustPendingAdjustment)
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Code != TrustBlockingReasonBlockingRisk {
		t.Fatalf("code: got %s", err.Code)
	}
}

func TestEnsureExecutionPrecondition_BlocksPendingRescan(t *testing.T) {
	err := EnsureExecutionPrecondition(SchemaTrustPendingRescan)
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Code != SchemaSyncErrCodeFailedPrecondition {
		t.Fatalf("code: got %s", err.Code)
	}
}
