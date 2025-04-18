--- The table library provides functions for manipulating Lua tables.
--- local table = require("table")
--- @module table


local table = {}

---
-- Returns the length of the table (number of elements).
--
-- @param tbl table The Lua table to inspect.
-- @return number # The number of elements in the table.
function table.getn(tbl)
end

---
-- Concatenates the elements of a table to form a string.
-- By default, sep is the empty string, i is 1, and j is #tbl.
--
-- @param tbl table The table whose elements to concatenate.
-- @param sep? string An optional separator to insert between elements (default "").
-- @param i? number   The start index (default 1).
-- @param j? number   The end index (default #tbl).
-- @return string     # The concatenated string.
function table.concat(tbl, sep, i, j)
end

---
-- Inserts a value into the table.
-- If called as `table.insert(tbl, value)`, appends to the end of the table.
-- If called as `table.insert(tbl, pos, value)`, inserts `value` at position `pos`.
--
-- @param tbl table          The table to modify.
-- @param posOrValue any     If only two parameters, this is the value to append.
--                           If three parameters, this is the position.
-- @param value? any         The value to insert when inserting by position.
function table.insert(tbl, posOrValue, value)
end

---
-- Returns the largest numeric index in the table.
--
-- @param tbl table The Lua table to inspect.
-- @return number # The maximum numeric index.
function table.maxn(tbl)
end

---
-- Removes an element from a table, shifting down other elements if necessary.
-- If `pos` is not specified, removes the last element.
--
-- @param tbl table  The table from which to remove an element.
-- @param pos? number The position to remove. If not given, removes the last element.
-- @return any # The removed element.
function table.remove(tbl, pos)
end

---
-- Sorts the table in-place. By default, sorts in ascending order.
-- If `comp` is given, it should be a function that receives two elements `(a, b)`
-- and returns true if `a` should appear before `b`.
--
-- @param tbl table The table to sort.
-- @param comp? function|nil Optional comparison function.
function table.sort(tbl, comp)
end

return table
