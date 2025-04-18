--- Debug library.
-- Provides functions to inspect and modify the running environment.
-- All functions are available in the global namespace as "debug".
-- @module debug

---
-- Returns the environment of the given object.
--
-- @param o any The object (typically a function) whose environment is to be returned.
-- @return table The environment of the object.
function debug.getfenv(o)
end

---
-- Returns a table with information about a function or a given level of the call stack.
--
-- When used with a function, returns data about that function. When used with a number,
-- returns data about the corresponding call stack level.
--
-- @param f function|number A function or a stack level number.
-- @param what? string A string specifying which fields to fill in (default is "Slunf").
-- @return table|nil A table containing debug information, or nil if not available.
function debug.getinfo(f, what)
end

---
-- Returns the name and value of a local variable at a given call stack level and index.
--
-- @param level number The current stack level.
-- @param index number The index of the local variable.
-- @return string|nil The name of the variable (or nil if none exists).
-- @return any The value of the local variable.
function debug.getlocal(level, index)
end

---
-- Returns the metatable of the given object.
--
-- @param o any The object whose metatable is to be retrieved.
-- @return table|nil The metatable of the object, or nil if no metatable is set.
function debug.getmetatable(o)
end

---
-- Returns the name and value of the upvalue for the given function at index.
--
-- @param f function The function.
-- @param index number The upvalue index.
-- @return string|nil The name of the upvalue.
-- @return any The value of the upvalue.
function debug.getupvalue(f, index)
end

---
-- Sets the environment of the given object.
--
-- @param o any The object whose environment is to be set.
-- @param env table The new environment.
function debug.setfenv(o, env)
end

---
-- Sets the value of a local variable in a given stack level.
--
-- @param level number The stack level.
-- @param index number The index of the local variable.
-- @param value any The new value to assign.
-- @return string|nil The name of the local variable if set, or nil.
function debug.setlocal(level, index, value)
end

---
-- Sets the metatable for the given object.
--
-- @param o any The object whose metatable is to be set.
-- @param mt table|nil The new metatable, or nil to remove it.
-- @return any The object, with its metatable updated.
function debug.setmetatable(o, mt)
end

---
-- Sets the value of an upvalue for the given function.
--
-- @param f function The function.
-- @param index number The index of the upvalue to set.
-- @param value any The new value to assign.
-- @return string|nil The name of the upvalue if set, or nil.
function debug.setupvalue(f, index, value)
end

---
-- Generates a traceback of the call stack.
-- Optionally, a custom message may be prepended.
--
-- @param message? string An optional error message to prepend.
-- @param level? number The level where to start the traceback (default is 1).
-- @return string A string containing the traceback.
function debug.traceback(message, level)
end
