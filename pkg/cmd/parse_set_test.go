package cmd

import (
	"encoding/json"
	"fmt"
	"testing"

	"sigs.k8s.io/yaml"

	"github.com/blang/rpack/pkg/rpack"
)

func TestParseSetFlags(t *testing.T) {
	tcs := []struct { //nolint:govet // fieldalignment is not critical in table-driven tests
		name    string
		flags   []string
		want    string
		wantErr bool
	}{
		// Scalars
		{name: "string", flags: []string{"name=Alice"}, want: `{"name":"Alice"}`},
		{name: "int", flags: []string{"count=42"}, want: `{"count":42}`},
		{name: "bool true", flags: []string{"enabled=true"}, want: `{"enabled":true}`},
		{name: "bool false", flags: []string{"enabled=false"}, want: `{"enabled":false}`},
		{name: "float", flags: []string{"ratio=2.5"}, want: `{"ratio":2.5}`},
		// YAML scalar resolution (parity with rpack.yaml file)
		{name: "float version", flags: []string{"version=3.10"}, want: `{"version":3.1}`},
		{name: "leading zero", flags: []string{"count=08"}, want: `{"count":8}`},
		{name: "exponential", flags: []string{"n=1e5"}, want: `{"n":100000}`},
		{name: "float int shape", flags: []string{"x=1.0"}, want: `{"x":1}`},
		{name: "bool upper", flags: []string{"enabled=TRUE"}, want: `{"enabled":true}`},
		{name: "negative number", flags: []string{"x=-5"}, want: `{"x":-5}`},
		{name: "yaml null", flags: []string{"x=null"}, want: `{"x":null}`},
		{name: "empty value is null", flags: []string{"x="}, want: `{"x":null}`},

		// Nested
		{name: "nested", flags: []string{"nested.key=value"}, want: `{"nested":{"key":"value"}}`},
		{name: "deep nested", flags: []string{"a.b.c=d"}, want: `{"a":{"b":{"c":"d"}}}`},

		// Duplicate keys → list
		{name: "single dup", flags: []string{"list=a"}, want: `{"list":"a"}`},
		{name: "two dups", flags: []string{"list=a", "list=b"}, want: `{"list":["a","b"]}`},
		{name: "three dups", flags: []string{"list=a", "list=b", "list=c"}, want: `{"list":["a","b","c"]}`},

		// Index notation
		{name: "index 0", flags: []string{"list.0=zero"}, want: `{"list":["zero"]}`},
		{name: "index two", flags: []string{"list.0=zero", "list.1=one"}, want: `{"list":["zero","one"]}`},
		{name: "index sparse", flags: []string{"list.2=two"}, want: `{"list":[null,null,"two"]}`},
		{name: "index with nested", flags: []string{"hooks.0.name=x"}, want: `{"hooks":[{"name":"x"}]}`},
		{name: "index multi nested",
			flags: []string{"hooks.0.name=a", "hooks.1.name=b"},
			want:  `{"hooks":[{"name":"a"},{"name":"b"}]}`},

		// Multiple keys
		{name: "mixed keys", flags: []string{"name=Alice", "count=42"},
			want: `{"count":42,"name":"Alice"}`},

		// Errors
		{name: "no equals", flags: []string{"bad"}, wantErr: true},
		{name: "root array", flags: []string{"0=bad"}, wantErr: true},
		{name: "mix scalar+index", flags: []string{"list.key=val", "list.0=zero"}, wantErr: true},
		{name: "yaml unresolvable", flags: []string{"x=.inf"}, wantErr: true},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseSetFlags(tc.flags)
			if tc.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			gotJSON, _ := json.Marshal(got)
			if string(gotJSON) != tc.want {
				t.Errorf("got  %s\nwant %s", gotJSON, tc.want)
			}
		})
	}
}

func TestSetNestedValue_IndexNotation(t *testing.T) {
	// Direct test for the array creation path
	m := make(map[string]any)
	err := setNestedValue(m, "list.0", "zero")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	err = setNestedValue(m, "list.1", "one")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	err = setNestedValue(m, "list.2", "two")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	list, ok := m["list"].([]any)
	if !ok {
		t.Fatalf("list is not an array, got %T: %v", m["list"], m["list"])
	}
	if len(list) != 3 {
		t.Fatalf("expected 3 elements, got %d: %v", len(list), list)
	}
	if list[0] != "zero" || list[1] != "one" || list[2] != "two" {
		t.Errorf("unexpected array contents: %v", list)
	}
}

func TestSetNestedValue_NestedIndex(t *testing.T) {
	m := make(map[string]any)
	// Set hooks.0.name = "trailing-whitespace"
	err := setNestedValue(m, "hooks.0.name", "trailing-whitespace")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	err = setNestedValue(m, "hooks.1.name", "end-of-file-fixer")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	hooks, ok := m["hooks"].([]any)
	if !ok {
		t.Fatalf("hooks is not an array, got %T: %v", m["hooks"], m["hooks"])
	}
	if len(hooks) != 2 {
		t.Fatalf("expected 2 hooks, got %d: %v", len(hooks), hooks)
	}
	h0, ok := hooks[0].(map[string]any)
	if !ok {
		t.Fatalf("hooks[0] is not a map: %T", hooks[0])
	}
	if h0["name"] != "trailing-whitespace" {
		t.Errorf("hooks[0].name = %v", h0["name"])
	}
}

// TestParseSetStringFlags verifies --set-string keeps every value verbatim as
// a string, regardless of a shape that --set would coerce (08, 3.10, true).
// This is the CLI equivalent of quoting a value in an rpack.yaml file.
func TestParseSetStringFlags(t *testing.T) {
	tcs := []struct { //nolint:govet // fieldalignment is not critical in table-driven tests
		name    string
		flags   []string
		want    string
		wantErr bool
	}{
		{name: "int as string", flags: []string{"count=42"}, want: `{"count":"42"}`},
		{name: "float as string", flags: []string{"version=3.10"}, want: `{"version":"3.10"}`},
		{name: "bool as string", flags: []string{"enabled=true"}, want: `{"enabled":"true"}`},
		{name: "leading zero kept", flags: []string{"count=08"}, want: `{"count":"08"}`},
		{name: "exponential kept", flags: []string{"n=1e5"}, want: `{"n":"1e5"}`},
		{name: "empty string", flags: []string{"x="}, want: `{"x":""}`},
		{name: "string value", flags: []string{"name=Alice"}, want: `{"name":"Alice"}`},
		{name: "nested", flags: []string{"a.b=42"}, want: `{"a":{"b":"42"}}`},

		// Duplicate keys -> list of strings (same model as --set).
		{name: "two dups", flags: []string{"list=a", "list=08"}, want: `{"list":["a","08"]}`},

		// Errors
		{name: "no equals", flags: []string{"bad"}, wantErr: true},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseSetStringFlags(tc.flags)
			if tc.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			gotJSON, _ := json.Marshal(got)
			if string(gotJSON) != tc.want {
				t.Errorf("got  %s\nwant %s", gotJSON, tc.want)
			}
		})
	}
}

// TestSetStringOverridesSet verifies --set-string is applied on top of --set
// using the same key model. A key set by both follows the duplicate-key -> list
// rule (not last-wins), matching how two --set on one key behave.
func TestSetStringOverridesSet(t *testing.T) {
	// Build the --set result, then apply --set-string into it.
	values, err := parseSetFlags([]string{"port=8080"})
	if err != nil {
		t.Fatalf("parseSetFlags: %v", err)
	}
	err = applySetStringValues(values, []string{"port=8080"})
	if err != nil {
		t.Fatalf("applySetStringValues: %v", err)
	}
	got, _ := json.Marshal(values)
	// First entry is a number (from --set, YAML-resolved), second a string
	// (from --set-string, verbatim) — duplicate key becomes a list.
	want := `{"port":[8080,"8080"]}`
	if string(got) != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}

	// set-string on a fresh key forces a string even for numeric shapes.
	values2, err := parseSetFlags(nil)
	if err != nil {
		t.Fatalf("parseSetFlags empty: %v", err)
	}
	err = applySetStringValues(values2, []string{"version=3.10"})
	if err != nil {
		t.Fatalf("applySetStringValues: %v", err)
	}
	got2, _ := json.Marshal(values2)
	if string(got2) != `{"version":"3.10"}` {
		t.Errorf("set-string not preserved as string: got %s", got2)
	}
}

// TestSetValueParityWithConfigFile is the load-bearing guard for the --set
// design: a value passed via --set must resolve to the identical typed value
// it would have in an rpack.yaml file. The file path uses sigs.k8s.io/yaml
// (configloader.go), and --set now uses the same resolver, so both produce
// float64 for every number. Were --set to drift back to bespoke int/float
// coercion (e.g. int 8080 vs file float64 8080), this test fails.
func TestSetValueParityWithConfigFile(t *testing.T) {
	// Mirrors how configloader.loadRPackFile resolves a config's values.
	const yamlDoc = `"@schema_version": "v1"
config:
  values:
    count: 42
    version: 3.10
    enabled: true
    name: Alice
`
	var cfg rpack.RPackConfig
	if err := yaml.Unmarshal([]byte(yamlDoc), &cfg); err != nil {
		t.Fatalf("file-path unmarshal: %v", err)
	}
	fileValues := cfg.Config.Values

	setValues, err := parseSetFlags([]string{
		"count=42", "version=3.10", "enabled=true", "name=Alice",
	})
	if err != nil {
		t.Fatalf("parseSetFlags: %v", err)
	}

	if len(fileValues) != len(setValues) {
		t.Fatalf("key count mismatch: file=%v set=%v", fileValues, setValues)
	}
	for k, fv := range fileValues {
		sv, ok := setValues[k]
		if !ok {
			t.Errorf("key %q missing from --set result", k)
			continue
		}
		if fmt.Sprintf("%T:%v", fv, fv) != fmt.Sprintf("%T:%v", sv, sv) {
			t.Errorf("key %q diverges: file=%T(%v) set=%T(%v)", k, fv, fv, sv, sv)
		}
	}
}
