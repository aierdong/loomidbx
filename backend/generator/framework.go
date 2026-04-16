// Package generator 提供 spec-03 生成器能力链：
// registry -> resolver -> validator -> config repository/service。
package generator

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	// GeneratorTypeIntRangeRandom 是 int 类型的默认生成器类型。
	GeneratorTypeIntRangeRandom GeneratorType = "int_range_random"

	// GeneratorTypeDecimalRangeRandom 是 decimal 类型的默认生成器类型。
	GeneratorTypeDecimalRangeRandom GeneratorType = "decimal_range_random"

	// GeneratorTypeStringRandomChars 是 string 类型的默认生成器类型。
	GeneratorTypeStringRandomChars GeneratorType = "string_random_chars"

	// GeneratorTypeBooleanRatio 是 boolean 类型的默认生成器类型。
	GeneratorTypeBooleanRatio GeneratorType = "boolean_ratio"

	// GeneratorTypeDatetimeRangeRandom 是 datetime 类型的默认生成器类型。
	GeneratorTypeDatetimeRangeRandom GeneratorType = "datetime_range_random"

	// GeneratorTypeEnumValue 是通用枚举值生成器。
	GeneratorTypeEnumValue GeneratorType = "enum_value"
)

const (
	// ModifiedSourceUIManual 表示来自 UI 的手工修改。
	ModifiedSourceUIManual = "ui_manual"

	// ModifiedSourceAutomap 表示来自自动映射的修改。
	ModifiedSourceAutomap = "automap"

	// ModifiedSourceSchemaSyncMigration 表示来自 schema 同步迁移的写入。
	ModifiedSourceSchemaSyncMigration = "schema_sync_migration"

	// ModifiedSourceImportRestore 表示来自导入/恢复流程的写入。
	ModifiedSourceImportRestore = "import_restore"

	// ModifiedSourceSystemPatch 表示来自系统补丁的写入。
	ModifiedSourceSystemPatch = "system_patch"
)

// GeneratorType 是稳定的生成器标识符类型。
type GeneratorType string

// GeneratorErrorCode 是生成器领域标准化错误码。
type GeneratorErrorCode string

const (
	// GeneratorErrInvalidArgument 表示请求或配置校验失败。
	GeneratorErrInvalidArgument GeneratorErrorCode = "INVALID_ARGUMENT"

	// GeneratorErrFailedPrecondition 表示依赖状态未就绪。
	GeneratorErrFailedPrecondition GeneratorErrorCode = "FAILED_PRECONDITION"

	// GeneratorErrUnsupported 表示生成器不支持该字段。
	GeneratorErrUnsupported GeneratorErrorCode = "UNSUPPORTED_GENERATOR"

	// GeneratorErrNotRegistered 表示生成器类型未注册到注册表。
	GeneratorErrNotRegistered GeneratorErrorCode = "GENERATOR_NOT_REGISTERED"

	// GeneratorErrConflict 表示生成器类型注册冲突。
	GeneratorErrConflict GeneratorErrorCode = "GENERATOR_CONFLICT"
)

// GeneratorMeta 描述生成器的静态能力信息。
type GeneratorMeta struct {
	// Type 是生成器类型唯一标识。
	Type GeneratorType

	// TypeTags 保存能力标签，例如 "supports:string"。
	TypeTags []string

	// Deterministic 表示固定 seed 时是否可复现输出。
	Deterministic bool
}

// GeneratorContext 定义一次生成调用的输入上下文。
type GeneratorContext struct {
	// ConnectionID 是连接范围标识。
	ConnectionID string

	// Table 是当前连接下的表名。
	Table string

	// Column 是执行生成的列名。
	Column string

	// FieldType 是抽象字段类型。
	FieldType string

	// Params 是已解析的生成器参数。
	Params map[string]interface{}

	// Seed 是本次调用生效的种子值。
	Seed *int64
}

// GeneratorError 是稳定的结构化领域错误。
type GeneratorError struct {
	// Code 是稳定的机器可读错误码。
	Code GeneratorErrorCode

	// Path 是可选的稳定字段路径。
	Path string

	// Message 是面向用户且不含敏感信息的错误消息。
	Message string
}

// Error 返回可读错误信息。
//
// 输入：
// - 无。
//
// 输出：
// - string: 当前错误对象的消息；当错误对象为空时返回空字符串。
func (e *GeneratorError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

// Generator 定义 spec-03 下统一的生成器接口。
type Generator interface {
	// Meta 返回生成器静态元数据。
	Meta() GeneratorMeta

	// Generate 在给定上下文下生成一个值。
	Generate(ctx context.Context, in GeneratorContext) (interface{}, error)

	// GenerateBatch 在给定上下文下生成指定数量的值。
	GenerateBatch(ctx context.Context, in GeneratorContext, count int) ([]interface{}, error)

	// Reset 重置内部状态，用于确定性会话边界。
	Reset() error
}

// StaticGenerator 是用于注册表测试的轻量内存生成器。
type StaticGenerator struct {
	// meta 保存静态能力元数据。
	meta GeneratorMeta
}

// NewStaticGenerator 创建静态生成器实例。
//
// 输入：
// - meta: 生成器静态能力元数据。
//
// 输出：
// - *StaticGenerator: 初始化后的静态生成器实例。
func NewStaticGenerator(meta GeneratorMeta) *StaticGenerator {
	return &StaticGenerator{meta: meta}
}

// Meta 返回静态元数据。
//
// 输入：
// - 无。
//
// 输出：
// - GeneratorMeta: 当前生成器的静态能力元数据。
func (g *StaticGenerator) Meta() GeneratorMeta { return g.meta }

// Generate 返回测试使用的稳定占位值。
//
// 输入：
// - ctx: 调用上下文（当前实现未使用）。
// - in: 生成输入上下文（当前实现未使用）。
//
// 输出：
// - interface{}: 基于生成器类型构造的稳定占位值。
// - error: 固定返回 nil。
func (g *StaticGenerator) Generate(context.Context, GeneratorContext) (interface{}, error) {
	return string(g.meta.Type), nil
}

// GenerateBatch 返回测试使用的稳定占位值数组。
//
// 输入：
// - ctx: 调用上下文（当前实现未使用）。
// - in: 生成输入上下文（当前实现未使用）。
// - count: 需要生成的值数量。
//
// 输出：
// - []interface{}: 指定数量的稳定占位值数组。
// - error: 固定返回 nil。
func (g *StaticGenerator) GenerateBatch(_ context.Context, _ GeneratorContext, count int) ([]interface{}, error) {
	out := make([]interface{}, 0, count)
	for i := 0; i < count; i++ {
		out = append(out, string(g.meta.Type))
	}
	return out, nil
}

// Reset 对静态生成器是空操作。
//
// 输入：
// - 无。
//
// 输出：
// - error: 固定返回 nil。
func (g *StaticGenerator) Reset() error { return nil }

// GeneratorRegistry 在进程内存中保存内置生成器。
type GeneratorRegistry struct {
	// mu 保护 generators，避免并发读写冲突。
	mu sync.RWMutex

	// generators 保存 type -> generator 实例映射。
	generators map[GeneratorType]Generator
}

// NewGeneratorRegistry 创建空的生成器注册表。
//
// 输入：
// - 无。
//
// 输出：
// - *GeneratorRegistry: 初始化后的空注册表。
func NewGeneratorRegistry() *GeneratorRegistry {
	return &GeneratorRegistry{generators: make(map[GeneratorType]Generator)}
}

// Register 向注册表注册一个生成器，并执行冲突检测。
//
// 输入：
// - generator: 待注册的生成器实例。
//
// 输出：
// - error: 注册失败时返回结构化错误；成功时返回 nil。
func (r *GeneratorRegistry) Register(generator Generator) error {
	if generator == nil {
		return &GeneratorError{Code: GeneratorErrInvalidArgument, Path: "generator", Message: "generator is required"}
	}
	meta := generator.Meta()
	if strings.TrimSpace(string(meta.Type)) == "" {
		return &GeneratorError{Code: GeneratorErrInvalidArgument, Path: "generator.meta.type", Message: "generator type is required"}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.generators[meta.Type]; exists {
		return &GeneratorError{Code: GeneratorErrConflict, Path: "generator.meta.type", Message: fmt.Sprintf("generator type %s already registered", meta.Type)}
	}
	r.generators[meta.Type] = generator
	return nil
}

// Resolve 按类型返回已注册生成器。
//
// 输入：
// - generatorType: 目标生成器类型标识。
//
// 输出：
// - Generator: 对应的生成器实例。
// - error: 未找到时返回结构化错误；成功时返回 nil。
func (r *GeneratorRegistry) Resolve(generatorType GeneratorType) (Generator, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	generator, ok := r.generators[generatorType]
	if !ok {
		return nil, &GeneratorError{Code: GeneratorErrNotRegistered, Path: "generator_type", Message: fmt.Sprintf("generator type %s not registered", generatorType)}
	}
	return generator, nil
}

// ListCapabilities 返回所有已注册生成器的元数据列表。
//
// 输入：
// - 无。
//
// 输出：
// - []GeneratorMeta: 按类型排序后的能力元数据列表。
func (r *GeneratorRegistry) ListCapabilities() []GeneratorMeta {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]GeneratorMeta, 0, len(r.generators))
	for _, generator := range r.generators {
		out = append(out, generator.Meta())
	}
	sort.Slice(out, func(i, j int) bool { return string(out[i].Type) < string(out[j].Type) })
	return out
}

// FieldSchema 是 resolver/validator/service 使用的最小字段描述。
type FieldSchema struct {
	// ConnectionID 是连接标识。
	ConnectionID string

	// Table 是表名。
	Table string

	// Column 是列名。
	Column string

	// ColumnID 是持久化唯一列 schema 标识。
	ColumnID string

	// AbstractType 是统一抽象类型。
	AbstractType string
}

// ResolveCandidatesResult 是类型解析器输出结果。
type ResolveCandidatesResult struct {
	// Candidates 是全部兼容的生成器类型。
	Candidates []GeneratorType

	// DefaultGeneratorType 是根据抽象类型给出的默认建议。
	DefaultGeneratorType GeneratorType
}

// GeneratorTypeResolver 根据字段 schema 解析候选与默认生成器。
type GeneratorTypeResolver struct {
	// registry 用于查询生成器能力。
	registry *GeneratorRegistry
}

// NewGeneratorTypeResolver 创建类型解析器。
//
// 输入：
// - registry: 生成器注册表实例。
//
// 输出：
// - *GeneratorTypeResolver: 初始化后的类型解析器。
func NewGeneratorTypeResolver(registry *GeneratorRegistry) *GeneratorTypeResolver {
	return &GeneratorTypeResolver{registry: registry}
}

// ResolveCandidates 解析兼容生成器与默认生成器。
//
// 输入：
// - field: 目标字段 schema 描述。
//
// 输出：
// - *ResolveCandidatesResult: 候选生成器列表与默认建议。
// - error: 解析失败时返回结构化错误；成功时返回 nil。
func (r *GeneratorTypeResolver) ResolveCandidates(field FieldSchema) (*ResolveCandidatesResult, error) {
	targetType := strings.TrimSpace(field.AbstractType)
	if targetType == "" {
		return nil, &GeneratorError{Code: GeneratorErrInvalidArgument, Path: "field.abstract_type", Message: "field abstract_type is required"}
	}
	candidates := make([]GeneratorType, 0)
	for _, meta := range r.registry.ListCapabilities() {
		if supportsAbstractType(meta.TypeTags, targetType) {
			candidates = append(candidates, meta.Type)
		}
	}
	if len(candidates) == 0 {
		return nil, &GeneratorError{
			Code: GeneratorErrInvalidArgument,
			Path: "field.generator_type",
			Message: fmt.Sprintf("no available generator for %s.%s.%s (%s)", field.ConnectionID, field.Table, field.Column, targetType),
		}
	}
	return &ResolveCandidatesResult{
		Candidates:           candidates,
		DefaultGeneratorType: defaultGeneratorByAbstractType(targetType),
	}, nil
}

// supportsAbstractType 检查类型标签是否包含目标类型能力。
//
// 输入：
// - typeTags: 生成器能力标签列表。
// - abstractType: 待匹配的抽象类型。
//
// 输出：
// - bool: 包含目标能力标签时返回 true，否则返回 false。
func supportsAbstractType(typeTags []string, abstractType string) bool {
	want := "supports:" + abstractType
	for _, typeTag := range typeTags {
		if strings.TrimSpace(typeTag) == want {
			return true
		}
	}
	return false
}

// defaultGeneratorByAbstractType 返回 spec-03 的默认类型映射基线。
//
// 输入：
// - abstractType: 字段抽象类型。
//
// 输出：
// - GeneratorType: 对应的默认生成器类型；未知类型回退到字符串随机生成器。
func defaultGeneratorByAbstractType(abstractType string) GeneratorType {
	switch abstractType {
	case "int":
		return GeneratorTypeIntRangeRandom
	case "decimal":
		return GeneratorTypeDecimalRangeRandom
	case "string":
		return GeneratorTypeStringRandomChars
	case "boolean":
		return GeneratorTypeBooleanRatio
	case "datetime":
		return GeneratorTypeDatetimeRangeRandom
	default:
		return GeneratorTypeStringRandomChars
	}
}

// FieldError 表示一个稳定的字段级校验错误。
type FieldError struct {
	// Code 是稳定的机器可读错误码。
	Code string

	// Path 是用于 UI 精确高亮的稳定字段路径。
	Path string

	// Message 是可读的修复提示信息。
	Message string

	// Suggestion 是可选的动作建议。
	Suggestion string
}

// ValidationError 表示聚合后的字段级校验失败。
type ValidationError struct {
	// FieldErrors 保存有序字段级错误列表。
	FieldErrors []FieldError
}

// Error 返回拼接后的校验错误摘要。
//
// 输入：
// - 无。
//
// 输出：
// - string: 首个字段错误消息；无错误时返回空字符串。
func (e *ValidationError) Error() string {
	if e == nil || len(e.FieldErrors) == 0 {
		return ""
	}
	return e.FieldErrors[0].Message
}

// GeneratorConfigValidator 校验字段生成器配置请求。
type GeneratorConfigValidator struct {
	// registry 用于检查生成器是否存在。
	registry *GeneratorRegistry
}

// NewGeneratorConfigValidator 创建配置校验器。
//
// 输入：
// - registry: 生成器注册表实例。
//
// 输出：
// - *GeneratorConfigValidator: 初始化后的配置校验器。
func NewGeneratorConfigValidator(registry *GeneratorRegistry) *GeneratorConfigValidator {
	return &GeneratorConfigValidator{registry: registry}
}

// Validate 基于字段 schema 与候选列表校验请求。
//
// 输入：
// - req: 字段生成器配置保存请求。
// - field: 目标字段 schema 描述。
// - candidates: 当前字段可用的候选生成器类型。
//
// 输出：
// - *ValidationError: 校验失败时返回聚合错误；校验通过时返回 nil。
func (v *GeneratorConfigValidator) Validate(req SaveFieldGeneratorConfigRequest, field FieldSchema, candidates []GeneratorType) *ValidationError {
	errs := make([]FieldError, 0)
	if strings.TrimSpace(req.ConnectionID) == "" {
		errs = append(errs, FieldError{Code: "INVALID_ARGUMENT", Path: "connection_id", Message: "connection_id is required", Suggestion: "provide connection_id"})
	}
	if strings.TrimSpace(req.Table) == "" {
		errs = append(errs, FieldError{Code: "INVALID_ARGUMENT", Path: "table", Message: "table is required", Suggestion: "provide table"})
	}
	if strings.TrimSpace(req.Column) == "" {
		errs = append(errs, FieldError{Code: "INVALID_ARGUMENT", Path: "column", Message: "column is required", Suggestion: "provide column"})
	}
	if strings.TrimSpace(string(req.GeneratorType)) == "" {
		errs = append(errs, FieldError{Code: "INVALID_ARGUMENT", Path: "generator_type", Message: "generator_type is required", Suggestion: "pick one candidate generator"})
	} else if !containsGeneratorType(candidates, req.GeneratorType) {
		errs = append(errs, FieldError{Code: "INVALID_ARGUMENT", Path: "generator_type", Message: "generator_type is not compatible with field type", Suggestion: "choose compatible generator_type"})
	}
	if err := validateModifiedSource(req.ModifiedSource); err != nil {
		errs = append(errs, FieldError{Code: "INVALID_ARGUMENT", Path: "modified_source", Message: err.Error(), Suggestion: "use one of fixed modified_source enums"})
	}
	if req.GeneratorType == GeneratorTypeEnumValue {
		enumErr := validateEnumValuesByType(field.AbstractType, req.GeneratorOpts)
		if enumErr != nil {
			errs = append(errs, *enumErr)
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return &ValidationError{FieldErrors: errs}
}

// containsGeneratorType 检查目标类型是否在候选列表中。
//
// 输入：
// - candidates: 候选生成器类型列表。
// - target: 目标生成器类型。
//
// 输出：
// - bool: 存在时返回 true，否则返回 false。
func containsGeneratorType(candidates []GeneratorType, target GeneratorType) bool {
	for _, candidate := range candidates {
		if candidate == target {
			return true
		}
	}
	return false
}

// validateModifiedSource 校验固定的 modified_source 枚举值。
//
// 输入：
// - modifiedSource: 待校验的修改来源值。
//
// 输出：
// - error: 非法时返回错误；合法时返回 nil。
func validateModifiedSource(modifiedSource string) error {
	switch strings.TrimSpace(modifiedSource) {
	case ModifiedSourceUIManual, ModifiedSourceAutomap, ModifiedSourceSchemaSyncMigration, ModifiedSourceImportRestore, ModifiedSourceSystemPatch:
		return nil
	default:
		return fmt.Errorf("modified_source is invalid")
	}
}

// validateEnumValuesByType 校验 enum 生成器 params.values 与目标类型的一致性。
//
// 输入：
// - abstractType: 目标字段抽象类型。
// - generatorOpts: 生成器参数集合。
//
// 输出：
// - *FieldError: 校验失败时返回字段级错误；校验通过时返回 nil。
func validateEnumValuesByType(abstractType string, generatorOpts map[string]interface{}) *FieldError {
	raw, ok := generatorOpts["values"]
	if !ok {
		return &FieldError{Code: "INVALID_ARGUMENT", Path: "generator_opts.values", Message: "generator_opts.values is required for enum_value", Suggestion: "provide candidate values list"}
	}
	items, ok := raw.([]interface{})
	if !ok || len(items) == 0 {
		return &FieldError{Code: "INVALID_ARGUMENT", Path: "generator_opts.values", Message: "generator_opts.values must be non-empty array", Suggestion: "provide non-empty values array"}
	}
	for idx, item := range items {
		if !isCompatibleValueType(abstractType, item) {
			return &FieldError{
				Code:       "INVALID_ARGUMENT",
				Path:       fmt.Sprintf("generator_opts.values[%d]", idx),
				Message:    "enum value type is incompatible with field abstract_type",
				Suggestion: "adjust enum values to target abstract type",
			}
		}
	}
	return nil
}

// isCompatibleValueType 检查值是否可用于目标抽象类型。
//
// 输入：
// - abstractType: 目标字段抽象类型。
// - value: 待校验的候选值。
//
// 输出：
// - bool: 类型兼容时返回 true，否则返回 false。
func isCompatibleValueType(abstractType string, value interface{}) bool {
	switch abstractType {
	case "int":
		switch value.(type) {
		case int, int32, int64, float64:
			return true
		}
	case "decimal":
		switch value.(type) {
		case int, int32, int64, float32, float64:
			return true
		}
	case "string":
		_, ok := value.(string)
		return ok
	case "boolean":
		_, ok := value.(bool)
		return ok
	case "datetime":
		_, ok := value.(string)
		return ok
	}
	return false
}

// SaveFieldGeneratorConfigRequest 是 SaveFieldGeneratorConfig 的输入载荷。
type SaveFieldGeneratorConfigRequest struct {
	// ConnectionID 标识目标连接。
	ConnectionID string

	// Table 标识目标表。
	Table string

	// Column 标识目标字段。
	Column string

	// GeneratorType 是选定生成器类型标识。
	GeneratorType GeneratorType

	// GeneratorOpts 保存生成器参数。
	GeneratorOpts map[string]interface{}

	// SeedPolicy 保存种子策略参数。
	SeedPolicy map[string]interface{}

	// NullPolicy 保存空值生成策略。
	NullPolicy string

	// IsEnabled 控制是否启用生成。
	IsEnabled bool

	// ModifiedSource 表示此次修改来源。
	ModifiedSource string
}

// FieldGeneratorConfig 是持久化的字段级生成器配置模型。
type FieldGeneratorConfig struct {
	// ColumnSchemaID 是持久化唯一键。
	ColumnSchemaID string

	// ConnectionID 标识读取路径中的连接。
	ConnectionID string

	// Table 标识读取路径中的表。
	Table string

	// Column 标识读取路径中的字段。
	Column string

	// GeneratorType 是选定生成器类型标识。
	GeneratorType GeneratorType

	// GeneratorOpts 保存生成器参数。
	GeneratorOpts map[string]interface{}

	// SeedPolicy 保存种子策略参数。
	SeedPolicy map[string]interface{}

	// NullPolicy 保存空值生成策略。
	NullPolicy string

	// IsEnabled 控制是否启用生成。
	IsEnabled bool

	// ConfigVersion 是乐观锁版本号。
	ConfigVersion int64

	// ModifiedSource 保存固定来源枚举值。
	ModifiedSource string

	// UpdatedAtUnix 是最后更新时间（Unix 秒）。
	UpdatedAtUnix int64
}

// GeneratorConfigRepository 定义配置持久化行为。
type GeneratorConfigRepository interface {
	// UpsertFieldConfig 按 column_schema_id 写入单条字段配置。
	UpsertFieldConfig(ctx context.Context, config FieldGeneratorConfig) (FieldGeneratorConfig, error)

	// GetByField 按 connection/table/column 位置读取配置。
	GetByField(ctx context.Context, connectionID string, table string, column string) (FieldGeneratorConfig, error)
}

// FieldSchemaProvider 提供按字段定位符查询当前 schema 的能力。
type FieldSchemaProvider interface {
	// GetFieldSchema 从当前 schema 源解析单个字段。
	GetFieldSchema(ctx context.Context, connectionID string, table string, column string) (FieldSchema, error)
}

// GeneratorConfigService 编排 schema->resolver->validator->repository 能力链。
type GeneratorConfigService struct {
	// schemaProvider 提供当前字段 schema 事实数据。
	schemaProvider FieldSchemaProvider

	// resolver 解析兼容生成器候选。
	resolver *GeneratorTypeResolver

	// validator 校验配置请求及约束。
	validator *GeneratorConfigValidator

	// repository 持久化已校验配置。
	repository GeneratorConfigRepository
}

// NewGeneratorConfigService 创建用于 Save/Get 配置的链式服务。
//
// 输入：
// - schemaProvider: 字段 schema 提供器。
// - resolver: 生成器类型解析器。
// - validator: 生成器配置校验器。
// - repository: 生成器配置仓储。
//
// 输出：
// - *GeneratorConfigService: 初始化后的配置服务实例。
func NewGeneratorConfigService(
	schemaProvider FieldSchemaProvider,
	resolver *GeneratorTypeResolver,
	validator *GeneratorConfigValidator,
	repository GeneratorConfigRepository,
) *GeneratorConfigService {
	return &GeneratorConfigService{
		schemaProvider: schemaProvider,
		resolver:       resolver,
		validator:      validator,
		repository:     repository,
	}
}

// SaveFieldGeneratorConfig 校验并持久化字段级配置。
//
// 输入：
// - ctx: 调用上下文。
// - req: 字段生成器配置保存请求。
//
// 输出：
// - FieldGeneratorConfig: 保存后的字段配置结果。
// - *ValidationError: 失败时返回校验错误；成功时返回 nil。
func (s *GeneratorConfigService) SaveFieldGeneratorConfig(ctx context.Context, req SaveFieldGeneratorConfigRequest) (FieldGeneratorConfig, *ValidationError) {
	field, err := s.schemaProvider.GetFieldSchema(ctx, req.ConnectionID, req.Table, req.Column)
	if err != nil {
		return FieldGeneratorConfig{}, &ValidationError{
			FieldErrors: []FieldError{{
				Code:       "CURRENT_SCHEMA_NOT_FOUND",
				Path:       "field",
				Message:    "current schema field not found",
				Suggestion: "rescan schema then retry",
			}},
		}
	}
	resolved, resolveErr := s.resolver.ResolveCandidates(field)
	if resolveErr != nil {
		return FieldGeneratorConfig{}, &ValidationError{
			FieldErrors: []FieldError{{
				Code:       string(GeneratorErrInvalidArgument),
				Path:       "generator_type",
				Message:    resolveErr.Error(),
				Suggestion: "pick a compatible generator",
			}},
		}
	}
	if validationErr := s.validator.Validate(req, field, resolved.Candidates); validationErr != nil {
		return FieldGeneratorConfig{}, validationErr
	}
	next := FieldGeneratorConfig{
		ColumnSchemaID: field.ColumnID,
		ConnectionID:   req.ConnectionID,
		Table:          req.Table,
		Column:         req.Column,
		GeneratorType:  req.GeneratorType,
		GeneratorOpts:  cloneMap(req.GeneratorOpts),
		SeedPolicy:     cloneMap(req.SeedPolicy),
		NullPolicy:     req.NullPolicy,
		IsEnabled:      req.IsEnabled,
		ModifiedSource: req.ModifiedSource,
		UpdatedAtUnix:  time.Now().Unix(),
	}
	saved, saveErr := s.repository.UpsertFieldConfig(ctx, next)
	if saveErr != nil {
		return FieldGeneratorConfig{}, &ValidationError{
			FieldErrors: []FieldError{{
				Code:       "FAILED_PRECONDITION",
				Path:       "repository",
				Message:    "save field generator config failed",
				Suggestion: "check repository state and retry",
			}},
		}
	}
	return saved, nil
}

// GetFieldGeneratorConfig 按字段位置加载单条配置。
//
// 输入：
// - ctx: 调用上下文。
// - connectionID: 连接标识。
// - table: 表名。
// - column: 列名。
//
// 输出：
// - FieldGeneratorConfig: 匹配到的字段配置。
// - error: 未找到或读取失败时返回错误；成功时返回 nil。
func (s *GeneratorConfigService) GetFieldGeneratorConfig(ctx context.Context, connectionID string, table string, column string) (FieldGeneratorConfig, error) {
	return s.repository.GetByField(ctx, connectionID, table, column)
}

// InMemoryConfigRepository 是面向服务测试的内存实现。
type InMemoryConfigRepository struct {
	// mu 保护内部 map 的并发访问。
	mu sync.RWMutex

	// byColumnID 按唯一 column_schema_id 保存配置。
	byColumnID map[string]FieldGeneratorConfig

	// byLocator 按字段定位符保存 column_schema_id。
	byLocator map[string]string
}

// NewInMemoryConfigRepository 创建测试用仓储。
//
// 输入：
// - 无。
//
// 输出：
// - *InMemoryConfigRepository: 初始化后的内存仓储实例。
func NewInMemoryConfigRepository() *InMemoryConfigRepository {
	return &InMemoryConfigRepository{
		byColumnID: make(map[string]FieldGeneratorConfig),
		byLocator:  make(map[string]string),
	}
}

// UpsertFieldConfig 保存配置，并在更新时递增配置版本号。
//
// 输入：
// - ctx: 调用上下文（当前实现未使用）。
// - config: 待写入的字段配置。
//
// 输出：
// - FieldGeneratorConfig: 写入后的字段配置（包含最新版本号）。
// - error: 写入失败时返回错误；成功时返回 nil。
func (r *InMemoryConfigRepository) UpsertFieldConfig(_ context.Context, config FieldGeneratorConfig) (FieldGeneratorConfig, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if strings.TrimSpace(config.ColumnSchemaID) == "" {
		return FieldGeneratorConfig{}, fmt.Errorf("column_schema_id is required")
	}
	prev, exists := r.byColumnID[config.ColumnSchemaID]
	if exists {
		config.ConfigVersion = prev.ConfigVersion + 1
	} else {
		config.ConfigVersion = 1
	}
	r.byColumnID[config.ColumnSchemaID] = config
	r.byLocator[locatorKey(config.ConnectionID, config.Table, config.Column)] = config.ColumnSchemaID
	return config, nil
}

// GetByField 按 connection/table/column 获取配置。
//
// 输入：
// - ctx: 调用上下文（当前实现未使用）。
// - connectionID: 连接标识。
// - table: 表名。
// - column: 列名。
//
// 输出：
// - FieldGeneratorConfig: 命中的字段配置。
// - error: 未命中或读取失败时返回错误；成功时返回 nil。
func (r *InMemoryConfigRepository) GetByField(_ context.Context, connectionID string, table string, column string) (FieldGeneratorConfig, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	columnID, ok := r.byLocator[locatorKey(connectionID, table, column)]
	if !ok {
		return FieldGeneratorConfig{}, fmt.Errorf("config not found")
	}
	config, ok := r.byColumnID[columnID]
	if !ok {
		return FieldGeneratorConfig{}, fmt.Errorf("config not found")
	}
	return config, nil
}

// locatorKey 构建标准化字段定位键。
//
// 输入：
// - connectionID: 连接标识。
// - table: 表名。
// - column: 列名。
//
// 输出：
// - string: 归一化后的字段定位键。
func locatorKey(connectionID string, table string, column string) string {
	return strings.ToLower(strings.TrimSpace(connectionID)) + "|" + strings.ToLower(strings.TrimSpace(table)) + "|" + strings.ToLower(strings.TrimSpace(column))
}

// StaticSchemaProvider 是测试使用的静态 schema 数据源。
type StaticSchemaProvider struct {
	// byLocator 按字段定位符保存字段 schema。
	byLocator map[string]FieldSchema
}

// NewStaticSchemaProvider 基于字段列表创建静态 schema 提供器。
//
// 输入：
// - fields: 初始字段 schema 列表。
//
// 输出：
// - *StaticSchemaProvider: 初始化后的静态 schema 提供器。
func NewStaticSchemaProvider(fields []FieldSchema) *StaticSchemaProvider {
	byLocator := make(map[string]FieldSchema, len(fields))
	for _, field := range fields {
		if strings.TrimSpace(field.ColumnID) == "" {
			field.ColumnID = field.ConnectionID + ":" + field.Table + ":" + field.Column
		}
		byLocator[locatorKey(field.ConnectionID, field.Table, field.Column)] = field
	}
	return &StaticSchemaProvider{byLocator: byLocator}
}

// GetFieldSchema 按定位符获取单个字段 schema。
//
// 输入：
// - ctx: 调用上下文（当前实现未使用）。
// - connectionID: 连接标识。
// - table: 表名。
// - column: 列名。
//
// 输出：
// - FieldSchema: 命中的字段 schema。
// - error: 未命中时返回错误；成功时返回 nil。
func (p *StaticSchemaProvider) GetFieldSchema(_ context.Context, connectionID string, table string, column string) (FieldSchema, error) {
	field, ok := p.byLocator[locatorKey(connectionID, table, column)]
	if !ok {
		return FieldSchema{}, fmt.Errorf("field schema not found")
	}
	return field, nil
}

// cloneMap 创建浅拷贝，用于隔离 map 持久化副作用。
//
// 输入：
// - input: 待复制的 map。
//
// 输出：
// - map[string]interface{}: 输入 map 的浅拷贝；输入为空时返回空 map。
func cloneMap(input map[string]interface{}) map[string]interface{} {
	if len(input) == 0 {
		return map[string]interface{}{}
	}
	out := make(map[string]interface{}, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}
