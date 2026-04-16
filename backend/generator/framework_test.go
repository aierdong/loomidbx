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

func mustRegister(t *testing.T, reg *GeneratorRegistry, generator Generator) {
	t.Helper()
	if err := reg.Register(generator); err != nil {
		t.Fatalf("register failed: %v", err)
	}
}
