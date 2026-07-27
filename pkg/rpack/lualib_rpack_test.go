package rpack

import (
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

// TODO: Create test for read_dir

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

// --- chmod & mode options (ADR 0001) ----------------------------------------

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
		{"owner-unreadable", `chmod("f", "000")`, "owner-readable"},
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

func TestRPackAPIWriteWithModeOpt(t *testing.T) {
	fs := NewInMemoryFS()
	api := NewRPackAPI(fs)
	L := newLuaTestAPI(t, fs, map[string]lua.LGFunction{"write": api.luaWrite})
	script := `
		write("deploy.sh", "#!/bin/sh\n", {mode = "755"})
		write("plain.txt", "hi", {mode = "600"})
		write("noop.txt", "hi", {})
		write("default.txt", "hi")
	`
	if err := L.DoString(script); err != nil {
		t.Fatalf("Script failed: %s", err)
	}
	if got := fs.Tree["deploy.sh"].Mode; got != 0o755 {
		t.Errorf("deploy.sh mode = %o, want 755", got)
	}
	if got := fs.Tree["plain.txt"].Mode; got != 0o600 {
		t.Errorf("plain.txt mode = %o, want 600", got)
	}
	if got := fs.Tree["noop.txt"].Mode; got != 0o644 {
		t.Errorf("noop.txt mode = %o, want 644 (empty opts is a no-op)", got)
	}
	if got := fs.Tree["default.txt"].Mode; got != 0o644 {
		t.Errorf("default.txt mode = %o, want 644", got)
	}
}

func TestRPackAPICopyWithModeOpt(t *testing.T) {
	fs := NewInMemoryFS()
	_ = fs.Write("src.sh", []byte("#!/bin/sh\n"))
	api := NewRPackAPI(fs)
	L := newLuaTestAPI(t, fs, map[string]lua.LGFunction{"copy": api.luaCopy})
	if err := L.DoString(`copy("src.sh", "dst.sh", {mode = "755"})`); err != nil {
		t.Fatalf("Script failed: %s", err)
	}
	e := fs.Tree["dst.sh"]
	if string(e.Content) != "#!/bin/sh\n" {
		t.Errorf("dst.sh content = %q", e.Content)
	}
	if e.Mode != 0o755 {
		t.Errorf("dst.sh mode = %o, want 755", e.Mode)
	}
}

func TestRPackAPIOptsErrors(t *testing.T) {
	tests := []struct {
		name    string
		script  string
		wantErr string
	}{
		{"unknown key typo", `write("f", "x", {mdoe = "755"})`, `unknown option "mdoe" (known: mode)`},
		{"non-string mode", `write("f", "x", {mode = 493})`, `mode must be an octal string like "755"`},
		{"invalid mode value", `write("f", "x", {mode = "999"})`, `invalid mode "999"`},
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
		})
	}
}

func TestRPackAPIWriteAfterChmodResets(t *testing.T) {
	fs := NewInMemoryFS()
	api := NewRPackAPI(fs)
	L := newLuaTestAPI(t, fs, map[string]lua.LGFunction{
		"write": api.luaWrite,
		"chmod": api.luaChmod,
	})
	script := `
		write("f.sh", "v1")
		chmod("f.sh", "755")
		write("f.sh", "v2")
	`
	if err := L.DoString(script); err != nil {
		t.Fatalf("Script failed: %s", err)
	}
	if got := fs.Tree["f.sh"].Mode; got != 0o644 {
		t.Fatalf("mode = %o, want 644 (a write resets the mode)", got)
	}
}
