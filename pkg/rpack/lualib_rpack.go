package rpack

import (
	"bytes"
	"encoding/json"
	"os"
	"text/template"

	"fmt"

	"github.com/itchyny/gojq"
	lua "github.com/yuin/gopher-lua"
	"sigs.k8s.io/yaml"
)

type LuaAPIFS interface {
	Write(name string, b []byte) error
	Read(name string) ([]byte, error)
	Chmod(name string, mode os.FileMode) error
	Stat(name string) (exists bool, dir bool, err error)
	ReadDir(name string) (_files []string, _dirs []string, _err error)
	ReadDirAll(name string) (_files []string, _dirs []string, _err error)
}

type RPackAPI struct {
	fs LuaAPIFS
}

func NewRPackAPI(fs LuaAPIFS) *RPackAPI {

	return &RPackAPI{
		fs: fs,
	}
}

func (a *RPackAPI) Funcs() map[string]lua.LGFunction {
	return map[string]lua.LGFunction{
		"copy":      a.luaCopy,
		"chmod":     a.luaChmod,
		"from_json": luaFromJSON,
		"to_json":   luaToJSON,
		"from_yaml": luaFromYAML,
		"to_yaml":   luaToYAML,
		"write":     a.luaWrite,
		"read":      a.luaRead,
		"read_dir":  a.luaReadDir,
		"template":  luaTemplate,
		"jq":        luaJQ,
	}
}

func (a *RPackAPI) RegisterFunc(name string) lua.LGFunction {
	return func(L *lua.LState) int {
		tabmod := L.RegisterModule(name, a.Funcs())
		L.Push(tabmod)
		return 1
	}
}

func (a *RPackAPI) luaCopy(L *lua.LState) int {
	checkLuaArity(L, 2, 3)
	in := L.CheckString(1)
	out := L.CheckString(2)
	mode, hasMode := checkOptsMode(L, 3)
	b, err := a.fs.Read(in)
	if err != nil {
		L.ArgError(1, err.Error())
		return 0
	}
	err = a.fs.Write(out, b)
	if err != nil {
		L.ArgError(2, err.Error())
		return 0
	}
	a.applyOptsMode(L, out, mode, hasMode)
	return 0
}

func (a *RPackAPI) luaWrite(L *lua.LState) int {
	checkLuaArity(L, 2, 3)
	friendly := L.CheckString(1)
	content := L.CheckString(2)
	mode, hasMode := checkOptsMode(L, 3)
	err := a.fs.Write(friendly, []byte(content))
	if err != nil {
		L.ArgError(1, err.Error())
		return 0
	}
	a.applyOptsMode(L, friendly, mode, hasMode)
	return 0
}

// luaChmod implements rpack.chmod(path, mode) (ADR 0001). It amends the mode
// of an already-staged file; a later write resets the mode to 0644.
func (a *RPackAPI) luaChmod(L *lua.LState) int {
	checkLuaArity(L, 2, 2)
	friendly := L.CheckString(1)
	mode := checkModeArg(L, 2)
	if err := a.fs.Chmod(friendly, mode); err != nil {
		L.ArgError(1, err.Error())
		return 0
	}
	return 0
}

// applyOptsMode applies the optional opts-table mode after a successful
// write/copy. The chmod goes through the same fs.Chmod path as rpack.chmod,
// so access control, purity tracking, and recording treat both spellings
// identically. Parse errors are attributed to the opts argument by
// checkOptsMode; a filesystem failure here raises with the friendly path.
func (a *RPackAPI) applyOptsMode(L *lua.LState, friendly string, mode os.FileMode, hasMode bool) {
	if !hasMode {
		return
	}
	if err := a.fs.Chmod(friendly, mode); err != nil {
		L.RaiseError("failed to chmod %s: %s", friendly, err.Error())
	}
}

// checkModeArg parses a required mode argument (octal string like "755").
// Numbers are rejected deliberately: decimal 493 is unreadable, which is why
// the grammar exists (ADR 0001).
func checkModeArg(L *lua.LState, n int) os.FileMode {
	str, ok := L.Get(n).(lua.LString)
	if !ok {
		L.ArgError(n, "mode must be an octal string like \"755\"")
		return 0
	}
	mode, err := ParseOctalMode(string(str))
	if err != nil {
		L.ArgError(n, err.Error())
		return 0
	}
	return mode
}

// checkOptsMode parses the optional trailing options table at argument n,
// returning the requested mode and whether one was given. An absent argument
// or nil means no options; an empty table is a valid no-op. Unknown keys are
// hard errors (rpack's explicitness rule: a typo'd key must not be silently
// ignored, and future keys like preserve_mode must not collide).
func checkOptsMode(L *lua.LState, n int) (mode os.FileMode, hasMode bool) {
	if L.GetTop() < n || L.Get(n) == lua.LNil {
		return 0, false
	}
	tbl := L.CheckTable(n)
	tbl.ForEach(func(k, v lua.LValue) {
		key, ok := k.(lua.LString)
		if !ok || string(key) != "mode" {
			L.ArgError(n, fmt.Sprintf("unknown option %q (known: mode)", k.String()))
			return
		}
		str, ok := v.(lua.LString)
		if !ok {
			L.ArgError(n, "mode must be an octal string like \"755\"")
			return
		}
		m, err := ParseOctalMode(string(str))
		if err != nil {
			L.ArgError(n, err.Error())
			return
		}
		mode, hasMode = m, true
	})
	return mode, hasMode
}

func (a *RPackAPI) luaRead(L *lua.LState) int {
	checkLuaArity(L, 1, 1)
	friendly := L.CheckString(1)
	b, err := a.fs.Read(friendly)
	if err != nil {
		L.ArgError(1, err.Error())
		return 0
	}
	L.Push(lua.LString(string(b)))
	return 1
}

func (a *RPackAPI) luaReadDir(L *lua.LState) int {
	checkLuaArity(L, 1, 2)
	friendly := L.CheckString(1)
	recursive := L.OptBool(2, false)
	var files []string
	var dirs []string
	var err error
	if recursive {
		files, dirs, err = a.fs.ReadDirAll(friendly)
	} else {
		files, dirs, err = a.fs.ReadDir(friendly)
	}
	if err != nil {
		L.ArgError(1, err.Error())
		return 0
	}
	L.Push(goToLValue(L, files))
	L.Push(goToLValue(L, dirs))
	return 2
}

func luaFromJSON(L *lua.LState) int {
	checkLuaArity(L, 1, 1)
	input := L.CheckString(1)
	var data any
	if err := json.Unmarshal([]byte(input), &data); err != nil {
		L.ArgError(1, fmt.Errorf("failed to unmarshal JSON: %w", err).Error())
		return 0
	}
	L.Push(goToLValue(L, data))
	return 1
}

// luaToJSON marshals a Lua table as JSON and writes it out.
func luaToJSON(L *lua.LState) int {
	checkLuaArity(L, 1, 1)
	val := L.CheckTable(1)
	goVal := luaTableToGo(val)
	jsonBytes, err := json.MarshalIndent(goVal, "", "  ")
	if err != nil {
		L.ArgError(1, fmt.Errorf("failed to marshal JSON: %w", err).Error())
		return 0
	}
	L.Push(lua.LString(string(jsonBytes)))
	return 1
}

func luaFromYAML(L *lua.LState) int {
	checkLuaArity(L, 1, 1)
	input := L.CheckString(1)
	var data any
	if err := yaml.Unmarshal([]byte(input), &data); err != nil {
		L.ArgError(1, fmt.Errorf("failed to unmarshal YAML: %w", err).Error())
		return 0
	}
	L.Push(goToLValue(L, data))
	return 1
}

func luaToYAML(L *lua.LState) int {
	checkLuaArity(L, 1, 1)
	val := L.CheckTable(1)
	goVal := luaTableToGo(val)
	yamlBytes, err := yaml.Marshal(goVal)
	if err != nil {
		L.ArgError(1, fmt.Errorf("failed to marshal YAML: %w", err).Error())
		return 0
	}
	L.Push(lua.LString(string(yamlBytes)))
	return 1
}

// luaTemplate treats the given string as a text/template,
// executes it with the provided Lua data (converted to a Go value), and returns the result.
// It supports optional start and end delimiters.
func luaTemplate(L *lua.LState) int {
	checkLuaArity(L, 2, 4)
	tplContent := L.CheckString(1)
	dataTable := L.CheckTable(2)
	data := luaTableToGo(dataTable)
	// Optional delimiters as arguments 3 and 4.
	leftDelim := L.OptString(3, "")
	rightDelim := L.OptString(4, "")
	tpl := template.New("tpl")
	if leftDelim != "" && rightDelim != "" {
		tpl = tpl.Delims(leftDelim, rightDelim)
	}
	tmpl, err := tpl.Parse(tplContent)
	if err != nil {
		L.ArgError(1, fmt.Errorf("failed to parse template: %w", err).Error())
		return 0
	}
	var buf bytes.Buffer
	if err = tmpl.Execute(&buf, data); err != nil {
		L.ArgError(2, fmt.Errorf("failed to execute template: %w", err).Error())
		return 0
	}
	L.Push(lua.LString(buf.String()))
	return 1
}

// luaJQ executes a gojq (https://github.com/itchyny/gojq) query
// on the provided data.
func luaJQ(L *lua.LState) int {
	checkLuaArity(L, 2, 2)
	queryStr := L.CheckString(1)
	val := L.CheckTable(2)
	goVal := luaTableToGo(val)

	query, err := gojq.Parse(queryStr)
	if err != nil {
		L.ArgError(1, fmt.Errorf("failed to parse query: %w", err).Error())
		return 0
	}
	iter := query.Run(goVal)
	var res []any
	for {
		v, ok := iter.Next()
		if !ok {
			break
		}
		if err, ok := v.(error); ok {
			if err, ok := err.(*gojq.HaltError); ok && err.Value() == nil {
				break
			}
			L.ArgError(2, fmt.Errorf("error executing query: %w", err).Error())
			return 0
		}
		res = append(res, v)
	}
	L.Push(goToLValue(L, res))
	return 1
}
