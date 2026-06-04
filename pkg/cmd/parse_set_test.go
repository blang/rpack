package cmd

import (
	"encoding/json"
	"testing"
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
