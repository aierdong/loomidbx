package schema

import (
	"strings"
	"testing"
)

func TestMergeConnectionExtraSchemaMeta_preservesUnrelatedKeys(t *testing.T) {
	existing := `{"charset":"utf8mb4","credential_ref":"keyring:abc"}`
	ts := SchemaTrustPendingRescan
	reason := "BLOCKING_RISK_UNRESOLVED"
	scan := int64(1700000001)
	sync := int64(1700000002)
	out, err := MergeConnectionExtraSchemaMeta(existing, ConnectionSchemaMetaPatch{
		TrustState:         &ts,
		LastBlockingReason: &reason,
		LastSchemaScanUnix: &scan,
		LastSchemaSyncUnix: &sync,
	})
	if err != nil {
		t.Fatal(err)
	}
	meta, err := ParseConnectionSchemaMeta(out)
	if err != nil {
		t.Fatal(err)
	}
	if meta.SchemaTrustState != SchemaTrustPendingRescan {
		t.Fatalf("trust: got %q", meta.SchemaTrustState)
	}
	if meta.SchemaLastBlockingReason != reason {
		t.Fatalf("reason: got %q", meta.SchemaLastBlockingReason)
	}
	if meta.LastSchemaScanUnix != scan || meta.LastSchemaSyncUnix != sync {
		t.Fatalf("times: %+v", meta)
	}
	if !strings.Contains(out, "charset") || !strings.Contains(out, "credential_ref") {
		t.Fatalf("lost unrelated keys: %s", out)
	}
}

func TestMergeConnectionExtraSchemaMeta_CompatibilityReportRoundTrip(t *testing.T) {
	existing := `{"credential_ref":"keyring:abc"}`
	out, err := MergeConnectionExtraSchemaMeta(existing, ConnectionSchemaMetaPatch{
		CompatibilityReport: &CompatibilityReportSnapshot{
			Status:          CompatibilityRecheckStatusSuccess,
			GeneratedAtUnix: 1700000010,
			Summary: CompatibilityReportSummary{
				Mode:          GeneratorCompatibilityModeConfigured,
				TotalRisks:    2,
				BlockingRisks: 1,
			},
			Risks: []GeneratorCompatibilityRisk{
				{ID: "r1", Severity: GeneratorCompatibilityRiskSeverityBlocking},
				{ID: "r2"},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	meta, err := ParseConnectionSchemaMeta(out)
	if err != nil {
		t.Fatal(err)
	}
	if meta.CompatibilityReport == nil {
		t.Fatalf("compatibility report should exist: %+v", meta)
	}
	if meta.CompatibilityReport.Status != CompatibilityRecheckStatusSuccess {
		t.Fatalf("status: got %q", meta.CompatibilityReport.Status)
	}
	if meta.CompatibilityReport.GeneratedAtUnix != 1700000010 {
		t.Fatalf("generated_at: got %d", meta.CompatibilityReport.GeneratedAtUnix)
	}
	if meta.CompatibilityReport.Summary.Mode != GeneratorCompatibilityModeConfigured || meta.CompatibilityReport.Summary.BlockingRisks != 1 {
		t.Fatalf("summary: %+v", meta.CompatibilityReport.Summary)
	}
	if len(meta.CompatibilityReport.Risks) != 2 {
		t.Fatalf("risks: %+v", meta.CompatibilityReport.Risks)
	}
	if !strings.Contains(out, "credential_ref") {
		t.Fatalf("lost unrelated keys: %s", out)
	}
}

func TestParseConnectionSchemaMeta_CompatibilityReportDefaultsRisksNonNil(t *testing.T) {
	meta, err := ParseConnectionSchemaMeta(`{"compatibility_report":{"status":"success","generated_at_unix":1700000020,"summary":{"mode":"configured","total_risks":0,"blocking_risks":0}}}`)
	if err != nil {
		t.Fatal(err)
	}
	if meta.CompatibilityReport == nil {
		t.Fatalf("compatibility report should exist: %+v", meta)
	}
	if meta.CompatibilityReport.Risks == nil {
		t.Fatal("risks should be non-nil slice")
	}
	if len(meta.CompatibilityReport.Risks) != 0 {
		t.Fatalf("risks should be empty: %+v", meta.CompatibilityReport.Risks)
	}
}

func TestMergeConnectionExtraSchemaMeta_nilPatchNoOp(t *testing.T) {
	existing := `{"x":1}`
	out, err := MergeConnectionExtraSchemaMeta(existing, ConnectionSchemaMetaPatch{})
	if err != nil {
		t.Fatal(err)
	}
	if out != existing {
		t.Fatalf("expected unchanged, got %s", out)
	}
}

func TestMergeConnectionExtraSchemaMeta_clearBlockingReason(t *testing.T) {
	existing := `{"schema_last_blocking_reason":"OLD"}`
	empty := ""
	out, err := MergeConnectionExtraSchemaMeta(existing, ConnectionSchemaMetaPatch{
		LastBlockingReason: &empty,
	})
	if err != nil {
		t.Fatal(err)
	}
	meta, err := ParseConnectionSchemaMeta(out)
	if err != nil {
		t.Fatal(err)
	}
	if meta.SchemaLastBlockingReason != "" {
		t.Fatalf("want empty reason, got %q", meta.SchemaLastBlockingReason)
	}
}

func TestParseConnectionSchemaMeta_defaults(t *testing.T) {
	meta, err := ParseConnectionSchemaMeta("{}")
	if err != nil {
		t.Fatal(err)
	}
	if meta.SchemaTrustState != SchemaTrustTrusted {
		t.Fatalf("default trust: %q", meta.SchemaTrustState)
	}
}
