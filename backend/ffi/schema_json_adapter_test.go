package ffi_test

import (
	"encoding/json"
	"strings"
	"testing"

	"loomidbx/ffi"
	"loomidbx/schema"
)

func TestSchemaFFIAdapter_StartSchemaScan_OK(t *testing.T) {
	adapter := ffi.NewSchemaFFIAdapter(schemaFFIDeps{
		Starter: &fakeStarter{
			startResult: &schema.SchemaScanStartResult{TaskID: "task-1", Status: schema.SchemaScanTaskRunning},
		},
	})
	resp := adapter.StartSchemaScan(`{"connection_id":"conn-1","scope":"all","trigger":"manual"}`)
	assertFFIOKWithData(t, resp)
	if strings.Contains(resp, "password") || strings.Contains(resp, "token") {
		t.Fatalf("response should be sanitized: %s", resp)
	}
}

func TestSchemaFFIAdapter_GetSchemaScanStatus_TaskNotFoundMapped(t *testing.T) {
	adapter := ffi.NewSchemaFFIAdapter(schemaFFIDeps{
		StatusReader: &fakeStatusReader{
			err: &schema.SchemaScanStatusError{
				Code:    schema.SchemaScanStatusErrCodeTaskNotFound,
				Message: "schema scan task not found",
			},
		},
	})
	resp := adapter.GetSchemaScanStatus(`{"task_id":"missing-task"}`)
	assertFFIErrorCode(t, resp, "FAILED_PRECONDITION")
}

func TestSchemaFFIAdapter_PreviewSchemaDiff_CurrentSchemaNotFound(t *testing.T) {
	adapter := ffi.NewSchemaFFIAdapter(schemaFFIDeps{
		Previewer: &fakePreviewer{
			err: &schema.SchemaDiffError{
				Code:    schema.SchemaDiffErrCodeCurrentSchemaNotFound,
				Message: "current persisted schema is missing",
			},
		},
	})
	resp := adapter.PreviewSchemaDiff(`{"task_id":"task-x"}`)
	assertFFIErrorCode(t, resp, schema.SchemaDiffErrCodeCurrentSchemaNotFound)
}

func TestSchemaFFIAdapter_PreviewSchemaDiff_UIContractAllowsSyncWhenNoBlockingRisk(t *testing.T) {
	adapter := ffi.NewSchemaFFIAdapter(schemaFFIDeps{
		Previewer: &fakePreviewer{
			result: &schema.SchemaDiffResult{},
		},
		RiskReader: &fakeRiskReader{
			result: schema.GeneratorCompatibilityRisksResult{
				Mode: schema.GeneratorCompatibilityModeConfigured,
				Risks: []schema.GeneratorCompatibilityRisk{
					{
						ID:       "r-warning",
						Severity: "",
					},
				},
			},
		},
	})
	resp := adapter.PreviewSchemaDiff(`{"task_id":"task-ok"}`)
	assertFFIOKWithData(t, resp)

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(resp), &parsed); err != nil {
		t.Fatalf("parse response failed: %v", err)
	}
	data := parsed["data"].(map[string]interface{})
	action := data["action"].(map[string]interface{})
	if action["can_apply_sync"] != true {
		t.Fatalf("expected can_apply_sync=true, got %v", action["can_apply_sync"])
	}
	if action["requires_adjustment_before_sync"] != false {
		t.Fatalf("expected requires_adjustment_before_sync=false, got %v", action["requires_adjustment_before_sync"])
	}
}

func TestSchemaFFIAdapter_PreviewSchemaDiff_UIContractBlocksSyncWhenBlockingRisk(t *testing.T) {
	adapter := ffi.NewSchemaFFIAdapter(schemaFFIDeps{
		Previewer: &fakePreviewer{
			result: &schema.SchemaDiffResult{},
		},
		RiskReader: &fakeRiskReader{
			result: schema.GeneratorCompatibilityRisksResult{
				Mode: schema.GeneratorCompatibilityModeConfigured,
				Risks: []schema.GeneratorCompatibilityRisk{
					{
						ID:       "r-blocking",
						Severity: schema.GeneratorCompatibilityRiskSeverityBlocking,
					},
				},
			},
		},
	})
	resp := adapter.PreviewSchemaDiff(`{"task_id":"task-blocking"}`)
	assertFFIOKWithData(t, resp)

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(resp), &parsed); err != nil {
		t.Fatalf("parse response failed: %v", err)
	}
	data := parsed["data"].(map[string]interface{})
	action := data["action"].(map[string]interface{})
	if action["can_apply_sync"] != false {
		t.Fatalf("expected can_apply_sync=false, got %v", action["can_apply_sync"])
	}
	if action["requires_adjustment_before_sync"] != true {
		t.Fatalf("expected requires_adjustment_before_sync=true, got %v", action["requires_adjustment_before_sync"])
	}
}

func TestSchemaFFIAdapter_GetGeneratorCompatibilityRisks_NoConfig(t *testing.T) {
	adapter := ffi.NewSchemaFFIAdapter(schemaFFIDeps{
		RiskReader: &fakeRiskReader{
			result: schema.GeneratorCompatibilityRisksResult{
				Mode:  schema.GeneratorCompatibilityModeNoGeneratorConfig,
				Risks: nil,
			},
		},
	})
	resp := adapter.GetGeneratorCompatibilityRisks(`{"task_id":"task-r"}`)
	assertFFIOKWithData(t, resp)
}

func TestSchemaFFIAdapter_ApplySchemaSync_BlockingRisk(t *testing.T) {
	adapter := ffi.NewSchemaFFIAdapter(schemaFFIDeps{
		Syncer: &fakeSyncer{
			err: &schema.SchemaSyncError{
				Code:    "BLOCKING_RISK_UNRESOLVED",
				Message: "blocking generator compatibility risks must be acknowledged before schema sync",
			},
		},
	})
	resp := adapter.ApplySchemaSync(`{"task_id":"task-2","ack_risk_ids":[]}`)
	assertFFIErrorCode(t, resp, "BLOCKING_RISK_UNRESOLVED")
}

func TestSchemaFFIAdapter_StartSchemaRescan_InvalidArgument(t *testing.T) {
	adapter := ffi.NewSchemaFFIAdapter(schemaFFIDeps{
		Starter: &fakeStarter{
			rescanErr: &schema.SchemaScanStartError{
				Code:    schema.SchemaScanErrCodeInvalidArgument,
				Message: "reason is required",
			},
		},
	})
	resp := adapter.StartSchemaRescan(`{"connection_id":"c1","strategy":"full","reason":"  "}`)
	assertFFIErrorCode(t, resp, schema.SchemaScanErrCodeInvalidArgument)
}

func TestSchemaFFIAdapter_GetCurrentSchema_OKAndSanitized(t *testing.T) {
	adapter := ffi.NewSchemaFFIAdapter(schemaFFIDeps{
		CurrentReader: &fakeCurrentReader{
			bundle: &schema.CurrentSchemaBundle{
				Tables: []schema.TableSchemaPersisted{
					{ID: "t1", ConnectionID: "conn-hidden", DatabaseName: "db1", TableName: "users"},
				},
				Columns: []schema.ColumnSchemaPersisted{
					{ID: "c1", TableSchemaID: "t1", ColumnName: "id", DataType: "int"},
				},
			},
		},
		TrustReader: &fakeTrustReader{
			view: schema.TrustStateView{State: schema.SchemaTrustTrusted},
		},
	})
	resp := adapter.GetCurrentSchema(`{"connection_id":"conn-1","scope":"all"}`)
	assertFFIOKWithData(t, resp)
	if strings.Contains(resp, "conn-hidden") || strings.Contains(resp, "password") || strings.Contains(resp, "token") {
		t.Fatalf("response should be sanitized: %s", resp)
	}
}

func TestSchemaFFIAdapter_GetSchemaTrustState_OK(t *testing.T) {
	adapter := ffi.NewSchemaFFIAdapter(schemaFFIDeps{
		TrustReader: &fakeTrustReader{
			view: schema.TrustStateView{
				State:              schema.SchemaTrustPendingRescan,
				LastBlockingReason: "BLOCKING_RISK_UNRESOLVED",
			},
		},
	})
	resp := adapter.GetSchemaTrustState(`{"connection_id":"conn-1"}`)
	assertFFIOKWithData(t, resp)

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(resp), &parsed); err != nil {
		t.Fatalf("parse response failed: %v", err)
	}
	data := parsed["data"].(map[string]interface{})
	if _, ok := data["compatibility_report"]; !ok {
		t.Fatalf("expected compatibility_report field, got %v", data)
	}
}

func TestSchemaFFIAdapter_ApplySchemaSync_SuccessIncludesCompatibilityRecheck(t *testing.T) {
	adapter := ffi.NewSchemaFFIAdapter(schemaFFIDeps{
		Syncer: &fakeSyncer{
			result: &schema.ApplySchemaSyncResult{
				SyncApplied: true,
				TrustState:  schema.SchemaTrustTrusted,
				CompatibilityRecheck: schema.CompatibilityReportSnapshot{
					Status:          schema.CompatibilityRecheckStatusSuccess,
					GeneratedAtUnix: 1700000000,
					Summary: schema.CompatibilityReportSummary{
						Mode:          schema.GeneratorCompatibilityModeNoGeneratorConfig,
						TotalRisks:    0,
						BlockingRisks: 0,
					},
					Risks: []schema.GeneratorCompatibilityRisk{},
				},
			},
		},
	})
	resp := adapter.ApplySchemaSync(`{"task_id":"task-1","ack_risk_ids":[]}`)
	assertFFIOKWithData(t, resp)

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(resp), &parsed); err != nil {
		t.Fatalf("parse response failed: %v", err)
	}
	data := parsed["data"].(map[string]interface{})
	if data["sync_applied"] != true {
		t.Fatalf("expected sync_applied=true, got %v", data["sync_applied"])
	}
	if data["trust_state"] != string(schema.SchemaTrustTrusted) && data["trust_state"] != schema.SchemaTrustTrusted {
		// json.Unmarshal 可能把枚举当 string
		t.Fatalf("unexpected trust_state: %v", data["trust_state"])
	}
	if _, ok := data["compatibility_recheck"]; !ok {
		t.Fatalf("expected compatibility_recheck field, got %v", data)
	}
}

func TestSchemaFFIAdapter_ParseErrorToInvalidArgument(t *testing.T) {
	adapter := ffi.NewSchemaFFIAdapter(schemaFFIDeps{})
	resp := adapter.StartSchemaScan(`not-json`)
	assertFFIErrorCode(t, resp, "INVALID_ARGUMENT")
}

func TestSchemaFFIAdapter_RejectExecutionRequest_OutOfScope(t *testing.T) {
	adapter := ffi.NewSchemaFFIAdapter(schemaFFIDeps{})
	resp := adapter.RejectExecutionRequest(`{"operation":"start_generation"}`)
	assertFFIErrorCode(t, resp, schema.SchemaBoundaryErrCodeOutOfScope)
}

type schemaFFIDeps = ffi.SchemaFFIDependencies

type fakeStarter struct {
	startResult *schema.SchemaScanStartResult
	startErr    *schema.SchemaScanStartError
	rescanErr   *schema.SchemaScanStartError
}

func (f *fakeStarter) StartSchemaScan(_ string, _ schema.SchemaScanScope, _ []string, _ string) (*schema.SchemaScanStartResult, *schema.SchemaScanStartError) {
	return f.startResult, f.startErr
}
func (f *fakeStarter) StartSchemaRescan(_ string, _ schema.SchemaRescanStrategy, _ string, _ []string) (*schema.SchemaScanStartResult, *schema.SchemaScanStartError) {
	return nil, f.rescanErr
}

type fakeStatusReader struct {
	snapshot schema.SchemaScanStatusSnapshot
	err      *schema.SchemaScanStatusError
}

func (f *fakeStatusReader) GetSchemaScanStatus(_ string) (schema.SchemaScanStatusSnapshot, *schema.SchemaScanStatusError) {
	return f.snapshot, f.err
}

type fakePreviewer struct {
	result *schema.SchemaDiffResult
	err    *schema.SchemaDiffError
}

func (f *fakePreviewer) PreviewSchemaDiff(_ string) (*schema.SchemaDiffResult, *schema.SchemaDiffError) {
	return f.result, f.err
}

type fakeRiskReader struct {
	result schema.GeneratorCompatibilityRisksResult
	err    error
}

func (f *fakeRiskReader) GetGeneratorCompatibilityRisks(_ string) (schema.GeneratorCompatibilityRisksResult, error) {
	return f.result, f.err
}

type fakeSyncer struct {
	result *schema.ApplySchemaSyncResult
	err    *schema.SchemaSyncError
}

func (f *fakeSyncer) ApplySchemaSync(_ string, _ []string) (*schema.ApplySchemaSyncResult, *schema.SchemaSyncError) {
	return f.result, f.err
}

type fakeCurrentReader struct {
	bundle *schema.CurrentSchemaBundle
	err    error
}

func (f *fakeCurrentReader) GetCurrentSchema(_ string, _ string) (*schema.CurrentSchemaBundle, error) {
	return f.bundle, f.err
}

type fakeTrustReader struct {
	view schema.TrustStateView
	err  error
}

func (f *fakeTrustReader) GetSchemaTrustState(_ string) (schema.TrustStateView, error) {
	return f.view, f.err
}

func assertFFIOKWithData(t *testing.T, resp string) {
	t.Helper()
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(resp), &parsed); err != nil {
		t.Fatalf("parse response failed: %v", err)
	}
	if parsed["ok"] != true {
		t.Fatalf("expected ok=true, got %s", resp)
	}
	if _, ok := parsed["data"]; !ok {
		t.Fatalf("expected data field, got %s", resp)
	}
	if parsed["error"] != nil {
		t.Fatalf("expected error=nil, got %s", resp)
	}
}

func assertFFIErrorCode(t *testing.T, resp string, code string) {
	t.Helper()
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(resp), &parsed); err != nil {
		t.Fatalf("parse response failed: %v", err)
	}
	if parsed["ok"] != false {
		t.Fatalf("expected ok=false, got %s", resp)
	}
	rawErr, ok := parsed["error"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected error object, got %s", resp)
	}
	if rawErr["code"] != code {
		t.Fatalf("expected error code %s, got %v", code, rawErr["code"])
	}
}

