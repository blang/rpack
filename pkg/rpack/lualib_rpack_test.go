package rpack

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	lua "github.com/yuin/gopher-lua"
)

func TestRPackAPIFromJSON(t *testing.T) {
	L := lua.NewState(lua.Options{SkipOpenLibs: false})
	defer L.Close()
	L.SetContext(t.Context())
	L.SetGlobal("fn", L.NewFunction(luaFromJSON))
	script := `
		t = fn([[
		{
			"string": "val",
			"int": 123,
			"strlist": ["a", "b"]
		}
		]])
		assert(t.string == "val")
		assert(t.int == 123)
		local function arrayEqual(a1, a2)
			-- Check length, or else the loop isn't valid.
			if #a1 ~= #a2 then
			  return false
			end

			-- Check each element.
			for i, v in ipairs(a1) do
			  if v ~= a2[i] then
				return false
			  end
			end

			-- We've checked everything.
			return true
		end
		local expected = {"a", "b"}
		assert(arrayEqual(t.strlist, expected))
	`
	if err := L.DoString(script); err != nil {
		t.Fatalf("Script failed: %s", err)
	}
}

func TestRPackAPIToJSON(t *testing.T) {
	L := lua.NewState(lua.Options{SkipOpenLibs: false})
	defer L.Close()
	L.SetContext(t.Context())
	L.SetGlobal("fn", L.NewFunction(luaToJSON))
	script := `
		local t = {
			string = "val",
			int = 123,
		}
		str = fn(t)
		assert(string.len(str) > 5)
		expected = [[{
  "int": 123,
  "string": "val"
}]]
		assert(expected == str)
	`
	if err := L.DoString(script); err != nil {
		t.Fatalf("Script failed: %s", err)
	}
}

func TestRPackAPIWrite(t *testing.T) {
	fs := NewInMemoryFS()
	api := NewRPackAPI(fs)
	L := lua.NewState(lua.Options{SkipOpenLibs: false})
	defer L.Close()
	L.SetContext(t.Context())
	L.SetGlobal("fn", L.NewFunction(api.luaWrite))
	script := `
		local str = "hello"
		fn("target.txt", str)
	`
	if err := L.DoString(script); err != nil {
		t.Fatalf("Script failed: %s", err)
	}

	if e, ok := fs.Tree["target.txt"]; !ok {
		t.Errorf("File not written")
	} else if string(e.Content) != "hello" {
		t.Errorf("Wrong content of file: %s", string(e.Content))
	}
}

func TestRPackAPIRead(t *testing.T) {
	fs := NewInMemoryFS()
	_ = fs.Write("target.txt", []byte("hello"))
	api := NewRPackAPI(fs)
	L := lua.NewState(lua.Options{SkipOpenLibs: false})
	defer L.Close()
	L.SetContext(t.Context())
	L.SetGlobal("fn", L.NewFunction(api.luaRead))
	script := `
		local str = fn("target.txt")
		assert(str == "hello")
	`
	if err := L.DoString(script); err != nil {
		t.Fatalf("Script failed: %s", err)
	}
}

func TestRPackAPIToAndFromYAML(t *testing.T) {
	L := lua.NewState(lua.Options{SkipOpenLibs: false})
	defer L.Close()
	L.SetContext(t.Context())
	L.SetGlobal("from_yaml", L.NewFunction(luaFromYAML))
	L.SetGlobal("to_yaml", L.NewFunction(luaToYAML))
	script := `
		local function arrayEqual(a1, a2)
			if #a1 ~= #a2 then
				return false
			end
			for i, v in ipairs(a1) do
				if v ~= a2[i] then
					return false
				end
			end
			return true
		end

		-- to_yaml must produce YAML, not indented JSON. A JSON object/array
		-- literal starts with '{' or '['; YAML maps/sequences do not. This
		-- assertion would have failed against the old json.MarshalIndent impl.
		local cases = {
			{ name = "simple map", input = { string = "val", int = 123, strlist = {"a", "b"} } },
			{ name = "nested",     input = { outer = { inner = "deep", n = 7 } } },
			{ name = "array root", input = { items = {"x", "y", "z"} } },
			{ name = "bool/null",  input = { flag = true, empty = nil } },
		}
		for _, c in ipairs(cases) do
			local ystr = to_yaml(c.input)
			assert(type(ystr) == "string", c.name .. ": to_yaml returned non-string")
			assert(ystr:sub(1, 1) ~= "{", c.name .. ": to_yaml produced JSON object literal")
			assert(ystr:sub(1, 1) ~= "[", c.name .. ": to_yaml produced JSON array literal")
			local got = from_yaml(ystr)
			assert(got.string == c.input.string or got.string == nil, c.name .. ": string round-trip")
			if c.input.int ~= nil then
				assert(got.int == c.input.int, c.name .. ": int round-trip (got " .. tostring(got.int) .. ")")
			end
			if c.input.strlist ~= nil then
				assert(arrayEqual(got.strlist, c.input.strlist), c.name .. ": strlist round-trip")
			end
			if c.input.outer ~= nil then
				assert(got.outer.inner == c.input.outer.inner, c.name .. ": nested inner round-trip")
			end
			if c.input.items ~= nil then
				assert(arrayEqual(got.items, c.input.items), c.name .. ": items round-trip")
			end
			if c.input.flag ~= nil then
				assert(got.flag == c.input.flag, c.name .. ": bool round-trip")
			end
		end
	`
	if err := L.DoString(script); err != nil {
		t.Fatalf("Script failed: %s", err)
	}
}

func TestRPackAPIReadDirDefaultsToNonRecursive(t *testing.T) {
	defDir := t.TempDir()
	if err := os.Mkdir(filepath.Join(defDir, "dir"), 0o750); err != nil { //nolint:gosec // test fixture
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(defDir, "dir", "file.txt"), []byte("x"), 0o600); err != nil { //nolint:gosec // test fixture
		t.Fatal(err)
	}
	fs := NewRPackFS(true, defDir, t.TempDir(), t.TempDir(), "", nil)
	err := ExecuteLuaWithData(t.Context(), `
local rpack = require("rpack.v1")
local files, dirs = rpack.read_dir("rpack:dir")
assert(type(files) == "table")
assert(type(dirs) == "table")
`, fs, nil)
	if err != nil {
		t.Fatalf("read_dir without recursive argument: %v", err)
	}
}

func TestRPackTemplate(t *testing.T) {
	L := lua.NewState(lua.Options{SkipOpenLibs: false})
	defer L.Close()
	L.SetContext(t.Context())
	L.SetGlobal("fn", L.NewFunction(luaTemplate))
	script := `
		tmpl = "{{.value}}"
		data = {
			value="hello"
		}
		local str = fn(tmpl, data)
		assert(str == "hello")
	`
	if err := L.DoString(script); err != nil {
		t.Fatalf("Script failed: %s", err)
	}
}

func TestRPackTemplateDelim(t *testing.T) {
	L := lua.NewState(lua.Options{SkipOpenLibs: false})
	defer L.Close()
	L.SetContext(t.Context())
	L.SetGlobal("fn", L.NewFunction(luaTemplate))
	script := `
		tmpl = "<<.value>>"
		data = {
			value="hello"
		}
		local str = fn(tmpl, data, "<<", ">>")
		assert(str == "hello")
	`
	if err := L.DoString(script); err != nil {
		t.Fatalf("Script failed: %s", err)
	}
}

func TestRPackJQ(t *testing.T) {
	L := lua.NewState(lua.Options{SkipOpenLibs: false})
	defer L.Close()
	L.SetContext(t.Context())
	L.SetGlobal("fn", L.NewFunction(luaJQ))
	script := `
		local data = {users={"alice","bob"}}
		local query = ".users[1]"
        local result = fn(query, data)
		assert(result[1] == "bob")
	`
	if err := L.DoString(script); err != nil {
		t.Fatalf("Script failed: %s", err)
	}
}

func TestRPackAPICopy(t *testing.T) {
	fs := NewInMemoryFS()
	_ = fs.Write("source.txt", []byte("hello"))
	api := NewRPackAPI(fs)
	L := lua.NewState(lua.Options{SkipOpenLibs: false})
	defer L.Close()
	L.SetContext(t.Context())
	L.SetGlobal("fn", L.NewFunction(api.luaCopy))
	script := `
		fn("source.txt", "target.txt")
	`
	if err := L.DoString(script); err != nil {
		t.Fatalf("Script failed: %s", err)
	}

	if e, ok := fs.Tree["target.txt"]; !ok {
		t.Errorf("File not written")
	} else if string(e.Content) != "hello" {
		t.Errorf("Wrong content of file: %s", string(e.Content))
	}
}

// --- chmod & permission options (issue #15) ----------------------------------

// newLuaTestAPI wires an RPackAPI backed by an InMemoryFS into a Lua state
// with the given global function names bound.
func newLuaTestAPI(t *testing.T, fs *InMemoryFS, binds map[string]lua.LGFunction) *lua.LState {
	t.Helper()
	L := lua.NewState(lua.Options{SkipOpenLibs: false})
	t.Cleanup(L.Close)
	L.SetContext(t.Context())
	for name, fn := range binds {
		L.SetGlobal(name, L.NewFunction(fn))
	}
	return L
}

func TestRPackAPIChmod(t *testing.T) {
	fs := NewInMemoryFS()
	api := NewRPackAPI(fs)
	L := newLuaTestAPI(t, fs, map[string]lua.LGFunction{
		"write": api.luaWrite,
		"chmod": api.luaChmod,
	})
	script := `
		write("deploy.sh", "#!/bin/sh\n")
		chmod("deploy.sh", "755")
	`
	if err := L.DoString(script); err != nil {
		t.Fatalf("Script failed: %s", err)
	}
	if fs.Tree["deploy.sh"].Mode != 0o755 {
		t.Fatalf("mode = %o, want 755", fs.Tree["deploy.sh"].Mode)
	}
}

func TestRPackAPIChmodErrors(t *testing.T) {
	tests := []struct {
		name    string
		script  string
		wantErr string
	}{
		{"missing file", `chmod("missing.sh", "755")`, "does not exist (write it first)"},
		{"non-string mode", `chmod("f", 493)`, `mode must be an octal string like "755"`},
		{"invalid octal", `chmod("f", "88x")`, `invalid mode "88x"`},
		{"special bits", `chmod("f", "1755")`, "special mode bits"},
		{"exact mode removed", `chmod("f", "600")`, `unsupported exact mode "600"`},
		{"owner-unreadable now unsupported", `chmod("f", "000")`, `unsupported exact mode "000"`},
		{"world-writable unsupported", `chmod("f", "777")`, `unsupported exact mode "777"`},
		{"directory", `chmod("adir", "755")`, "directories is not supported"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fs := NewInMemoryFS()
			_ = fs.Write("f", []byte("x"))
			fs.Mkdir("adir")
			L := newLuaTestAPI(t, fs, map[string]lua.LGFunction{"chmod": NewRPackAPI(fs).luaChmod})
			err := L.DoString(tc.script)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("want error containing %q, got %v", tc.wantErr, err)
			}
		})
	}
}

// TestRPackAPIWriteWithModeOpt pins the issue #15 option surface: mode is a
// canonical alias, executable is a strict boolean, and both resolve to the
// same two canonical modes. The default (no options or empty table) is the
// non-executable mode established by the write's own reset.
func TestRPackAPIWriteWithModeOpt(t *testing.T) {
	fs := NewInMemoryFS()
	api := NewRPackAPI(fs)
	L := newLuaTestAPI(t, fs, map[string]lua.LGFunction{"write": api.luaWrite})
	script := `
		write("deploy.sh", "#!/bin/sh\n", {mode = "755"})
		write("alias.sh", "#!/bin/sh\n", {mode = "0755"})
		write("exec.sh", "#!/bin/sh\n", {executable = true})
		write("plain.txt", "hi", {mode = "644"})
		write("zero.txt", "hi", {mode = "0644"})
		write("noexec.sh", "#!/bin/sh\n", {executable = false})
		write("noop.txt", "hi", {})
		write("default.txt", "hi")
	`
	if err := L.DoString(script); err != nil {
		t.Fatalf("Script failed: %s", err)
	}
	executable := []string{"deploy.sh", "alias.sh", "exec.sh"}
	for _, name := range executable {
		if got := fs.Tree[name].Mode; got != ExecutableMode {
			t.Errorf("%s mode = %o, want 755", name, got)
		}
	}
	nonExecutable := []string{"plain.txt", "zero.txt", "noexec.sh", "noop.txt", "default.txt"}
	for _, name := range nonExecutable {
		if got := fs.Tree[name].Mode; got != NonExecutableMode {
			t.Errorf("%s mode = %o, want 644", name, got)
		}
	}
}

// TestRPackAPICopyWithModeOpt pins copy's option surface and the no-inheritance
// rule: without options a copy lands at the canonical default, never at the
// source's mode.
func TestRPackAPICopyWithModeOpt(t *testing.T) {
	fs := NewInMemoryFS()
	_ = fs.Write("src.sh", []byte("#!/bin/sh\n"))
	// Make the source executable: a plain copy must not inherit this.
	if err := fs.Chmod("src.sh", ExecutableMode); err != nil {
		t.Fatal(err)
	}
	api := NewRPackAPI(fs)
	L := newLuaTestAPI(t, fs, map[string]lua.LGFunction{"copy": api.luaCopy})
	script := `
		copy("src.sh", "dst.sh", {mode = "755"})
		copy("src.sh", "alias.sh", {mode = "0755"})
		copy("src.sh", "exec.sh", {executable = true})
		copy("src.sh", "plain.sh", {executable = false})
		copy("src.sh", "noexec.txt", {mode = "644"})
		copy("src.sh", "default.sh")
	`
	if err := L.DoString(script); err != nil {
		t.Fatalf("Script failed: %s", err)
	}
	for _, name := range []string{"dst.sh", "alias.sh", "exec.sh"} {
		e := fs.Tree[name]
		if string(e.Content) != "#!/bin/sh\n" {
			t.Errorf("%s content = %q", name, e.Content)
		}
		if e.Mode != ExecutableMode {
			t.Errorf("%s mode = %o, want 755", name, e.Mode)
		}
	}
	for _, name := range []string{"plain.sh", "noexec.txt", "default.sh"} {
		if got := fs.Tree[name].Mode; got != NonExecutableMode {
			t.Errorf("%s mode = %o, want 644 (no source-mode inheritance)", name, got)
		}
	}
}

// TestRPackAPIOptsErrors pins option validation: unknown keys, wrong types,
// removed exact modes, and mode/executable mutual exclusion — all rejected
// before any file is written.
func TestRPackAPIOptsErrors(t *testing.T) {
	tests := []struct {
		name    string
		script  string
		wantErr string
	}{
		{"unknown key typo", `write("f", "x", {mdoe = "755"})`, `unknown option "mdoe" (known: executable, mode)`},
		{"unknown key executable typo", `write("f", "x", {executabel = true})`, `unknown option "executabel" (known: executable, mode)`},
		{"non-string mode", `write("f", "x", {mode = 493})`, `mode must be an octal string like "755"`},
		{"invalid mode value", `write("f", "x", {mode = "999"})`, `invalid mode "999"`},
		{"exact mode removed", `write("f", "x", {mode = "600"})`, `unsupported exact mode "600"`},
		{"non-bool executable string", `write("f", "x", {executable = "true"})`, "executable must be a boolean"},
		{"non-bool executable number", `write("f", "x", {executable = 1})`, "executable must be a boolean"},
		{"mode and executable", `write("f", "x", {mode = "755", executable = true})`, "mutually exclusive"},
		{"mode and executable agreeing", `write("f", "x", {mode = "644", executable = false})`, "mutually exclusive"},
		{"string instead of table", `write("f", "x", "755")`, "table expected"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fs := NewInMemoryFS()
			api := NewRPackAPI(fs)
			L := newLuaTestAPI(t, fs, map[string]lua.LGFunction{"write": api.luaWrite})
			err := L.DoString(tc.script)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("want error containing %q, got %v", tc.wantErr, err)
			}
			// The complete options table is validated before filesystem
			// mutation: a rejected write must not stage the file.
			if _, exists := fs.Tree["f"]; exists {
				t.Fatal("write staged the file before rejecting invalid options")
			}
		})
	}
}

// TestRPackAPICopyOptsErrorBeforeMutation pins that copy validates its whole
// options table before touching the filesystem: an invalid option must
// reject the copy without staging the target.
func TestRPackAPICopyOptsErrorBeforeMutation(t *testing.T) {
	fs := NewInMemoryFS()
	if err := fs.Write("src.txt", []byte("x")); err != nil {
		t.Fatal(err)
	}
	api := NewRPackAPI(fs)
	L := newLuaTestAPI(t, fs, map[string]lua.LGFunction{"copy": api.luaCopy})
	err := L.DoString(`copy("src.txt", "dst.txt", {mode = "600"})`)
	if err == nil || !strings.Contains(err.Error(), `unsupported exact mode "600"`) {
		t.Fatalf("want unsupported exact mode error, got %v", err)
	}
	if _, exists := fs.Tree["dst.txt"]; exists {
		t.Fatal("copy staged the target before rejecting invalid options")
	}
}

func TestDefinitionContractV1RejectsExtraLuaArguments(t *testing.T) {
	tests := []struct {
		name    string
		call    string
		wantErr string
	}{
		{name: "copy", call: `rpack.copy("source", "copy", {}, "extra")`, wantErr: "expected 2 to 3 arguments, got 4"},
		{name: "chmod", call: `rpack.chmod("source", "755", "extra")`, wantErr: "expected 2 arguments, got 3"},
		{name: "from_json", call: `rpack.from_json("{}", "extra")`, wantErr: "expected 1 argument, got 2"},
		{name: "to_json", call: `rpack.to_json({}, "extra")`, wantErr: "expected 1 argument, got 2"},
		{name: "from_yaml", call: `rpack.from_yaml("a: b", "extra")`, wantErr: "expected 1 argument, got 2"},
		{name: "to_yaml", call: `rpack.to_yaml({}, "extra")`, wantErr: "expected 1 argument, got 2"},
		{name: "write missing content", call: `rpack.write("output")`, wantErr: "expected 2 to 3 arguments, got 1"},
		{name: "write extra", call: `rpack.write("output", "x", {}, "extra")`, wantErr: "expected 2 to 3 arguments, got 4"},
		{name: "read", call: `rpack.read("source", "extra")`, wantErr: "expected 1 argument, got 2"},
		{name: "read_dir", call: `rpack.read_dir("dir", false, "extra")`, wantErr: "expected 1 to 2 arguments, got 3"},
		{name: "template", call: `rpack.template("{{.x}}", {x = "x"}, "{{", "}}", "extra")`, wantErr: "expected 2 to 4 arguments, got 5"},
		{name: "jq", call: `rpack.jq(".", {}, "extra")`, wantErr: "expected 2 arguments, got 3"},
		{name: "read_lines", call: `rpack.read_lines("source", "extra")`, wantErr: "expected 1 argument, got 2"},
		{name: "write_lines", call: `rpack.write_lines("output", {"x"}, "\n", true, "extra")`, wantErr: "expected 2 to 4 arguments, got 5"},
		{name: "injected values", call: `rpack.values("extra")`, wantErr: "expected 0 arguments, got 1"},
		{name: "injected inputs", call: `rpack.inputs("extra")`, wantErr: "expected 0 arguments, got 1"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fs := NewInMemoryFS()
			if err := fs.Write("source", []byte("x")); err != nil {
				t.Fatal(err)
			}
			fs.Mkdir("dir")
			script := `local rpack = require("rpack.v1"); ` + tc.call
			err := ExecuteLuaWithData(t.Context(), script, fs, map[string]any{
				"values": map[string]any{},
				"inputs": []string{},
			})
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("want error containing %q, got %v", tc.wantErr, err)
			}
			if _, exists := fs.Tree["output"]; exists {
				t.Fatal("function performed a write before rejecting extra arguments")
			}
		})
	}
}

// TestRPackAPIWriteAfterChmodResets pins the ordering rule (issue #15): a
// fresh write establishes its own intent (non-executable by default), and a
// later chmod or permission option amends it. Both spellings — mode alias
// and executable boolean — reset and amend identically.
func TestRPackAPIWriteAfterChmodResets(t *testing.T) {
	fs := NewInMemoryFS()
	api := NewRPackAPI(fs)
	L := newLuaTestAPI(t, fs, map[string]lua.LGFunction{
		"write": api.luaWrite,
		"chmod": api.luaChmod,
	})
	script := `
		-- chmod after write amends; a later plain write resets.
		write("a.sh", "v1")
		chmod("a.sh", "755")
		write("a.sh", "v2")

		-- executable option after write amends; a later plain write resets.
		write("b.sh", "v1", {executable = true})
		write("b.sh", "v2")

		-- mode alias behaves the same.
		write("c.sh", "v1", {mode = "0755"})
		write("c.sh", "v2")

		-- a reset write can be re-amended by a new option.
		write("d.sh", "v1", {executable = true})
		write("d.sh", "v2")
		write("d.sh", "v3", {executable = true})
	`
	if err := L.DoString(script); err != nil {
		t.Fatalf("Script failed: %s", err)
	}
	for _, name := range []string{"a.sh", "b.sh", "c.sh"} {
		if got := fs.Tree[name].Mode; got != NonExecutableMode {
			t.Errorf("%s mode = %o, want 644 (a write resets the mode)", name, got)
		}
	}
	if got := fs.Tree["d.sh"].Mode; got != ExecutableMode {
		t.Errorf("d.sh mode = %o, want 755 (option after reset amends)", got)
	}
}
