package rpack

import (
	"bytes"
	"encoding/json"
	"text/template"

	"github.com/itchyny/gojq"
	"github.com/pkg/errors"
	lua "github.com/yuin/gopher-lua"
	"sigs.k8s.io/yaml"
)

type LuaAPIFS interface {
	Write(name string, b []byte) error
	Read(name string) ([]byte, error)
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
	in := L.CheckString(1)
	out := L.CheckString(2)
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
	return 0
}

func (a *RPackAPI) luaWrite(L *lua.LState) int {
	friendly := L.CheckString(1)
	content := L.CheckString(2)
	err := a.fs.Write(friendly, []byte(content))
	if err != nil {
		L.ArgError(1, err.Error())
		return 0
	}
	return 0
}

func (a *RPackAPI) luaRead(L *lua.LState) int {
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
	friendly := L.CheckString(1)
	recursive := L.CheckBool(2)
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
	input := L.CheckString(1)
	var data any
	if err := json.Unmarshal([]byte(input), &data); err != nil {
		L.ArgError(1, errors.Wrap(err, "failed to unmarshal JSON").Error())
		return 0
	}
	L.Push(goToLValue(L, data))
	return 1
}

// luaToJSON marshals a Lua table as JSON and writes it out.
func luaToJSON(L *lua.LState) int {
	val := L.CheckTable(1)
	goVal := luaTableToGo(val)
	jsonBytes, err := json.MarshalIndent(goVal, "", "  ")
	if err != nil {
		L.ArgError(1, errors.Wrap(err, "failed to marshal JSON").Error())
		return 0
	}
	L.Push(lua.LString(string(jsonBytes)))
	return 1
}

func luaFromYAML(L *lua.LState) int {
	input := L.CheckString(1)
	var data any
	if err := yaml.Unmarshal([]byte(input), &data); err != nil {
		L.ArgError(1, errors.Wrap(err, "failed to unmarshal YAML").Error())
		return 0
	}
	L.Push(goToLValue(L, data))
	return 1
}

func luaToYAML(L *lua.LState) int {
	val := L.CheckTable(1)
	goVal := luaTableToGo(val)
	jsonBytes, err := json.MarshalIndent(goVal, "", "  ")
	if err != nil {
		L.ArgError(1, errors.Wrap(err, "failed to marshal YAML").Error())
		return 0
	}
	L.Push(lua.LString(string(jsonBytes)))
	return 1
}

// luaTemplate treats the given string as a text/template,
// executes it with the provided Lua data (converted to a Go value), and returns the result.
// It supports optional start and end delimiters.
func luaTemplate(L *lua.LState) int {
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
		L.ArgError(1, errors.Wrap(err, "failed to parse template").Error())
		return 0
	}
	var buf bytes.Buffer
	if err = tmpl.Execute(&buf, data); err != nil {
		L.ArgError(2, errors.Wrap(err, "failed to execute template").Error())
		return 0
	}
	L.Push(lua.LString(buf.String()))
	return 1
}

// luaJQ executes a gojq (https://github.com/itchyny/gojq) query
// on the provided data.
func luaJQ(L *lua.LState) int {
	queryStr := L.CheckString(1)
	val := L.CheckTable(2)
	goVal := luaTableToGo(val)

	query, err := gojq.Parse(queryStr)
	if err != nil {
		L.ArgError(1, errors.Wrap(err, "failed to parse query").Error())
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
			L.ArgError(2, errors.Wrap(err, "error executing query").Error())
			return 0
		}
		res = append(res, v)
	}
	L.Push(goToLValue(L, res))
	return 1
}
