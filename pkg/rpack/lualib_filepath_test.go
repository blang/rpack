package rpack

import (
	"testing"

	lua "github.com/yuin/gopher-lua"
)

// func registerFunction(L lua.LState, fn lua.LGFunction) error {
// 	L.SetGlobal("print", L.NewFunction(luaPrint))
// }

func TestFilepathBase(t *testing.T) {
	L := lua.NewState(lua.Options{SkipOpenLibs: false})
	defer L.Close()
	L.SetContext(t.Context())
	L.SetGlobal("fn", L.NewFunction(luaFilepathBase))
	script := `
		assert(fn("/foo/bar/baz.js") == "baz.js")
		assert(fn("/") == "/")
	`
	if err := L.DoString(script); err != nil {
		t.Fatalf("Script failed: %s", err)
	}
}

func TestFilepathClean(t *testing.T) {
	L := lua.NewState(lua.Options{SkipOpenLibs: false})
	defer L.Close()
	L.SetContext(t.Context())
	L.SetGlobal("fn", L.NewFunction(luaFilepathClean))
	script := `
		assert(fn("//baz.js") == "/baz.js")
		assert(fn("/././baz.js") == "/baz.js")
		assert(fn(".///baz.js") == "baz.js")
		assert(fn("baz.js") == "baz.js")
	`
	if err := L.DoString(script); err != nil {
		t.Fatalf("Script failed: %s", err)
	}
}

func TestFilepathDir(t *testing.T) {
	L := lua.NewState(lua.Options{SkipOpenLibs: false})
	defer L.Close()
	L.SetContext(t.Context())
	L.SetGlobal("fn", L.NewFunction(luaFilepathDir))
	script := `
		assert(fn("/foo/bar/baz.js") == "/foo/bar")
		assert(fn("/foo/bar") == "/foo")
		assert(fn(".") == ".")
	`
	if err := L.DoString(script); err != nil {
		t.Fatalf("Script failed: %s", err)
	}
}

func TestFilepathExt(t *testing.T) {
	L := lua.NewState(lua.Options{SkipOpenLibs: false})
	defer L.Close()
	L.SetContext(t.Context())
	L.SetGlobal("fn", L.NewFunction(luaFilepathExt))
	script := `
		assert(fn("/foo/bar/baz") == "")
		assert(fn("/foo/bar/baz.js") == ".js")
		assert(fn("baz.test.js") == ".js")
	`
	if err := L.DoString(script); err != nil {
		t.Fatalf("Script failed: %s", err)
	}
}

func TestFilepathIsAbs(t *testing.T) {
	L := lua.NewState(lua.Options{SkipOpenLibs: false})
	defer L.Close()
	L.SetContext(t.Context())
	L.SetGlobal("fn", L.NewFunction(luaFilepathIsAbs))
	script := `
		assert(fn("/foo/bar/baz") == true)
		assert(fn("./foo/bar/baz.js") == false)
		assert(fn(".") == false)
		assert(fn("/") == true)
	`
	if err := L.DoString(script); err != nil {
		t.Fatalf("Script failed: %s", err)
	}
}

func TestFilepathIsLocal(t *testing.T) {
	L := lua.NewState(lua.Options{SkipOpenLibs: false})
	defer L.Close()
	L.SetContext(t.Context())
	L.SetGlobal("fn", L.NewFunction(luaFilepathIsLocal))
	script := `
		assert(fn("./foo/bar/baz") == true)
		assert(fn("../") == false)
		assert(fn("./../bar") == false)
		assert(fn("./foo/../bar") == true)
	`
	if err := L.DoString(script); err != nil {
		t.Fatalf("Script failed: %s", err)
	}
}

func TestFilepathJoin(t *testing.T) {
	L := lua.NewState(lua.Options{SkipOpenLibs: false})
	defer L.Close()
	L.SetContext(t.Context())
	L.SetGlobal("fn", L.NewFunction(luaFilepathJoin))
	script := `
		assert(fn("./foo/bar", "baz.js") == "foo/bar/baz.js")
		assert(fn("foo/bar","baz.js") == "foo/bar/baz.js")
		assert(fn("foo", "bar", "baz.js") == "foo/bar/baz.js")
		assert(fn("foo", "bar", "baz",  "baz.js") == "foo/bar/baz/baz.js")
		assert(pcall(function() fn("foo") end) == false) -- error
	`
	if err := L.DoString(script); err != nil {
		t.Fatalf("Script failed: %s", err)
	}
}

func TestFilepathSplit(t *testing.T) {
	L := lua.NewState(lua.Options{SkipOpenLibs: false})
	defer L.Close()
	L.SetContext(t.Context())
	L.SetGlobal("fn", L.NewFunction(luaFilepathSplit))
	script := `
		dir, file = fn("/home/arnie/amelia.jpg")
		assert(dir == "/home/arnie/" and file == "amelia.jpg")

		dir, file = fn("amelia.jpg")
		assert(dir == "" and file == "amelia.jpg")
	`
	if err := L.DoString(script); err != nil {
		t.Fatalf("Script failed: %s", err)
	}
}

func TestFilepathLocation(t *testing.T) {
	L := lua.NewState(lua.Options{SkipOpenLibs: false})
	defer L.Close()
	L.SetContext(t.Context())
	L.SetGlobal("fn", L.NewFunction(luaFilepathLocation))
	script := `
		location, restpath = fn("rpack:arnie/amelia.jpg")
		assert(location == "rpack" and restpath == "arnie/amelia.jpg")

		location, restpath = fn("arnie/amelia.jpg")
		assert(location == "target" and restpath == "arnie/amelia.jpg")
	`
	if err := L.DoString(script); err != nil {
		t.Fatalf("Script failed: %s", err)
	}
}
