// Package ffi 提供 Generator FFI JSON 适配器。
package ffi_test

import (
	"encoding/json"
	"strings"
	"testing"

	"loomidbx/ffi"
	"loomidbx/generator"
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
			"table": "",
			"column": "name",
			"generator_type": "unknown_generator",
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

	return ffi.NewGeneratorFFIAdapter(reg, configRepo, schemaProvider, generator.NewGeneratorRuntime())
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