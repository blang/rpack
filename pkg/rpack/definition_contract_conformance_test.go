package rpack

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefinitionContractV1Conformance(t *testing.T) {
	defDir := writeDef(t, map[string]string{
		"rpack.yaml": minimalDefYAML("v1-conformance"),
		"script.lua": `
local rpack = require("rpack.v1")
local rendered = rpack.template("hello {{.name}}", {name = "v1"})
rpack.write("contract.sh", rendered, {mode = "755"})
rpack.write("intent.sh", rendered, {executable = true})
rpack.write("plain.txt", rendered)
`,
	})
	target := t.TempDir()
	if err := runDirect(t, &Executor{}, defDir, nil, target); err != nil {
		t.Fatalf("execute v1 definition: %v", err)
	}

	outputPath := filepath.Join(target, "contract.sh")
	content, err := os.ReadFile(outputPath) //nolint:gosec // test fixture
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(content), "hello v1"; got != want {
		t.Fatalf("content = %q, want %q", got, want)
	}
	// Assert the emitted files' owner-execute classification, not exact
	// staging modes: materialization of the canonical intents is
	// environment policy (issue #15), so 0755, 0775, and 0700 are all
	// conforming executable outputs.
	assertCanonicalMode := func(name, want string) {
		t.Helper()
		info, err := os.Stat(filepath.Join(target, name))
		if err != nil {
			t.Fatal(err)
		}
		if got := CanonicalMode(info.Mode()); got != want {
			t.Fatalf("%s canonical mode = %q (perm %o), want %q", name, got, info.Mode().Perm(), want)
		}
	}
	assertCanonicalMode("contract.sh", "755")
	assertCanonicalMode("intent.sh", "755")
	assertCanonicalMode("plain.txt", "644")
}

func TestDefinitionContractV1RejectsLuaModuleMismatchBeforeRelocation(t *testing.T) {
	defDir := writeDef(t, map[string]string{
		"rpack.yaml": minimalDefYAML("module-mismatch"),
		"script.lua": `
local rpack = require("rpack.v2")
rpack.write("must-not-exist", "wrong contract")
`,
	})
	target := t.TempDir()
	err := runDirect(t, &Executor{}, defDir, nil, target)
	if err == nil || !errors.Is(err, ErrLuaExecution) {
		t.Fatalf("expected Lua module mismatch, got: %v", err)
	}
	if !strings.Contains(err.Error(), "package.preload['rpack.v2']") {
		t.Fatalf("expected unavailable-module error, got: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(target, "must-not-exist")); !os.IsNotExist(statErr) {
		t.Fatalf("mismatched contract relocated output: %v", statErr)
	}
}

func TestUnsupportedDefinitionContractFailsBeforeLua(t *testing.T) {
	defDir := writeDef(t, map[string]string{
		"rpack.yaml": "\"@schema_version\": \"v999\"\nname: \"future\"\n",
		"script.lua": `
local rpack = require("rpack.v1")
rpack.write("must-not-exist", "script should not run")
`,
	})
	target := t.TempDir()
	err := runDirect(t, &Executor{}, defDir, nil, target)
	if err == nil || !strings.Contains(err.Error(), `unsupported rpack definition contract "v999"`) {
		t.Fatalf("expected unsupported-contract error, got: %v", err)
	}
	if errors.Is(err, ErrLuaExecution) {
		t.Fatalf("unsupported contract reached Lua execution: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(target, "must-not-exist")); !os.IsNotExist(statErr) {
		t.Fatalf("unsupported contract produced output: %v", statErr)
	}
}
