package schema

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestSchemaScanRuntimeStore_GetSchemaScanStatusLifecycle(t *testing.T) {
	store := NewSchemaScanRuntimeStore()

	taskID := "task-1"
	store.StartTask(SchemaScanRuntimeStartRequest{
		TaskID:       taskID,
		ConnectionID: "conn-1",
		Scope:        SchemaScanScopeAll,
		Trigger:      "manual",
	})

	status, err := store.GetSchemaScanStatus(taskID)
	if err != nil {
		t.Fatalf("get running status: %v", err)
	}
	if status.Status != SchemaScanTaskRunning {
		t.Fatalf("expected running, got %s", status.Status)
	}
	if status.Progress != 0 {
		t.Fatalf("expected zero progress, got %f", status.Progress)
	}
	if status.PreviewReady {
		t.Fatal("running task should not be preview ready")
	}

	store.UpdateProgress(taskID, 0.56)
	store.MarkCompleted(taskID, true)

	status, err = store.GetSchemaScanStatus(taskID)
	if err != nil {
		t.Fatalf("get completed status: %v", err)
	}
	if status.Status != SchemaScanTaskCompleted {
		t.Fatalf("expected completed, got %s", status.Status)
	}
	if status.Progress != 1 {
		t.Fatalf("expected completed progress 1, got %f", status.Progress)
	}
	if !status.PreviewReady {
		t.Fatal("completed task should be preview ready")
	}
}

func TestSchemaScanRuntimeStore_GetSchemaScanStatusFailedWithSanitizedError(t *testing.T) {
	store := NewSchemaScanRuntimeStore()
	taskID := "task-2"
	store.StartTask(SchemaScanRuntimeStartRequest{
		TaskID:       taskID,
		ConnectionID: "conn-2",
		Scope:        SchemaScanScopeTables,
		TableNames:   []string{"users"},
		Trigger:      "manual",
	})

	rawErr := errors.New("permission denied password=secret-pwd token=abc")
	store.MarkFailed(context.Background(), taskID, rawErr, "secret-pwd", "abc")

	status, err := store.GetSchemaScanStatus(taskID)
	if err != nil {
		t.Fatalf("get failed status: %v", err)
	}
	if status.Status != SchemaScanTaskFailed {
		t.Fatalf("expected failed, got %s", status.Status)
	}
	if status.ErrorCode != UpstreamCodePermissionDenied {
		t.Fatalf("expected %s, got %s", UpstreamCodePermissionDenied, status.ErrorCode)
	}
	if status.ErrorMessage == "" {
		t.Fatal("expected error message")
	}
	if containsSensitiveValue(status.ErrorMessage, "secret-pwd", "abc") {
		t.Fatalf("error message leaks sensitive info: %s", status.ErrorMessage)
	}
}

func TestSchemaScanRuntimeStore_GetSchemaScanStatusCancelled(t *testing.T) {
	store := NewSchemaScanRuntimeStore()
	taskID := "task-3"
	store.StartTask(SchemaScanRuntimeStartRequest{
		TaskID:       taskID,
		ConnectionID: "conn-3",
		Scope:        SchemaScanScopeAll,
		Trigger:      "manual",
	})

	store.CancelTask(taskID)
	status, err := store.GetSchemaScanStatus(taskID)
	if err != nil {
		t.Fatalf("get cancelled status: %v", err)
	}
	if status.Status != SchemaScanTaskCancelled {
		t.Fatalf("expected cancelled, got %s", status.Status)
	}
	if status.ErrorCode != "" || status.ErrorMessage != "" {
		t.Fatal("cancelled status should not contain failure payload")
	}
}

func TestSchemaScanRuntimeStore_GetSchemaScanStatusTaskNotFound(t *testing.T) {
	store := NewSchemaScanRuntimeStore()

	_, err := store.GetSchemaScanStatus("missing-task")
	if err == nil {
		t.Fatal("expected task not found error")
	}
	if err.Code != SchemaScanStatusErrCodeTaskNotFound {
		t.Fatalf("expected %s, got %s", SchemaScanStatusErrCodeTaskNotFound, err.Code)
	}
}

func TestSchemaScanRuntimeStore_GetRuntimeContext(t *testing.T) {
	store := NewSchemaScanRuntimeStore()
	taskID := "task-ctx-1"
	store.StartTask(SchemaScanRuntimeStartRequest{
		TaskID:       taskID,
		ConnectionID: "conn-ctx",
		Scope:        SchemaScanScopeAll,
		Trigger:      "manual",
	})
	taskCtx, ok := store.GetRuntimeContext(taskID)
	if !ok {
		t.Fatal("expected context")
	}
	if taskCtx.ConnectionID != "conn-ctx" || taskCtx.TaskID != taskID {
		t.Fatalf("unexpected ctx: %+v", taskCtx)
	}
	if _, ok := store.GetRuntimeContext("missing"); ok {
		t.Fatal("expected missing")
	}
}

func containsSensitiveValue(v string, secrets ...string) bool {
	for _, s := range secrets {
		if s != "" && strings.Contains(v, s) {
			return true
		}
	}
	return false
}
