package schema

import "testing"

func TestParseSchemaTrustState_knownValues(t *testing.T) {
	for _, s := range []string{"trusted", "pending_rescan", "pending_adjustment"} {
		if _, err := ParseSchemaTrustState(s); err != nil {
			t.Fatalf("%q: %v", s, err)
		}
	}
}

func TestParseSchemaTrustState_emptyDefaultsToTrusted(t *testing.T) {
	got, err := ParseSchemaTrustState("")
	if err != nil {
		t.Fatal(err)
	}
	if got != SchemaTrustTrusted {
		t.Fatalf("got %q", got)
	}
}

func TestParseSchemaTrustState_unknown(t *testing.T) {
	if _, err := ParseSchemaTrustState("nope"); err == nil {
		t.Fatal("expected error")
	}
}

func TestSchemaTrustAllowsDownstreamExecution(t *testing.T) {
	if !SchemaTrustAllowsDownstreamExecution(SchemaTrustTrusted) {
		t.Fatal("trusted should allow downstream")
	}
	if SchemaTrustAllowsDownstreamExecution(SchemaTrustPendingRescan) {
		t.Fatal("pending_rescan must block downstream")
	}
	if SchemaTrustAllowsDownstreamExecution(SchemaTrustPendingAdjustment) {
		t.Fatal("pending_adjustment must block downstream")
	}
	if !SchemaTrustAllowsDownstreamExecution("") {
		t.Fatal("empty state defaults to trusted in parser; should allow downstream")
	}
}
