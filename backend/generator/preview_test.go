// Package generator 提供 spec-03 生成器预览与运行时能力。
package generator

import (
	"context"
	"testing"
)

// mustRegisterPreview 是测试辅助函数，注册生成器。
func mustRegisterPreview(t *testing.T, reg *GeneratorRegistry, generator Generator) {
	t.Helper()
	if err := reg.Register(generator); err != nil {
		t.Fatalf("register failed: %v", err)
	}
}

// TestPreviewFieldScopeReturnsSamples 测试单字段预览返回样本数组。
func TestPreviewFieldScopeReturnsSamples(t *testing.T) {
	reg := NewGeneratorRegistry()
	mustRegisterPreview(t, reg, NewStaticGenerator(GeneratorMeta{
		Type:          GeneratorTypeStringRandomChars,
		TypeTags:      []string{"supports:string"},
		Deterministic: true,
	}))

	repo := NewPreviewableConfigRepository()
	_, _ = repo.UpsertFieldConfig(context.Background(), FieldGeneratorConfig{
		ColumnSchemaID: "col-1",
		ConnectionID:   "c1",
		Table:          "users",
		Column:         "name",
		GeneratorType:  GeneratorTypeStringRandomChars,
		GeneratorOpts:  map[string]interface{}{},
		IsEnabled:      true,
	})

	schemaProvider := NewPreviewableSchemaProvider([]FieldSchema{
		{ConnectionID: "c1", Table: "users", Column: "name", AbstractType: "string", ColumnID: "col-1"},
	})

	svc := NewGeneratorPreviewService(reg, repo, schemaProvider, NewGeneratorRuntime(), nil)

	result, err := svc.PreviewGeneration(context.Background(), PreviewRequest{
		ConnectionID: "c1",
		Scope:        PreviewScopeField,
		Table:        "users",
		Column:       "name",
		SampleSize:   3,
		Seed:         nil,
	})

	if err != nil {
		t.Fatalf("preview failed: %v", err)
	}

	if result.Scope.Type != PreviewScopeField {
		t.Fatalf("unexpected scope type: %s", result.Scope.Type)
	}

	samples, ok := result.Samples.([]interface{})
	if !ok {
		t.Fatalf("expected []interface{} for field scope samples, got %T", result.Samples)
	}

	if len(samples) != 3 {
		t.Fatalf("expected 3 samples, got %d", len(samples))
	}
}

// TestPreviewTableScopeReturnsColumnMap 测试单表预览返回列样本映射。
func TestPreviewTableScopeReturnsColumnMap(t *testing.T) {
	reg := NewGeneratorRegistry()
	mustRegisterPreview(t, reg, NewStaticGenerator(GeneratorMeta{
		Type:          GeneratorTypeIntRangeRandom,
		TypeTags:      []string{"supports:int"},
		Deterministic: true,
	}))
	mustRegisterPreview(t, reg, NewStaticGenerator(GeneratorMeta{
		Type:          GeneratorTypeStringRandomChars,
		TypeTags:      []string{"supports:string"},
		Deterministic: true,
	}))

	repo := NewPreviewableConfigRepository()
	_, _ = repo.UpsertFieldConfig(context.Background(), FieldGeneratorConfig{
		ColumnSchemaID: "col-id",
		ConnectionID:   "c1",
		Table:          "users",
		Column:         "id",
		GeneratorType:  GeneratorTypeIntRangeRandom,
		GeneratorOpts:  map[string]interface{}{},
		IsEnabled:      true,
	})
	_, _ = repo.UpsertFieldConfig(context.Background(), FieldGeneratorConfig{
		ColumnSchemaID: "col-name",
		ConnectionID:   "c1",
		Table:          "users",
		Column:         "name",
		GeneratorType:  GeneratorTypeStringRandomChars,
		GeneratorOpts:  map[string]interface{}{},
		IsEnabled:      true,
	})

	schemaProvider := NewPreviewableSchemaProvider([]FieldSchema{
		{ConnectionID: "c1", Table: "users", Column: "id", AbstractType: "int", ColumnID: "col-id"},
		{ConnectionID: "c1", Table: "users", Column: "name", AbstractType: "string", ColumnID: "col-name"},
	})

	svc := NewGeneratorPreviewService(reg, repo, schemaProvider, NewGeneratorRuntime(), nil)

	result, err := svc.PreviewGeneration(context.Background(), PreviewRequest{
		ConnectionID: "c1",
		Scope:        PreviewScopeTable,
		Table:        "users",
		SampleSize:   3,
	})

	if err != nil {
		t.Fatalf("preview failed: %v", err)
	}

	if result.Scope.Type != PreviewScopeTable {
		t.Fatalf("unexpected scope type: %s", result.Scope.Type)
	}

	samplesMap, ok := result.Samples.(map[string][]interface{})
	if !ok {
		t.Fatalf("expected map[string][]interface{} for table scope samples, got %T", result.Samples)
	}

	if len(samplesMap) != 2 {
		t.Fatalf("expected 2 columns in samples map, got %d", len(samplesMap))
	}

	if len(samplesMap["id"]) != 3 {
		t.Fatalf("expected 3 samples for id column, got %d", len(samplesMap["id"]))
	}

	if len(samplesMap["name"]) != 3 {
		t.Fatalf("expected 3 samples for name column, got %d", len(samplesMap["name"]))
	}
}

// TestPreviewWithFixedSeedReproducible 测试固定种子可复现。
func TestPreviewWithFixedSeedReproducible(t *testing.T) {
	reg := NewGeneratorRegistry()
	mustRegisterPreview(t, reg, NewStaticGenerator(GeneratorMeta{
		Type:          GeneratorTypeStringRandomChars,
		TypeTags:      []string{"supports:string"},
		Deterministic: true,
	}))

	repo := NewPreviewableConfigRepository()
	_, _ = repo.UpsertFieldConfig(context.Background(), FieldGeneratorConfig{
		ColumnSchemaID: "col-1",
		ConnectionID:   "c1",
		Table:          "users",
		Column:         "name",
		GeneratorType:  GeneratorTypeStringRandomChars,
		GeneratorOpts:  map[string]interface{}{},
		IsEnabled:      true,
	})

	schemaProvider := NewPreviewableSchemaProvider([]FieldSchema{
		{ConnectionID: "c1", Table: "users", Column: "name", AbstractType: "string", ColumnID: "col-1"},
	})

	svc := NewGeneratorPreviewService(reg, repo, schemaProvider, NewGeneratorRuntime(), nil)

	seed := int64(42)

	result1, err := svc.PreviewGeneration(context.Background(), PreviewRequest{
		ConnectionID: "c1",
		Scope:        PreviewScopeField,
		Table:        "users",
		Column:       "name",
		SampleSize:   5,
		Seed:         &seed,
	})
	if err != nil {
		t.Fatalf("first preview failed: %v", err)
	}

	result2, err := svc.PreviewGeneration(context.Background(), PreviewRequest{
		ConnectionID: "c1",
		Scope:        PreviewScopeField,
		Table:        "users",
		Column:       "name",
		SampleSize:   5,
		Seed:         &seed,
	})
	if err != nil {
		t.Fatalf("second preview failed: %v", err)
	}

	samples1 := result1.Samples.([]interface{})
	samples2 := result2.Samples.([]interface{})

	for i := range samples1 {
		if samples1[i] != samples2[i] {
			t.Fatalf("samples at index %d differ: %v vs %v", i, samples1[i], samples2[i])
		}
	}

	if !result1.Metadata.Deterministic {
		t.Fatalf("expected deterministic=true in metadata")
	}

	if result1.Metadata.Seed != seed {
		t.Fatalf("expected seed=%d in metadata, got %d", seed, result1.Metadata.Seed)
	}
}

// TestPreviewMetadataContainsRequiredFields 测试 metadata 包含必需字段。
func TestPreviewMetadataContainsRequiredFields(t *testing.T) {
	reg := NewGeneratorRegistry()
	mustRegisterPreview(t, reg, NewStaticGenerator(GeneratorMeta{
		Type:          GeneratorTypeStringRandomChars,
		TypeTags:      []string{"supports:string"},
		Deterministic: true,
	}))

	repo := NewPreviewableConfigRepository()
	_, _ = repo.UpsertFieldConfig(context.Background(), FieldGeneratorConfig{
		ColumnSchemaID: "col-1",
		ConnectionID:   "c1",
		Table:          "users",
		Column:         "name",
		GeneratorType:  GeneratorTypeStringRandomChars,
		GeneratorOpts:  map[string]interface{}{},
		IsEnabled:      true,
	})

	schemaProvider := NewPreviewableSchemaProvider([]FieldSchema{
		{ConnectionID: "c1", Table: "users", Column: "name", AbstractType: "string", ColumnID: "col-1"},
	})

	svc := NewGeneratorPreviewService(reg, repo, schemaProvider, NewGeneratorRuntime(), nil)

	seed := int64(12345)
	result, err := svc.PreviewGeneration(context.Background(), PreviewRequest{
		ConnectionID: "c1",
		Scope:        PreviewScopeField,
		Table:        "users",
		Column:       "name",
		SampleSize:   3,
		Seed:         &seed,
	})

	if err != nil {
		t.Fatalf("preview failed: %v", err)
	}

	if result.Metadata.GeneratorType != GeneratorTypeStringRandomChars {
		t.Fatalf("unexpected generator_type: %s", result.Metadata.GeneratorType)
	}

	if result.Metadata.SampleSize != 3 {
		t.Fatalf("unexpected sample_size: %d", result.Metadata.SampleSize)
	}

	if !result.Metadata.Deterministic {
		t.Fatalf("expected deterministic=true")
	}

	if result.Metadata.Seed != seed {
		t.Fatalf("unexpected seed: %d", result.Metadata.Seed)
	}

	if result.Metadata.GeneratedAt.IsZero() {
		t.Fatalf("generated_at should not be zero")
	}
}

// TestPreviewDisabledFieldReturnsError 测试禁用字段在 field scope 返回错误。
func TestPreviewDisabledFieldReturnsError(t *testing.T) {
	reg := NewGeneratorRegistry()
	mustRegisterPreview(t, reg, NewStaticGenerator(GeneratorMeta{
		Type:          GeneratorTypeStringRandomChars,
		TypeTags:      []string{"supports:string"},
		Deterministic: true,
	}))

	repo := NewPreviewableConfigRepository()
	_, _ = repo.UpsertFieldConfig(context.Background(), FieldGeneratorConfig{
		ColumnSchemaID: "col-1",
		ConnectionID:   "c1",
		Table:          "users",
		Column:         "email",
		GeneratorType:  GeneratorTypeStringRandomChars,
		GeneratorOpts:  map[string]interface{}{},
		IsEnabled:      false, // 禁用
	})

	schemaProvider := NewPreviewableSchemaProvider([]FieldSchema{
		{ConnectionID: "c1", Table: "users", Column: "email", AbstractType: "string", ColumnID: "col-1"},
	})

	svc := NewGeneratorPreviewService(reg, repo, schemaProvider, NewGeneratorRuntime(), nil)

	_, err := svc.PreviewGeneration(context.Background(), PreviewRequest{
		ConnectionID: "c1",
		Scope:        PreviewScopeField,
		Table:        "users",
		Column:       "email",
		SampleSize:   3,
	})

	if err == nil {
		t.Fatalf("expected error for disabled field in field scope")
	}

	gErr, ok := err.(*GeneratorError)
	if !ok {
		t.Fatalf("expected GeneratorError, got %T", err)
	}

	if gErr.Code != GeneratorErrFailedPrecondition {
		t.Fatalf("expected FAILED_PRECONDITION, got %s", gErr.Code)
	}
}

// TestPreviewNoSeedMarkedNonDeterministic 测试无种子时标记为非确定性。
func TestPreviewNoSeedMarkedNonDeterministic(t *testing.T) {
	reg := NewGeneratorRegistry()
	mustRegisterPreview(t, reg, NewStaticGenerator(GeneratorMeta{
		Type:          GeneratorTypeStringRandomChars,
		TypeTags:      []string{"supports:string"},
		Deterministic: true,
	}))

	repo := NewPreviewableConfigRepository()
	_, _ = repo.UpsertFieldConfig(context.Background(), FieldGeneratorConfig{
		ColumnSchemaID: "col-1",
		ConnectionID:   "c1",
		Table:          "users",
		Column:         "name",
		GeneratorType:  GeneratorTypeStringRandomChars,
		GeneratorOpts:  map[string]interface{}{},
		SeedPolicy:     nil,
		IsEnabled:      true,
	})

	schemaProvider := NewPreviewableSchemaProvider([]FieldSchema{
		{ConnectionID: "c1", Table: "users", Column: "name", AbstractType: "string", ColumnID: "col-1"},
	})

	svc := NewGeneratorPreviewService(reg, repo, schemaProvider, NewGeneratorRuntime(), nil)

	result, err := svc.PreviewGeneration(context.Background(), PreviewRequest{
		ConnectionID: "c1",
		Scope:        PreviewScopeField,
		Table:        "users",
		Column:       "name",
		SampleSize:   3,
		Seed:         nil,
	})

	if err != nil {
		t.Fatalf("preview failed: %v", err)
	}

	if result.Metadata.Deterministic {
		t.Fatalf("expected deterministic=false when no seed provided")
	}

	if result.Metadata.Seed != 0 {
		t.Fatalf("expected seed=0 when no seed, got %d", result.Metadata.Seed)
	}

	if result.Metadata.SeedSource != "none" {
		t.Fatalf("expected seed_source=none, got %s", result.Metadata.SeedSource)
	}
}

type staticGlobalSeedProvider struct {
	seed *int64
}

func (p staticGlobalSeedProvider) GetGlobalSeed(_ context.Context, _ string) (*int64, bool) {
	if p.seed == nil || *p.seed == 0 {
		return nil, false
	}
	return p.seed, true
}

// TestPreviewGlobalSeedUsedWhenNoRequestOrFixedSeed 测试全局种子优先级（priority=3）。
func TestPreviewGlobalSeedUsedWhenNoRequestOrFixedSeed(t *testing.T) {
	reg := NewGeneratorRegistry()
	mustRegisterPreview(t, reg, NewStaticGenerator(GeneratorMeta{
		Type:          GeneratorTypeStringRandomChars,
		TypeTags:      []string{"supports:string"},
		Deterministic: true,
	}))

	globalSeed := int64(123)
	repo := NewPreviewableConfigRepository()
	_, _ = repo.UpsertFieldConfig(context.Background(), FieldGeneratorConfig{
		ColumnSchemaID: "col-1",
		ConnectionID:   "c1",
		Table:          "users",
		Column:         "name",
		GeneratorType:  GeneratorTypeStringRandomChars,
		GeneratorOpts:  map[string]interface{}{},
		SeedPolicy:     map[string]interface{}{"mode": "inherit_global"},
		IsEnabled:      true,
	})

	schemaProvider := NewPreviewableSchemaProvider([]FieldSchema{
		{ConnectionID: "c1", Table: "users", Column: "name", AbstractType: "string", ColumnID: "col-1"},
	})

	svc := NewGeneratorPreviewService(reg, repo, schemaProvider, NewGeneratorRuntime(), staticGlobalSeedProvider{seed: &globalSeed})

	result, err := svc.PreviewGeneration(context.Background(), PreviewRequest{
		ConnectionID: "c1",
		Scope:        PreviewScopeField,
		Table:        "users",
		Column:       "name",
		SampleSize:   3,
		Seed:         nil,
	})
	if err != nil {
		t.Fatalf("preview failed: %v", err)
	}
	if result.Metadata.Seed != globalSeed {
		t.Fatalf("expected seed=%d (global), got %d", globalSeed, result.Metadata.Seed)
	}
	if result.Metadata.SeedSource != "global" {
		t.Fatalf("expected seed_source=global, got %s", result.Metadata.SeedSource)
	}
	if !result.Metadata.Deterministic {
		t.Fatalf("expected deterministic=true with global seed")
	}
}

// TestPreviewSeedPolicyFixed 测试字段 seed_policy.fixed 种子策略。
func TestPreviewSeedPolicyFixed(t *testing.T) {
	reg := NewGeneratorRegistry()
	mustRegisterPreview(t, reg, NewStaticGenerator(GeneratorMeta{
		Type:          GeneratorTypeStringRandomChars,
		TypeTags:      []string{"supports:string"},
		Deterministic: true,
	}))

	fieldSeed := int64(999)

	repo := NewPreviewableConfigRepository()
	_, _ = repo.UpsertFieldConfig(context.Background(), FieldGeneratorConfig{
		ColumnSchemaID: "col-1",
		ConnectionID:   "c1",
		Table:          "users",
		Column:         "name",
		GeneratorType:  GeneratorTypeStringRandomChars,
		GeneratorOpts:  map[string]interface{}{},
		SeedPolicy:     map[string]interface{}{"mode": "fixed", "seed": float64(fieldSeed)},
		IsEnabled:      true,
	})

	schemaProvider := NewPreviewableSchemaProvider([]FieldSchema{
		{ConnectionID: "c1", Table: "users", Column: "name", AbstractType: "string", ColumnID: "col-1"},
	})

	svc := NewGeneratorPreviewService(reg, repo, schemaProvider, NewGeneratorRuntime(), nil)

	result, err := svc.PreviewGeneration(context.Background(), PreviewRequest{
		ConnectionID: "c1",
		Scope:        PreviewScopeField,
		Table:        "users",
		Column:       "name",
		SampleSize:   3,
		Seed:         nil,
	})

	if err != nil {
		t.Fatalf("preview failed: %v", err)
	}

	if result.Metadata.Seed != fieldSeed {
		t.Fatalf("expected seed=%d (field fixed), got %d", fieldSeed, result.Metadata.Seed)
	}

	if !result.Metadata.Deterministic {
		t.Fatalf("expected deterministic=true with fixed seed policy")
	}

	if result.Metadata.SeedSource != "field_fixed" {
		t.Fatalf("expected seed_source=field_fixed, got %s", result.Metadata.SeedSource)
	}
}

// TestPreviewRequestSeedOverridesFieldPolicy 测试请求种子覆盖字段策略。
func TestPreviewRequestSeedOverridesFieldPolicy(t *testing.T) {
	reg := NewGeneratorRegistry()
	mustRegisterPreview(t, reg, NewStaticGenerator(GeneratorMeta{
		Type:          GeneratorTypeStringRandomChars,
		TypeTags:      []string{"supports:string"},
		Deterministic: true,
	}))

	fieldSeed := int64(999)
	requestSeed := int64(42)

	repo := NewPreviewableConfigRepository()
	_, _ = repo.UpsertFieldConfig(context.Background(), FieldGeneratorConfig{
		ColumnSchemaID: "col-1",
		ConnectionID:   "c1",
		Table:          "users",
		Column:         "name",
		GeneratorType:  GeneratorTypeStringRandomChars,
		GeneratorOpts:  map[string]interface{}{},
		SeedPolicy:     map[string]interface{}{"mode": "fixed", "seed": float64(fieldSeed)},
		IsEnabled:      true,
	})

	schemaProvider := NewPreviewableSchemaProvider([]FieldSchema{
		{ConnectionID: "c1", Table: "users", Column: "name", AbstractType: "string", ColumnID: "col-1"},
	})

	svc := NewGeneratorPreviewService(reg, repo, schemaProvider, NewGeneratorRuntime(), nil)

	result, err := svc.PreviewGeneration(context.Background(), PreviewRequest{
		ConnectionID: "c1",
		Scope:        PreviewScopeField,
		Table:        "users",
		Column:       "name",
		SampleSize:   3,
		Seed:         &requestSeed,
	})

	if err != nil {
		t.Fatalf("preview failed: %v", err)
	}

	if result.Metadata.Seed != requestSeed {
		t.Fatalf("expected seed=%d (request override), got %d", requestSeed, result.Metadata.Seed)
	}

	if result.Metadata.SeedSource != "preview_override" {
		t.Fatalf("expected seed_source=preview_override, got %s", result.Metadata.SeedSource)
	}
}

// TestPreviewGeneratorsResetBetweenRequests 测试生成器在请求间重置。
func TestPreviewGeneratorsResetBetweenRequests(t *testing.T) {
	reg := NewGeneratorRegistry()
	gen := NewCountingGenerator(GeneratorMeta{
		Type:          GeneratorType("counting_generator"),
		TypeTags:      []string{"supports:string"},
		Deterministic: true,
	})
	mustRegisterPreview(t, reg, gen)

	repo := NewPreviewableConfigRepository()
	_, _ = repo.UpsertFieldConfig(context.Background(), FieldGeneratorConfig{
		ColumnSchemaID: "col-1",
		ConnectionID:   "c1",
		Table:          "users",
		Column:         "name",
		GeneratorType:  GeneratorType("counting_generator"),
		GeneratorOpts:  map[string]interface{}{},
		IsEnabled:      true,
	})

	schemaProvider := NewPreviewableSchemaProvider([]FieldSchema{
		{ConnectionID: "c1", Table: "users", Column: "name", AbstractType: "string", ColumnID: "col-1"},
	})

	svc := NewGeneratorPreviewService(reg, repo, schemaProvider, NewGeneratorRuntime(), nil)

	seed := int64(1)

	result1, _ := svc.PreviewGeneration(context.Background(), PreviewRequest{
		ConnectionID: "c1",
		Scope:        PreviewScopeField,
		Table:        "users",
		Column:       "name",
		SampleSize:   3,
		Seed:         &seed,
	})

	result2, _ := svc.PreviewGeneration(context.Background(), PreviewRequest{
		ConnectionID: "c1",
		Scope:        PreviewScopeField,
		Table:        "users",
		Column:       "name",
		SampleSize:   3,
		Seed:         &seed,
	})

	samples1 := result1.Samples.([]interface{})
	samples2 := result2.Samples.([]interface{})

	for i := range samples1 {
		if samples1[i] != samples2[i] {
			t.Fatalf("after reset, samples at index %d differ: %v vs %v", i, samples1[i], samples2[i])
		}
	}
}

// TestPreviewConfigNotFound 测试字段配置不存在时的行为。
func TestPreviewConfigNotFound(t *testing.T) {
	reg := NewGeneratorRegistry()
	mustRegisterPreview(t, reg, NewStaticGenerator(GeneratorMeta{
		Type:          GeneratorTypeStringRandomChars,
		TypeTags:      []string{"supports:string"},
		Deterministic: true,
	}))

	repo := NewPreviewableConfigRepository() // 空 repo

	schemaProvider := NewPreviewableSchemaProvider([]FieldSchema{
		{ConnectionID: "c1", Table: "users", Column: "name", AbstractType: "string", ColumnID: "col-1"},
	})

	svc := NewGeneratorPreviewService(reg, repo, schemaProvider, NewGeneratorRuntime(), nil)

	_, err := svc.PreviewGeneration(context.Background(), PreviewRequest{
		ConnectionID: "c1",
		Scope:        PreviewScopeField,
		Table:        "users",
		Column:       "name",
		SampleSize:   3,
	})

	if err == nil {
		t.Fatalf("expected error when config not found")
	}

	gErr, ok := err.(*GeneratorError)
	if !ok {
		t.Fatalf("expected GeneratorError, got %T", err)
	}

	if gErr.Code != GeneratorErrNotRegistered {
		t.Fatalf("expected GENERATOR_NOT_REGISTERED for missing config, got %s", gErr.Code)
	}
}

// TestPreviewTablePartialSuccess 测试部分失败场景（4.4/4.5）。
func TestPreviewTablePartialSuccess(t *testing.T) {
	reg := NewGeneratorRegistry()
	mustRegisterPreview(t, reg, NewStaticGenerator(GeneratorMeta{
		Type:          GeneratorTypeIntRangeRandom,
		TypeTags:      []string{"supports:int"},
		Deterministic: true,
	}))
	mustRegisterPreview(t, reg, NewStaticGenerator(GeneratorMeta{
		Type:          GeneratorTypeStringRandomChars,
		TypeTags:      []string{"supports:string"},
		Deterministic: true,
	}))

	repo := NewPreviewableConfigRepository()
	// id: 成功
	_, _ = repo.UpsertFieldConfig(context.Background(), FieldGeneratorConfig{
		ColumnSchemaID: "col-id",
		ConnectionID:   "c1",
		Table:          "users",
		Column:         "id",
		GeneratorType:  GeneratorTypeIntRangeRandom,
		GeneratorOpts:  map[string]interface{}{},
		IsEnabled:      true,
	})
	// name: 成功
	_, _ = repo.UpsertFieldConfig(context.Background(), FieldGeneratorConfig{
		ColumnSchemaID: "col-name",
		ConnectionID:   "c1",
		Table:          "users",
		Column:         "name",
		GeneratorType:  GeneratorTypeStringRandomChars,
		GeneratorOpts:  map[string]interface{}{},
		IsEnabled:      true,
	})
	// email: 禁用 -> skipped
	_, _ = repo.UpsertFieldConfig(context.Background(), FieldGeneratorConfig{
		ColumnSchemaID: "col-email",
		ConnectionID:   "c1",
		Table:          "users",
		Column:         "email",
		GeneratorType:  GeneratorTypeStringRandomChars,
		GeneratorOpts:  map[string]interface{}{},
		IsEnabled:      false,
	})
	// phone: 未注册生成器 -> failed
	_, _ = repo.UpsertFieldConfig(context.Background(), FieldGeneratorConfig{
		ColumnSchemaID: "col-phone",
		ConnectionID:   "c1",
		Table:          "users",
		Column:         "phone",
		GeneratorType:  GeneratorType("unknown_generator"),
		GeneratorOpts:  map[string]interface{}{},
		IsEnabled:      true,
	})

	schemaProvider := NewPreviewableSchemaProvider([]FieldSchema{
		{ConnectionID: "c1", Table: "users", Column: "id", AbstractType: "int", ColumnID: "col-id"},
		{ConnectionID: "c1", Table: "users", Column: "name", AbstractType: "string", ColumnID: "col-name"},
		{ConnectionID: "c1", Table: "users", Column: "email", AbstractType: "string", ColumnID: "col-email"},
		{ConnectionID: "c1", Table: "users", Column: "phone", AbstractType: "string", ColumnID: "col-phone"},
	})

	svc := NewGeneratorPreviewService(reg, repo, schemaProvider, NewGeneratorRuntime(), nil)

	result, err := svc.PreviewGeneration(context.Background(), PreviewRequest{
		ConnectionID: "c1",
		Scope:        PreviewScopeTable,
		Table:        "users",
		SampleSize:   3,
	})

	if err != nil {
		t.Fatalf("partial success should not return error: %v", err)
	}

	// 验证部分成功标记
	if !result.Metadata.PartialSuccess {
		t.Fatalf("expected partial_success=true")
	}

	// 验证 samples 只包含成功字段
	samplesMap := result.Samples.(map[string][]interface{})
	if len(samplesMap) != 2 {
		t.Fatalf("expected 2 successful columns in samples, got %d", len(samplesMap))
	}

	// 验证 field_results 包含所有字段
	if len(result.FieldResults) != 4 {
		t.Fatalf("expected 4 field_results, got %d", len(result.FieldResults))
	}

	// 验证 field_results 状态
	statusMap := make(map[string]string)
	for _, fr := range result.FieldResults {
		statusMap[fr.Field] = fr.Status
	}

	if statusMap["id"] != "ok" {
		t.Fatalf("expected id status=ok, got %s", statusMap["id"])
	}
	if statusMap["name"] != "ok" {
		t.Fatalf("expected name status=ok, got %s", statusMap["name"])
	}
	if statusMap["email"] != "skipped" {
		t.Fatalf("expected email status=skipped, got %s", statusMap["email"])
	}
	if statusMap["phone"] != "failed" {
		t.Fatalf("expected phone status=failed, got %s", statusMap["phone"])
	}

	// 验证 warnings 包含失败/跳过字段信息
	if len(result.Warnings) < 2 {
		t.Fatalf("expected at least 2 warnings for skipped/failed fields")
	}

	// 验证 field_results.status=ok 与 samples 字段集合一致
	okFields := make([]string, 0)
	for _, fr := range result.FieldResults {
		if fr.Status == "ok" {
			okFields = append(okFields, fr.Field)
		}
	}
	for colName := range samplesMap {
		found := false
		for _, okField := range okFields {
			if okField == colName {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("samples column %s not found in ok field_results", colName)
		}
	}
}

// TestPreviewFieldResultsConsistencyWithSamples 测试 field_results 与 samples 一致性。
func TestPreviewFieldResultsConsistencyWithSamples(t *testing.T) {
	reg := NewGeneratorRegistry()
	mustRegisterPreview(t, reg, NewStaticGenerator(GeneratorMeta{
		Type:          GeneratorTypeStringRandomChars,
		TypeTags:      []string{"supports:string"},
		Deterministic: true,
	}))

	repo := NewPreviewableConfigRepository()
	_, _ = repo.UpsertFieldConfig(context.Background(), FieldGeneratorConfig{
		ColumnSchemaID: "col-1",
		ConnectionID:   "c1",
		Table:          "users",
		Column:         "name",
		GeneratorType:  GeneratorTypeStringRandomChars,
		GeneratorOpts:  map[string]interface{}{},
		IsEnabled:      true,
	})

	schemaProvider := NewPreviewableSchemaProvider([]FieldSchema{
		{ConnectionID: "c1", Table: "users", Column: "name", AbstractType: "string", ColumnID: "col-1"},
	})

	svc := NewGeneratorPreviewService(reg, repo, schemaProvider, NewGeneratorRuntime(), nil)

	result, err := svc.PreviewGeneration(context.Background(), PreviewRequest{
		ConnectionID: "c1",
		Scope:        PreviewScopeTable,
		Table:        "users",
		SampleSize:   3,
	})

	if err != nil {
		t.Fatalf("preview failed: %v", err)
	}

	samplesMap := result.Samples.(map[string][]interface{})

	// 验证一致性规则：status=ok 字段集合必须与 samples 字段集合一致
	okFieldSet := make(map[string]bool)
	for _, fr := range result.FieldResults {
		if fr.Status == "ok" {
			okFieldSet[fr.Field] = true
		}
	}

	samplesFieldSet := make(map[string]bool)
	for colName := range samplesMap {
		samplesFieldSet[colName] = true
	}

	// 双向检查
	for field := range okFieldSet {
		if !samplesFieldSet[field] {
			t.Fatalf("field %s is ok in field_results but missing in samples", field)
		}
	}
	for field := range samplesFieldSet {
		if !okFieldSet[field] {
			t.Fatalf("field %s is in samples but not ok in field_results", field)
		}
	}
}

// TestPreviewExternalDependencyNotReady 测试外部依赖未就绪场景（4.3）。
func TestPreviewExternalDependencyNotReady(t *testing.T) {
	reg := NewGeneratorRegistry()
	mustRegisterPreview(t, reg, NewStaticGenerator(GeneratorMeta{
		Type:          GeneratorType("external_feed_generator"),
		TypeTags:      []string{"supports:string", "requires_external_feed"},
		Deterministic: false,
	}))

	repo := NewPreviewableConfigRepository()
	_, _ = repo.UpsertFieldConfig(context.Background(), FieldGeneratorConfig{
		ColumnSchemaID: "col-1",
		ConnectionID:   "c1",
		Table:          "users",
		Column:         "external_ref",
		GeneratorType:  GeneratorType("external_feed_generator"),
		GeneratorOpts:  map[string]interface{}{"external_feed_id": "feed-1"},
		IsEnabled:      true,
	})

	schemaProvider := NewPreviewableSchemaProvider([]FieldSchema{
		{ConnectionID: "c1", Table: "users", Column: "external_ref", AbstractType: "string", ColumnID: "col-1"},
	})

	// 使用带依赖检查的运行时
	runtime := NewGeneratorRuntimeWithDependencyCheck(func(g Generator) (bool, string) {
		meta := g.Meta()
		for _, tag := range meta.TypeTags {
			if tag == "requires_external_feed" {
				return false, "external_feed not ready"
			}
		}
		return true, ""
	})

	svc := NewGeneratorPreviewService(reg, repo, schemaProvider, runtime, nil)

	_, err := svc.PreviewGeneration(context.Background(), PreviewRequest{
		ConnectionID: "c1",
		Scope:        PreviewScopeField,
		Table:        "users",
		Column:       "external_ref",
		SampleSize:   3,
	})

	if err == nil {
		t.Fatalf("expected FAILED_PRECONDITION when external dependency not ready")
	}

	gErr, ok := err.(*GeneratorError)
	if !ok {
		t.Fatalf("expected GeneratorError, got %T", err)
	}

	if gErr.Code != GeneratorErrFailedPrecondition {
		t.Fatalf("expected FAILED_PRECONDITION, got %s", gErr.Code)
	}

	// 错误消息应指明上游依赖
	if !containsSubstring(gErr.Message, "external") {
		t.Fatalf("error message should mention external dependency: %s", gErr.Message)
	}
}

// TestPreviewComputedContextDependencyNotReady 测试计算字段上下文依赖未就绪（6.4，对齐 spec-09）。
func TestPreviewComputedContextDependencyNotReady(t *testing.T) {
	reg := NewGeneratorRegistry()
	mustRegisterPreview(t, reg, NewStaticGenerator(GeneratorMeta{
		Type:          GeneratorType("computed_context_generator"),
		TypeTags:      []string{"supports:string", "requires_computed_context"},
		Deterministic: false,
	}))

	repo := NewPreviewableConfigRepository()
	_, _ = repo.UpsertFieldConfig(context.Background(), FieldGeneratorConfig{
		ColumnSchemaID: "col-1",
		ConnectionID:   "c1",
		Table:          "users",
		Column:         "computed_col",
		GeneratorType:  GeneratorType("computed_context_generator"),
		GeneratorOpts:  map[string]interface{}{},
		IsEnabled:      true,
	})

	schemaProvider := NewPreviewableSchemaProvider([]FieldSchema{
		{ConnectionID: "c1", Table: "users", Column: "computed_col", AbstractType: "string", ColumnID: "col-1"},
	})

	runtime := NewGeneratorRuntimeWithDependencyCheck(func(g Generator) (bool, string) {
		meta := g.Meta()
		for _, tag := range meta.TypeTags {
			if tag == "requires_computed_context" {
				return false, "computed context spec-09 not ready"
			}
		}
		return true, ""
	})

	svc := NewGeneratorPreviewService(reg, repo, schemaProvider, runtime, nil)

	_, err := svc.PreviewGeneration(context.Background(), PreviewRequest{
		ConnectionID: "c1",
		Scope:        PreviewScopeField,
		Table:        "users",
		Column:       "computed_col",
		SampleSize:   3,
	})
	if err == nil {
		t.Fatalf("expected FAILED_PRECONDITION when computed context not ready")
	}
	gErr, ok := err.(*GeneratorError)
	if !ok {
		t.Fatalf("expected GeneratorError, got %T", err)
	}
	if gErr.Code != GeneratorErrFailedPrecondition {
		t.Fatalf("expected FAILED_PRECONDITION, got %s", gErr.Code)
	}
	if !containsSubstring(gErr.Message, "spec-09") {
		t.Fatalf("error message should mention spec-09: %s", gErr.Message)
	}
}

// TestPreviewTableScopeExternalDependencyReturnsFieldResult 测试 table scope 下外部依赖失败返回 field_result。
func TestPreviewTableScopeExternalDependencyReturnsFieldResult(t *testing.T) {
	reg := NewGeneratorRegistry()
	mustRegisterPreview(t, reg, NewStaticGenerator(GeneratorMeta{
		Type:          GeneratorTypeStringRandomChars,
		TypeTags:      []string{"supports:string"},
		Deterministic: true,
	}))
	mustRegisterPreview(t, reg, NewStaticGenerator(GeneratorMeta{
		Type:          GeneratorType("external_feed_generator"),
		TypeTags:      []string{"supports:string", "requires_external_feed"},
		Deterministic: false,
	}))

	repo := NewPreviewableConfigRepository()
	_, _ = repo.UpsertFieldConfig(context.Background(), FieldGeneratorConfig{
		ColumnSchemaID: "col-name",
		ConnectionID:   "c1",
		Table:          "users",
		Column:         "name",
		GeneratorType:  GeneratorTypeStringRandomChars,
		GeneratorOpts:  map[string]interface{}{},
		IsEnabled:      true,
	})
	_, _ = repo.UpsertFieldConfig(context.Background(), FieldGeneratorConfig{
		ColumnSchemaID: "col-ref",
		ConnectionID:   "c1",
		Table:          "users",
		Column:         "external_ref",
		GeneratorType:  GeneratorType("external_feed_generator"),
		GeneratorOpts:  map[string]interface{}{},
		IsEnabled:      true,
	})

	schemaProvider := NewPreviewableSchemaProvider([]FieldSchema{
		{ConnectionID: "c1", Table: "users", Column: "name", AbstractType: "string", ColumnID: "col-name"},
		{ConnectionID: "c1", Table: "users", Column: "external_ref", AbstractType: "string", ColumnID: "col-ref"},
	})

	runtime := NewGeneratorRuntimeWithDependencyCheck(func(g Generator) (bool, string) {
		meta := g.Meta()
		for _, tag := range meta.TypeTags {
			if tag == "requires_external_feed" {
				return false, "external_feed spec-08 not ready"
			}
		}
		return true, ""
	})

	svc := NewGeneratorPreviewService(reg, repo, schemaProvider, runtime, nil)

	result, err := svc.PreviewGeneration(context.Background(), PreviewRequest{
		ConnectionID: "c1",
		Scope:        PreviewScopeTable,
		Table:        "users",
		SampleSize:   3,
	})

	if err != nil {
		t.Fatalf("table scope partial success should not return top-level error")
	}

	// name 字段应成功
	samplesMap := result.Samples.(map[string][]interface{})
	if len(samplesMap["name"]) != 3 {
		t.Fatalf("expected name samples")
	}

	// external_ref 字段应失败并记录在 field_results
	refResult := findFieldResult(result.FieldResults, "external_ref")
	if refResult == nil {
		t.Fatalf("expected external_ref in field_results")
	}
	if refResult.Status != "failed" {
		t.Fatalf("expected external_ref status=failed")
	}
	if refResult.ErrorCode != string(GeneratorErrFailedPrecondition) {
		t.Fatalf("expected FAILED_PRECONDITION error code")
	}

	// 警告应包含上游依赖信息
	found := false
	for _, w := range result.Warnings {
		if w.Field == "external_ref" && containsSubstring(w.Message, "spec-08") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected warning mentioning spec-08 for external_ref")
	}
}

// ===== 辅助类型和函数 =====

// CountingGenerator 是用于测试重置行为的计数生成器。
type CountingGenerator struct {
	meta    GeneratorMeta
	counter int
}

// NewCountingGenerator 创建计数生成器。
func NewCountingGenerator(meta GeneratorMeta) *CountingGenerator {
	return &CountingGenerator{meta: meta, counter: 0}
}

// Meta 返回静态元数据。
func (g *CountingGenerator) Meta() GeneratorMeta { return g.meta }

// Generate 返回递增值。
func (g *CountingGenerator) Generate(ctx context.Context, in GeneratorContext) (interface{}, error) {
	g.counter++
	return g.counter, nil
}

// GenerateBatch 返回递增值数组。
func (g *CountingGenerator) GenerateBatch(ctx context.Context, in GeneratorContext, count int) ([]interface{}, error) {
	out := make([]interface{}, count)
	for i := 0; i < count; i++ {
		g.counter++
		out[i] = g.counter
	}
	return out, nil
}

// Reset 重置计数器。
func (g *CountingGenerator) Reset() error {
	g.counter = 0
	return nil
}

// containsSubstring 检查字符串是否包含子串。
func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstringHelper(s, substr))
}

func containsSubstringHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// findFieldResult 在 field_results 中查找指定字段。
func findFieldResult(results []PreviewFieldResult, field string) *PreviewFieldResult {
	for _, r := range results {
		if r.Field == field {
			return &r
		}
	}
	return nil
}

// GeneratorRuntimeWithDependencyCheck 是带依赖检查的运行时。
type GeneratorRuntimeWithDependencyCheck struct {
	base          *GeneratorRuntime
	dependencyCheck func(g Generator) (ready bool, reason string)
}

// NewGeneratorRuntimeWithDependencyCheck 创建带依赖检查的运行时。
func NewGeneratorRuntimeWithDependencyCheck(check func(g Generator) (bool, string)) *GeneratorRuntimeWithDependencyCheck {
	return &GeneratorRuntimeWithDependencyCheck{
		base:           NewGeneratorRuntime(),
		dependencyCheck: check,
	}
}

// GenerateBatch 执行批量生成（带依赖检查）。
func (r *GeneratorRuntimeWithDependencyCheck) GenerateBatch(ctx context.Context, generator Generator, genCtx GeneratorContext, count int) ([]interface{}, error) {
	if r.dependencyCheck != nil {
		ready, reason := r.dependencyCheck(generator)
		if !ready {
			return nil, &GeneratorError{
				Code:    GeneratorErrFailedPrecondition,
				Path:    "dependency",
				Message: reason,
			}
		}
	}
	return r.base.GenerateBatch(ctx, generator, genCtx, count)
}

// Generate 执行单值生成（带依赖检查）。
func (r *GeneratorRuntimeWithDependencyCheck) Generate(ctx context.Context, generator Generator, genCtx GeneratorContext) (interface{}, error) {
	if r.dependencyCheck != nil {
		ready, reason := r.dependencyCheck(generator)
		if !ready {
			return nil, &GeneratorError{
				Code:    GeneratorErrFailedPrecondition,
				Path:    "dependency",
				Message: reason,
			}
		}
	}
	return r.base.Generate(ctx, generator, genCtx)
}