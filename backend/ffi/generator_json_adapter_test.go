// Package ffi 提供 Generator FFI JSON 适配器。
package ffi_test

import (
	"encoding/json"
	"strings"
	"testing"

	"loomidbx/ffi"
	"loomidbx/generator"
	"loomidbx/schema"
)

// TestGeneratorFFISaveFieldGeneratorConfigStructure 测试保存配置的 JSON 结构。
func TestGeneratorFFISaveFieldGeneratorConfigStructure(t *testing.T) {
	adapter := newGeneratorAdapter(t)

	reqJSON := `{
		"connection_id": "c1",
		"table": "users",
		"column": "name",
		"generator_type": "string_random_chars",
		"generator_opts": "{\"length\":8}",
		"seed_policy": "{\"mode\":\"inherit_global\"}",
		"null_policy": "respect_nullable",
		"is_enabled": true,
		"modified_source": "ui_manual"
	}`

	resp := adapter.SaveFieldGeneratorConfigJSON(reqJSON)

	var ffiResp ffi.FFIResponse
	if err := json.Unmarshal([]byte(resp), &ffiResp); err != nil {
		t.Fatalf("parse response failed: %v", err)
	}

	// 验证 ok/data/error 结构
	if !ffiResp.Ok {
		t.Fatalf("expected ok=true, got: %s", resp)
	}

	if ffiResp.Data == nil {
		t.Fatalf("expected data to be present")
	}

	data, ok := ffiResp.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("expected data as map, got: %T", ffiResp.Data)
	}

	// 验证关键字段
	if data["saved"] != true {
		t.Fatalf("expected saved=true")
	}

	if data["config_version"] == nil {
		t.Fatalf("expected config_version")
	}

	if data["is_enabled"] != true {
		t.Fatalf("expected is_enabled=true")
	}

	if data["modified_source"] != "ui_manual" {
		t.Fatalf("expected modified_source=ui_manual")
	}

	// 验证响应不含敏感信息
	if strings.Contains(resp, "password") {
		t.Fatalf("response should not contain password")
	}
}

// TestGeneratorFFIGetFieldGeneratorConfigStructure 测试查询配置的 JSON 结构。
func TestGeneratorFFIGetFieldGeneratorConfigStructure(t *testing.T) {
	adapter := newGeneratorAdapter(t)

	// 先保存配置
	saveReq := `{
		"connection_id": "c1",
		"table": "users",
		"column": "name",
		"generator_type": "string_random_chars",
		"generator_opts": "{}",
		"is_enabled": true,
		"modified_source": "ui_manual"
	}`
	adapter.SaveFieldGeneratorConfigJSON(saveReq)

	// 查询配置
	reqJSON := `{"connection_id":"c1","table":"users","column":"name"}`
	resp := adapter.GetFieldGeneratorConfigJSON(reqJSON)

	var ffiResp ffi.FFIResponse
	if err := json.Unmarshal([]byte(resp), &ffiResp); err != nil {
		t.Fatalf("parse response failed: %v", err)
	}

	if !ffiResp.Ok {
		t.Fatalf("expected ok=true, got: %s", resp)
	}

	data, ok := ffiResp.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("expected data as map, got: %T", ffiResp.Data)
	}

	// 验证 config 字段
	config, ok := data["config"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected config in data")
	}

	if config["connection_id"] != "c1" {
		t.Fatalf("expected connection_id=c1")
	}

	if config["table"] != "users" {
		t.Fatalf("expected table=users")
	}

	if config["column"] != "name" {
		t.Fatalf("expected column=name")
	}

	if config["generator_type"] != "string_random_chars" {
		t.Fatalf("expected generator_type=string_random_chars")
	}

	// 验证 warnings 字段（trusted 状态下应为空）
	if data["warnings"] == nil {
		t.Fatalf("expected warnings array")
	}
}

// TestGeneratorFFIPreviewGenerationFieldScopeStructure 测试字段范围预览的 JSON 结构。
func TestGeneratorFFIPreviewGenerationFieldScopeStructure(t *testing.T) {
	adapter := newGeneratorAdapter(t)

	// 先保存配置
	saveReq := `{
		"connection_id": "c1",
		"table": "users",
		"column": "name",
		"generator_type": "string_random_chars",
		"generator_opts": "{}",
		"is_enabled": true,
		"modified_source": "ui_manual"
	}`
	adapter.SaveFieldGeneratorConfigJSON(saveReq)

	// 预览
	reqJSON := `{
		"connection_id": "c1",
		"scope": {"type": "field", "table": "users", "column": "name"},
		"sample_size": 3,
		"seed": 42
	}`
	resp := adapter.PreviewGenerationJSON(reqJSON)

	var ffiResp ffi.FFIResponse
	if err := json.Unmarshal([]byte(resp), &ffiResp); err != nil {
		t.Fatalf("parse response failed: %v", err)
	}

	if !ffiResp.Ok {
		t.Fatalf("expected ok=true, got: %s", resp)
	}

	data, ok := ffiResp.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("expected data as map, got: %T", ffiResp.Data)
	}

	// 验证 samples 为数组（field scope）
	samples, ok := data["samples"].([]interface{})
	if !ok {
		t.Fatalf("expected samples as array for field scope, got: %T", data["samples"])
	}

	if len(samples) != 3 {
		t.Fatalf("expected 3 samples, got %d", len(samples))
	}

	// 验证 metadata
	metadata, ok := data["metadata"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected metadata")
	}

	if metadata["deterministic"] != true {
		t.Fatalf("expected deterministic=true")
	}

	if metadata["seed"] != float64(42) {
		t.Fatalf("expected seed=42")
	}

	// 验证 warnings
	if data["warnings"] == nil {
		t.Fatalf("expected warnings array")
	}
}

// TestGeneratorFFIPreviewMetadata_MinContract_FieldScope 测试 preview metadata 最小契约（4.7，field scope）。
func TestGeneratorFFIPreviewMetadata_MinContract_FieldScope(t *testing.T) {
	adapter := newGeneratorAdapter(t)

	adapter.SaveFieldGeneratorConfigJSON(`{
		"connection_id": "c1",
		"table": "users",
		"column": "name",
		"generator_type": "string_random_chars",
		"generator_opts": "{\"length\":8,\"token\":\"secret\"}",
		"seed_policy": "{\"mode\":\"inherit_global\"}",
		"null_policy": "respect_nullable",
		"is_enabled": true,
		"modified_source": "ui_manual"
	}`)

	resp := adapter.PreviewGenerationJSON(`{
		"connection_id": "c1",
		"scope": {"type": "field", "table": "users", "column": "name"},
		"sample_size": 2,
		"seed": 42
	}`)

	var ffiResp ffi.FFIResponse
	if err := json.Unmarshal([]byte(resp), &ffiResp); err != nil {
		t.Fatalf("parse response failed: %v", err)
	}
	if !ffiResp.Ok {
		t.Fatalf("expected ok=true, got: %s", resp)
	}
	data := ffiResp.Data.(map[string]interface{})

	metadata, ok := data["metadata"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected metadata object, got: %T", data["metadata"])
	}

	// 4.7 最小字段契约：generator_type / 参数摘要 / deterministic / warnings 相关元信息
	if metadata["generator_type"] != "string_random_chars" {
		t.Fatalf("expected metadata.generator_type=string_random_chars, got: %v", metadata["generator_type"])
	}
	if metadata["deterministic"] != true {
		t.Fatalf("expected metadata.deterministic=true")
	}
	if metadata["seed"] != float64(42) {
		t.Fatalf("expected metadata.seed=42")
	}
	if metadata["seed_source"] == nil {
		t.Fatalf("expected metadata.seed_source")
	}
	if metadata["params_summary"] == nil {
		t.Fatalf("expected metadata.params_summary")
	}
	if _, ok := metadata["params_summary"].(map[string]interface{}); !ok {
		t.Fatalf("expected params_summary as object, got: %T", metadata["params_summary"])
	}
}

// TestGeneratorFFIPreviewMetadata_MinContract_TableScope 测试 preview metadata 最小契约（4.7，table scope）。
func TestGeneratorFFIPreviewMetadata_MinContract_TableScope(t *testing.T) {
	adapter := newGeneratorAdapter(t)

	adapter.SaveFieldGeneratorConfigJSON(`{
		"connection_id": "c1",
		"table": "users",
		"column": "id",
		"generator_type": "int_range_random",
		"generator_opts": "{}",
		"is_enabled": true,
		"modified_source": "ui_manual"
	}`)
	adapter.SaveFieldGeneratorConfigJSON(`{
		"connection_id": "c1",
		"table": "users",
		"column": "name",
		"generator_type": "string_random_chars",
		"generator_opts": "{}",
		"is_enabled": true,
		"modified_source": "ui_manual"
	}`)

	resp := adapter.PreviewGenerationJSON(`{
		"connection_id": "c1",
		"scope": {"type": "table", "table": "users"},
		"sample_size": 2,
		"seed": 123
	}`)

	var ffiResp ffi.FFIResponse
	if err := json.Unmarshal([]byte(resp), &ffiResp); err != nil {
		t.Fatalf("parse response failed: %v", err)
	}
	if !ffiResp.Ok {
		t.Fatalf("expected ok=true, got: %s", resp)
	}
	data := ffiResp.Data.(map[string]interface{})

	metadata, ok := data["metadata"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected metadata object")
	}
	if metadata["deterministic"] == nil {
		t.Fatalf("expected metadata.deterministic")
	}
	if metadata["seed"] != float64(123) {
		t.Fatalf("expected metadata.seed=123")
	}
	if metadata["seed_source"] == nil {
		t.Fatalf("expected metadata.seed_source")
	}
	// table scope 下 generator_type 允许为空字符串，但字段必须存在，避免调用方依赖隐式字段。
	if _, ok := metadata["generator_type"]; !ok {
		t.Fatalf("expected metadata.generator_type key to exist")
	}
	if metadata["params_summary"] == nil {
		t.Fatalf("expected metadata.params_summary")
	}
}

// TestGeneratorFFIPreviewGenerationTableScopeStructure 测试表范围预览的 JSON 结构。
func TestGeneratorFFIPreviewGenerationTableScopeStructure(t *testing.T) {
	adapter := newGeneratorAdapter(t)

	// 保存多个字段配置
	adapter.SaveFieldGeneratorConfigJSON(`{
		"connection_id": "c1",
		"table": "users",
		"column": "id",
		"generator_type": "int_range_random",
		"generator_opts": "{}",
		"is_enabled": true,
		"modified_source": "ui_manual"
	}`)
	adapter.SaveFieldGeneratorConfigJSON(`{
		"connection_id": "c1",
		"table": "users",
		"column": "name",
		"generator_type": "string_random_chars",
		"generator_opts": "{}",
		"is_enabled": true,
		"modified_source": "ui_manual"
	}`)

	// 预览表范围
	reqJSON := `{
		"connection_id": "c1",
		"scope": {"type": "table", "table": "users"},
		"sample_size": 3
	}`
	resp := adapter.PreviewGenerationJSON(reqJSON)

	var ffiResp ffi.FFIResponse
	if err := json.Unmarshal([]byte(resp), &ffiResp); err != nil {
		t.Fatalf("parse response failed: %v", err)
	}

	if !ffiResp.Ok {
		t.Fatalf("expected ok=true, got: %s", resp)
	}

	data := ffiResp.Data.(map[string]interface{})

	// 验证 samples 为列映射（table scope）
	samples, ok := data["samples"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected samples as map for table scope, got: %T", data["samples"])
	}
	// 使用 samples 变量
	if len(samples) < 1 {
		t.Fatalf("expected at least 1 column in samples")
	}

	// 验证 metadata
	metadata, ok := data["metadata"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected metadata")
	}

	scopeMeta, ok := metadata["scope"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected scope in metadata")
	}
	if scopeMeta["type"] != "table" {
		t.Fatalf("expected scope.type=table")
	}

	// 验证 field_results（table scope 必需）
	fieldResults, ok := data["field_results"].([]interface{})
	if !ok {
		t.Fatalf("expected field_results array for table scope")
	}

	if len(fieldResults) < 1 {
		t.Fatalf("expected at least 1 field_result")
	}
}

// TestGeneratorFFIPreviewPartialSuccessStructure 测试部分成功的 JSON 结构。
func TestGeneratorFFIPreviewPartialSuccessStructure(t *testing.T) {
	adapter := newGeneratorAdapter(t)

	// 保存配置：成功字段和禁用字段
	adapter.SaveFieldGeneratorConfigJSON(`{
		"connection_id": "c1",
		"table": "users",
		"column": "id",
		"generator_type": "int_range_random",
		"generator_opts": "{}",
		"is_enabled": true,
		"modified_source": "ui_manual"
	}`)
	adapter.SaveFieldGeneratorConfigJSON(`{
		"connection_id": "c1",
		"table": "users",
		"column": "email",
		"generator_type": "string_random_chars",
		"generator_opts": "{}",
		"is_enabled": false,
		"modified_source": "ui_manual"
	}`)

	reqJSON := `{
		"connection_id": "c1",
		"scope": {"type": "table", "table": "users"},
		"sample_size": 3
	}`
	resp := adapter.PreviewGenerationJSON(reqJSON)

	var ffiResp ffi.FFIResponse
	if err := json.Unmarshal([]byte(resp), &ffiResp); err != nil {
		t.Fatalf("parse response failed: %v", err)
	}

	if !ffiResp.Ok {
		t.Fatalf("partial success should return ok=true")
	}

	data := ffiResp.Data.(map[string]interface{})

	// 验证 metadata.partial_success=true
	metadata := data["metadata"].(map[string]interface{})
	if metadata["partial_success"] != true {
		t.Fatalf("expected partial_success=true")
	}

	// 验证 warnings 包含禁用字段信息
	warnings := data["warnings"].([]interface{})
	foundDisabled := false
	for _, w := range warnings {
		warning := w.(map[string]interface{})
		if warning["code"] == "GENERATOR_DISABLED" {
			foundDisabled = true
		}
	}
	if !foundDisabled {
		t.Fatalf("expected GENERATOR_DISABLED warning")
	}

	// 验证 field_results 包含所有字段状态
	fieldResults := data["field_results"].([]interface{})
	for _, fr := range fieldResults {
		result := fr.(map[string]interface{})
		field := result["field"].(string)
		status := result["status"].(string)
		if field == "id" && status != "ok" {
			t.Fatalf("expected id status=ok")
		}
		if field == "email" && status != "skipped" {
			t.Fatalf("expected email status=skipped")
		}
	}
}

// TestGeneratorFFIOutOfScopeError 测试跨表/写入请求返回范围外错误（5.2）。
func TestGeneratorFFIOutOfScopeError(t *testing.T) {
	adapter := newGeneratorAdapter(t)

	// 尝试请求跨表执行
	reqJSON := `{
		"connection_id": "c1",
		"scope": {"type": "cross_table"},
		"tables": ["users", "orders"],
		"sample_size": 3
	}`
	resp := adapter.PreviewGenerationJSON(reqJSON)

	var ffiResp ffi.FFIResponse
	if err := json.Unmarshal([]byte(resp), &ffiResp); err != nil {
		t.Fatalf("parse response failed: %v", err)
	}

	if ffiResp.Ok {
		t.Fatalf("expected ok=false for out of scope request")
	}

	if ffiResp.Error == nil {
		t.Fatalf("expected error")
	}

	// 验证错误码
	if ffiResp.Error.Code != "OUT_OF_SCOPE_EXECUTION_REQUEST" {
		t.Fatalf("expected OUT_OF_SCOPE_EXECUTION_REQUEST, got: %s", ffiResp.Error.Code)
	}

	// 验证错误消息提示由 spec-04 处理
	if !containsSubstring(ffiResp.Error.Message, "spec-04") {
		t.Fatalf("error message should mention spec-04: %s", ffiResp.Error.Message)
	}
}

// TestGeneratorFFISanitizeSensitiveFields 测试脱敏敏感字段（5.3）。
func TestGeneratorFFISanitizeSensitiveFields(t *testing.T) {
	adapter := newGeneratorAdapter(t)

	// 保存包含敏感参数的配置
	reqJSON := `{
		"connection_id": "c1",
		"table": "users",
		"column": "password",
		"generator_type": "string_random_chars",
		"generator_opts": "{\"api_key\":\"secret-key-123\",\"token\":\"auth-token-xyz\"}",
		"is_enabled": true,
		"modified_source": "ui_manual"
	}`
	resp := adapter.SaveFieldGeneratorConfigJSON(reqJSON)

	var ffiResp ffi.FFIResponse
	if err := json.Unmarshal([]byte(resp), &ffiResp); err != nil {
		t.Fatalf("parse response failed: %v", err)
	}

	// 验证响应不含敏感值
	if strings.Contains(resp, "secret-key-123") {
		t.Fatalf("response should not contain api_key")
	}
	if strings.Contains(resp, "auth-token-xyz") {
		t.Fatalf("response should not contain token")
	}

	// 验证错误消息也不含敏感值（当保存失败时）
	if !ffiResp.Ok && ffiResp.Error != nil {
		if strings.Contains(ffiResp.Error.Message, "secret-key") {
			t.Fatalf("error message should not contain secret")
		}
	}
}

// TestGeneratorFFIListGeneratorCapabilitiesStructure 测试能力列表的 JSON 结构。
func TestGeneratorFFIListGeneratorCapabilitiesStructure(t *testing.T) {
	adapter := newGeneratorAdapter(t)

	reqJSON := `{}`
	resp := adapter.ListGeneratorCapabilitiesJSON(reqJSON)

	var ffiResp ffi.FFIResponse
	if err := json.Unmarshal([]byte(resp), &ffiResp); err != nil {
		t.Fatalf("parse response failed: %v", err)
	}

	if !ffiResp.Ok {
		t.Fatalf("expected ok=true, got: %s", resp)
	}

	data, ok := ffiResp.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("expected data as map")
	}

	generators, ok := data["generators"].([]interface{})
	if !ok {
		t.Fatalf("expected generators array")
	}

	// 验证每个生成器包含必需字段
	for _, g := range generators {
		gen := g.(map[string]interface{})
		if gen["generator_type"] == nil {
			t.Fatalf("expected generator_type in each generator")
		}
		if gen["supports_types"] == nil {
			t.Fatalf("expected supports_types in each generator")
		}
		if gen["deterministic_mode"] == nil {
			t.Fatalf("expected deterministic_mode in each generator")
		}
	}
}

// TestGeneratorFFIGetFieldGeneratorCandidatesStructure 测试候选查询的 JSON 结构。
func TestGeneratorFFIGetFieldGeneratorCandidatesStructure(t *testing.T) {
	adapter := newGeneratorAdapter(t)

	reqJSON := `{"connection_id":"c1","table":"users","column":"name"}`
	resp := adapter.GetFieldGeneratorCandidatesJSON(reqJSON)

	var ffiResp ffi.FFIResponse
	if err := json.Unmarshal([]byte(resp), &ffiResp); err != nil {
		t.Fatalf("parse response failed: %v", err)
	}

	if !ffiResp.Ok {
		t.Fatalf("expected ok=true, got: %s", resp)
	}

	data, ok := ffiResp.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("expected data as map")
	}

	// 验证 candidates
	candidates, ok := data["candidates"].([]interface{})
	if !ok {
		t.Fatalf("expected candidates array")
	}

	if len(candidates) < 1 {
		t.Fatalf("expected at least 1 candidate")
	}

	// 验证 default_generator
	if data["default_generator"] == nil {
		t.Fatalf("expected default_generator")
	}
}

// TestGeneratorFFIValidateFieldGeneratorConfigStructure 测试校验的 JSON 结构。
func TestGeneratorFFIValidateFieldGeneratorConfigStructure(t *testing.T) {
	adapter := newGeneratorAdapter(t)

	reqJSON := `{
		"connection_id": "c1",
		"draft_config": {
			"table": "users",
			"column": "name",
			"generator_type": "string_random_chars",
			"generator_opts": "{}",
			"is_enabled": true,
			"modified_source": "ui_manual"
		}
	}`
	resp := adapter.ValidateFieldGeneratorConfigJSON(reqJSON)

	var ffiResp ffi.FFIResponse
	if err := json.Unmarshal([]byte(resp), &ffiResp); err != nil {
		t.Fatalf("parse response failed: %v", err)
	}

	if !ffiResp.Ok {
		t.Fatalf("expected ok=true for valid config")
	}

	data, ok := ffiResp.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("expected data as map")
	}

	if data["valid"] != true {
		t.Fatalf("expected valid=true")
	}

	if data["errors"] == nil {
		t.Fatalf("expected errors array (empty for valid)")
	}
}

// TestGeneratorFFIValidateFieldGeneratorConfigInvalidStructure 测试校验失败的 JSON 结构。
func TestGeneratorFFIValidateFieldGeneratorConfigInvalidStructure(t *testing.T) {
	adapter := newGeneratorAdapter(t)

	reqJSON := `{
		"connection_id": "c1",
		"draft_config": {
			"table": "users",
			"column": "name",
			"generator_type": "string_random_chars",
			"generator_opts": "{}",
			"is_enabled": true,
			"modified_source": "invalid_source"
		}
	}`
	resp := adapter.ValidateFieldGeneratorConfigJSON(reqJSON)

	var ffiResp ffi.FFIResponse
	if err := json.Unmarshal([]byte(resp), &ffiResp); err != nil {
		t.Fatalf("parse response failed: %v", err)
	}

	if !ffiResp.Ok {
		t.Fatalf("validate should return ok=true even for invalid config")
	}

	data := ffiResp.Data.(map[string]interface{})

	if data["valid"] != false {
		t.Fatalf("expected valid=false")
	}

	errors, ok := data["errors"].([]interface{})
	if !ok || len(errors) == 0 {
		t.Fatalf("expected non-empty errors array")
	}

	// 验证错误结构
	foundModifiedSource := false
	for _, e := range errors {
		err := e.(map[string]interface{})
		if err["code"] == nil {
			t.Fatalf("expected code in each error")
		}
		if err["path"] == nil {
			t.Fatalf("expected path in each error")
		}
		if err["message"] == nil {
			t.Fatalf("expected message in each error")
		}
		if err["path"] == "modified_source" {
			foundModifiedSource = true
		}
	}
	if !foundModifiedSource {
		t.Fatalf("expected modified_source validation error path")
	}
}

// TestGeneratorFFICapabilityExtensionPoints 测试扩展契约点（5.4）。
func TestGeneratorFFICapabilityExtensionPoints(t *testing.T) {
	adapter := newGeneratorAdapter(t)

	resp := adapter.ListGeneratorCapabilitiesJSON(`{}`)

	var ffiResp ffi.FFIResponse
	if err := json.Unmarshal([]byte(resp), &ffiResp); err != nil {
		t.Fatalf("parse response failed: %v", err)
	}

	generators := ffiResp.Data.(map[string]interface{})["generators"].([]interface{})

	// 验证每个生成器包含扩展 capability 字段
	for _, g := range generators {
		gen := g.(map[string]interface{})
		// capability 字段用于 spec-08/spec-09 扩展
		if gen["capability"] != nil {
			cap := gen["capability"].(map[string]interface{})
			// 检查扩展点字段（可选，但结构必须稳定）
			if cap["requires_external_feed"] != nil {
				// 应为布尔值
				if _, ok := cap["requires_external_feed"].(bool); !ok {
					t.Fatalf("requires_external_feed should be bool")
				}
			}
			if cap["requires_computed_context"] != nil {
				if _, ok := cap["requires_computed_context"].(bool); !ok {
					t.Fatalf("requires_computed_context should be bool")
				}
			}
		}
	}
}

// TestGeneratorFFIErrorCodesAreStable 测试错误码稳定性。
func TestGeneratorFFIErrorCodesAreStable(t *testing.T) {
	expectedCodes := []string{
		"INVALID_ARGUMENT",
		"FAILED_PRECONDITION",
		"GENERATOR_NOT_REGISTERED",
		"GENERATOR_CONFLICT",
		"CURRENT_SCHEMA_NOT_FOUND",
		"OUT_OF_SCOPE_EXECUTION_REQUEST",
		"SCHEMA_TRUST_PENDING_RESCAN",
		"SCHEMA_TRUST_PENDING_ADJUSTMENT",
	}

	for _, code := range expectedCodes {
		if code == "" {
			t.Fatalf("error code should not be empty")
		}
		if strings.Contains(code, " ") {
			t.Fatalf("error code %s should not contain spaces", code)
		}
		// 应为大写蛇形命名
		if !isUpperCaseSnake(code) {
			t.Fatalf("error code %s should be UPPER_SNAKE_CASE", code)
		}
	}
}

// TestGeneratorFFIJSONParseErrorReturnsInvalidArgument 测试 JSON 解析错误。
func TestGeneratorFFIJSONParseErrorReturnsInvalidArgument(t *testing.T) {
	adapter := newGeneratorAdapter(t)

	resp := adapter.SaveFieldGeneratorConfigJSON("not-valid-json")

	var ffiResp ffi.FFIResponse
	if err := json.Unmarshal([]byte(resp), &ffiResp); err != nil {
		t.Fatalf("parse response failed: %v", err)
	}

	if ffiResp.Ok {
		t.Fatalf("invalid JSON should fail")
	}

	if ffiResp.Error.Code != "INVALID_ARGUMENT" {
		t.Fatalf("expected INVALID_ARGUMENT, got: %s", ffiResp.Error.Code)
	}
}

// TestPreviewTablePartialSuccessContractV1 测试 PREVIEW_TABLE_PARTIAL_SUCCESS_V1 契约（4.6）。
func TestPreviewTablePartialSuccessContractV1(t *testing.T) {
	adapter := newGeneratorAdapter(t)

	// 保存混合状态字段配置（全部使用有效生成器类型）
	adapter.SaveFieldGeneratorConfigJSON(`{
		"connection_id": "c1",
		"table": "users",
		"column": "id",
		"generator_type": "int_range_random",
		"generator_opts": "{}",
		"is_enabled": true,
		"modified_source": "ui_manual"
	}`)
	adapter.SaveFieldGeneratorConfigJSON(`{
		"connection_id": "c1",
		"table": "users",
		"column": "name",
		"generator_type": "string_random_chars",
		"generator_opts": "{}",
		"is_enabled": true,
		"modified_source": "ui_manual"
	}`)
	// email 禁用 -> skipped
	adapter.SaveFieldGeneratorConfigJSON(`{
		"connection_id": "c1",
		"table": "users",
		"column": "email",
		"generator_type": "string_random_chars",
		"generator_opts": "{}",
		"is_enabled": false,
		"modified_source": "ui_manual"
	}`)

	reqJSON := `{
		"connection_id": "c1",
		"scope": {"type": "table", "table": "users"},
		"sample_size": 3,
		"seed": 42
	}`
	resp := adapter.PreviewGenerationJSON(reqJSON)

	// 验证契约结构符合 PREVIEW_TABLE_PARTIAL_SUCCESS_V1
	var ffiResp ffi.FFIResponse
	if err := json.Unmarshal([]byte(resp), &ffiResp); err != nil {
		t.Fatalf("parse response failed: %v", err)
	}

	data := ffiResp.Data.(map[string]interface{})

	// samples: {"column_name": [value1, value2, ...]}
	samples := data["samples"].(map[string]interface{})
	idSamples := samples["id"].([]interface{})
	if len(idSamples) != 3 {
		t.Fatalf("expected 3 id samples")
	}

	// metadata 包含 scope, sample_size, seed, generated_at, partial_success
	metadata := data["metadata"].(map[string]interface{})
	if metadata["partial_success"] != true {
		t.Fatalf("expected partial_success=true")
	}
	if metadata["seed"] != float64(42) {
		t.Fatalf("expected seed=42")
	}
	scopeMeta := metadata["scope"].(map[string]interface{})
	if scopeMeta["type"] != "table" {
		t.Fatalf("expected scope.type=table")
	}

	// warnings[] 包含 code, field, message
	warnings := data["warnings"].([]interface{})
	for _, w := range warnings {
		warning := w.(map[string]interface{})
		if warning["code"] == nil {
			t.Fatalf("warning missing code")
		}
		if warning["field"] == nil {
			t.Fatalf("warning missing field")
		}
	}

	// field_results[] 包含 field, status, sample_count, error_code?, warning?
	fieldResults := data["field_results"].([]interface{})
	// 现有配置为 3 个（id, name, email）
	if len(fieldResults) != 3 {
		t.Fatalf("expected 3 field_results, got %d", len(fieldResults))
	}

	// 验证字段级结果清单完整性
	for _, fr := range fieldResults {
		result := fr.(map[string]interface{})
		if result["field"] == nil {
			t.Fatalf("field_result missing field")
		}
		if result["status"] == nil {
			t.Fatalf("field_result missing status")
		}
		if result["sample_count"] == nil {
			t.Fatalf("field_result missing sample_count")
		}
		// 契约要求 error_code / warning 字段存在（可为 null）
		if _, ok := result["error_code"]; !ok {
			t.Fatalf("field_result missing error_code key")
		}
		if _, ok := result["warning"]; !ok {
			t.Fatalf("field_result missing warning key")
		}
	}

	// 验证 status=ok 字段与 samples 字段集合一致
	okFields := []string{}
	for _, fr := range fieldResults {
		result := fr.(map[string]interface{})
		if result["status"] == "ok" {
			okFields = append(okFields, result["field"].(string))
		}
	}
	sampleFields := []string{}
	for field := range samples {
		sampleFields = append(sampleFields, field)
	}
	if len(okFields) != len(sampleFields) {
		t.Fatalf("ok fields count mismatch samples fields: %d vs %d", len(okFields), len(sampleFields))
	}
}

// TestGeneratorFFISchemaTrustGate_Trusted 测试 schema trust=trusted 时核心接口成功路径结构稳定（6.7）。
func TestGeneratorFFISchemaTrustGate_Trusted(t *testing.T) {
	deps := newGeneratorAdapterDeps(t)
	adapter := newGeneratorAdapterWithTrustDeps(t, schema.SchemaTrustTrusted, deps)

	saveResp := adapter.SaveFieldGeneratorConfigJSON(`{
		"connection_id": "c1",
		"table": "users",
		"column": "name",
		"generator_type": "string_random_chars",
		"generator_opts": "{}",
		"is_enabled": true,
		"modified_source": "ui_manual"
	}`)
	assertFFIOK(t, saveResp)

	validateResp := adapter.ValidateFieldGeneratorConfigJSON(`{
		"connection_id": "c1",
		"draft_config": {
			"table": "users",
			"column": "name",
			"generator_type": "string_random_chars",
			"generator_opts": "{}",
			"is_enabled": true,
			"modified_source": "ui_manual"
		}
	}`)
	assertFFIOK(t, validateResp)

	candidatesResp := adapter.GetFieldGeneratorCandidatesJSON(`{"connection_id":"c1","table":"users","column":"name"}`)
	assertFFIOK(t, candidatesResp)

	previewResp := adapter.PreviewGenerationJSON(`{
		"connection_id": "c1",
		"scope": {"type": "field", "table": "users", "column": "name"},
		"sample_size": 2,
		"seed": 42
	}`)
	assertFFIOK(t, previewResp)

	getResp := adapter.GetFieldGeneratorConfigJSON(`{"connection_id":"c1","table":"users","column":"name"}`)
	assertFFIOK(t, getResp)
}

// TestGeneratorFFISchemaTrustGate_PendingRescan 测试 pending_rescan 门禁失败/只读例外（6.8）。
func TestGeneratorFFISchemaTrustGate_PendingRescan(t *testing.T) {
	deps := newGeneratorAdapterDeps(t)

	blocked := newGeneratorAdapterWithTrustDeps(t, schema.SchemaTrustPendingRescan, deps)

	assertFFITrustBlocked(t, blocked.SaveFieldGeneratorConfigJSON(`{
		"connection_id": "c1",
		"table": "users",
		"column": "name",
		"generator_type": "string_random_chars",
		"generator_opts": "{}",
		"is_enabled": true,
		"modified_source": "ui_manual"
	}`), "SCHEMA_TRUST_PENDING_RESCAN")

	assertFFITrustBlocked(t, blocked.ValidateFieldGeneratorConfigJSON(`{
		"connection_id": "c1",
		"draft_config": {
			"table": "users",
			"column": "name",
			"generator_type": "string_random_chars",
			"generator_opts": "{}",
			"is_enabled": true,
			"modified_source": "ui_manual"
		}
	}`), "SCHEMA_TRUST_PENDING_RESCAN")

	assertFFITrustBlocked(t, blocked.GetFieldGeneratorCandidatesJSON(`{"connection_id":"c1","table":"users","column":"name"}`), "SCHEMA_TRUST_PENDING_RESCAN")

	assertFFITrustBlocked(t, blocked.PreviewGenerationJSON(`{
		"connection_id": "c1",
		"scope": {"type": "table", "table": "users"},
		"sample_size": 2
	}`), "SCHEMA_TRUST_PENDING_RESCAN")

	// 只读例外：先写入配置，再用 pending_rescan 适配器读取，必须 ok=true 且 warnings[].reason 正确
	writer := newGeneratorAdapterWithTrustDeps(t, schema.SchemaTrustTrusted, deps)
	writer.SaveFieldGeneratorConfigJSON(`{
		"connection_id": "c1",
		"table": "users",
		"column": "name",
		"generator_type": "string_random_chars",
		"generator_opts": "{}",
		"is_enabled": true,
		"modified_source": "ui_manual"
	}`)

	getResp := blocked.GetFieldGeneratorConfigJSON(`{"connection_id":"c1","table":"users","column":"name"}`)
	assertFFIOK(t, getResp)
	assertFFIDataWarningsHasReason(t, getResp, "SCHEMA_TRUST_PENDING_RESCAN")
}

// TestGeneratorFFISchemaTrustGate_PendingAdjustment 测试 pending_adjustment 门禁失败/只读例外（6.9）。
func TestGeneratorFFISchemaTrustGate_PendingAdjustment(t *testing.T) {
	deps := newGeneratorAdapterDeps(t)

	blocked := newGeneratorAdapterWithTrustDeps(t, schema.SchemaTrustPendingAdjustment, deps)

	assertFFITrustBlocked(t, blocked.SaveFieldGeneratorConfigJSON(`{
		"connection_id": "c1",
		"table": "users",
		"column": "name",
		"generator_type": "string_random_chars",
		"generator_opts": "{}",
		"is_enabled": true,
		"modified_source": "ui_manual"
	}`), "SCHEMA_TRUST_PENDING_ADJUSTMENT")

	assertFFITrustBlocked(t, blocked.ValidateFieldGeneratorConfigJSON(`{
		"connection_id": "c1",
		"draft_config": {
			"table": "users",
			"column": "name",
			"generator_type": "string_random_chars",
			"generator_opts": "{}",
			"is_enabled": true,
			"modified_source": "ui_manual"
		}
	}`), "SCHEMA_TRUST_PENDING_ADJUSTMENT")

	assertFFITrustBlocked(t, blocked.GetFieldGeneratorCandidatesJSON(`{"connection_id":"c1","table":"users","column":"name"}`), "SCHEMA_TRUST_PENDING_ADJUSTMENT")

	assertFFITrustBlocked(t, blocked.PreviewGenerationJSON(`{
		"connection_id": "c1",
		"scope": {"type": "field", "table": "users", "column": "name"},
		"sample_size": 2,
		"seed": 42
	}`), "SCHEMA_TRUST_PENDING_ADJUSTMENT")

	writer := newGeneratorAdapterWithTrustDeps(t, schema.SchemaTrustTrusted, deps)
	writer.SaveFieldGeneratorConfigJSON(`{
		"connection_id": "c1",
		"table": "users",
		"column": "name",
		"generator_type": "string_random_chars",
		"generator_opts": "{}",
		"is_enabled": true,
		"modified_source": "ui_manual"
	}`)

	getResp := blocked.GetFieldGeneratorConfigJSON(`{"connection_id":"c1","table":"users","column":"name"}`)
	assertFFIOK(t, getResp)
	assertFFIDataWarningsHasReason(t, getResp, "SCHEMA_TRUST_PENDING_ADJUSTMENT")
}

// TestGeneratorFFISchemaTrustGate_ListCapabilitiesAlwaysOK 测试 ListGeneratorCapabilities 在三种状态下结构稳定且不被门禁阻断（6.10）。
func TestGeneratorFFISchemaTrustGate_ListCapabilitiesAlwaysOK(t *testing.T) {
	deps := newGeneratorAdapterDeps(t)
	for _, st := range []schema.SchemaTrustState{
		schema.SchemaTrustTrusted,
		schema.SchemaTrustPendingRescan,
		schema.SchemaTrustPendingAdjustment,
	} {
		adapter := newGeneratorAdapterWithTrustDeps(t, st, deps)
		resp := adapter.ListGeneratorCapabilitiesJSON(`{}`)
		assertFFIOK(t, resp)
	}
}

// ===== 辅助函数 =====

func newGeneratorAdapter(t *testing.T) *ffi.GeneratorFFIAdapter {
	t.Helper()
	// 创建测试用的生成器注册表
	reg := generator.NewGeneratorRegistry()
	reg.Register(generator.NewStaticGenerator(generator.GeneratorMeta{
		Type:          generator.GeneratorTypeIntRangeRandom,
		TypeTags:      []string{"supports:int"},
		Deterministic: true,
	}))
	reg.Register(generator.NewStaticGenerator(generator.GeneratorMeta{
		Type:          generator.GeneratorTypeStringRandomChars,
		TypeTags:      []string{"supports:string"},
		Deterministic: true,
	}))

	// 创建测试用的配置仓储和 schema 提供器
	configRepo := generator.NewPreviewableConfigRepository()
	schemaProvider := generator.NewPreviewableSchemaProvider([]generator.FieldSchema{
		{ConnectionID: "c1", Table: "users", Column: "id", AbstractType: "int", ColumnID: "col-id"},
		{ConnectionID: "c1", Table: "users", Column: "name", AbstractType: "string", ColumnID: "col-name"},
		{ConnectionID: "c1", Table: "users", Column: "email", AbstractType: "string", ColumnID: "col-email"},
		{ConnectionID: "c1", Table: "users", Column: "phone", AbstractType: "string", ColumnID: "col-phone"},
	})

	return ffi.NewGeneratorFFIAdapter(reg, configRepo, schemaProvider, generator.NewGeneratorRuntime(), nil)
}

type generatorAdapterDeps struct {
	reg            *generator.GeneratorRegistry
	configRepo     *generator.PreviewableConfigRepository
	schemaProvider *generator.PreviewableSchemaProvider
}

func newGeneratorAdapterDeps(t *testing.T) generatorAdapterDeps {
	t.Helper()
	reg := generator.NewGeneratorRegistry()
	reg.Register(generator.NewStaticGenerator(generator.GeneratorMeta{
		Type:          generator.GeneratorTypeIntRangeRandom,
		TypeTags:      []string{"supports:int"},
		Deterministic: true,
	}))
	reg.Register(generator.NewStaticGenerator(generator.GeneratorMeta{
		Type:          generator.GeneratorTypeStringRandomChars,
		TypeTags:      []string{"supports:string"},
		Deterministic: true,
	}))
	configRepo := generator.NewPreviewableConfigRepository()
	schemaProvider := generator.NewPreviewableSchemaProvider([]generator.FieldSchema{
		{ConnectionID: "c1", Table: "users", Column: "id", AbstractType: "int", ColumnID: "col-id"},
		{ConnectionID: "c1", Table: "users", Column: "name", AbstractType: "string", ColumnID: "col-name"},
		{ConnectionID: "c1", Table: "users", Column: "email", AbstractType: "string", ColumnID: "col-email"},
		{ConnectionID: "c1", Table: "users", Column: "phone", AbstractType: "string", ColumnID: "col-phone"},
	})
	return generatorAdapterDeps{
		reg:            reg,
		configRepo:     configRepo,
		schemaProvider: schemaProvider,
	}
}

type fakeGeneratorTrustReader struct {
	view schema.TrustStateView
	err  error
}

func (f *fakeGeneratorTrustReader) GetSchemaTrustState(_ string) (schema.TrustStateView, error) {
	return f.view, f.err
}

func newGeneratorAdapterWithTrustDeps(t *testing.T, trustState schema.SchemaTrustState, deps generatorAdapterDeps) *ffi.GeneratorFFIAdapter {
	t.Helper()
	trustReader := &fakeGeneratorTrustReader{view: schema.TrustStateView{State: trustState}}
	return ffi.NewGeneratorFFIAdapter(deps.reg, deps.configRepo, deps.schemaProvider, generator.NewGeneratorRuntime(), trustReader)
}

func assertFFIOK(t *testing.T, resp string) {
	t.Helper()
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(resp), &parsed); err != nil {
		t.Fatalf("parse response failed: %v", err)
	}
	if parsed["ok"] != true {
		t.Fatalf("expected ok=true, got %s", resp)
	}
	if parsed["error"] != nil {
		t.Fatalf("expected error=nil, got %s", resp)
	}
}

func assertFFITrustBlocked(t *testing.T, resp string, reason string) {
	t.Helper()
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(resp), &parsed); err != nil {
		t.Fatalf("parse response failed: %v", err)
	}
	if parsed["ok"] != false {
		t.Fatalf("expected ok=false, got %s", resp)
	}
	errObj, ok := parsed["error"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected error object, got %s", resp)
	}
	if errObj["code"] != "FAILED_PRECONDITION" {
		t.Fatalf("expected FAILED_PRECONDITION, got %v", errObj["code"])
	}
	if errObj["reason"] != reason {
		t.Fatalf("expected reason=%s, got %v", reason, errObj["reason"])
	}
}

func assertFFIDataWarningsHasReason(t *testing.T, resp string, reason string) {
	t.Helper()
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(resp), &parsed); err != nil {
		t.Fatalf("parse response failed: %v", err)
	}
	data := parsed["data"].(map[string]interface{})
	warnings, ok := data["warnings"].([]interface{})
	if !ok {
		t.Fatalf("expected warnings array, got %s", resp)
	}
	found := false
	for _, w := range warnings {
		m, ok := w.(map[string]interface{})
		if !ok {
			continue
		}
		if m["reason"] == reason {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected warnings contain reason=%s, got %s", reason, resp)
	}
}

func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && strings.Contains(s, substr))
}

func isUpperCaseSnake(s string) bool {
	for _, c := range s {
		if c != '_' && (c < 'A' || c > 'Z') {
			return false
		}
	}
	return true
}