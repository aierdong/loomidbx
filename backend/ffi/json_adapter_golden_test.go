package ffi_test

import (
	"encoding/json"
	"strings"
	"testing"

	"loomidbx/app"
	"loomidbx/ffi"
	"loomidbx/storage"
)

// ===== 5.3 FFI Golden/契约快照测试 =====

// 本组测试冻结 FFI JSON 响应形状，供后续契约演进回归验证。
// 所有快照遵循 docs/schema.md §六 的统一格式 {"ok": true/false, "data": ..., "error": {...}}。

// ===== 成功响应快照 =====

// TestConnection 成功响应快照格式验证。
func TestGolden_TestConnection_SuccessFormat(t *testing.T) {
	adapter, _ := newGoldenAdapter(t)

	resp := adapter.TestConnectionJSON("{\"db_type\":\"sqlite\",\"database\":\"test.db\"}")

	// 验收：响应必须包含 ok:true
	if !strings.Contains(resp, "\"ok\":true") {
		t.Errorf("TestConnection success response should contain ok:true: %s", resp)
	}

	// 验收：响应不包含 data（TestConnection 成功仅返回 ok）
	var parsed ffi.FFIResponse
	if err := json.Unmarshal([]byte(resp), &parsed); err != nil {
		t.Fatalf("parse response failed: %v", err)
	}
	if parsed.Ok != true {
		t.Errorf("ok should be true")
	}
	// TestConnection 成功时 data 为 nil 或不返回
	if parsed.Error != nil {
		t.Errorf("error should be nil on success: %v", parsed.Error)
	}
}

// SaveConnection 成功响应快照格式验证。
func TestGolden_SaveConnection_SuccessFormat(t *testing.T) {
	adapter, _ := newGoldenAdapter(t)

	resp := adapter.SaveConnectionJSON("{\"name\":\"golden-test\",\"db_type\":\"sqlite\"}")

	var parsed ffi.FFIResponse
	if err := json.Unmarshal([]byte(resp), &parsed); err != nil {
		t.Fatalf("parse response failed: %v", err)
	}

	// 验收：ok 必须为 true
	if parsed.Ok != true {
		t.Errorf("ok should be true, got response: %s", resp)
	}

	// 验收：data 必须包含 id 字段（UUID 格式）
	data, ok := parsed.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("data should be a map: %v", parsed.Data)
	}
	if data["id"] == nil || data["id"] == "" {
		t.Errorf("data.id should be non-empty UUID")
	}

	// 验收：响应结构符合契约
	if !strings.Contains(resp, "\"ok\":true") {
		t.Errorf("response should contain ok:true")
	}
	if !strings.Contains(resp, "\"data\":") {
		t.Errorf("response should contain data field")
	}
	if !strings.Contains(resp, "\"id\":") {
		t.Errorf("response should contain id in data")
	}

	// 验收：响应不包含 password 字段
	if strings.Contains(resp, "\"password\"") {
		t.Errorf("response should not contain password field: %s", resp)
	}
}

// ListConnections 成功响应快照格式验证。
func TestGolden_ListConnections_SuccessFormat(t *testing.T) {
	adapter, _ := newGoldenAdapter(t)

	// 先保存一个连接
	_ = adapter.SaveConnectionJSON("{\"name\":\"list-golden\",\"db_type\":\"mysql\",\"host\":\"127.0.0.1\"}")

	resp := adapter.ListConnectionsJSON()

	var parsed ffi.FFIResponse
	if err := json.Unmarshal([]byte(resp), &parsed); err != nil {
		t.Fatalf("parse response failed: %v", err)
	}

	// 验收：ok 必须为 true
	if parsed.Ok != true {
		t.Errorf("ok should be true, got response: %s", resp)
	}

	// 验收：data 必须是数组
	data, ok := parsed.Data.([]interface{})
	if !ok {
		t.Fatalf("data should be an array: %v", parsed.Data)
	}
	if len(data) == 0 {
		t.Errorf("data array should not be empty after save")
	}

	// 验收：数组元素必须包含 ConnectionSummary 字段
	for _, item := range data {
		conn, ok := item.(map[string]interface{})
		if !ok {
			t.Fatalf("connection item should be a map: %v", item)
		}
		// 必须字段：id, name, db_type
		if conn["id"] == nil {
			t.Errorf("connection missing id")
		}
		if conn["name"] == nil {
			t.Errorf("connection missing name")
		}
		if conn["db_type"] == nil {
			t.Errorf("connection missing db_type")
		}
		// 禁止字段：password
		if conn["password"] != nil {
			t.Errorf("connection should not have password field")
		}
	}

	// 验收：响应不包含 password 字段
	if strings.Contains(resp, "\"password\"") {
		t.Errorf("ListConnections response should not contain password: %s", resp)
	}
}

// DeleteConnection 成功响应快照格式验证。
func TestGolden_DeleteConnection_SuccessFormat(t *testing.T) {
	adapter, _ := newGoldenAdapter(t)

	// 先保存一个连接
	saveResp := adapter.SaveConnectionJSON("{\"name\":\"delete-golden\",\"db_type\":\"sqlite\"}")
	var saveParsed ffi.FFIResponse
	json.Unmarshal([]byte(saveResp), &saveParsed)
	data := saveParsed.Data.(map[string]interface{})
	id := data["id"].(string)

	// 删除（带确认）
	resp := adapter.DeleteConnectionJSON("{\"id\":\"" + id + "\",\"confirm_cascade\":true}")

	// 验收：响应符合格式
	if !strings.Contains(resp, "\"ok\":true") {
		t.Errorf("DeleteConnection success response should contain ok:true: %s", resp)
	}

	var parsed ffi.FFIResponse
	if err := json.Unmarshal([]byte(resp), &parsed); err != nil {
		t.Fatalf("parse response failed: %v", err)
	}
	if parsed.Ok != true {
		t.Errorf("ok should be true")
	}
}

// ===== 错误响应快照 =====

// TestConnection 不支持的 db_type 错误快照格式验证。
func TestGolden_TestConnection_UnsupportedDbType(t *testing.T) {
	adapter, _ := newGoldenAdapter(t)

	resp := adapter.TestConnectionJSON("{\"db_type\":\"oracle\"}")

	var parsed ffi.FFIResponse
	if err := json.Unmarshal([]byte(resp), &parsed); err != nil {
		t.Fatalf("parse response failed: %v", err)
	}

	// 验收：ok 必须为 false
	if parsed.Ok != false {
		t.Errorf("ok should be false for error")
	}

	// 验收：error 必须存在且包含 code/message
	if parsed.Error == nil {
		t.Fatal("error should be present")
	}
	if parsed.Error.Code == "" {
		t.Error("error.code should be non-empty")
	}
	if parsed.Error.Message == "" {
		t.Error("error.message should be non-empty")
	}

	// 验收：错误码为 PROTOCOL_ERROR
	if parsed.Error.Code != app.CodeProtocolError {
		t.Errorf("expected PROTOCOL_ERROR, got: %s", parsed.Error.Code)
	}

	// 验收：响应结构符合错误模板 {"ok":false,"error":{"code":"...","message":"..."}}
	if !strings.Contains(resp, "\"ok\":false") {
		t.Errorf("error response should contain ok:false")
	}
	if !strings.Contains(resp, "\"error\":") {
		t.Errorf("error response should contain error field")
	}
	if !strings.Contains(resp, "\"code\":") {
		t.Errorf("error response should contain code field")
	}
	if !strings.Contains(resp, "\"message\":") {
		t.Errorf("error response should contain message field")
	}
}

// DeleteConnection 未确认错误快照格式验证。
func TestGolden_DeleteConnection_RequiresConfirmation(t *testing.T) {
	adapter, _ := newGoldenAdapter(t)

	// 先保存一个连接
	saveResp := adapter.SaveConnectionJSON("{\"name\":\"delete-confirm-golden\",\"db_type\":\"sqlite\"}")
	var saveParsed ffi.FFIResponse
	json.Unmarshal([]byte(saveResp), &saveParsed)
	data := saveParsed.Data.(map[string]interface{})
	id := data["id"].(string)

	// 删除不带确认
	resp := adapter.DeleteConnectionJSON("{\"id\":\"" + id + "\",\"confirm_cascade\":false}")

	var parsed ffi.FFIResponse
	if err := json.Unmarshal([]byte(resp), &parsed); err != nil {
		t.Fatalf("parse response failed: %v", err)
	}

	// 验收：ok 为 false
	if parsed.Ok != false {
		t.Errorf("ok should be false")
	}

	// 验收：error.code 为 CONFIRMATION_REQUIRED
	if parsed.Error == nil {
		t.Fatal("error should be present")
	}
	if parsed.Error.Code != app.CodeConfirmationRequired {
		t.Errorf("expected CONFIRMATION_REQUIRED, got: %s", parsed.Error.Code)
	}

	// 验收：响应结构符合错误模板
	if !strings.Contains(resp, "\"ok\":false") {
		t.Errorf("error response should contain ok:false")
	}
}

// JSON 解析错误快照格式验证。
func TestGolden_InvalidJSON(t *testing.T) {
	adapter, _ := newGoldenAdapter(t)

	resp := adapter.TestConnectionJSON("not-valid-json")

	var parsed ffi.FFIResponse
	if err := json.Unmarshal([]byte(resp), &parsed); err != nil {
		t.Fatalf("parse response failed: %v", err)
	}

	// 验收：ok 为 false
	if parsed.Ok != false {
		t.Errorf("ok should be false for invalid JSON")
	}

	// 验收：error.code 为 INVALID_ARGUMENT
	if parsed.Error == nil {
		t.Fatal("error should be present")
	}
	if parsed.Error.Code != app.CodeInvalidArgument {
		t.Errorf("expected INVALID_ARGUMENT, got: %s", parsed.Error.Code)
	}

	// 验收：error.message 提示 JSON 解析问题
	if !strings.Contains(parsed.Error.Message, "JSON") && !strings.Contains(parsed.Error.Message, "json") {
		t.Errorf("error message should mention JSON: %s", parsed.Error.Message)
	}
}

// ===== 错误码完整性快照 =====

// 所有错误码定义快照（冻结供后续契约演进对比）。
var goldenErrorCodes = map[string]string{
	"INVALID_ARGUMENT":     app.CodeInvalidArgument,
	"STORAGE_ERROR":        app.CodeStorageError,
	"NOT_FOUND":            app.CodeNotFound,
	"CONFIRMATION_REQUIRED": app.CodeConfirmationRequired,
	"DEADLINE_EXCEEDED":    app.CodeDeadlineExceeded,
	"UPSTREAM_UNAVAILABLE": app.CodeUpstreamUnavailable,
	"AUTH_FAILED":          app.CodeAuthFailed,
	"TLS_ERROR":            app.CodeTLSError,
	"PROTOCOL_ERROR":       app.CodeProtocolError,
	"KEYRING_UNAVAILABLE":  app.CodeKeyringUnavailable,
	"KEYRING_ACCESS_DENIED": app.CodeKeyringAccessDenied,
}

func TestGolden_ErrorCodesSnapshot(t *testing.T) {
	// 验收：所有 golden 错误码与实际定义一致
	for goldenName, actualCode := range goldenErrorCodes {
		if actualCode != goldenName {
			t.Errorf("error code mismatch: expected %s, got %s", goldenName, actualCode)
		}
		// 验收：错误码格式正确（大写蛇形命名）
		if strings.Contains(actualCode, " ") {
			t.Errorf("error code %s should not contain spaces", actualCode)
		}
		if strings.ToLower(actualCode) == actualCode && actualCode != "" {
			t.Errorf("error code %s should be uppercase", actualCode)
		}
	}
}

// ===== 脱敏快照 =====

// 错误响应不泄露敏感值快照验证。
func TestGolden_ErrorResponseNoSensitiveLeak(t *testing.T) {
	adapter, _ := newGoldenAdapter(t)

	secret := "golden-secret-password-xyz"
	resp := adapter.TestConnectionJSON("{\"db_type\":\"mysql\",\"host\":\"10.255.255.1\",\"port\":3306,\"password\":\"" + secret + "\",\"timeout_sec\":2}")

	// 验收：响应不包含明文密码
	if strings.Contains(resp, secret) {
		t.Errorf("response leaks plaintext password: %s", resp)
	}

	// 验收：error.details 不包含密码
	var parsed ffi.FFIResponse
	json.Unmarshal([]byte(resp), &parsed)
	if parsed.Error != nil && parsed.Error.Details != nil {
		for k, v := range parsed.Error.Details {
			if strings.Contains(v, secret) {
				t.Errorf("error detail %s leaks password: %s", k, v)
			}
		}
	}
}

// ===== 辅助函数 =====

func newGoldenAdapter(t *testing.T) (*ffi.FFIAdapter, *storage.ConnectionStore) {
	return newAdapter(t) // 复用 newAdapter 的 t.Cleanup 关闭逻辑
}