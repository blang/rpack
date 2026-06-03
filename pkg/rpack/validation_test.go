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
				"rpack.yaml": "name: 123\n",
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
