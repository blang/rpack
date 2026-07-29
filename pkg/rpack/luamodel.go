package rpack

import (
	"context"
	"fmt"
	"maps"
	"strings"

	"log/slog"

	lua "github.com/yuin/gopher-lua"
)

// Resolver is used by Lua functions to resolve file paths.
// TODO: DEPRECATED: in favor of RPackFS
type Resolver interface {
	ResolveInput(name string) (*ControlledFile, error)
	ResolveOutput(name string) (*ControlledFile, error)
}

// LuaModel encapsulates the Lua state, a Resolver, and tracks execution results.
// It also holds external injected values.
type LuaModel struct {
	L          *lua.LState
	fs         FS
	definition *RPackDef
	extValues  map[string]any // External values to expose (keys come from developer)
}

// NewLuaModel creates a Lua model using definition contract v1. Definition
// execution uses newLuaModelForDefinitionContract so the definition's selected
// contract controls the available module and behavior.
func NewLuaModel(ctx context.Context, fs FS, initialData map[string]any) (*LuaModel, error) {
	contract, err := definitionContractFor(definitionContractV1)
	if err != nil {
		return nil, err
	}
	return newLuaModelForDefinitionContract(ctx, fs, initialData, contract, nil)
}

// newLuaModelForDefinitionContract creates a Lua model for one exact
// definition contract. The additional parameter initialData contains external
// values to be injected.
//
// TODO: Provide an error function to lua code
func newLuaModelForDefinitionContract(
	ctx context.Context,
	fs FS,
	initialData map[string]any,
	contract *DefinitionContract,
	definition *RPackDef,
) (*LuaModel, error) {
	if contract == nil {
		return nil, fmt.Errorf("definition contract is nil")
	}
	L := lua.NewState(lua.Options{SkipOpenLibs: true})
	L.SetContext(ctx)
	if contract.openLuaLibraries == nil {
		L.Close()
		return nil, fmt.Errorf("definition contract %q has no Lua library opener", contract.version)
	}
	if err := contract.openLuaLibraries(L); err != nil {
		L.Close()
		return nil, err
	}

	// Ensure extValues is not nil.
	if initialData == nil {
		initialData = make(map[string]any)
	}
	lm := &LuaModel{
		L:          L,
		fs:         fs,
		definition: definition,
		extValues:  initialData,
	}
	if err := lm.preloadRpackModule(contract); err != nil {
		L.Close()
		return nil, err
	}

	if err := sandbox(L); err != nil {
		L.Close()
		return nil, fmt.Errorf("could not sandbox lua state: %w", err)
	}
	return lm, nil
}

// Close cleans up the Lua state.
func (lm *LuaModel) Close() {
	if lm.L != nil {
		lm.L.Close()
	}
}

// Exec executes the given Lua script.
func (lm *LuaModel) Exec(script string) error {
	return lm.L.DoString(script)
}

// openLibs opens a standard set of Lua libraries.
func openLibs(L *lua.LState, filepathLibrary lua.LGFunction) error {
	libs := []struct {
		name string
		open lua.LGFunction
	}{
		{lua.LoadLibName, lua.OpenPackage}, // Must be first
		{lua.BaseLibName, lua.OpenBase},
		{lua.TabLibName, lua.OpenTable},
		{lua.StringLibName, lua.OpenString},
		{lua.MathLibName, lua.OpenMath},
		{lua.DebugLibName, lua.OpenDebug},
		{"filepath", filepathLibrary},
	}
	for _, lib := range libs {
		if err := L.CallByParam(lua.P{
			Fn:      L.NewFunction(lib.open),
			NRet:    0,
			Protect: true,
		}, lua.LString(lib.name)); err != nil {
			return fmt.Errorf("failed to set up %s: %w", lib.name, err)
		}
	}
	return nil
}
func luaPrint(L *lua.LState) int {
	top := L.GetTop()
	var logStrs []string
	for i := 1; i <= top; i++ {
		logStrs = append(logStrs, L.ToStringMeta(L.Get(i)).String())
	}
	slog.Info(fmt.Sprintf("Script: %s", strings.Join(logStrs, " ")))
	return 0
}

// sandbox applies sandboxing rules to the lua environment
func sandbox(L *lua.LState) error {
	L.SetGlobal("print", L.NewFunction(luaPrint))
	L.SetGlobal("loadfile", lua.LNil)
	L.SetGlobal("dofile", lua.LNil)

	// Change loaders to only allow preloaded functions and remove loading capability
	// hidden in global variables
	loaders := L.CreateTable(1, 0)
	L.RawSetInt(loaders, 1, L.NewFunction(loLoaderPreload))
	L.SetField(L.Get(lua.RegistryIndex), "_LOADERS", loaders)
	pkg := L.GetGlobal("package")
	L.SetField(pkg, "loaders", loaders)
	L.SetField(pkg, "path", lua.LString("jail"))
	L.SetField(pkg, "cpath", lua.LString("jail"))
	L.SetField(pkg, "config", lua.LString("jail"))
	return nil
}

func loLoaderPreload(L *lua.LState) int {
	name := L.CheckString(1)
	preload := L.GetField(L.GetField(L.Get(lua.EnvironIndex), "package"), "preload")
	if _, ok := preload.(*lua.LTable); !ok {
		L.RaiseError("package.preload must be a table")
	}
	lv := L.GetField(preload, name)
	if lv == lua.LNil {
		L.Push(lua.LString(fmt.Sprintf("no field package.preload['%s']", name)))
		return 1
	}
	L.Push(lv)
	return 1
}

func definitionContractV1LuaFunctions(lm *LuaModel) map[string]lua.LGFunction {
	functions := map[string]lua.LGFunction{
		"read_lines":  lm.luaReadLines,
		"write_lines": lm.luaWriteLines,
	}
	maps.Copy(functions, NewRPackAPI(lm.fs).Funcs())
	for key := range lm.extValues {
		k := key
		functions[k] = func(L *lua.LState) int {
			checkLuaArity(L, 0, 0)
			L.Push(goToLValue(L, lm.extValues[k]))
			return 1
		}
	}
	return functions
}

// preloadRpackModule preloads only the module selected by the definition
// contract. It deliberately does not expose other supported contract modules.
func (lm *LuaModel) preloadRpackModule(contract *DefinitionContract) error {
	if contract.luaModuleName == "" {
		return fmt.Errorf("definition contract %q has no Lua module", contract.version)
	}
	if contract.luaFunctions == nil {
		return fmt.Errorf("definition contract %q has no Lua functions", contract.version)
	}
	functions := contract.luaFunctions(lm)
	loader := func(L *lua.LState) int {
		mod := L.NewTable()
		// Set built-in functions.
		for name, fun := range functions {
			L.SetField(mod, name, L.NewFunction(fun))
		}
		L.Push(mod)
		return 1
	}
	lm.L.PreloadModule(contract.luaModuleName, loader)
	return nil
}

// luaReadLines reads a file returning a table with lines, separator, and finalNewline.
func (lm *LuaModel) luaReadLines(L *lua.LState) int {
	checkLuaArity(L, 1, 1)
	friendly := L.CheckString(1)
	contentBytes, err := lm.fs.Read(friendly)
	if err != nil {
		L.ArgError(1, err.Error())
		return 0
	}
	content := string(contentBytes)
	sep := "\n"
	if strings.Contains(content, "\r\n") {
		sep = "\r\n"
	}
	finalNewline := strings.HasSuffix(content, sep)
	linesArr := strings.Split(content, sep)
	if finalNewline && len(linesArr) > 0 && linesArr[len(linesArr)-1] == "" {
		linesArr = linesArr[:len(linesArr)-1]
	}
	linesTable := L.NewTable()
	for i, line := range linesArr {
		linesTable.RawSetInt(i+1, lua.LString(line))
	}
	ret := L.NewTable()
	ret.RawSetString("lines", linesTable)
	ret.RawSetString("separator", lua.LString(sep))
	ret.RawSetString("finalNewline", lua.LBool(finalNewline))
	L.Push(ret)
	return 1
}

// luaWriteLines writes lines to a file with the given separator and final newline option.
func (lm *LuaModel) luaWriteLines(L *lua.LState) int {
	checkLuaArity(L, 2, 4)
	friendly := L.CheckString(1)
	linesTbl := L.CheckTable(2)
	sep := L.OptString(3, "\n")
	finalNewline := L.OptBool(4, true)
	var lines []string
	n := linesTbl.Len()
	for i := 1; i <= n; i++ {
		lv := linesTbl.RawGetInt(i)
		lines = append(lines, lv.String())
	}
	content := strings.Join(lines, sep)
	if finalNewline {
		content += sep
	}
	err := lm.fs.Write(friendly, []byte(content))
	if err != nil {
		L.ArgError(1, err.Error())
		return 0
	}

	return 0
}

// goToLValue converts a Go type into a Lua value.
// TODO: Potential problem with typed slices
func goToLValue(L *lua.LState, val any) lua.LValue {
	switch v := val.(type) {
	case nil:
		return lua.LNil
	case bool:
		return lua.LBool(v)
	case int:
		return lua.LNumber(v)
	case int64:
		return lua.LNumber(v)
	case float64:
		return lua.LNumber(v)
	case string:
		return lua.LString(v)
	case []any:
		tbl := L.NewTable()
		for i, item := range v {
			tbl.RawSetInt(i+1, goToLValue(L, item))
		}
		return tbl
	case []string:
		tbl := L.NewTable()
		for i, item := range v {
			tbl.RawSetInt(i+1, goToLValue(L, item))
		}
		return tbl
	case map[string]any:
		tbl := L.NewTable()
		for key, item := range v {
			tbl.RawSetString(key, goToLValue(L, item))
		}
		return tbl
	default:
		return lua.LString(fmt.Sprintf("%v", v))
	}
}

// luaTableToGo converts a Lua table into a Go native type.
//
// A table with only integer keys (1..n) becomes a []any (array). A table with
// any non-integer key becomes a map[string]any; numeric keys are preserved as
// their stringified form (e.g. key 1 -> "1"), so mixed-key tables lose no
// entries. The array-vs-map decision is made on key types, not on contiguity.
func luaTableToGo(tbl *lua.LTable) any {
	var arr []any
	isArray := true
	tbl.ForEach(func(key lua.LValue, value lua.LValue) {
		if key.Type() != lua.LTNumber {
			isArray = false
		} else {
			arr = append(arr, lValueToGo(value))
		}
	})
	if isArray {
		return arr
	}
	m := make(map[string]any)
	tbl.ForEach(func(key lua.LValue, value lua.LValue) {
		m[key.String()] = lValueToGo(value)
	})
	return m
}

// lValueToGo converts a Lua value into a Go native type.
//
// Type rules:
//   - LString stays a string. A previous strconv.Atoi guess silently retyped
//     numeric-looking strings ("007" -> 7), corrupting path-like strings and
//     zero-padded IDs handed to template/jq/to_json.
//   - LNumber is emitted as int when its value is whole (matching author intent
//     for IDs, ports, counts), and as float64 otherwise.
//   - LTable is fed through luaTableToGo (see below).
func lValueToGo(val lua.LValue) any {
	switch v := val.(type) {
	case lua.LBool:
		return bool(v)
	case lua.LNumber:
		f := float64(v)
		if i := int64(f); f == float64(i) {
			return i
		}
		return f
	case lua.LString:
		return string(v)
	case *lua.LTable:
		return luaTableToGo(v)
	default:
		return v.String()
	}
}

// ExecuteLuaWithData executes a script using definition contract v1.
func ExecuteLuaWithData(ctx context.Context, script string, fs FS, data map[string]any) error {
	contract, err := definitionContractFor(definitionContractV1)
	if err != nil {
		return err
	}
	return executeLuaWithDefinitionContract(ctx, script, fs, data, contract)
}

func executeLuaWithDefinitionContract(
	ctx context.Context,
	script string,
	fs FS,
	data map[string]any,
	contract *DefinitionContract,
) error {
	return executeDefinitionLua(ctx, script, fs, data, contract, nil)
}

func executeDefinitionLua(
	ctx context.Context,
	script string,
	fs FS,
	data map[string]any,
	contract *DefinitionContract,
	definition *RPackDef,
) error {
	lm, err := newLuaModelForDefinitionContract(ctx, fs, data, contract, definition)
	if err != nil {
		return fmt.Errorf("failed to initialize Lua environment: %w", err)
	}
	defer lm.Close()
	if err = lm.Exec(script); err != nil {
		return fmt.Errorf("failed to execute script: %w", err)
	}
	return nil
}
