// Package generator 提供 spec-03 生成器预览与运行时能力。
package generator

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

// PreviewScopeType 定义预览范围类型。
type PreviewScopeType string

const (
	// PreviewScopeField 表示单字段预览范围。
	PreviewScopeField PreviewScopeType = "field"

	// PreviewScopeTable 表示单表预览范围。
	PreviewScopeTable PreviewScopeType = "table"
)

// PreviewScope 描述预览范围结构。
type PreviewScope struct {
	// Type 是范围类型（field 或 table）。
	Type PreviewScopeType

	// Table 是目标表名。
	Table string

	// Column 是目标字段名（仅 field scope 需要）。
	Column string
}

// PreviewRequest 是预览请求参数结构。
type PreviewRequest struct {
	// ConnectionID 是连接标识。
	ConnectionID string

	// Scope 是预览范围类型。
	Scope PreviewScopeType

	// Table 是目标表名。
	Table string

	// Column 是目标字段名（仅 field scope 需要）。
	Column string

	// SampleSize 是样本数量。
	SampleSize int

	// Seed 是可选种子值（用于确定性复现）。
	Seed *int64
}

// PreviewWarning 表示预览中的警告信息。
type PreviewWarning struct {
	// Code 是稳定的警告码。
	Code string

	// Field 是相关字段名。
	Field string

	// Message 是可读描述。
	Message string
}

// PreviewFieldResult 表示 scope=table 时的字段级结果清单。
type PreviewFieldResult struct {
	// Field 是字段名。
	Field string

	// Status 是字段状态（ok, skipped, failed）。
	Status string

	// SampleCount 是生成的样本数量。
	SampleCount int

	// ErrorCode 是可选错误码。
	ErrorCode string

	// Warning 是可选警告提示。
	Warning string
}

// PreviewMetadata 包含预览元信息。
type PreviewMetadata struct {
	// Scope 是预览范围结构。
	Scope PreviewScope

	// GeneratorType 是使用的生成器类型（field scope 时）。
	GeneratorType GeneratorType

	// SampleSize 是请求的样本数量。
	SampleSize int

	// Seed 是生效的种子值。
	Seed int64

	// Deterministic 表示本次输出是否可确定性复现。
	Deterministic bool

	// GeneratedAt 是生成时间戳。
	GeneratedAt time.Time

	// PartialSuccess 表示是否为部分成功（仅 table scope）。
	PartialSuccess bool

	// SeedSource 表示种子来源。
	SeedSource string // preview_override, field_fixed, global, none
}

// PreviewResult 是预览服务的完整响应结构。
type PreviewResult struct {
	// Scope 是预览范围结构。
	Scope PreviewScope

	// Samples 是样本数据（field scope 时为 []interface{}，table scope 时为 map[string][]interface{}）。
	Samples interface{}

	// Metadata 包含预览元信息。
	Metadata PreviewMetadata

	// Warnings 包含警告信息列表。
	Warnings []PreviewWarning

	// FieldResults 包含字段级结果清单（仅 table scope 时必填）。
	FieldResults []PreviewFieldResult
}

// PreviewServiceConfigRepository 扩展配置仓储接口以支持表范围查询。
type PreviewServiceConfigRepository interface {
	GeneratorConfigRepository
	// ListByTable 按 connection/table 列出所有字段配置。
	ListByTable(ctx context.Context, connectionID string, table string) ([]FieldGeneratorConfig, error)
}

// PreviewServiceSchemaProvider 扩展 schema 提供器以支持表范围查询。
type PreviewServiceSchemaProvider interface {
	FieldSchemaProvider
	// ListFieldsByTable 按 connection/table 列出所有字段 schema。
	ListFieldsByTable(ctx context.Context, connectionID string, table string) ([]FieldSchema, error)
}

// PreviewRuntime 接口定义预览运行时的行为。
type PreviewRuntime interface {
	// GenerateBatch 执行批量生成。
	GenerateBatch(ctx context.Context, generator Generator, genCtx GeneratorContext, count int) ([]interface{}, error)
	// Generate 执行单值生成。
	Generate(ctx context.Context, generator Generator, genCtx GeneratorContext) (interface{}, error)
}

// GeneratorPreviewService 提供预览能力，不触发真实写入。
type GeneratorPreviewService struct {
	// registry 用于查询生成器实例。
	registry *GeneratorRegistry

	// configRepo 用于读取字段配置。
	configRepo PreviewServiceConfigRepository

	// schemaProvider 用于读取字段 schema。
	schemaProvider PreviewServiceSchemaProvider

	// runtime 用于执行生成器调用。
	runtime PreviewRuntime
}

// NewGeneratorPreviewService 创建预览服务实例。
//
// 输入：
// - registry: 生成器注册表实例。
// - configRepo: 扩展的配置仓储实例。
// - schemaProvider: 扩展的 schema 提供器实例。
// - runtime: 生成器运行时实例。
//
// 输出：
// - *GeneratorPreviewService: 初始化后的预览服务实例。
func NewGeneratorPreviewService(
	registry *GeneratorRegistry,
	configRepo PreviewServiceConfigRepository,
	schemaProvider PreviewServiceSchemaProvider,
	runtime PreviewRuntime,
) *GeneratorPreviewService {
	return &GeneratorPreviewService{
		registry:       registry,
		configRepo:     configRepo,
		schemaProvider: schemaProvider,
		runtime:        runtime,
	}
}

// PreviewGeneration 执行预览生成，返回样本数据与元信息。
//
// 输入：
// - ctx: 调用上下文。
// - req: 预览请求参数。
//
// 输出：
// - *PreviewResult: 预览结果（包含 samples、metadata、warnings、field_results）。
// - error: 预览失败时返回结构化错误。
func (s *GeneratorPreviewService) PreviewGeneration(ctx context.Context, req PreviewRequest) (*PreviewResult, error) {
	// 参数校验
	if strings.TrimSpace(req.ConnectionID) == "" {
		return nil, &GeneratorError{Code: GeneratorErrInvalidArgument, Path: "connection_id", Message: "connection_id is required"}
	}
	if strings.TrimSpace(req.Table) == "" {
		return nil, &GeneratorError{Code: GeneratorErrInvalidArgument, Path: "table", Message: "table is required"}
	}
	if req.SampleSize <= 0 {
		return nil, &GeneratorError{Code: GeneratorErrInvalidArgument, Path: "sample_size", Message: "sample_size must be positive"}
	}

	switch req.Scope {
	case PreviewScopeField:
		return s.previewFieldScope(ctx, req)
	case PreviewScopeTable:
		return s.previewTableScope(ctx, req)
	default:
		return nil, &GeneratorError{Code: GeneratorErrInvalidArgument, Path: "scope", Message: "scope must be field or table"}
	}
}

// previewFieldScope 执行单字段预览。
func (s *GeneratorPreviewService) previewFieldScope(ctx context.Context, req PreviewRequest) (*PreviewResult, error) {
	if strings.TrimSpace(req.Column) == "" {
		return nil, &GeneratorError{Code: GeneratorErrInvalidArgument, Path: "column", Message: "column is required for field scope"}
	}

	// 加载字段配置
	config, err := s.configRepo.GetByField(ctx, req.ConnectionID, req.Table, req.Column)
	if err != nil {
		return nil, &GeneratorError{Code: GeneratorErrNotRegistered, Path: "config", Message: "field generator config not found"}
	}

	// 检查是否启用
	if !config.IsEnabled {
		return nil, &GeneratorError{Code: GeneratorErrFailedPrecondition, Path: "is_enabled", Message: "field generator is disabled by config"}
	}

	// 加载字段 schema
	fieldSchema, err := s.schemaProvider.GetFieldSchema(ctx, req.ConnectionID, req.Table, req.Column)
	if err != nil {
		return nil, &GeneratorError{Code: GeneratorErrFailedPrecondition, Path: "field_schema", Message: "field schema not found"}
	}

	// 解析生成器
	generator, err := s.registry.Resolve(config.GeneratorType)
	if err != nil {
		return nil, err
	}

	// 计算生效种子
	effectiveSeed, seedSource := resolveEffectiveSeed(req.Seed, config.SeedPolicy)

	// 构造生成上下文
	genCtx := GeneratorContext{
		ConnectionID: req.ConnectionID,
		Table:        req.Table,
		Column:       req.Column,
		FieldType:    fieldSchema.AbstractType,
		Params:       config.GeneratorOpts,
		Seed:         effectiveSeed,
	}

	// 执行生成
	samples, err := s.runtime.GenerateBatch(ctx, generator, genCtx, req.SampleSize)
	if err != nil {
		return nil, err
	}

	// 构造结果
	meta := generator.Meta()
	deterministic := effectiveSeed != nil && meta.Deterministic

	result := &PreviewResult{
		Scope: PreviewScope{
			Type:   PreviewScopeField,
			Table:  req.Table,
			Column: req.Column,
		},
		Samples: samples,
		Metadata: PreviewMetadata{
			Scope: PreviewScope{
				Type:   PreviewScopeField,
				Table:  req.Table,
				Column: req.Column,
			},
			GeneratorType: config.GeneratorType,
			SampleSize:    req.SampleSize,
			Seed:          seedToInt64(effectiveSeed),
			Deterministic: deterministic,
			GeneratedAt:   time.Now().UTC(),
			SeedSource:    seedSource,
		},
		Warnings:     []PreviewWarning{},
		FieldResults: nil, // field scope 不需要 field_results
	}

	return result, nil
}

// previewTableScope 执行单表预览（支持部分成功）。
func (s *GeneratorPreviewService) previewTableScope(ctx context.Context, req PreviewRequest) (*PreviewResult, error) {
	// 加载表范围字段配置
	configs, err := s.configRepo.ListByTable(ctx, req.ConnectionID, req.Table)
	if err != nil {
		return nil, &GeneratorError{Code: GeneratorErrFailedPrecondition, Path: "configs", Message: "failed to load field configs for table"}
	}

	// 加载表范围字段 schema
	fieldSchemas, err := s.schemaProvider.ListFieldsByTable(ctx, req.ConnectionID, req.Table)
	if err != nil {
		return nil, &GeneratorError{Code: GeneratorErrFailedPrecondition, Path: "field_schemas", Message: "failed to load field schemas for table"}
	}

	// 建立配置映射
	configMap := make(map[string]FieldGeneratorConfig)
	for _, cfg := range configs {
		configMap[cfg.Column] = cfg
	}

	// 建立 schema 映射
	schemaMap := make(map[string]FieldSchema)
	for _, fs := range fieldSchemas {
		schemaMap[fs.Column] = fs
	}

	// 执行逐字段生成
	samplesMap := make(map[string][]interface{})
	warnings := make([]PreviewWarning, 0)
	fieldResults := make([]PreviewFieldResult, 0)
	hasSuccess := false
	hasFailure := false

	for colName, cfg := range configMap {
		fieldResult := PreviewFieldResult{
			Field:       colName,
			SampleCount: 0,
		}

		// 检查是否启用
		if !cfg.IsEnabled {
			fieldResult.Status = "skipped"
			fieldResult.Warning = "GENERATOR_DISABLED"
			warnings = append(warnings, PreviewWarning{
				Code:    "GENERATOR_DISABLED",
				Field:   colName,
				Message: "field generator is disabled by config",
			})
			fieldResults = append(fieldResults, fieldResult)
			hasFailure = true
			continue
		}

		// 获取字段 schema
		fieldSchema, ok := schemaMap[colName]
		if !ok {
			fieldResult.Status = "failed"
			fieldResult.ErrorCode = "CURRENT_SCHEMA_NOT_FOUND"
			warnings = append(warnings, PreviewWarning{
				Code:    "CURRENT_SCHEMA_NOT_FOUND",
				Field:   colName,
				Message: "field schema not found",
			})
			fieldResults = append(fieldResults, fieldResult)
			hasFailure = true
			continue
		}

		// 解析生成器
		generator, err := s.registry.Resolve(cfg.GeneratorType)
		if err != nil {
			fieldResult.Status = "failed"
			gErr, ok := err.(*GeneratorError)
			if ok {
				fieldResult.ErrorCode = string(gErr.Code)
			} else {
				fieldResult.ErrorCode = "GENERATOR_NOT_REGISTERED"
			}
			warnings = append(warnings, PreviewWarning{
				Code:    fieldResult.ErrorCode,
				Field:   colName,
				Message: err.Error(),
			})
			fieldResults = append(fieldResults, fieldResult)
			hasFailure = true
			continue
		}

		// 计算生效种子（表范围使用同一请求种子）
		effectiveSeed, _ := resolveEffectiveSeed(req.Seed, cfg.SeedPolicy)

		// 构造生成上下文
		genCtx := GeneratorContext{
			ConnectionID: req.ConnectionID,
			Table:        req.Table,
			Column:       colName,
			FieldType:    fieldSchema.AbstractType,
			Params:       cfg.GeneratorOpts,
			Seed:         effectiveSeed,
		}

		// 执行生成
		samples, genErr := s.runtime.GenerateBatch(ctx, generator, genCtx, req.SampleSize)
		if genErr != nil {
			fieldResult.Status = "failed"
			gErr, ok := genErr.(*GeneratorError)
			if ok {
				fieldResult.ErrorCode = string(gErr.Code)
			} else {
				fieldResult.ErrorCode = "INTERNAL"
			}
			warnings = append(warnings, PreviewWarning{
				Code:    fieldResult.ErrorCode,
				Field:   colName,
				Message: genErr.Error(),
			})
			fieldResults = append(fieldResults, fieldResult)
			hasFailure = true
			continue
		}

		// 成功字段
		fieldResult.Status = "ok"
		fieldResult.SampleCount = len(samples)
		samplesMap[colName] = samples
		fieldResults = append(fieldResults, fieldResult)
		hasSuccess = true
	}

	// 确定性判断：检查所有成功字段的生成器是否都支持确定性
	deterministic := false
	if req.Seed != nil {
		deterministic = true
		for colName := range samplesMap {
			cfg := configMap[colName]
			gen, _ := s.registry.Resolve(cfg.GeneratorType)
			if gen != nil && !gen.Meta().Deterministic {
				deterministic = false
				break
			}
		}
	}

	result := &PreviewResult{
		Scope: PreviewScope{
			Type:  PreviewScopeTable,
			Table: req.Table,
		},
		Samples: samplesMap,
		Metadata: PreviewMetadata{
			Scope: PreviewScope{
				Type:  PreviewScopeTable,
				Table: req.Table,
			},
			SampleSize:     req.SampleSize,
			Seed:           seedToInt64(req.Seed),
			Deterministic:  deterministic,
			GeneratedAt:    time.Now().UTC(),
			PartialSuccess: hasSuccess && hasFailure,
		},
		Warnings:     warnings,
		FieldResults: fieldResults,
	}

	return result, nil
}

// resolveEffectiveSeed 计算生效种子与来源。
func resolveEffectiveSeed(requestSeed *int64, fieldSeedPolicy map[string]interface{}) (*int64, string) {
	// 优先级 1: 请求显式种子（最高）
	if requestSeed != nil {
		return requestSeed, "preview_override"
	}

	// 优先级 2: 字段 seed_policy.mode=fixed 且提供 seed
	if fieldSeedPolicy != nil {
		mode, ok := fieldSeedPolicy["mode"].(string)
		if ok && strings.TrimSpace(mode) == "fixed" {
			seedRaw, ok := fieldSeedPolicy["seed"]
			if ok {
				seed := toInt64(seedRaw)
				if seed != 0 {
					return &seed, "field_fixed"
				}
			}
		}
	}

	// 优先级 3-4: 全局种子或无种子（MVP 暂不支持全局种子）
	return nil, "none"
}

// seedToInt64 转换种子为 int64（nil 时返回 0）。
func seedToInt64(seed *int64) int64 {
	if seed == nil {
		return 0
	}
	return *seed
}

// toInt64 从 interface{} 转换为 int64。
func toInt64(v interface{}) int64 {
	switch val := v.(type) {
	case int:
		return int64(val)
	case int32:
		return int64(val)
	case int64:
		return val
	case float64:
		return int64(val)
	case float32:
		return int64(val)
	default:
		return 0
	}
}

// GeneratorRuntime 执行单字段生成调用。
type GeneratorRuntime struct{}

// NewGeneratorRuntime 创建运行时实例。
//
// 输入：
// - 无。
//
// 输出：
// - *GeneratorRuntime: 初始化后的运行时实例。
func NewGeneratorRuntime() *GeneratorRuntime {
	return &GeneratorRuntime{}
}

// GenerateBatch 执行批量生成。
//
// 输入：
// - ctx: 调用上下文。
// - generator: 目标生成器实例。
// - genCtx: 生成上下文。
// - count: 样本数量。
//
// 输出：
// - []interface{}: 生成的样本数组。
// - error: 生成失败时返回结构化错误。
func (r *GeneratorRuntime) GenerateBatch(ctx context.Context, generator Generator, genCtx GeneratorContext, count int) ([]interface{}, error) {
	if generator == nil {
		return nil, &GeneratorError{Code: GeneratorErrInvalidArgument, Path: "generator", Message: "generator is required"}
	}

	// 重置生成器状态（确保确定性边界）
	if err := generator.Reset(); err != nil {
		return nil, &GeneratorError{Code: GeneratorErrFailedPrecondition, Path: "generator.reset", Message: fmt.Sprintf("generator reset failed: %v", err)}
	}

	// 执行批量生成
	values, err := generator.GenerateBatch(ctx, genCtx, count)
	if err != nil {
		// 归一化未知错误
		if gErr, ok := err.(*GeneratorError); ok {
			return nil, gErr
		}
		return nil, &GeneratorError{Code: GeneratorErrFailedPrecondition, Path: "generator.generate_batch", Message: err.Error()}
	}

	return values, nil
}

// Generate 执行单值生成。
//
// 输入：
// - ctx: 调用上下文。
// - generator: 目标生成器实例。
// - genCtx: 生成上下文。
//
// 输出：
// - interface{}: 生成的单个值。
// - error: 生成失败时返回结构化错误。
func (r *GeneratorRuntime) Generate(ctx context.Context, generator Generator, genCtx GeneratorContext) (interface{}, error) {
	if generator == nil {
		return nil, &GeneratorError{Code: GeneratorErrInvalidArgument, Path: "generator", Message: "generator is required"}
	}

	// 重置生成器状态
	if err := generator.Reset(); err != nil {
		return nil, &GeneratorError{Code: GeneratorErrFailedPrecondition, Path: "generator.reset", Message: fmt.Sprintf("generator reset failed: %v", err)}
	}

	// 执行单值生成
	value, err := generator.Generate(ctx, genCtx)
	if err != nil {
		if gErr, ok := err.(*GeneratorError); ok {
			return nil, gErr
		}
		return nil, &GeneratorError{Code: GeneratorErrFailedPrecondition, Path: "generator.generate", Message: err.Error()}
	}

	return value, nil
}

// PreviewableConfigRepository 扩展 InMemoryConfigRepository 支持表范围查询。
type PreviewableConfigRepository struct {
	*InMemoryConfigRepository
}

// NewPreviewableConfigRepository 创建支持预览的配置仓储。
//
// 输入：
// - 无。
//
// 输出：
// - *PreviewableConfigRepository: 初始化后的仓储实例。
func NewPreviewableConfigRepository() *PreviewableConfigRepository {
	return &PreviewableConfigRepository{
		InMemoryConfigRepository: NewInMemoryConfigRepository(),
	}
}

// ListByTable 按 connection/table 列出所有字段配置。
//
// 输入：
// - ctx: 调用上下文（当前实现未使用）。
// - connectionID: 连接标识。
// - table: 表名。
//
// 输出：
// - []FieldGeneratorConfig: 匹配的字段配置列表。
// - error: 查询失败时返回错误；成功时返回 nil。
func (r *PreviewableConfigRepository) ListByTable(_ context.Context, connectionID string, table string) ([]FieldGeneratorConfig, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	prefix := strings.ToLower(strings.TrimSpace(connectionID)) + "|" + strings.ToLower(strings.TrimSpace(table)) + "|"
	results := make([]FieldGeneratorConfig, 0)

	for locator, columnID := range r.byLocator {
		if strings.HasPrefix(locator, prefix) {
			cfg, ok := r.byColumnID[columnID]
			if ok {
				results = append(results, cfg)
			}
		}
	}

	return results, nil
}

// PreviewableSchemaProvider 扩展 StaticSchemaProvider 支持表范围查询。
type PreviewableSchemaProvider struct {
	*StaticSchemaProvider
	// byTable 按 connection|table 保存字段列表
	byTable map[string][]FieldSchema
}

// NewPreviewableSchemaProvider 创建支持预览的 schema 提供器。
//
// 输入：
// - fields: 初始字段 schema 列表。
//
// 输出：
// - *PreviewableSchemaProvider: 初始化后的 schema 提供器实例。
func NewPreviewableSchemaProvider(fields []FieldSchema) *PreviewableSchemaProvider {
	base := NewStaticSchemaProvider(fields)
	byTable := make(map[string][]FieldSchema)
	for _, field := range fields {
		if strings.TrimSpace(field.ColumnID) == "" {
			field.ColumnID = field.ConnectionID + ":" + field.Table + ":" + field.Column
		}
		tableKey := strings.ToLower(strings.TrimSpace(field.ConnectionID)) + "|" + strings.ToLower(strings.TrimSpace(field.Table))
		byTable[tableKey] = append(byTable[tableKey], field)
	}
	return &PreviewableSchemaProvider{
		StaticSchemaProvider: base,
		byTable:              byTable,
	}
}

// ListFieldsByTable 按 connection/table 列出所有字段 schema。
//
// 输入：
// - ctx: 调用上下文（当前实现未使用）。
// - connectionID: 连接标识。
// - table: 表名。
//
// 输出：
// - []FieldSchema: 匹配的字段 schema 列表。
// - error: 查询失败时返回错误；成功时返回 nil。
func (p *PreviewableSchemaProvider) ListFieldsByTable(_ context.Context, connectionID string, table string) ([]FieldSchema, error) {
	tableKey := strings.ToLower(strings.TrimSpace(connectionID)) + "|" + strings.ToLower(strings.TrimSpace(table))
	fields, ok := p.byTable[tableKey]
	if !ok {
		return []FieldSchema{}, nil // 返回空列表，不是错误
	}
	return fields, nil
}

// ===== 并发安全包装（用于跨字段并发生成） =====

// ConcurrentPreviewService 包装预览服务，支持字段级并发生成。
type ConcurrentPreviewService struct {
	// base 是基础预览服务。
	base *GeneratorPreviewService

	// parallelism 控制最大并发数。
	parallelism int
}

// NewConcurrentPreviewService 创建支持并发预览的服务。
//
// 输入：
// - base: 基础预览服务实例。
// - parallelism: 最大并发数。
//
// 输出：
// - *ConcurrentPreviewService: 初始化后的并发预览服务实例。
func NewConcurrentPreviewService(base *GeneratorPreviewService, parallelism int) *ConcurrentPreviewService {
	return &ConcurrentPreviewService{
		base:       base,
		parallelism: parallelism,
	}
}

// PreviewGeneration 执行预览（当前实现委托给基础服务）。
func (s *ConcurrentPreviewService) PreviewGeneration(ctx context.Context, req PreviewRequest) (*PreviewResult, error) {
	return s.base.PreviewGeneration(ctx, req)
}

// fieldGenerateTask 表示单字段生成任务。
type fieldGenerateTask struct {
	colName string
	config  FieldGeneratorConfig
	schema  FieldSchema
}

// fieldGenerateResult 表示单字段生成结果。
type fieldGenerateResult struct {
	colName string
	samples []interface{}
	err     error
}

// generateFieldsParallel 并发生成多字段样本。
func (s *ConcurrentPreviewService) generateFieldsParallel(
	ctx context.Context,
	tasks []fieldGenerateTask,
	requestSeed *int64,
	sampleSize int,
) []fieldGenerateResult {
	results := make([]fieldGenerateResult, len(tasks))

	// 使用 semaphore 控制并发
	sem := make(chan struct{}, s.parallelism)
	var wg sync.WaitGroup

	for i, task := range tasks {
		wg.Add(1)
		go func(idx int, t fieldGenerateTask) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			// 计算生效种子
			effectiveSeed, _ := resolveEffectiveSeed(requestSeed, t.config.SeedPolicy)

			// 构造上下文
			genCtx := GeneratorContext{
				ConnectionID: t.schema.ConnectionID,
				Table:        t.schema.Table,
				Column:       t.colName,
				FieldType:    t.schema.AbstractType,
				Params:       t.config.GeneratorOpts,
				Seed:         effectiveSeed,
			}

			// 解析生成器
			generator, err := s.base.registry.Resolve(t.config.GeneratorType)
			if err != nil {
				results[idx] = fieldGenerateResult{colName: t.colName, err: err}
				return
			}

			// 执行生成
			samples, err := s.base.runtime.GenerateBatch(ctx, generator, genCtx, sampleSize)
			results[idx] = fieldGenerateResult{colName: t.colName, samples: samples, err: err}
		}(i, task)
	}

	wg.Wait()
	return results
}