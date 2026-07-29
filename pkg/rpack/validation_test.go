package rpack

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCueValidator(t *testing.T) {
	const schema = `#Schema: { field!: string & "right-choice"  }`
	v, err := NewCueValidator([]byte(schema), "#Schema")
	if err != nil {
		t.Fatalf("Failed setting up validation: %s", err)
	}
	// Valid
	err = v.Validate(struct {
		Field string `json:"field"`
	}{
		Field: "right-choice",
	})
	if err != nil {
		t.Fatalf("Validation failed: %s", err)
	}

	// Invalid
	err = v.Validate(struct {
		Field string `json:"field"`
	}{
		Field: "wrong-choice",
	})
	if err == nil {
		t.Fatalf("Validation should have failed for `wrong-choice`")
	}
}

func TestCueValidatorRejectsMissingRequiredField(t *testing.T) {
	validator, err := NewCueValidator([]byte(`#Schema: { required!: string }`), "#Schema")
	if err != nil {
		t.Fatal(err)
	}
	if err = validator.Validate(map[string]any{}); err == nil || !strings.Contains(err.Error(), "required") {
		t.Fatalf("expected missing-required-field error, got: %v", err)
	}
}

func TestEmptyValidator(t *testing.T) {
	v := &EmptyValidator{}
	err := v.Validate(nil)
	if err != nil {
		t.Fatalf("Validation failed")
	}
}

func TestValidateRPackDef(t *testing.T) {
	tests := []struct {
		files   map[string]string
		name    string
		errMsg  string
		wantErr bool
	}{
		{
			name:    "valid minimal",
			wantErr: false,
			files: map[string]string{
				"rpack.yaml": "\"@schema_version\": \"v1\"\nname: \"mypack\"\n",
				"script.lua": "print(\"hello\")",
			},
		},
		{
			name:    "valid with schema",
			wantErr: false,
			files: map[string]string{
				"rpack.yaml": "\"@schema_version\": \"v1\"\nname: \"mypack\"\n",
				"script.lua": "print(\"hello\")",
				"schema.cue": "#Schema: {\n    test: string\n}",
			},
		},
		{
			name:    "missing rpack.yaml",
			wantErr: true,
			errMsg:  "rpack definition file",
			files: map[string]string{
				"script.lua": "print(\"hello\")",
			},
		},
		{
			name:    "invalid schema",
			wantErr: true,
			errMsg:  "schema validation",
			files: map[string]string{
				"rpack.yaml": "\"@schema_version\": \"v1\"\n",
				"script.lua": "print(\"hello\")",
			},
		},
		{
			name:    "missing script.lua",
			wantErr: true,
			errMsg:  "script file",
			files: map[string]string{
				"rpack.yaml": "\"@schema_version\": \"v1\"\nname: \"mypack\"\n",
			},
		},
		{
			name:    "unparseable schema.cue surfaces compile error",
			wantErr: true,
			errMsg:  "compiling CUE schema: expected '}'",
			files: map[string]string{
				"rpack.yaml": "\"@schema_version\": \"v1\"\nname: \"mypack\"\n",
				"script.lua": "print(\"hello\")",
				"schema.cue": "#Schema: { field: string\n",
			},
		},
		{
			name:    "unparseable schema.cue",
			wantErr: true,
			errMsg:  "validation context",
			files: map[string]string{
				"rpack.yaml": "\"@schema_version\": \"v1\"\nname: \"mypack\"\n",
				"script.lua": "print(\"hello\")",
				"schema.cue": "not valid cue {{{{{",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			for fname, content := range tt.files {
				_ = os.WriteFile(filepath.Join(dir, fname), []byte(content), 0o644) //nolint:gosec // test files
			}
			_, err := ValidateRPackDef(dir)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.errMsg)
				} else if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("expected error containing %q, got: %v", tt.errMsg, err)
				}
			} else if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

// TestNewCueValidator_CompileError verifies that an unparseable schema surfaces
// the actual compiler error pointing at the offending line, rather than the
// opaque "#Schema does not exist" fallback.
func TestNewCueValidator_CompileError(t *testing.T) {
	// Missing closing brace -> compiler error.
	_, err := NewCueValidator([]byte("#Schema: { field: string\n"), "#Schema")
	if err == nil {
		t.Fatalf("expected a compile error, got nil")
	}
	if !strings.Contains(err.Error(), "compiling CUE schema") {
		t.Errorf("error should mention compiling CUE schema, got: %v", err)
	}
	// The actual compiler message should survive (it points at the problem).
	if !strings.Contains(err.Error(), "expected '}'") {
		t.Errorf("error should contain the compiler message, got: %v", err)
	}
}

// TestNewCueValidator_MissingPath verifies the path-not-found fallback now reads
// clearly and does not lose the path name.
func TestNewCueValidator_MissingPath(t *testing.T) {
	_, err := NewCueValidator([]byte("#SomethingElse: { field: string }\n"), "#Schema")
	if err == nil {
		t.Fatalf("expected an error for missing path, got nil")
	}
	if !strings.Contains(err.Error(), "#Schema") || !strings.Contains(err.Error(), "does not exist") {
		t.Errorf("error should name the missing path, got: %v", err)
	}
}

// TestCueValidator_ValidateHumanMessage verifies a validation failure renders
// a human-readable message: the internal "#Schema." root is stripped and the
// location is prefixed with "rpack.yaml:".
func TestCueValidator_ValidateHumanMessage(t *testing.T) {
	schema := []byte("#Schema: { field!: string }\n")
	v, err := NewCueValidator(schema, "#Schema")
	if err != nil {
		t.Fatalf("NewCueValidator: %v", err)
	}
	err = v.Validate(struct {
		Field int `json:"field"`
	}{Field: 42})
	if err == nil {
		t.Fatalf("expected validation error, got nil")
	}
	msg := err.Error()
	if !strings.HasPrefix(msg, "rpack.yaml: ") {
		t.Errorf("message should be prefixed with \"rpack.yaml: \", got: %q", msg)
	}
	if strings.Contains(msg, "#Schema.") || strings.Contains(msg, "#Schema field") {
		t.Errorf("internal CUE root path should be stripped, got: %q", msg)
	}
	if !strings.Contains(msg, "field") {
		t.Errorf("message should reference the failing field, got: %q", msg)
	}
	if !strings.Contains(msg, "string") || !strings.Contains(msg, "int") {
		t.Errorf("message should mention both expected and actual types, got: %q", msg)
	}
}
