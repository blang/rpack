package rpack

import (
	"fmt"

	lua "github.com/yuin/gopher-lua"
)

// checkLuaArity rejects arguments outside a function's stable contract before
// the function performs validation or side effects. max < 0 means variadic.
func checkLuaArity(L *lua.LState, minArgs, maxArgs int) {
	got := L.GetTop()
	if got >= minArgs && (maxArgs < 0 || got <= maxArgs) {
		return
	}

	var want string
	switch {
	case maxArgs < 0:
		want = fmt.Sprintf("at least %d %s", minArgs, luaArgumentWord(minArgs))
	case minArgs == maxArgs:
		want = fmt.Sprintf("%d %s", minArgs, luaArgumentWord(minArgs))
	default:
		want = fmt.Sprintf("%d to %d arguments", minArgs, maxArgs)
	}
	L.RaiseError("expected %s, got %d", want, got)
}

func luaArgumentWord(count int) string {
	if count == 1 {
		return "argument"
	}
	return "arguments"
}
