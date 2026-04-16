package generator

import (
	"context"
	"testing"

	"loomidbx/schema"
)

func TestRegistryRejectsDuplicateGeneratorType(t *testing.T) {
	reg := NewGeneratorRegistry()
	gen := NewStaticGenerator(GeneratorMeta{
		Type:          GeneratorType("string_random_chars"),
		TypeTags:      []string{"supports:string"},
		Deterministic: true,
	})
	if err := reg.Register(gen); err != nil {
		t.Fatalf("first register failed: %v", err)
	}
	err := reg.Register(gen)
	if err == nil {
		t.Fatalf("expected duplicate register error")
	}
	gErr, ok := err.(*GeneratorError)
	if !ok {
		t.Fatalf("expected GeneratorError, got %T", err)
	}
	if gErr.Code != GeneratorErrConflict {
		t.Fatalf("unexpected code: %s", gErr.Code)
	}
}

func TestResolverReturnsDefaultAndCandidates(t *testing.T) {
	reg := NewGeneratorRegistry()
	mustRegister(t, reg, NewStaticGenerator(GeneratorMeta{
		Type:          GeneratorType("string_random_chars"),
		TypeTags:      []string{"supports:string", "deterministic"},
		Deterministic: true,
	}))
	mustRegister(t, reg, NewStaticGenerator(GeneratorMeta{
		Type:          GeneratorType("enum_value"),
		TypeTags:      []string{"supports:string", "supports:int"},
		Deterministic: true,
	}))

	resolver := NewGeneratorTypeResolver(reg)
	out, err := resolver.ResolveCandidates(FieldSchema{
		ConnectionID: "c1",
		Table:        "users",
		Column:       "name",
		AbstractType: schema.AbstractTypeString,
	})
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	if out.DefaultGeneratorType != GeneratorTypeStringRandomChars {
		t.Fatalf("unexpected default: %s", out.DefaultGeneratorType)
	}
	if len(out.Candidates) != 2 {
		t.Fatalf("unexpected candidates count: %d", len(out.Candidates))
	}
}

func TestResolverReturnsStableErrorWhenNoGenerator(t *testing.T) {
	reg := NewGeneratorRegistry()
	resolver := NewGeneratorTypeResolver(reg)
	_, err := resolver.ResolveCandidates(FieldSchema{
		ConnectionID: "c1",
		Table:        "users",
		Column:       "created_at",
		AbstractType: schema.AbstractTypeDatetime,
	})
	if err == nil {
		t.Fatalf("expected error")
	}
	gErr, ok := err.(*GeneratorError)
	if !ok {
		t.Fatalf("expected GeneratorError, got %T", err)
	}
	if gErr.Path != "field.generator_type" {
		t.Fatalf("unexpected path: %s", gErr.Path)
	}
	if gErr.Code != GeneratorErrInvalidArgument {
		t.Fatalf("unexpected code: %s", gErr.Code)
	}
}

func TestSaveFieldGeneratorConfigReturnsStableFieldErrorPath(t *testing.T) {
	reg := NewGeneratorRegistry()
	mustRegister(t, reg, NewStaticGenerator(GeneratorMeta{
		Type:          GeneratorTypeStringRandomChars,
		TypeTags:      []string{"supports:string"},
		Deterministic: true,
	}))
	repo := NewInMemoryConfigRepository()
	cfgSvc := NewGeneratorConfigService(
		NewStaticSchemaProvider([]FieldSchema{{
			ConnectionID: "c1",
			Table:        "users",
			Column:       "name",
			AbstractType: schema.AbstractTypeString,
		}}),
		NewGeneratorTypeResolver(reg),
		NewGeneratorConfigValidator(reg),
		repo,
	)

	_, err := cfgSvc.SaveFieldGeneratorConfig(context.Background(), SaveFieldGeneratorConfigRequest{
		ConnectionID:   "c1",
		Table:          "users",
		Column:         "name",
		GeneratorType:  GeneratorTypeStringRandomChars,
		GeneratorOpts:  map[string]interface{}{},
		SeedPolicy:     map[string]interface{}{"mode": "inherit_global"},
		NullPolicy:     "respect_nullable",
		IsEnabled:      true,
		ModifiedSource: "unknown_source",
	})
	if err == nil {
		t.Fatalf("expected validation error")
	}
	if len(err.FieldErrors) == 0 {
		t.Fatalf("expected field errors")
	}
	if err.FieldErrors[0].Path != "modified_source" {
		t.Fatalf("unexpected path: %s", err.FieldErrors[0].Path)
	}
}

func TestSaveFieldGeneratorConfigPersistsAndCanReadBack(t *testing.T) {
	reg := NewGeneratorRegistry()
	mustRegister(t, reg, NewStaticGenerator(GeneratorMeta{
		Type:          GeneratorTypeStringRandomChars,
		TypeTags:      []string{"supports:string"},
		Deterministic: true,
	}))
	repo := NewInMemoryConfigRepository()
	cfgSvc := NewGeneratorConfigService(
		NewStaticSchemaProvider([]FieldSchema{{
			ConnectionID: "c1",
			Table:        "users",
			Column:       "name",
			AbstractType: schema.AbstractTypeString,
			ColumnID:     "col-1",
		}}),
		NewGeneratorTypeResolver(reg),
		NewGeneratorConfigValidator(reg),
		repo,
	)

	saved, err := cfgSvc.SaveFieldGeneratorConfig(context.Background(), SaveFieldGeneratorConfigRequest{
		ConnectionID:   "c1",
		Table:          "users",
		Column:         "name",
		GeneratorType:  GeneratorTypeStringRandomChars,
		GeneratorOpts:  map[string]interface{}{"length": 8},
		SeedPolicy:     map[string]interface{}{"mode": "fixed", "seed": 7},
		NullPolicy:     "respect_nullable",
		IsEnabled:      true,
		ModifiedSource: ModifiedSourceUIManual,
	})
	if err != nil {
		t.Fatalf("save failed: %v", err)
	}
	if saved.ConfigVersion != 1 {
		t.Fatalf("unexpected version: %d", saved.ConfigVersion)
	}
	loaded, getErr := cfgSvc.GetFieldGeneratorConfig(context.Background(), "c1", "users", "name")
	if getErr != nil {
		t.Fatalf("get failed: %v", getErr)
	}
	if loaded.ModifiedSource != ModifiedSourceUIManual {
		t.Fatalf("unexpected modified source: %s", loaded.ModifiedSource)
	}
}

// TestSchemaChangeRecomputesCandidatesAndRejectsIncompatibleConfig 测试 schema 变化后候选重算与配置再校验（6.4，对齐 spec-02）。
func TestSchemaChangeRecomputesCandidatesAndRejectsIncompatibleConfig(t *testing.T) {
	reg := NewGeneratorRegistry()
	mustRegister(t, reg, NewStaticGenerator(GeneratorMeta{
		Type:          GeneratorTypeIntRangeRandom,
		TypeTags:      []string{"supports:int"},
		Deterministic: true,
	}))
	mustRegister(t, reg, NewStaticGenerator(GeneratorMeta{
		Type:          GeneratorTypeStringRandomChars,
		TypeTags:      []string{"supports:string"},
		Deterministic: true,
	}))

	repo := NewInMemoryConfigRepository()

	// 初始 schema：name 为 string
	cfgSvcString := NewGeneratorConfigService(
		NewStaticSchemaProvider([]FieldSchema{{
			ConnectionID: "c1",
			Table:        "users",
			Column:       "name",
			AbstractType: schema.AbstractTypeString,
			ColumnID:     "col-1",
		}}),
		NewGeneratorTypeResolver(reg),
		NewGeneratorConfigValidator(reg),
		repo,
	)

	_, err := cfgSvcString.SaveFieldGeneratorConfig(context.Background(), SaveFieldGeneratorConfigRequest{
		ConnectionID:   "c1",
		Table:          "users",
		Column:         "name",
		GeneratorType:  GeneratorTypeStringRandomChars,
		GeneratorOpts:  map[string]interface{}{"length": 8},
		SeedPolicy:     map[string]interface{}{"mode": "inherit_global"},
		NullPolicy:     "respect_nullable",
		IsEnabled:      true,
		ModifiedSource: ModifiedSourceUIManual,
	})
	if err != nil {
		t.Fatalf("save should succeed on string schema, got: %v", err)
	}

	// schema 变化：name 变为 int，再次保存同一 generator_type 应被拒绝
	cfgSvcInt := NewGeneratorConfigService(
		NewStaticSchemaProvider([]FieldSchema{{
			ConnectionID: "c1",
			Table:        "users",
			Column:       "name",
			AbstractType: schema.AbstractTypeInt,
			ColumnID:     "col-1",
		}}),
		NewGeneratorTypeResolver(reg),
		NewGeneratorConfigValidator(reg),
		repo,
	)

	_, err2 := cfgSvcInt.SaveFieldGeneratorConfig(context.Background(), SaveFieldGeneratorConfigRequest{
		ConnectionID:   "c1",
		Table:          "users",
		Column:         "name",
		GeneratorType:  GeneratorTypeStringRandomChars,
		GeneratorOpts:  map[string]interface{}{"length": 8},
		SeedPolicy:     map[string]interface{}{"mode": "inherit_global"},
		NullPolicy:     "respect_nullable",
		IsEnabled:      true,
		ModifiedSource: ModifiedSourceUIManual,
	})
	if err2 == nil {
		t.Fatalf("expected validation error after schema change")
	}
	if len(err2.FieldErrors) == 0 {
		t.Fatalf("expected field errors")
	}
	if err2.FieldErrors[0].Path != "generator_type" {
		t.Fatalf("expected generator_type error path, got: %s", err2.FieldErrors[0].Path)
	}
}

// TestDefaultGeneratorMappingStable 测试默认生成器映射表的稳定性（6.13）。
func TestDefaultGeneratorMappingStable(t *testing.T) {
	cases := []struct {
		abstractType string
		want         GeneratorType
	}{
		{abstractType: "int", want: GeneratorTypeIntRangeRandom},
		{abstractType: "decimal", want: GeneratorTypeDecimalRangeRandom},
		{abstractType: "string", want: GeneratorTypeStringRandomChars},
		{abstractType: "boolean", want: GeneratorTypeBooleanRatio},
		{abstractType: "datetime", want: GeneratorTypeDatetimeRangeRandom},
	}

	for _, tc := range cases {
		if got := defaultGeneratorByAbstractType(tc.abstractType); got != tc.want {
			t.Fatalf("defaultGeneratorByAbstractType(%s) = %s, want %s", tc.abstractType, got, tc.want)
		}
	}
}

// TestResolverDefaultSuggestionFollowsAbstractType 测试 schema 变化后二次解析仍返回与抽象类型一致的默认建议（6.13）。
func TestResolverDefaultSuggestionFollowsAbstractType(t *testing.T) {
	reg := NewGeneratorRegistry()
	// 注册五种默认生成器，确保每种抽象类型都有候选。
	mustRegister(t, reg, NewStaticGenerator(GeneratorMeta{
		Type:          GeneratorTypeIntRangeRandom,
		TypeTags:      []string{"supports:int"},
		Deterministic: true,
	}))
	mustRegister(t, reg, NewStaticGenerator(GeneratorMeta{
		Type:          GeneratorTypeDecimalRangeRandom,
		TypeTags:      []string{"supports:decimal"},
		Deterministic: true,
	}))
	mustRegister(t, reg, NewStaticGenerator(GeneratorMeta{
		Type:          GeneratorTypeStringRandomChars,
		TypeTags:      []string{"supports:string"},
		Deterministic: true,
	}))
	mustRegister(t, reg, NewStaticGenerator(GeneratorMeta{
		Type:          GeneratorTypeBooleanRatio,
		TypeTags:      []string{"supports:boolean"},
		Deterministic: true,
	}))
	mustRegister(t, reg, NewStaticGenerator(GeneratorMeta{
		Type:          GeneratorTypeDatetimeRangeRandom,
		TypeTags:      []string{"supports:datetime"},
		Deterministic: true,
	}))

	resolver := NewGeneratorTypeResolver(reg)

	out1, err := resolver.ResolveCandidates(FieldSchema{
		ConnectionID: "c1",
		Table:        "users",
		Column:       "x",
		AbstractType: "string",
	})
	if err != nil {
		t.Fatalf("resolve string failed: %v", err)
	}
	if out1.DefaultGeneratorType != GeneratorTypeStringRandomChars {
		t.Fatalf("unexpected default for string: %s", out1.DefaultGeneratorType)
	}

	// 模拟 schema 变化：同字段从 string 变为 int，再次解析默认项应随之变化。
	out2, err := resolver.ResolveCandidates(FieldSchema{
		ConnectionID: "c1",
		Table:        "users",
		Column:       "x",
		AbstractType: "int",
	})
	if err != nil {
		t.Fatalf("resolve int failed: %v", err)
	}
	if out2.DefaultGeneratorType != GeneratorTypeIntRangeRandom {
		t.Fatalf("unexpected default for int: %s", out2.DefaultGeneratorType)
	}
}

func mustRegister(t *testing.T, reg *GeneratorRegistry, generator Generator) {
	t.Helper()
	if err := reg.Register(generator); err != nil {
		t.Fatalf("register failed: %v", err)
	}
}
