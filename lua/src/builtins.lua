------------------------
--- Lua Builtins
---
--- The basic library provides core Lua functions and utilities.
--- These functions are preloaded into the global namespace.
---
--- All functions below are available globally without any module loading.
--- @module builtin

---
-- Asserts that expression `v` is true.
-- If not, raises an error with message `message` (or "assertion failed!" by default).
--
-- @param v any Value to test
-- @param message? string Optional error message if v is false or nil.
-- @return any Returns all given arguments if assertion passes.
function assert(v, message)
end

---
-- Performs an immediate garbage collection cycle.
--
-- @return nil
function collectgarbage()
end

---
-- Raises an error.
-- Terminates the current function and prints error information.
--
-- @param message any The error message or value.
-- @param level? number Optional level to indicate where the error occurred.
function error(message, level)
end

---
-- Returns the environment of the given function or the level-th function in the call stack.
--
-- @param fOrLevel? any A function or a number indicating the call level (default 1)
-- @return table The environment table.
function getfenv(fOrLevel)
end

---
-- Returns the metatable of the given object.
--
-- @param object any The object whose metatable is to be retrieved.
-- @return table|nil The metatable of the object, or nil if none exists.
function getmetatable(object)
end

---
-- Loads a chunk (a function) from a reader function.
-- The reader function should return parts of the chunk as strings.
--
-- @param reader function The function that returns strings.
-- @param chunkname? string Optional name for the chunk.
-- @return function|nil The loaded chunk or nil on error.
-- @return string|nil An error message in case of failure.
function load(reader, chunkname)
end

---
-- Loads a chunk from a string.
--
-- @param s string The string containing the chunk.
-- @param chunkname? string Optional name for the chunk (default "<string>").
-- @return function|nil The loaded chunk or nil on error.
-- @return string|nil An error message in case of failure.
function loadstring(s, chunkname)
end

---
-- Allows iteration over a table.
-- Returns the next key–value pair. Use in a for loop.
--
-- @param tbl table The table to iterate.
-- @param index? any The current index (nil to start iteration).
-- @return any The next key, or nil if at end.
-- @return any The value associated with the key.
function next(tbl, index)
end

---
-- Calls a function in protected mode.
-- Returns a boolean status plus any returned values.
--
-- @param f function The function to call.
-- @param ... Any extra arguments to pass to the function.
-- @return boolean True plus function results if no error; otherwise false and error message.
function pcall(f, ...)
end

---
-- Prints its arguments to stdout.
--
-- @param ... any Values to print (converted to strings).
function print(...)
end

---
-- Checks whether two values are equal (without invoking metamethods).
--
-- @param v1 any First value.
-- @param v2 any Second value.
-- @return boolean True if the two values are raw equal, false otherwise.
function rawequal(v1, v2)
end

---
-- Gets the raw value from a table (bypassing metamethods).
--
-- @param tbl table The table.
-- @param key any The key to lookup.
-- @return any The value stored at key.
function rawget(tbl, key)
end

---
-- Sets a raw value in a table (ignores metamethods).
--
-- @param tbl table The table.
-- @param key any The key to set.
-- @param value any The value to assign.
function rawset(tbl, key, value)
end

---
-- When given a number or string as first argument, returns the total number of extra arguments.
-- If the first argument is the literal string "#", returns the number of extra arguments.
--
-- @param i number|string A starting index or the literal "#".
-- @param ... any Extra arguments.
-- @return number The number of extra arguments.
function select(i, ...)
end

---
-- [Internal] Prints the current registers (useful only for debugging).
--
-- @return nil
function _printregs()
end

---
-- Sets the environment of a function or a call level.
--
-- @param fOrLevel any A function or a call level number.
-- @param env table The new environment table.
-- @return function|nil The function with new environment or raises an error in failure.
function setfenv(fOrLevel, env)
end

---
-- Sets the metatable for the given object.
--
-- @param object any The object to assign a metatable to.
-- @param mt table|nil The metatable to set (or nil to remove).
-- @return any The object.
function setmetatable(object, mt)
end

---
-- Converts its argument to a number.
-- If conversion fails, returns nil.
--
-- @param x any The value to convert.
-- @param base? number The numeric base for conversion (default 10).
-- @return number|nil The number or nil if conversion fails.
function tonumber(x, base)
end

---
-- Converts its argument to a string using Lua’s coercion rules.
--
-- @param x any The value to convert.
-- @return string The string representation.
function tostring(x)
end

---
-- Returns the type of its argument as a string.
--
-- @param v any The value.
-- @return string The type name ("number", "string", "table", etc.).
function type(v)
end

---
-- Returns the elements of a table.
-- Optionally, you can specify a starting and ending index.
--
-- @param tbl table The table to unpack.
-- @param i? number The starting index (default 1).
-- @param j? number The ending index (default #tbl).
-- @return ... The list of elements from the table.
function unpack(tbl, i, j)
end

---
-- Calls a function in protected mode with an error handler.
-- Returns a boolean status plus any results.
--
-- @param f function The function to call.
-- @param err function The error handling function.
-- @param ... any Extra arguments to pass to the function.
-- @return boolean True plus function returns if no error; false and error message upon error.
function xpcall(f, err, ...)
end

---
-- Creates and registers a module.
-- The module is placed in the loaded modules table and set as the environment for the calling function.
--
-- @param name string The name of the module.
-- @param ... any Additional arguments used to initialize the module.
-- @return table The module table.
function module(name, ...)
end

---
-- Loads the given module.
-- If the module is already loaded, returns it; otherwise, finds and loads it.
--
-- @param name string The module name.
-- @return any The module value.
function require(name)
end

---
-- Creates a new userdata proxy.
-- Depending on the argument, creates a blank or clone of an existing proxy.
--
-- @param arg? boolean|userdata If true, creates a proxy with a new metatable; if userdata, clones its metatable.
-- @return userdata The new proxy userdata.
function newproxy(arg)
end

---
-- Iterates over arrays in a table.
-- Returns an iterator function for use with generic for.
--
-- @param tbl table The table to iterate.
-- @return function The iterator function.
function ipairs(tbl)
end

---
-- Iterates over key–value pairs in a table.
-- Returns an iterator function for use with generic for.
--
-- @param tbl table The table to iterate.
-- @return function The iterator function.
function pairs(tbl)
end
