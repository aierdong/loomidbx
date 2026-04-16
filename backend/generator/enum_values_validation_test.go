package generator

import (
	"testing"
)

func TestEnumValueValidation_ByAbstractType(t *testing.T) {
	tests := []struct {
		name         string
		abstractType string
		opts         map[string]interface{}
		wantErrPath  string
	}{
		{
			name:         "int_ok",
			abstractType: "int",
			opts:         map[string]interface{}{"values": []interface{}{float64(1), float64(2), float64(3)}},
			wantErrPath:  "",
		},
		{
			name:         "decimal_ok",
			abstractType: "decimal",
			opts:         map[string]interface{}{"values": []interface{}{float64(1.1), float64(2.2)}},
			wantErrPath:  "",
		},
		{
			name:         "string_ok",
			abstractType: "string",
			opts:         map[string]interface{}{"values": []interface{}{"x", "y"}},
			wantErrPath:  "",
		},
		{
			name:         "boolean_ok",
			abstractType: "boolean",
			opts:         map[string]interface{}{"values": []interface{}{true, false}},
			wantErrPath:  "",
		},
		{
			name:         "datetime_ok_string_values",
			abstractType: "datetime",
			opts:         map[string]interface{}{"values": []interface{}{"2026-04-16T10:15:30Z"}},
			wantErrPath:  "",
		},
		{
			name:         "missing_values",
			abstractType: "string",
			opts:         map[string]interface{}{},
			wantErrPath:  "generator_opts.values",
		},
		{
			name:         "empty_values",
			abstractType: "string",
			opts:         map[string]interface{}{"values": []interface{}{}},
			wantErrPath:  "generator_opts.values",
		},
		{
			name:         "mixed_types_in_string",
			abstractType: "string",
			opts:         map[string]interface{}{"values": []interface{}{"x", float64(1)}},
			wantErrPath:  "generator_opts.values[1]",
		},
		{
			name:         "incompatible_type_in_int",
			abstractType: "int",
			opts:         map[string]interface{}{"values": []interface{}{"1"}},
			wantErrPath:  "generator_opts.values[0]",
		},
		{
			name:         "incompatible_type_in_boolean",
			abstractType: "boolean",
			opts:         map[string]interface{}{"values": []interface{}{true, "false"}},
			wantErrPath:  "generator_opts.values[1]",
		},
	}

	validator := NewGeneratorConfigValidator(NewGeneratorRegistry())
	candidates := []GeneratorType{GeneratorTypeEnumValue}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := SaveFieldGeneratorConfigRequest{
				ConnectionID:   "c1",
				Table:          "users",
				Column:         "status",
				GeneratorType:  GeneratorTypeEnumValue,
				GeneratorOpts:  tt.opts,
				IsEnabled:      true,
				ModifiedSource: ModifiedSourceUIManual,
			}
			field := FieldSchema{
				ConnectionID: "c1",
				Table:        "users",
				Column:       "status",
				AbstractType: tt.abstractType,
				ColumnID:     "col-status",
			}

			err := validator.Validate(req, field, candidates)
			if tt.wantErrPath == "" {
				if err != nil {
					t.Fatalf("expected no error, got: %v", err)
				}
				return
			}

			if err == nil || len(err.FieldErrors) == 0 {
				t.Fatalf("expected validation error, got nil")
			}
			if err.FieldErrors[0].Path != tt.wantErrPath {
				t.Fatalf("expected error path=%s, got=%s", tt.wantErrPath, err.FieldErrors[0].Path)
			}
		})
	}
}

