--- String library.
-- This library is preloaded but must be required by the user:
--   local string = require("string")
-- It provides functions for manipulating and querying strings.
-- @module string

---
-- Returns the numerical codes of the characters in a string.
-- If called with one number, returns the code at that position.
-- If called with two numbers, returns the codes in that range.
--
-- @param s string The string.
-- @param i? number The starting index (default 1).
-- @param j? number The ending index (default is i if not provided).
-- @return ... number The numerical codes of the characters.
function string.byte(s, i, j)
end

---
-- Returns a string created from the given list of numerical codes.
--
-- @param ... number One or more numerical codes.
-- @return string The resulting string.
function string.char(...)
end

---
-- Returns a binary representation of the given function.
-- GopherLua does not support string.dump.
--
-- @param f function The function to dump.
-- @return nil
function string.dump(f)
end

---
-- Searches for the first match of pattern in the string.
-- If found, returns the start and end indices plus any captured substrings.
--
-- @param s string The subject string.
-- @param pattern string The pattern to search for.
-- @param init? number The optional starting position (default 1).
-- @param plain? boolean If true, perform a plain (non-pattern) search.
-- @return number|nil The starting index of the match, or nil if not found.
-- @return number|nil The ending index of the match.
-- @return ... string Any captured substrings.
function string.find(s, pattern, init, plain)
end

---
-- Returns a formatted version of its variable number of arguments.
-- The first argument is a string containing the format.
--
-- @param fmt string A format string.
-- @param ... any Values to substitute into the format.
-- @return string The formatted string.
function string.format(fmt, ...)
end

---
-- Returns a copy of s in which all (or the first n, if given) occurrences of the pattern have been replaced by a replacement.
-- The replacement can be a string, a table, or a function.
--
-- @param s string The subject string.
-- @param pattern string The pattern to replace.
-- @param repl string|table|function The replacement value.
-- @param n? number The maximum number of substitutions (default is all).
-- @return string The modified string.
-- @return number The number of substitutions made.
function string.gsub(s, pattern, repl, n)
end

---
-- Returns the length of s.
--
-- @param s string The subject string.
-- @return number The length of the string.
function string.len(s)
end

---
-- Returns a copy of s with all uppercase letters transformed to lowercase.
--
-- @param s string The subject string.
-- @return string The lowercased string.
function string.lower(s)
end

---
-- Returns the first match of pattern in s.
-- If the pattern has captures, then the captured values are returned.
--
-- @param s string The subject string.
-- @param pattern string The pattern to match.
-- @param init? number The optional starting position (default is 1).
-- @return string|nil The matched string or captured values, or nil if no match.
function string.match(s, pattern, init)
end

---
-- Returns a string that is the concatenation of s repeated n times.
--
-- @param s string The subject string.
-- @param n number The number of times to repeat.
-- @return string The repeated string.
function string.rep(s, n)
end

---
-- Returns a string that is the reverse of s.
--
-- @param s string The subject string.
-- @return string The reversed string.
function string.reverse(s)
end

---
-- Returns the substring of s that starts at i and continues until j.
--
-- @param s string The subject string.
-- @param i number The starting index.
-- @param j? number The ending index (default is the end of the string).
-- @return string The specified substring.
function string.sub(s, i, j)
end

---
-- Returns a copy of s with all lowercase letters transformed to uppercase.
--
-- @param s string The subject string.
-- @return string The uppercased string.
function string.upper(s)
end

---
-- Returns an iterator function that, each time it is called, returns the next substring of s that matches the pattern.
-- Alias: string.gfind is provided as an alias for string.gmatch.
--
-- @param s string The subject string.
-- @param pattern string The pattern to match.
-- @return function An iterator function.
function string.gmatch(s, pattern)
end

-- Alias for string.gmatch.
string.gfind = string.gmatch
