package rpack

import (
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
		local t = {
			string = "val",
			int = 123,
			strlist = {"a", "b"},
		}
		local ystr = to_yaml(t)
		local got = from_yaml(ystr)
		assert(got.string == "val")
		assert(got.int == 123)
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
		assert(arrayEqual(got.strlist, t.strlist))
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
