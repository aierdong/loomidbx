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
