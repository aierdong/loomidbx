package schema

import (
	"context"
	"database/sql"
	"errors"
	"testing"
)

func TestSQLSchemaInspector_MySQLScopeValidation(t *testing.T) {
	ins := NewMySQLSchemaInspector()
	_, upErr := ins.Inspect(context.Background(), &sql.DB{}, InspectScope{
		Scope: SchemaScanScopeAll,
		// mysql 必须给 database_name
	})
	if upErr == nil {
		t.Fatalf("expected classified error")
	}
	if upErr.Code != UpstreamCodeUpstreamUnavailable {
		t.Fatalf("expected %s, got %s", UpstreamCodeUpstreamUnavailable, upErr.Code)
	}
}

func TestSQLSchemaInspector_ErrorMappingPermissionDenied(t *testing.T) {
	e := errors.New("ERROR: permission denied for relation information_schema.columns")
	ce := ClassifyUpstreamError(context.Background(), e)
	if ce == nil {
		t.Fatalf("expected classified error")
	}
	if ce.Code != UpstreamCodePermissionDenied {
		t.Fatalf("expected %s, got %s", UpstreamCodePermissionDenied, ce.Code)
	}
}

