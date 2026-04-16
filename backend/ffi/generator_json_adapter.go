// Package ffi 提供 Generator FFI JSON 适配器。
//
// 本包将 Generator 应用服务层方法包装为符合 FFI 契约的 JSON 响应：
// 成功响应: {"ok": true, "data": {...}, "error": null}
// 失败响应: {"ok": false, "data": null, "error": {"code": "...", "message": "...", "reason": "..."}}
//
// 所有响应不包含明文密码、密钥或 token 等敏感信息。
package ffi

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"loomidbx/app"
	"loomidbx/generator"
	"loomidbx/schema"
)

const (
	// generatorFFITrustReasonPendingRescan 为 schema trust 门禁 pending_rescan 的稳定 reason。
	generatorFFITrustReasonPendingRescan = "SCHEMA_TRUST_PENDING_RESCAN"

	// generatorFFITrustReasonPendingAdjustment 为 schema trust 门禁 pending_adjustment 的稳定 reason。
	generatorFFITrustReasonPendingAdjustment = "SCHEMA_TRUST_PENDING_ADJUSTMENT"
)

// GeneratorTrustStateReader 定义 Generator FFI 层读取 schema trust 状态的最小依赖接口。
type GeneratorTrustStateReader interface {
	// GetSchemaTrustState 返回连接可信度状态视图。
	GetSchemaTrustState(connectionID string) (schema.TrustStateView, error)
}

// GeneratorFFIAdapter 是 Generator FFI JSON 适配器。
type GeneratorFFIAdapter struct {
	// registry 用于查询生成器实例。
	registry *generator.GeneratorRegistry

	// configRepo 用于读取字段配置。
	configRepo generator.PreviewServiceConfigRepository

	// schemaProvider 用于读取字段 schema。
	schemaProvider generator.PreviewServiceSchemaProvider

	// runtime 用于执行生成器调用。
	runtime generator.PreviewRuntime

	// previewService 用于执行预览。
	previewService *generator.GeneratorPreviewService

	// trustReader 用于读取连接 schema trust 状态；为空时视为 trusted（兼容无闸门依赖场景）。
	trustReader GeneratorTrustStateReader
}

// NewGeneratorFFIAdapter 创建 Generator FFI 适配器实例。
//
// 输入：
// - registry: 生成器注册表实例。
// - configRepo: 扩展的配置仓储实例。
// - schemaProvider: 扩展的 schema 提供器实例。
// - runtime: 生成器运行时实例。
// - trustReader: schema trust 状态读取器（可为空）。
//
// 输出：
// - *GeneratorFFIAdapter: 初始化后的适配器实例。
func NewGeneratorFFIAdapter(
	registry *generator.GeneratorRegistry,
	configRepo generator.PreviewServiceConfigRepository,
	schemaProvider generator.PreviewServiceSchemaProvider,
	runtime generator.PreviewRuntime,
	trustReader GeneratorTrustStateReader,
) *GeneratorFFIAdapter {
	previewService := generator.NewGeneratorPreviewService(registry, configRepo, schemaProvider, runtime, nil)
	return &GeneratorFFIAdapter{
		registry:       registry,
		configRepo:     configRepo,
		schemaProvider: schemaProvider,
		runtime:        runtime,
		previewService: previewService,
		trustReader:    trustReader,
	}
}

// SaveFieldGeneratorConfigRequest 是保存配置的请求结构。
type SaveFieldGeneratorConfigRequest struct {
	// ConnectionID 是连接标识。
	ConnectionID string `json:"connection_id"`

	// Table 是表名。
	Table string `json:"table"`

	// Column 是字段名。
	Column string `json:"column"`

	// GeneratorType 是生成器类型。
	GeneratorType string `json:"generator_type"`

	// GeneratorOpts 是生成器参数（JSON 字符串）。
	GeneratorOpts string `json:"generator_opts"`

	// SeedPolicy 是种子策略（JSON 字符串）。
	SeedPolicy string `json:"seed_policy"`

	// NullPolicy 是空值策略。
	NullPolicy string `json:"null_policy"`

	// IsEnabled 是否启用。
	IsEnabled bool `json:"is_enabled"`

	// ModifiedSource 是修改来源。
	ModifiedSource string `json:"modified_source"`
}

// SaveFieldGeneratorConfigResponse 是保存配置的响应数据。
type SaveFieldGeneratorConfigResponse struct {
	// Saved 表示保存成功。
	Saved bool `json:"saved"`

	// ConfigVersion 是配置版本号。
	ConfigVersion int64 `json:"config_version"`

	// IsEnabled 是否启用。
	IsEnabled bool `json:"is_enabled"`

	// ModifiedSource 是修改来源。
	ModifiedSource string `json:"modified_source"`

	// Warnings 包含警告信息。
	Warnings []FFIWarning `json:"warnings"`
}

// GetFieldGeneratorConfigRequest 是查询配置的请求结构。
type GetFieldGeneratorConfigRequest struct {
	// ConnectionID 是连接标识。
	ConnectionID string `json:"connection_id"`

	// Table 是表名。
	Table string `json:"table"`

	// Column 是字段名。
	Column string `json:"column"`
}

// GetFieldGeneratorConfigResponse 是查询配置的响应数据。
type GetFieldGeneratorConfigResponse struct {
	// Config 是字段配置详情。
	Config FieldGeneratorConfigData `json:"config"`

	// Warnings 包含警告信息（pending 状态时包含 trust 警告）。
	Warnings []FFIWarning `json:"warnings"`
}

// FieldGeneratorConfigData 是字段配置详情。
type FieldGeneratorConfigData struct {
	// ConnectionID 是连接标识。
	ConnectionID string `json:"connection_id"`

	// Table 是表名。
	Table string `json:"table"`

	// Column 是字段名。
	Column string `json:"column"`

	// GeneratorType 是生成器类型。
	GeneratorType string `json:"generator_type"`

	// GeneratorOpts 是生成器参数（脱敏后）。
	GeneratorOpts string `json:"generator_opts"`

	// SeedPolicy 是种子策略。
	SeedPolicy string `json:"seed_policy"`

	// NullPolicy 是空值策略。
	NullPolicy string `json:"null_policy"`

	// IsEnabled 是否启用。
	IsEnabled bool `json:"is_enabled"`

	// ConfigVersion 是配置版本号。
	ConfigVersion int64 `json:"config_version"`

	// ModifiedSource 是修改来源。
	ModifiedSource string `json:"modified_source"`
}

// PreviewGenerationRequest 是预览请求结构。
type PreviewGenerationRequest struct {
	// ConnectionID 是连接标识。
	ConnectionID string `json:"connection_id"`

	// Scope 是预览范围。
	Scope PreviewScopeData `json:"scope"`

	// SampleSize 是样本数量。
	SampleSize int `json:"sample_size"`

	// Seed 是可选种子值。
	Seed *int64 `json:"seed"`
}

// PreviewScopeData 是预览范围结构。
type PreviewScopeData struct {
	// Type 是范围类型（field 或 table）。
	Type string `json:"type"`

	// Table 是表名。
	Table string `json:"table"`

	// Column 是字段名（仅 field scope）。
	Column string `json:"column"`

	// Tables 是跨表列表（仅 cross_table scope，MVP 不支持）。
	Tables []string `json:"tables,omitempty"`
}

// PreviewGenerationResponse 是预览响应数据。
type PreviewGenerationResponse struct {
	// Samples 是样本数据。
	Samples interface{} `json:"samples"`

	// Metadata 包含元信息。
	Metadata PreviewMetadataData `json:"metadata"`

	// Warnings 包含警告信息。
	Warnings []FFIWarning `json:"warnings"`

	// FieldResults 包含字段级结果清单（仅 table scope）。
	FieldResults []PreviewFieldResultData `json:"field_results,omitempty"`
}

// PreviewMetadataData 是预览元信息结构。
type PreviewMetadataData struct {
	// Scope 是预览范围。
	Scope PreviewScopeData `json:"scope"`

	// GeneratorType 是使用的生成器类型（field scope 时；table scope 为空字符串但字段存在）。
	GeneratorType string `json:"generator_type"`

	// ParamsSummary 是参数摘要（field scope 必填；table scope 返回空对象）。
	ParamsSummary map[string]interface{} `json:"params_summary"`

	// SampleSize 是样本数量。
	SampleSize int `json:"sample_size"`

	// Seed 是生效种子。
	Seed int64 `json:"seed"`

	// GeneratedAt 是生成时间。
	GeneratedAt string `json:"generated_at"`

	// Deterministic 表示是否可确定性复现。
	Deterministic bool `json:"deterministic"`

	// PartialSuccess 表示是否为部分成功。
	PartialSuccess bool `json:"partial_success"`

	// SeedSource 表示种子来源（preview_override/field_fixed/global/none）。
	SeedSource string `json:"seed_source"`
}

// PreviewFieldResultData 是字段级结果结构。
type PreviewFieldResultData struct {
	// Field 是字段名。
	Field string `json:"field"`

	// Status 是状态（ok, skipped, failed）。
	Status string `json:"status"`

	// SampleCount 是样本数量。
	SampleCount int `json:"sample_count"`

	// ErrorCode 是错误码（nullable；契约要求字段存在，成功场景为 null）。
	ErrorCode *string `json:"error_code"`

	// Warning 是警告提示（nullable；契约要求字段存在，成功场景为 null）。
	Warning *string `json:"warning"`
}

// FFIWarning 是 FFI 警告结构。
type FFIWarning struct {
	// Code 是稳定警告码。
	Code string `json:"code"`

	// Reason 是可选稳定原因码。
	Reason string `json:"reason,omitempty"`

	// Field 是相关字段名（可选）。
	Field string `json:"field,omitempty"`

	// Message 是可读描述。
	Message string `json:"message"`
}

// GeneratorCapabilityData 是生成器能力结构。
type GeneratorCapabilityData struct {
	// GeneratorType 是生成器类型。
	GeneratorType string `json:"generator_type"`

	// SupportsTypes 是支持的字段类型列表。
	SupportsTypes []string `json:"supports_types"`

	// DeterministicMode 是否支持确定性复现。
	DeterministicMode bool `json:"deterministic_mode"`

	// Capability 包含扩展能力声明。
	Capability GeneratorCapabilityExtension `json:"capability"`
}

// GeneratorCapabilityExtension 是扩展能力声明。
type GeneratorCapabilityExtension struct {
	// RequiresExternalFeed 是否依赖外部 feed（spec-08）。
	RequiresExternalFeed bool `json:"requires_external_feed"`

	// RequiresComputedContext 是否依赖计算字段上下文（spec-09）。
	RequiresComputedContext bool `json:"requires_computed_context"`

	// AcceptsEnumValues 是否支持候选值集合输入。
	AcceptsEnumValues bool `json:"accepts_enum_values"`
}

// GetFieldGeneratorCandidatesRequest 是候选查询请求结构。
type GetFieldGeneratorCandidatesRequest struct {
	// ConnectionID 是连接标识。
	ConnectionID string `json:"connection_id"`

	// Table 是表名。
	Table string `json:"table"`

	// Column 是字段名。
	Column string `json:"column"`
}

// GetFieldGeneratorCandidatesResponse 是候选查询响应数据。
type GetFieldGeneratorCandidatesResponse struct {
	// Candidates 是候选生成器列表。
	Candidates []GeneratorCandidateData `json:"candidates"`

	// DefaultGenerator 是默认生成器类型。
	DefaultGenerator string `json:"default_generator"`
}

// GeneratorCandidateData 是候选生成器结构。
type GeneratorCandidateData struct {
	// GeneratorType 是生成器类型。
	GeneratorType string `json:"generator_type"`

	// SupportsTypes 是支持的字段类型。
	SupportsTypes []string `json:"supports_types"`
}

// ValidateFieldGeneratorConfigRequest 是校验请求结构。
type ValidateFieldGeneratorConfigRequest struct {
	// ConnectionID 是连接标识。
	ConnectionID string `json:"connection_id"`

	// DraftConfig 是待校验的配置草稿。
	DraftConfig SaveFieldGeneratorConfigRequest `json:"draft_config"`
}

// ValidateFieldGeneratorConfigResponse 是校验响应数据。
type ValidateFieldGeneratorConfigResponse struct {
	// Valid 表示校验是否通过。
	Valid bool `json:"valid"`

	// Errors 包含错误列表。
	Errors []FieldErrorData `json:"errors"`
}

// FieldErrorData 是字段错误结构。
type FieldErrorData struct {
	// Code 是稳定错误码。
	Code string `json:"code"`

	// Path 是错误路径。
	Path string `json:"path"`

	// Message 是可读描述。
	Message string `json:"message"`

	// Suggestion 是修复建议。
	Suggestion string `json:"suggestion,omitempty"`
}

// FFIGeneratorError 是 Generator FFI 错误结构。
type FFIGeneratorError struct {
	// Code 是稳定错误码。
	Code string `json:"code"`

	// Reason 是稳定原因码。
	Reason string `json:"reason,omitempty"`

	// Message 是可读描述（脱敏后）。
	Message string `json:"message"`

	// Details 包含可选详情。
	Details map[string]string `json:"details,omitempty"`
}

// ===== FFI JSON 方法实现 =====

// SaveFieldGeneratorConfigJSON 执行配置保存并返回 FFI JSON 响应。
//
// 输入：
// - reqJSON: 请求 JSON 字串。
//
// 输出：
// - string: FFI JSON 响应。
func (a *GeneratorFFIAdapter) SaveFieldGeneratorConfigJSON(reqJSON string) string {
	var req SaveFieldGeneratorConfigRequest
	if err := json.Unmarshal([]byte(reqJSON), &req); err != nil {
		return marshalGeneratorResponse(ffiGeneratorResponseFromParseError(err))
	}

	if blocked := a.checkSchemaTrustGate(strings.TrimSpace(req.ConnectionID)); blocked != nil {
		return marshalGeneratorResponse(blocked)
	}

	// 解析 generator_opts
	opts := make(map[string]interface{})
	if req.GeneratorOpts != "" {
		if err := json.Unmarshal([]byte(req.GeneratorOpts), &opts); err != nil {
			opts = make(map[string]interface{})
		}
	}

	// 脱敏敏感参数
	opts = sanitizeGeneratorOpts(opts)

	// 解析 seed_policy
	seedPolicy := make(map[string]interface{})
	if req.SeedPolicy != "" {
		if err := json.Unmarshal([]byte(req.SeedPolicy), &seedPolicy); err != nil {
			seedPolicy = make(map[string]interface{})
		}
	}

	// 构造内部请求
	internalReq := generator.SaveFieldGeneratorConfigRequest{
		ConnectionID:   req.ConnectionID,
		Table:          req.Table,
		Column:         req.Column,
		GeneratorType:  generator.GeneratorType(req.GeneratorType),
		GeneratorOpts:  opts,
		SeedPolicy:     seedPolicy,
		NullPolicy:     req.NullPolicy,
		IsEnabled:      req.IsEnabled,
		ModifiedSource: req.ModifiedSource,
	}

	// 执行保存
	config, validationErr := a.saveFieldConfig(context.Background(), internalReq)
	if validationErr != nil {
		return marshalGeneratorResponse(ffiGeneratorResponseFromValidationError(validationErr))
	}

	// 构造响应
	resp := &FFIResponse{
		Ok: true,
		Data: SaveFieldGeneratorConfigResponse{
			Saved:         true,
			ConfigVersion: config.ConfigVersion,
			IsEnabled:     config.IsEnabled,
			ModifiedSource: config.ModifiedSource,
			Warnings:      []FFIWarning{},
		},
	}

	return marshalGeneratorResponse(resp)
}

// GetFieldGeneratorConfigJSON 执行配置查询并返回 FFI JSON 响应。
//
// 输入：
// - reqJSON: 请求 JSON 字串。
//
// 输出：
// - string: FFI JSON 响应。
func (a *GeneratorFFIAdapter) GetFieldGeneratorConfigJSON(reqJSON string) string {
	var req GetFieldGeneratorConfigRequest
	if err := json.Unmarshal([]byte(reqJSON), &req); err != nil {
		return marshalGeneratorResponse(ffiGeneratorResponseFromParseError(err))
	}

	trustWarning := a.schemaTrustWarning(strings.TrimSpace(req.ConnectionID))

	config, err := a.configRepo.GetByField(context.Background(), req.ConnectionID, req.Table, req.Column)
	if err != nil {
		return marshalGeneratorResponse(ffiGeneratorResponseFromError(
			&generator.GeneratorError{
				Code:    generator.GeneratorErrNotRegistered,
				Path:    "config",
				Message: "field generator config not found",
			},
		))
	}

	// 脱敏 generator_opts
	optsJSON := sanitizeGeneratorOptsJSON(config.GeneratorOpts)

	resp := &FFIResponse{
		Ok: true,
		Data: GetFieldGeneratorConfigResponse{
			Config: FieldGeneratorConfigData{
				ConnectionID:   config.ConnectionID,
				Table:          config.Table,
				Column:         config.Column,
				GeneratorType:  string(config.GeneratorType),
				GeneratorOpts:  optsJSON,
				SeedPolicy:     sanitizeSeedPolicyJSON(config.SeedPolicy),
				NullPolicy:     config.NullPolicy,
				IsEnabled:      config.IsEnabled,
				ConfigVersion:  config.ConfigVersion,
				ModifiedSource: config.ModifiedSource,
			},
			Warnings: trustWarning,
		},
	}

	return marshalGeneratorResponse(resp)
}

// PreviewGenerationJSON 执行预览生成并返回 FFI JSON 响应。
//
// 输入：
// - reqJSON: 请求 JSON 字串。
//
// 输出：
// - string: FFI JSON 响应。
func (a *GeneratorFFIAdapter) PreviewGenerationJSON(reqJSON string) string {
	var req PreviewGenerationRequest
	if err := json.Unmarshal([]byte(reqJSON), &req); err != nil {
		return marshalGeneratorResponse(ffiGeneratorResponseFromParseError(err))
	}

	if blocked := a.checkSchemaTrustGate(strings.TrimSpace(req.ConnectionID)); blocked != nil {
		return marshalGeneratorResponse(blocked)
	}

	// 检查跨表请求（5.2）
	if req.Scope.Type == "cross_table" || len(req.Scope.Tables) > 1 {
		return marshalGeneratorResponse(ffiGeneratorResponseFromError(
			&generator.GeneratorError{
				Code:    generator.GeneratorErrorCode("OUT_OF_SCOPE_EXECUTION_REQUEST"),
				Path:    "scope",
				Message: "cross-table execution requests are handled by spec-04 execution engine, not generator preview service",
			},
		))
	}

	// 构造内部请求
	internalReq := generator.PreviewRequest{
		ConnectionID: req.ConnectionID,
		Scope:        generator.PreviewScopeType(req.Scope.Type),
		Table:        req.Scope.Table,
		Column:       req.Scope.Column,
		SampleSize:   req.SampleSize,
		Seed:         req.Seed,
	}

	// 执行预览
	result, err := a.previewService.PreviewGeneration(context.Background(), internalReq)
	if err != nil {
		return marshalGeneratorResponse(ffiGeneratorResponseFromError(err))
	}

	// 构造响应
	resp := &FFIResponse{
		Ok: true,
		Data: PreviewGenerationResponse{
			Samples:  result.Samples,
			Metadata: convertPreviewMetadata(result.Metadata),
			Warnings: convertPreviewWarnings(result.Warnings),
			FieldResults: convertPreviewFieldResults(result.FieldResults),
		},
	}

	return marshalGeneratorResponse(resp)
}

// ListGeneratorCapabilitiesJSON 执行能力查询并返回 FFI JSON 响应。
//
// 输入：
// - reqJSON: 请求 JSON 字串（可选过滤参数）。
//
// 输出：
// - string: FFI JSON 响应。
func (a *GeneratorFFIAdapter) ListGeneratorCapabilitiesJSON(reqJSON string) string {
	metas := a.registry.ListCapabilities()

	generators := make([]GeneratorCapabilityData, len(metas))
	for i, meta := range metas {
		supportsTypes := extractSupportsTypes(meta.TypeTags)
		capability := extractCapability(meta.TypeTags)

		generators[i] = GeneratorCapabilityData{
			GeneratorType:   string(meta.Type),
			SupportsTypes:   supportsTypes,
			DeterministicMode: meta.Deterministic,
			Capability:      capability,
		}
	}

	resp := &FFIResponse{
		Ok: true,
		Data: map[string]interface{}{
			"generators": generators,
		},
	}

	return marshalGeneratorResponse(resp)
}

// GetFieldGeneratorCandidatesJSON 执行候选查询并返回 FFI JSON 响应。
//
// 输入：
// - reqJSON: 请求 JSON 字串。
//
// 输出：
// - string: FFI JSON 响应。
func (a *GeneratorFFIAdapter) GetFieldGeneratorCandidatesJSON(reqJSON string) string {
	var req GetFieldGeneratorCandidatesRequest
	if err := json.Unmarshal([]byte(reqJSON), &req); err != nil {
		return marshalGeneratorResponse(ffiGeneratorResponseFromParseError(err))
	}

	if blocked := a.checkSchemaTrustGate(strings.TrimSpace(req.ConnectionID)); blocked != nil {
		return marshalGeneratorResponse(blocked)
	}

	// 获取字段 schema
	fieldSchema, err := a.schemaProvider.GetFieldSchema(context.Background(), req.ConnectionID, req.Table, req.Column)
	if err != nil {
		return marshalGeneratorResponse(ffiGeneratorResponseFromError(
			&generator.GeneratorError{
				Code:    generator.GeneratorErrorCode("CURRENT_SCHEMA_NOT_FOUND"),
				Path:    "field_schema",
				Message: "field schema not found",
			},
		))
	}

	// 解析候选
	resolver := generator.NewGeneratorTypeResolver(a.registry)
	result, err := resolver.ResolveCandidates(fieldSchema)
	if err != nil {
		return marshalGeneratorResponse(ffiGeneratorResponseFromError(err))
	}

	// 构造候选列表
	candidates := make([]GeneratorCandidateData, len(result.Candidates))
	for i, genType := range result.Candidates {
		gen, _ := a.registry.Resolve(genType)
		supportsTypes := []string{}
		if gen != nil {
			supportsTypes = extractSupportsTypes(gen.Meta().TypeTags)
		}
		candidates[i] = GeneratorCandidateData{
			GeneratorType:  string(genType),
			SupportsTypes:  supportsTypes,
		}
	}

	resp := &FFIResponse{
		Ok: true,
		Data: GetFieldGeneratorCandidatesResponse{
			Candidates:      candidates,
			DefaultGenerator: string(result.DefaultGeneratorType),
		},
	}

	return marshalGeneratorResponse(resp)
}

// ValidateFieldGeneratorConfigJSON 执行配置校验并返回 FFI JSON 响应。
//
// 输入：
// - reqJSON: 请求 JSON 字串。
//
// 输出：
// - string: FFI JSON 响应。
func (a *GeneratorFFIAdapter) ValidateFieldGeneratorConfigJSON(reqJSON string) string {
	var req ValidateFieldGeneratorConfigRequest
	if err := json.Unmarshal([]byte(reqJSON), &req); err != nil {
		return marshalGeneratorResponse(ffiGeneratorResponseFromParseError(err))
	}

	if blocked := a.checkSchemaTrustGate(strings.TrimSpace(req.ConnectionID)); blocked != nil {
		return marshalGeneratorResponse(blocked)
	}

	// 获取字段 schema
	fieldSchema, err := a.schemaProvider.GetFieldSchema(context.Background(), req.ConnectionID, req.DraftConfig.Table, req.DraftConfig.Column)
	if err != nil {
		// Schema 不存在时返回校验失败
		resp := &FFIResponse{
			Ok: true,
			Data: ValidateFieldGeneratorConfigResponse{
				Valid: false,
				Errors: []FieldErrorData{
					{Code: "CURRENT_SCHEMA_NOT_FOUND", Path: "field_schema", Message: "field schema not found"},
				},
			},
		}
		return marshalGeneratorResponse(resp)
	}

	// 解析候选
	resolver := generator.NewGeneratorTypeResolver(a.registry)
	result, _ := resolver.ResolveCandidates(fieldSchema)

	// 执行校验
	validator := generator.NewGeneratorConfigValidator(a.registry)
	draftReq := generator.SaveFieldGeneratorConfigRequest{
		ConnectionID:   req.ConnectionID,
		Table:          req.DraftConfig.Table,
		Column:         req.DraftConfig.Column,
		GeneratorType:  generator.GeneratorType(req.DraftConfig.GeneratorType),
		GeneratorOpts:  parseGeneratorOpts(req.DraftConfig.GeneratorOpts),
		ModifiedSource: req.DraftConfig.ModifiedSource,
		IsEnabled:      req.DraftConfig.IsEnabled,
	}
	validationErr := validator.Validate(draftReq, fieldSchema, result.Candidates)

	var errors []FieldErrorData
	var valid bool
	if validationErr == nil {
		valid = true
		errors = []FieldErrorData{}
	} else {
		valid = false
		errors = make([]FieldErrorData, len(validationErr.FieldErrors))
		for i, fe := range validationErr.FieldErrors {
			errors[i] = FieldErrorData{
				Code:       fe.Code,
				Path:       fe.Path,
				Message:    app.SanitizeErrorForTest(fe.Message), // 脱敏
				Suggestion: fe.Suggestion,
			}
		}
	}

	resp := &FFIResponse{
		Ok: true,
		Data: ValidateFieldGeneratorConfigResponse{
			Valid:  valid,
			Errors: errors,
		},
	}

	return marshalGeneratorResponse(resp)
}

// ===== 辅助方法 =====

// saveFieldConfig 执行配置保存（内部方法）。
func (a *GeneratorFFIAdapter) saveFieldConfig(ctx context.Context, req generator.SaveFieldGeneratorConfigRequest) (generator.FieldGeneratorConfig, *generator.ValidationError) {
	// 获取字段 schema
	fieldSchema, err := a.schemaProvider.GetFieldSchema(ctx, req.ConnectionID, req.Table, req.Column)
	if err != nil {
		return generator.FieldGeneratorConfig{}, &generator.ValidationError{
			FieldErrors: []generator.FieldError{
				{Code: "CURRENT_SCHEMA_NOT_FOUND", Path: "field_schema", Message: "field schema not found"},
			},
		}
	}

	// 解析候选
	resolver := generator.NewGeneratorTypeResolver(a.registry)
	result, resolveErr := resolver.ResolveCandidates(fieldSchema)
	if resolveErr != nil {
		gErr, ok := resolveErr.(*generator.GeneratorError)
		if ok {
			return generator.FieldGeneratorConfig{}, &generator.ValidationError{
				FieldErrors: []generator.FieldError{
					{Code: string(gErr.Code), Path: gErr.Path, Message: gErr.Message},
				},
			}
		}
		return generator.FieldGeneratorConfig{}, &generator.ValidationError{
			FieldErrors: []generator.FieldError{
				{Code: "INTERNAL", Path: "resolver", Message: "resolve candidates failed"},
			},
		}
	}

	// 校验
	validator := generator.NewGeneratorConfigValidator(a.registry)
	validationErr := validator.Validate(req, fieldSchema, result.Candidates)
	if validationErr != nil {
		return generator.FieldGeneratorConfig{}, validationErr
	}

	// 保存
	config := generator.FieldGeneratorConfig{
		ColumnSchemaID: fieldSchema.ColumnID,
		ConnectionID:   req.ConnectionID,
		Table:          req.Table,
		Column:         req.Column,
		GeneratorType:  req.GeneratorType,
		GeneratorOpts:  req.GeneratorOpts,
		SeedPolicy:     req.SeedPolicy,
		NullPolicy:     req.NullPolicy,
		IsEnabled:      req.IsEnabled,
		ModifiedSource: req.ModifiedSource,
		UpdatedAtUnix:  time.Now().Unix(),
	}

	saved, saveErr := a.configRepo.UpsertFieldConfig(ctx, config)
	if saveErr != nil {
		return generator.FieldGeneratorConfig{}, &generator.ValidationError{
			FieldErrors: []generator.FieldError{
				{Code: "STORAGE_ERROR", Path: "repository", Message: app.SanitizeErrorForTest(saveErr.Error())},
			},
		}
	}

	return saved, nil
}

// ===== 转换函数 =====

func convertPreviewMetadata(meta generator.PreviewMetadata) PreviewMetadataData {
	paramsSummary := meta.ParamsSummary
	if paramsSummary == nil {
		paramsSummary = map[string]interface{}{}
	}
	return PreviewMetadataData{
		Scope: PreviewScopeData{
			Type:   string(meta.Scope.Type),
			Table:  meta.Scope.Table,
			Column: meta.Scope.Column,
		},
		GeneratorType: string(meta.GeneratorType),
		ParamsSummary: paramsSummary,
		SampleSize:     meta.SampleSize,
		Seed:           meta.Seed,
		GeneratedAt:    meta.GeneratedAt.Format(time.RFC3339),
		Deterministic:  meta.Deterministic,
		PartialSuccess: meta.PartialSuccess,
		SeedSource:     meta.SeedSource,
	}
}

func convertPreviewWarnings(warnings []generator.PreviewWarning) []FFIWarning {
	result := make([]FFIWarning, len(warnings))
	for i, w := range warnings {
		result[i] = FFIWarning{
			Code:    w.Code,
			Field:   w.Field,
			Message: app.SanitizeErrorForTest(w.Message),
		}
	}
	return result
}

func convertPreviewFieldResults(results []generator.PreviewFieldResult) []PreviewFieldResultData {
	if results == nil {
		return nil
	}
	out := make([]PreviewFieldResultData, len(results))
	for i, r := range results {
		var errCodePtr *string
		if strings.TrimSpace(r.ErrorCode) != "" {
			tmp := r.ErrorCode
			errCodePtr = &tmp
		}
		var warningPtr *string
		if strings.TrimSpace(r.Warning) != "" {
			tmp := r.Warning
			warningPtr = &tmp
		}
		out[i] = PreviewFieldResultData{
			Field:       r.Field,
			Status:      r.Status,
			SampleCount: r.SampleCount,
			ErrorCode:   errCodePtr,
			Warning:     warningPtr,
		}
	}
	return out
}

func extractSupportsTypes(typeTags []string) []string {
	result := []string{}
	for _, tag := range typeTags {
		if strings.HasPrefix(tag, "supports:") {
			result = append(result, strings.TrimPrefix(tag, "supports:"))
		}
	}
	return result
}

func extractCapability(typeTags []string) GeneratorCapabilityExtension {
	cap := GeneratorCapabilityExtension{}
	for _, tag := range typeTags {
		switch strings.TrimSpace(tag) {
		case "requires_external_feed":
			cap.RequiresExternalFeed = true
		case "requires_computed_context":
			cap.RequiresComputedContext = true
		case "accepts_enum_values":
			cap.AcceptsEnumValues = true
		}
	}
	return cap
}

func parseGeneratorOpts(optsJSON string) map[string]interface{} {
	if optsJSON == "" {
		return map[string]interface{}{}
	}
	opts := map[string]interface{}{}
	if err := json.Unmarshal([]byte(optsJSON), &opts); err != nil {
		return map[string]interface{}{}
	}
	return opts
}

func sanitizeGeneratorOpts(opts map[string]interface{}) map[string]interface{} {
	// 脱敏敏感字段
	sensitiveKeys := []string{"api_key", "token", "password", "secret", "credential"}
	for _, key := range sensitiveKeys {
		if _, ok := opts[key]; ok {
			opts[key] = "***"
		}
	}
	return opts
}

func sanitizeGeneratorOptsJSON(opts map[string]interface{}) string {
	sanitized := sanitizeGeneratorOpts(opts)
	b, err := json.Marshal(sanitized)
	if err != nil {
		return "{}"
	}
	return string(b)
}

func sanitizeSeedPolicyJSON(policy map[string]interface{}) string {
	if policy == nil {
		return ""
	}
	b, err := json.Marshal(policy)
	if err != nil {
		return ""
	}
	return string(b)
}

// ===== 响应构造函数 =====

func ffiGeneratorResponseFromParseError(err error) *FFIResponse {
	return &FFIResponse{
		Ok: false,
		Error: &FFIError{
			Code:    app.CodeInvalidArgument,
			Message: "invalid request JSON",
			Details: map[string]string{"cause": err.Error()},
		},
	}
}

func ffiGeneratorResponseFromError(err error) *FFIResponse {
	gErr, ok := err.(*generator.GeneratorError)
	if ok {
		return &FFIResponse{
			Ok: false,
			Error: &FFIError{
				Code:    string(gErr.Code),
				Message: app.SanitizeErrorForTest(gErr.Message),
				Details: map[string]string{"path": gErr.Path},
			},
		}
	}
	return &FFIResponse{
		Ok: false,
		Error: &FFIError{
			Code:    "INTERNAL",
			Message: app.SanitizeErrorForTest(err.Error()),
		},
	}
}

func ffiGeneratorResponseFromValidationError(err *generator.ValidationError) *FFIResponse {
	if err == nil || len(err.FieldErrors) == 0 {
		return &FFIResponse{
			Ok: true,
			Data: ValidateFieldGeneratorConfigResponse{
				Valid:  true,
				Errors: []FieldErrorData{},
			},
		}
	}

	errors := make([]FieldErrorData, len(err.FieldErrors))
	for i, fe := range err.FieldErrors {
		errors[i] = FieldErrorData{
			Code:       fe.Code,
			Path:       fe.Path,
			Message:    app.SanitizeErrorForTest(fe.Message),
			Suggestion: fe.Suggestion,
		}
	}

	return &FFIResponse{
		Ok: false,
		Error: &FFIError{
			Code:    string(generator.GeneratorErrInvalidArgument),
			Message: fmt.Sprintf("validation failed: %s", err.FieldErrors[0].Message),
			Details: map[string]string{"errors": fmt.Sprintf("%d", len(err.FieldErrors))},
		},
	}
}

func marshalGeneratorResponse(resp *FFIResponse) string {
	b, err := json.Marshal(resp)
	if err != nil {
		return "{\"ok\":false,\"error\":{\"code\":\"INTERNAL\",\"message\":\"response serialization failed\"}}"
	}
	return string(b)
}

// checkSchemaTrustGate 检查 schema trust 门禁；被阻断时返回失败响应，否则返回 nil。
func (a *GeneratorFFIAdapter) checkSchemaTrustGate(connectionID string) *FFIResponse {
	if a == nil || a.trustReader == nil {
		return nil
	}
	view, err := a.trustReader.GetSchemaTrustState(connectionID)
	if err != nil {
		// trust 状态读取失败时，不冒进执行业务；按前置条件失败处理，避免基于不可信状态继续下游链路。
		return &FFIResponse{
			Ok: false,
			Error: &FFIError{
				Code:    string(generator.GeneratorErrFailedPrecondition),
				Reason:  "SCHEMA_TRUST_STATE_UNAVAILABLE",
				Message: "schema trust gate blocked",
			},
		}
	}
	switch view.State {
	case schema.SchemaTrustPendingRescan:
		return &FFIResponse{
			Ok: false,
			Error: &FFIError{
				Code:    string(generator.GeneratorErrFailedPrecondition),
				Reason:  generatorFFITrustReasonPendingRescan,
				Message: "schema trust gate blocked",
			},
		}
	case schema.SchemaTrustPendingAdjustment:
		return &FFIResponse{
			Ok: false,
			Error: &FFIError{
				Code:    string(generator.GeneratorErrFailedPrecondition),
				Reason:  generatorFFITrustReasonPendingAdjustment,
				Message: "schema trust gate blocked",
			},
		}
	default:
		return nil
	}
}

// schemaTrustWarning 为 GetFieldGeneratorConfig 的只读例外构造 warnings[]。
func (a *GeneratorFFIAdapter) schemaTrustWarning(connectionID string) []FFIWarning {
	if a == nil || a.trustReader == nil {
		return []FFIWarning{}
	}
	view, err := a.trustReader.GetSchemaTrustState(connectionID)
	if err != nil {
		return []FFIWarning{{
			Code:    string(generator.GeneratorErrFailedPrecondition),
			Reason:  "SCHEMA_TRUST_STATE_UNAVAILABLE",
			Message: "schema trust state is unavailable",
		}}
	}
	switch view.State {
	case schema.SchemaTrustPendingRescan:
		return []FFIWarning{{
			Code:    string(generator.GeneratorErrFailedPrecondition),
			Reason:  generatorFFITrustReasonPendingRescan,
			Message: "schema trust gate blocked",
		}}
	case schema.SchemaTrustPendingAdjustment:
		return []FFIWarning{{
			Code:    string(generator.GeneratorErrFailedPrecondition),
			Reason:  generatorFFITrustReasonPendingAdjustment,
			Message: "schema trust gate blocked",
		}}
	default:
		return []FFIWarning{}
	}
}