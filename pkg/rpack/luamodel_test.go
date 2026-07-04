// luamodel_test.go
package rpack

import (
	"encoding/json"
	"reflect"
	"testing"

	lua "github.com/yuin/gopher-lua"

	"sigs.k8s.io/yaml"
)

func TestLuaReadLines(t *testing.T) {
	contentWithNL := "alpha\nbeta\ngamma\n"
	contentWithoutNL := "one\ntwo\nthree"
	fs := NewInMemoryFS()
	err := fs.Write("friendlyWithNL", []byte(contentWithNL))
	if err != nil {
		t.Fatalf("Could not write to fs: %s", err)
	}

	err = fs.Write("friendlyWithoutNL", []byte(contentWithoutNL))
	if err != nil {
		t.Fatalf("Could not write to fs: %s", err)
	}
	script := `
        local rpack = require("rpack.v1")
        local res = rpack.read_lines("friendlyWithNL")
		local resStr = rpack.to_json(res)
		rpack.write("friendlyJsonOut", resStr)
    `
	err = ExecuteLuaWithData(t.Context(), script, fs, nil)
	if err != nil {
		t.Fatalf("ExecuteLua (read_lines with NL) error: %s", err)
	}
	outBytes, err := fs.Read("friendlyJsonOut")
	if err != nil {
		t.Fatalf("failed to read JSON output: %s", err)
	}
	var result map[string]any
	if err := json.Unmarshal(outBytes, &result); err != nil {
		t.Fatalf("failed to unmarshal JSON output: %s", err)
	}
	lines, ok := result["lines"].([]any)
	if !ok || len(lines) != 3 {
		t.Errorf("expected 3 lines, got %v", result["lines"])
	}
	if sep, ok := result["separator"].(string); !ok || sep != "\n" {
		t.Errorf("expected separator '\\n', got %v", result["separator"])
	}
	if final, ok := result["finalNewline"].(bool); !ok || final != true {
		t.Errorf("expected finalNewline true, got %v", result["finalNewline"])
	}

	script = `
        local rpack = require("rpack.v1")
        local res = rpack.read_lines("friendlyWithoutNL")
        rpack.write("friendlyJsonOutWithoutNL", rpack.to_json(res))
    `
	err = ExecuteLuaWithData(t.Context(), script, fs, nil)
	if err != nil {
		t.Fatalf("ExecuteLua (read_lines without NL) error: %s", err)
	}
	outBytes, err = fs.Read("friendlyJsonOutWithoutNL")
	if err != nil {
		t.Fatalf("failed to read JSON output (no NL): %s", err)
	}
	if err := json.Unmarshal(outBytes, &result); err != nil {
		t.Fatalf("failed to unmarshal JSON output (no NL): %s", err)
	}
	lines, ok = result["lines"].([]any)
	if !ok || len(lines) != 3 {
		t.Errorf("expected 3 lines, got %v", result["lines"])
	}
	if sep, ok := result["separator"].(string); !ok || sep != "\n" {
		t.Errorf("expected separator '\\n', got %v", result["separator"])
	}
	if final, ok := result["finalNewline"].(bool); !ok || final != false {
		t.Errorf("expected finalNewline false, got %v", result["finalNewline"])
	}
}

func TestLuaWriteLines(t *testing.T) {
	fs := NewInMemoryFS()
	script := `
        local rpack = require("rpack.v1")
        local lines = { "first line", "second line", "third line" }
        rpack.write_lines("friendlyWrite1", lines, "\n", false)
    `
	err := ExecuteLuaWithData(t.Context(), script, fs, nil)
	if err != nil {
		t.Fatalf("ExecuteLua (write_lines) error: %s", err)
	}
	bytes, err := fs.Read("friendlyWrite1")
	if err != nil {
		t.Fatalf("failed to read file written by write_lines: %s", err)
	}
	expected := "first line\nsecond line\nthird line"
	if string(bytes) != expected {
		t.Errorf("expected file content %q, got %q", expected, string(bytes))
	}

	script = `
        local rpack = require("rpack.v1")
        local lines = { "alpha", "beta", "gamma" }
        rpack.write_lines("friendlyWrite2", lines)
    `
	err = ExecuteLuaWithData(t.Context(), script, fs, nil)
	if err != nil {
		t.Fatalf("ExecuteLua (write_lines default final NL) error: %s", err)
	}
	bytes, err = fs.Read("friendlyWrite2")
	if err != nil {
		t.Fatalf("failed to read file written by write_lines (default final NL): %s", err)
	}
	expected = "alpha\nbeta\ngamma\n"
	if string(bytes) != expected {
		t.Errorf("expected file content %q, got %q", expected, string(bytes))
	}
}

// TestLuaExternalData verifies that external data injected via NewLuaModel appear top-level
// in the rpack module as functions. The keys provided in the initialData map are exposed and, when called,
// return the corresponding value.
func TestLuaExternalData(t *testing.T) {

	// Prepare external data to be injected.
	externalData := map[string]any{
		// For instance, "config" can be any complex Go object.
		"config": map[string]any{
			"user":  "alice",
			"theme": "dark",
			"nested": map[string]any{
				"level": 1,
			},
		},
		// "values" can be a list or any other type.
		"values": []any{1, 2, 3, 4, 5},
	}

	// Lua script to read the external data from the module.
	script := `
        local rpack = require("rpack.v1")
        local result = {
            config = rpack.config(),
            values = rpack.values()
        }
        rpack.write("friendlyJsonOut", rpack.to_json(result))
    `

	fs := NewInMemoryFS()
	// Execute the script with external data
	err := ExecuteLuaWithData(t.Context(), script, fs, externalData)
	if err != nil {
		t.Fatalf("ExecuteLuaWithData error: %s", err)
	}

	// Read and unmarshal the JSON output.
	outBytes, err := fs.Read("friendlyJsonOut")
	if err != nil {
		t.Fatalf("failed to read JSON output for external data: %s", err)
	}

	var result map[string]any
	if err := json.Unmarshal(outBytes, &result); err != nil {
		t.Fatalf("failed to unmarshal JSON output for external data: %s", err)
	}

	// Verify that the returned values match those provided.
	configExpectedBytes, err := yaml.Marshal(externalData["config"])
	if err != nil {
		t.Fatalf("failed to marshal expected config: %s", err)
	}
	configGotBytes, err := yaml.Marshal(result["config"])
	if err != nil {
		t.Fatalf("failed to marshal returned config: %s", err)
	}
	if string(configExpectedBytes) != string(configGotBytes) {
		t.Errorf("unexpected config value, got %s, want %s", string(configGotBytes), string(configExpectedBytes))
	}
	// JSON unmarshalling may convert numbers to float64.
	// Use YAML marshal/unmarshal to compare the structures in a forgiving way.
	wantBytes, err := yaml.Marshal(externalData["values"])
	if err != nil {
		t.Fatalf("failed to marshal external values: %s", err)
	}
	gotBytes, err := yaml.Marshal(result["values"])
	if err != nil {
		t.Fatalf("failed to marshal returned values: %s", err)
	}
	if string(wantBytes) != string(gotBytes) {
		t.Errorf("unexpected values, got %s, want %s", string(gotBytes), string(wantBytes))
	}
}

func TestLuaSandbox(t *testing.T) {
	fs := NewInMemoryFS()
	script := `
		print("test from slog")
    `
	err := ExecuteLuaWithData(t.Context(), script, fs, nil)
	if err != nil {
		t.Fatalf("ExecuteLua error: %s", err)
	}
}

func TestLValueToGo(t *testing.T) {
	tests := []struct {
		name string
		in   lua.LValue
		want any
	}{
		// Strings stay strings: a previous strconv.Atoi guess retyped numeric
		// strings into ints, corrupting zero-padded IDs and path-like strings.
		{name: "numeric string 007 stays string", in: lua.LString("007"), want: "007"},
		{name: "plain string stays string", in: lua.LString("hello"), want: "hello"},
		{name: "leading-zero port stays string", in: lua.LString("08080"), want: "08080"},
		// Whole Lua numbers become int (author intent for IDs, ports, counts);
		// fractional numbers stay float64.
		{name: "whole number becomes int64", in: lua.LNumber(7), want: int64(7)},
		{name: "port becomes int64", in: lua.LNumber(8080), want: int64(8080)},
		{name: "zero becomes int64", in: lua.LNumber(0), want: int64(0)},
		{name: "fractional stays float64", in: lua.LNumber(7.5), want: float64(7.5)},
		{name: "negative whole becomes int64", in: lua.LNumber(-5), want: int64(-5)},
		// Bools and nil pass through.
		{name: "bool true", in: lua.LBool(true), want: true},
		{name: "bool false", in: lua.LBool(false), want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := lValueToGo(tt.in)
			if reflect.TypeOf(got) != reflect.TypeOf(tt.want) {
				t.Fatalf("type mismatch: want %T, got %T (%v)", tt.want, got, got)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("value mismatch: want %#v, got %#v", tt.want, got)
			}
		})
	}
}

// TestLValueToGo_WholeNumberJSON verifies whole Lua numbers marshal as bare
// integers (no fractional noise) in JSON after conversion.
func TestLValueToGo_WholeNumberJSON(t *testing.T) {
	v := lValueToGo(lua.LNumber(42))
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %s", err)
	}
	if string(b) != "42" {
		t.Fatalf("expected \"42\", got %q", string(b))
	}
}

func TestLuaTableToGo(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	mkArr := func() *lua.LTable {
		tbl := L.NewTable()
		tbl.RawSetInt(1, lua.LString("a"))
		tbl.RawSetInt(2, lua.LString("b"))
		return tbl
	}
	mkMixed := func() *lua.LTable {
		tbl := L.NewTable()
		tbl.RawSetInt(1, lua.LString("a"))
		tbl.RawSetInt(2, lua.LString("b"))
		tbl.RawSetString("name", lua.LString("x"))
		return tbl
	}
	mkMap := func() *lua.LTable {
		tbl := L.NewTable()
		tbl.RawSetString("name", lua.LString("x"))
		return tbl
	}

	tests := []struct {
		name    string
		in      *lua.LTable
		want    any
		wantErr bool
	}{
		{
			name: "array table becomes slice",
			in:   mkArr(),
			want: []any{"a", "b"},
		},
		{
			name: "mixed table merges numeric keys as strings",
			in:   mkMixed(),
			want: map[string]any{"1": "a", "2": "b", "name": "x"},
		},
		{
			name: "string-keyed table becomes map",
			in:   mkMap(),
			want: map[string]any{"name": "x"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := luaTableToGo(tt.in)
			if tt.wantErr {
				if got != nil {
					t.Fatalf("expected nil/err, got %#v", got)
				}
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("mismatch:\nwant %#v\ngot  %#v", tt.want, got)
			}
		})
	}
}
