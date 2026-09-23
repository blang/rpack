package rpack

import (
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"

	lua "github.com/yuin/gopher-lua"
)

func writeDefinitionFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), RPackDefDefaultFilename)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil { //nolint:gosec // test fixture
		t.Fatal(err)
	}
	return path
}

func TestLoadRPackDef_RejectsUnsupportedContractFromHeader(t *testing.T) {
	// unknown_future_field proves version selection happens from the small raw
	// header before strict full-document decoding. The runtime must report the
	// unknown contract, not try to interpret it using a supported shape.
	path := writeDefinitionFile(t, `
"@schema_version": "v999"
name: "future"
unknown_future_field: true
`)

	_, err := LoadRPackDef(path)
	if err == nil {
		t.Fatal("expected unsupported-contract error")
	}
	if !strings.Contains(err.Error(), `unsupported rpack definition contract "v999"`) {
		t.Fatalf("expected explicit unsupported-contract error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "supported: v1") {
		t.Fatalf("expected supported-version list, got: %v", err)
	}
	if strings.Contains(err.Error(), "unknown_future_field") {
		t.Fatalf("future document was decoded as v1 before version rejection: %v", err)
	}
}

func TestLoadRPackDef_RequiresContractVersion(t *testing.T) {
	path := writeDefinitionFile(t, `
name: "missing-version"
`)

	_, err := LoadRPackDef(path)
	if err == nil {
		t.Fatal("expected missing-contract error")
	}
	if !strings.Contains(err.Error(), "definition contract version is required") {
		t.Fatalf("expected required-version error, got: %v", err)
	}
}

func TestLoadRPackDef_RejectsUnknownFieldInKnownContract(t *testing.T) {
	path := writeDefinitionFile(t, `
"@schema_version": "v1"
name: "strict"
mdoe: "755"
`)

	_, err := LoadRPackDef(path)
	if err == nil {
		t.Fatal("expected strict-decode error")
	}
	if !strings.Contains(err.Error(), `unknown field "mdoe"`) {
		t.Fatalf("expected unknown-field error, got: %v", err)
	}
}

func TestLoadRPackDef_RejectsDuplicateField(t *testing.T) {
	path := writeDefinitionFile(t, `
"@schema_version": "v1"
name: "first"
name: "second"
`)

	_, err := LoadRPackDef(path)
	if err == nil {
		t.Fatal("expected duplicate-field error")
	}
	if !strings.Contains(err.Error(), "name") || !strings.Contains(err.Error(), "already set") {
		t.Fatalf("expected duplicate-name error, got: %v", err)
	}
}

func TestRPackDefValidateSchema_RejectsUnsupportedContract(t *testing.T) {
	def := &RPackDef{SchemaVersion: "v999", Name: "future"}
	if err := def.ValidateSchema(); err == nil ||
		!strings.Contains(err.Error(), `unsupported rpack definition contract "v999"`) {
		t.Fatalf("expected explicit unsupported-contract error, got: %v", err)
	}
}

func TestExecuteLuaWithDefinitionContract_UsesSelectedLuaLibraries(t *testing.T) {
	contract := &DefinitionContract{
		version:       "test-libraries",
		luaModuleName: "rpack.test-libraries",
		openLuaLibraries: func(L *lua.LState) error {
			functions := make(map[string]lua.LGFunction, len(filepathFuncs)+1)
			maps.Copy(functions, filepathFuncs)
			functions["contract_marker"] = func(L *lua.LState) int {
				checkLuaArity(L, 0, 0)
				L.Push(lua.LString("selected-library"))
				return 1
			}
			return openLibs(L, registerFilepath("filepath", functions))
		},
		luaFunctions: func(_ *LuaModel) map[string]lua.LGFunction {
			return map[string]lua.LGFunction{}
		},
	}

	err := executeLuaWithDefinitionContract(t.Context(), `
local filepath = require("filepath")
assert(filepath.contract_marker() == "selected-library")
`, NewInMemoryFS(), nil, contract)
	if err != nil {
		t.Fatalf("selected libraries should execute: %v", err)
	}
}

func TestExecuteLuaWithDefinitionContract_PreloadsOnlySelectedModule(t *testing.T) {
	contract := &DefinitionContract{
		version:          "test",
		luaModuleName:    "rpack.test",
		openLuaLibraries: definitionContractV1LuaLibraries,
		luaFunctions: func(_ *LuaModel) map[string]lua.LGFunction {
			return map[string]lua.LGFunction{
				"marker": func(L *lua.LState) int {
					L.Push(lua.LString("selected"))
					return 1
				},
			}
		},
	}

	err := executeLuaWithDefinitionContract(t.Context(), `
local rpack = require("rpack.test")
assert(rpack.marker() == "selected")
`, NewInMemoryFS(), nil, contract)
	if err != nil {
		t.Fatalf("matching module should execute: %v", err)
	}

	err = executeLuaWithDefinitionContract(t.Context(), `
require("rpack.v1")
`, NewInMemoryFS(), nil, contract)
	if err == nil {
		t.Fatal("non-selected rpack.v1 module should not be available")
	}
	if !strings.Contains(err.Error(), "package.preload['rpack.v1']") {
		t.Fatalf("expected unavailable-module error, got: %v", err)
	}
}
