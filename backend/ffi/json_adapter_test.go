package ffi_test

import (
	"encoding/json"
	"strings"
	"testing"

	"loomidbx/backend/app"
	"loomidbx/backend/ffi"
	"loomidbx/backend/storage"
)

// FFI 响应应包含 ok/data/error 结构。
func TestFFIResponseStructure(t *testing.T) {
	adapter, _ := newAdapter(t)

	// TestConnection 成功响应
	resp := adapter.TestConnectionJSON("{\"db_type\":\"sqlite\",\"database\":\"test.db\"}")
	if !hasOkField(resp) {
		t.Errorf("TestConnection response missing 'ok' field: %s", resp)
	}

	// SaveConnection 成功响应
	resp = adapter.SaveConnectionJSON("{\"name\":\"test\",\"db_type\":\"sqlite\"}")
	if !hasOkField(resp) {
		t.Errorf("SaveConnection response missing 'ok' field: %s", resp)
	}
	if !hasDataField(resp) {
		t.Errorf("SaveConnection response missing 'data' field: %s", resp)
	}

	// ListConnections 成功响应
	resp = adapter.ListConnectionsJSON()
	if !hasOkField(resp) {
		t.Errorf("ListConnections response missing 'ok' field: %s", resp)
	}
	if !hasDataField(resp) {
		t.Errorf("ListConnections response missing 'data' field: %s", resp)
	}
}

// SaveConnection 响应应返回连接 ID 且不含明文密码。
func TestSaveConnectionResponseNoPassword(t *testing.T) {
	adapter, _ := newAdapter(t)

	reqJSON := "{\"name\":\"test-conn\",\"db_type\":\"sqlite\",\"password\":\"secret123\"}"
	resp := adapter.SaveConnectionJSON(reqJSON)

	var ffiResp ffi.FFIResponse
	if err := json.Unmarshal([]byte(resp), &ffiResp); err != nil {
		t.Fatalf("parse response failed: %v", err)
	}

	if !ffiResp.Ok {
		t.Fatalf("save failed: %s", resp)
	}

	// 检查 data 中包含 id
	data, ok := ffiResp.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("data is not a map: %v", ffiResp.Data)
	}
	if data["id"] == nil || data["id"] == "" {
		t.Errorf("response missing 'id' in data: %v", data)
	}

	// 检查响应中不包含 password 字段
	if strings.Contains(resp, "password") {
		t.Errorf("response contains 'password' field: %s", resp)
	}
	if strings.Contains(resp, "secret123") {
		t.Errorf("response leaks plaintext password: %s", resp)
	}
}

// ListConnections 响应应不含明文密码。
func TestListConnectionsResponseNoPassword(t *testing.T) {
	adapter, _ := newAdapter(t)

	// 先保存一个带密码的连接
	_ = adapter.SaveConnectionJSON("{\"name\":\"pwd-conn\",\"db_type\":\"mysql\",\"password\":\"my-secret-pwd\"}")

	// 列出连接
	resp := adapter.ListConnectionsJSON()

	var ffiResp ffi.FFIResponse
	if err := json.Unmarshal([]byte(resp), &ffiResp); err != nil {
		t.Fatalf("parse response failed: %v", err)
	}

	if !ffiResp.Ok {
		t.Fatalf("list failed: %s", resp)
	}

	// 检查响应中不包含 password 字段或明文密码
	if strings.Contains(resp, "\"password\"") {
		t.Errorf("ListConnections response contains 'password' field: %s", resp)
	}
	if strings.Contains(resp, "my-secret-pwd") {
		t.Errorf("ListConnections response leaks plaintext password: %s", resp)
	}
}

// DeleteConnection 未携带确认标志应返回 CONFIRMATION_REQUIRED。
func TestDeleteConnectionRequiresConfirmation(t *testing.T) {
	adapter, _ := newAdapter(t)

	// 先保存连接
	resp := adapter.SaveConnectionJSON("{\"name\":\"del-conn\",\"db_type\":\"sqlite\"}")
	var saveResp ffi.FFIResponse
	if err := json.Unmarshal([]byte(resp), &saveResp); err != nil {
		t.Fatalf("parse save response failed: %v", err)
	}
	data := saveResp.Data.(map[string]interface{})
	id := data["id"].(string)

	// 删除不携带确认标志
	resp = adapter.DeleteConnectionJSON("{\"id\":\"" + id + "\",\"confirm_cascade\":false}")

	var ffiResp ffi.FFIResponse
	if err := json.Unmarshal([]byte(resp), &ffiResp); err != nil {
		t.Fatalf("parse response failed: %v", err)
	}

	if ffiResp.Ok {
		t.Fatal("delete without confirmation should fail")
	}

	if ffiResp.Error == nil {
		t.Fatal("error should be present")
	}

	if ffiResp.Error.Code != app.CodeConfirmationRequired {
		t.Errorf("expected CONFIRMATION_REQUIRED, got: %s", ffiResp.Error.Code)
	}
}

// DeleteConnection 携带确认标志应成功。
func TestDeleteConnectionWithConfirmation(t *testing.T) {
	adapter, _ := newAdapter(t)

	// 先保存连接
	resp := adapter.SaveConnectionJSON("{\"name\":\"del-confirmed\",\"db_type\":\"sqlite\"}")
	var saveResp ffi.FFIResponse
	if err := json.Unmarshal([]byte(resp), &saveResp); err != nil {
		t.Fatalf("parse save response failed: %v", err)
	}
	data := saveResp.Data.(map[string]interface{})
	id := data["id"].(string)

	// 删除携带确认标志
	resp = adapter.DeleteConnectionJSON("{\"id\":\"" + id + "\",\"confirm_cascade\":true}")

	var ffiResp ffi.FFIResponse
	if err := json.Unmarshal([]byte(resp), &ffiResp); err != nil {
		t.Fatalf("parse response failed: %v", err)
	}

	if !ffiResp.Ok {
		t.Fatalf("delete with confirmation should succeed: %s", resp)
	}
}

// TestConnection 失败应返回结构化错误。
func TestConnectionFailureStructuredError(t *testing.T) {
	adapter, _ := newAdapter(t)

	resp := adapter.TestConnectionJSON("{\"db_type\":\"oracle\"}")

	var ffiResp ffi.FFIResponse
	if err := json.Unmarshal([]byte(resp), &ffiResp); err != nil {
		t.Fatalf("parse response failed: %v", err)
	}

	if ffiResp.Ok {
		t.Fatal("test unsupported db_type should fail")
	}

	if ffiResp.Error == nil {
		t.Fatal("error should be present")
	}

	// 错误对象应有稳定错误码
	if ffiResp.Error.Code == "" {
		t.Error("error code should not be empty")
	}
	if ffiResp.Error.Message == "" {
		t.Error("error message should not be empty")
	}
}

// 错误详情不应包含明文密码。
func TestErrorDetailsNoPasswordLeak(t *testing.T) {
	adapter, _ := newAdapter(t)

	secret := "super-secret-password-xyz"
	resp := adapter.TestConnectionJSON("{\"db_type\":\"mysql\",\"host\":\"10.255.255.1\",\"port\":3306,\"password\":\"" + secret + "\",\"timeout_sec\":2}")

	var ffiResp ffi.FFIResponse
	if err := json.Unmarshal([]byte(resp), &ffiResp); err != nil {
		t.Fatalf("parse response failed: %v", err)
	}

	if ffiResp.Ok {
		t.Fatal("test should fail for unreachable host")
	}

	// 检查错误详情不包含明文密码
	if ffiResp.Error != nil && ffiResp.Error.Details != nil {
		for k, v := range ffiResp.Error.Details {
			if strings.Contains(v, secret) {
				t.Errorf("error detail %s leaks password: %s", k, v)
			}
		}
	}

	// 检查整个响应不包含明文密码
	if strings.Contains(resp, secret) {
		t.Errorf("response leaks plaintext password: %s", resp)
	}
}

// 错误码应为稳定字符串。
func TestErrorCodesAreStable(t *testing.T) {
	expectedCodes := []string{
		app.CodeInvalidArgument,
		app.CodeStorageError,
		app.CodeNotFound,
		app.CodeConfirmationRequired,
		app.CodeDeadlineExceeded,
		app.CodeUpstreamUnavailable,
		app.CodeAuthFailed,
		app.CodeTLSError,
		app.CodeProtocolError,
		app.CodeKeyringUnavailable,
		app.CodeKeyringAccessDenied,
	}

	for _, code := range expectedCodes {
		if code == "" {
			t.Errorf("error code should not be empty")
		}
		// 错误码应为大写蛇形命名
		if strings.Contains(code, " ") {
			t.Errorf("error code %s should not contain spaces", code)
		}
	}
}

// JSON 解析失败应返回 INVALID_ARGUMENT。
func TestInvalidJSONReturnsInvalidArgument(t *testing.T) {
	adapter, _ := newAdapter(t)

	resp := adapter.TestConnectionJSON("not-valid-json")

	var ffiResp ffi.FFIResponse
	if err := json.Unmarshal([]byte(resp), &ffiResp); err != nil {
		t.Fatalf("parse response failed: %v", err)
	}

	if ffiResp.Ok {
		t.Fatal("invalid JSON should fail")
	}

	if ffiResp.Error.Code != app.CodeInvalidArgument {
		t.Errorf("expected INVALID_ARGUMENT, got: %s", ffiResp.Error.Code)
	}
}

// ===== 辅助函数 =====

func newAdapter(t *testing.T) (*ffi.FFIAdapter, *storage.ConnectionStore) {
	store := newTestStore(t)
	svc := app.NewConnectionService(store)
	return ffi.NewFFIAdapter(svc), store
}

func newTestStore(t *testing.T) *storage.ConnectionStore {
	tmp := t.TempDir() + "/meta.db"
	store, err := storage.NewConnectionStore(tmp)
	if err != nil {
		t.Fatalf("create store failed: %v", err)
	}
	t.Cleanup(func() {
		if closeErr := store.Close(); closeErr != nil {
			t.Fatalf("close store failed: %v", closeErr)
		}
	})
	return store
}

func hasOkField(jsonStr string) bool {
	return strings.Contains(jsonStr, "\"ok\":")
}

func hasDataField(jsonStr string) bool {
	return strings.Contains(jsonStr, "\"data\":")
}