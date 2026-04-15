package schema

import (
	"context"
	"strings"
	"testing"
)

func TestSchemaScanStarter_StartSchemaScan_InvalidScopeTableWithoutNames(t *testing.T) {
	st := NewSchemaScanStarter(NewSchemaScanRuntimeStore())
	_, err := st.StartSchemaScan(context.Background(), StartSchemaScanRequest{
		ConnectionID: "c1",
		Scope:        SchemaScanScopeTables,
		TableNames:   nil,
		Trigger:      "manual",
	})
	if err == nil {
		t.Fatal("expected invalid argument")
	}
	if err.Code != SchemaScanErrCodeInvalidArgument {
		t.Fatalf("code: got %s want %s", err.Code, SchemaScanErrCodeInvalidArgument)
	}
}

func TestSchemaScanStarter_StartSchemaScan_InvalidAllWithTableNames(t *testing.T) {
	st := NewSchemaScanStarter(NewSchemaScanRuntimeStore())
	_, err := st.StartSchemaScan(context.Background(), StartSchemaScanRequest{
		ConnectionID: "c1",
		Scope:        SchemaScanScopeAll,
		TableNames:   []string{"t1"},
		Trigger:      "manual",
	})
	if err == nil {
		t.Fatal("expected invalid argument")
	}
	if err.Code != SchemaScanErrCodeInvalidArgument {
		t.Fatalf("code: got %s", err.Code)
	}
}

func TestSchemaScanStarter_StartSchemaScan_OK_TableScope(t *testing.T) {
	store := NewSchemaScanRuntimeStore()
	st := NewSchemaScanStarterWithTaskID(store, func() string { return "fixed-task" })

	res, err := st.StartSchemaScan(context.Background(), StartSchemaScanRequest{
		ConnectionID: "conn-x",
		Scope:        SchemaScanScopeTables,
		TableNames:   []string{"  Users ", "users", "orders"},
		Trigger:      "manual",
	})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if res.TaskID != "fixed-task" || res.Status != SchemaScanTaskRunning {
		t.Fatalf("unexpected result: %+v", res)
	}

	snap, sErr := store.GetSchemaScanStatus("fixed-task")
	if sErr != nil {
		t.Fatalf("status: %v", sErr)
	}
	if snap.Scope != SchemaScanScopeTables {
		t.Fatalf("scope: %s", snap.Scope)
	}
	if len(snap.TableNames) != 2 {
		t.Fatalf("table names: %v", snap.TableNames)
	}
	if snap.Trigger != "manual" {
		t.Fatalf("trigger: %s", snap.Trigger)
	}
	if snap.RescanReason != "" || snap.RescanStrategy != "" {
		t.Fatalf("rescan fields should be empty for normal scan")
	}
}

func TestSchemaScanStarter_StartSchemaRescan_Full(t *testing.T) {
	store := NewSchemaScanRuntimeStore()
	st := NewSchemaScanStarterWithTaskID(store, func() string { return "r1" })

	res, err := st.StartSchemaRescan(context.Background(), StartSchemaRescanRequest{
		ConnectionID: "c1",
		Strategy:     SchemaRescanStrategyFull,
		Reason:       "connection dsn changed",
	})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if res.Status != SchemaScanTaskRunning {
		t.Fatal(res.Status)
	}

	snap, sErr := store.GetSchemaScanStatus("r1")
	if sErr != nil {
		t.Fatalf("status: %v", sErr)
	}
	if snap.Scope != SchemaScanScopeAll {
		t.Fatalf("scope %s", snap.Scope)
	}
	if len(snap.TableNames) != 0 {
		t.Fatalf("tables: %v", snap.TableNames)
	}
	if snap.RescanStrategy != string(SchemaRescanStrategyFull) {
		t.Fatalf("strategy %s", snap.RescanStrategy)
	}
	if snap.RescanReason != "connection dsn changed" {
		t.Fatalf("reason %q", snap.RescanReason)
	}
	if snap.Trigger != SchemaScanTriggerRescanFull {
		t.Fatalf("trigger %s", snap.Trigger)
	}
}

func TestSchemaScanStarter_StartSchemaRescan_Impacted(t *testing.T) {
	store := NewSchemaScanRuntimeStore()
	st := NewSchemaScanStarterWithTaskID(store, func() string { return "r2" })

	res, err := st.StartSchemaRescan(context.Background(), StartSchemaRescanRequest{
		ConnectionID:       "c1",
		Strategy:           SchemaRescanStrategyImpacted,
		Reason:             "diff detected fk targets",
		ImpactedTableNames: []string{"orders", " users "},
	})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if res.TaskID != "r2" {
		t.Fatal(res.TaskID)
	}

	snap, sErr := store.GetSchemaScanStatus("r2")
	if sErr != nil {
		t.Fatalf("status: %v", sErr)
	}
	if snap.Scope != SchemaScanScopeTables {
		t.Fatalf("scope %s", snap.Scope)
	}
	if len(snap.TableNames) != 2 || !strings.EqualFold(snap.TableNames[0], "orders") {
		t.Fatalf("tables %v", snap.TableNames)
	}
	if snap.RescanStrategy != string(SchemaRescanStrategyImpacted) {
		t.Fatalf("strategy %s", snap.RescanStrategy)
	}
	if snap.Trigger != SchemaScanTriggerRescanImpacted {
		t.Fatalf("trigger %s", snap.Trigger)
	}
}

func TestSchemaScanStarter_StartSchemaRescan_InvalidEmptyReason(t *testing.T) {
	st := NewSchemaScanStarter(NewSchemaScanRuntimeStore())
	_, err := st.StartSchemaRescan(context.Background(), StartSchemaRescanRequest{
		ConnectionID: "c1",
		Strategy:     SchemaRescanStrategyFull,
		Reason:       "   ",
	})
	if err == nil || err.Code != SchemaScanErrCodeInvalidArgument {
		t.Fatalf("err: %v", err)
	}
}

func TestSchemaScanStarter_StartSchemaRescan_InvalidImpactedWithoutTables(t *testing.T) {
	st := NewSchemaScanStarter(NewSchemaScanRuntimeStore())
	_, err := st.StartSchemaRescan(context.Background(), StartSchemaRescanRequest{
		ConnectionID: "c1",
		Strategy:     SchemaRescanStrategyImpacted,
		Reason:       "x",
	})
	if err == nil || err.Code != SchemaScanErrCodeInvalidArgument {
		t.Fatalf("err: %v", err)
	}
}
