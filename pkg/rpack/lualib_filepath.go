package rpack

import (
	"path/filepath"
	"strings"

	lua "github.com/yuin/gopher-lua"
)

func RegisterFilepath(name string) lua.LGFunction {
	return func(L *lua.LState) int {
		tabmod := L.RegisterModule(name, filepathFuncs)
		L.Push(tabmod)
		return 1
	}
}

var filepathFuncs = map[string]lua.LGFunction{
	"base":     luaFilepathBase,
	"clean":    luaFilepathClean,
	"dir":      luaFilepathDir,
	"ext":      luaFilepathExt,
	"isAbs":    luaFilepathIsAbs,
	"isLocal":  luaFilepathIsLocal,
	"join":     luaFilepathJoin,
	"split":    luaFilepathSplit,
	"location": luaFilepathLocation,
}

func luaFilepathBase(L *lua.LState) int {
	path := L.CheckString(1)
	base := filepath.Base(path)
	L.Push(lua.LString(base))
	return 1
}

func luaFilepathClean(L *lua.LState) int {
	path := L.CheckString(1)
	ret := filepath.Clean(path)
	L.Push(lua.LString(ret))
	return 1
}

func luaFilepathDir(L *lua.LState) int {
	path := L.CheckString(1)
	ret := filepath.Dir(path)
	L.Push(lua.LString(ret))
	return 1
}

func luaFilepathExt(L *lua.LState) int {
	path := L.CheckString(1)
	ret := filepath.Ext(path)
	L.Push(lua.LString(ret))
	return 1
}

func luaFilepathIsAbs(L *lua.LState) int {
	path := L.CheckString(1)
	ret := filepath.IsAbs(path)
	L.Push(lua.LBool(ret))
	return 1
}

func luaFilepathIsLocal(L *lua.LState) int {
	path := L.CheckString(1)
	ret := filepath.IsLocal(path)
	L.Push(lua.LBool(ret))
	return 1
}

func luaFilepathJoin(L *lua.LState) int {
	var args []string
	first := L.CheckString(1)
	second := L.CheckString(2)
	args = append(args, first, second)
	argNum := L.GetTop()
	for i := 3; i <= argNum; i++ {
		args = append(args, L.CheckString(i))
	}
	ret := filepath.Join(args...)
	L.Push(lua.LString(ret))
	return 1
}

func luaFilepathSplit(L *lua.LState) int {
	path := L.CheckString(1)
	dir, file := filepath.Split(path)
	L.Push(lua.LString(dir))
	L.Push(lua.LString(file))
	return 2
}

func luaFilepathLocation(L *lua.LState) int {
	path := L.CheckString(1)
	before, after, found := strings.Cut(path, ":")
	if found {
		L.Push(lua.LString(before))
		L.Push(lua.LString(after))
	} else {
		L.Push(lua.LString("target"))
		L.Push(lua.LString(before))
	}
	return 2
}
