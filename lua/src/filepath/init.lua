--- Filepath library.
-- This library is preloaded but must be required by the user:
--   local filepath = require("filepath")
-- It's exposing a large portion of Golang's "path/filepath" module functionality.
-- Source: https://pkg.go.dev/path/filepath
--
-- @module filepath
local filepath = {}

---
-- Returns the last element of the given path.
--
-- The function mimics Go's filepath.Base behavior and returns the final
-- component of the path.
--
-- @param path string The path string.
-- @return string The base (last element) of the path.
function filepath.base(path)
  -- actual implementation in Go backend
end

---
-- Cleans up the given path by applying lexical processing.
--
-- This function applies a series of transformations to the path:
--   - Removing redundant separators.
--   - Resolving any . or .. elements.
--
-- It mimics the behavior of Go's filepath.Clean.
--
-- @param path string The path string to clean.
-- @return string The cleaned path.
function filepath.clean(path)
  -- actual implementation in Go backend
end

---
-- Returns all but the last element of the path.
--
-- The function mimics the behavior of Go's filepath.Dir by stripping the final
-- element from the path, leaving the directory path.
--
-- @param path string The path string.
-- @return string The directory portion of the path.
function filepath.dir(path)
  -- actual implementation in Go backend
end

---
-- Returns the file extension of the given path.
--
-- This function extracts the substring starting at the final dot in the base
-- component of the path, if one exists.
--
-- @param path string The path string.
-- @return string The file extension (including the dot) or an empty string if none.
function filepath.ext(path)
  -- actual implementation in Go backend
end

---
-- Checks if the given path is absolute.
--
-- The function returns true if the path is absolute, matching Go's filepath.IsAbs.
--
-- @param path string The path string.
-- @return boolean True if the path is absolute, false otherwise.
function filepath.isAbs(path)
  -- actual implementation in Go backend
end

---
-- Checks if the given path is local.
--
-- It mimics a similar function as in Go's filepath, determining if the path
-- is relative to a current or specified local directory.
--
-- @param path string The path string.
-- @return boolean True if the path is local, false otherwise.
function filepath.isLocal(path)
  -- actual implementation in Go backend
end

---
-- Joins multiple path elements into a single path.
--
-- This function concatenates the path elements using the system's file separator.
-- It takes two required parameters and several optional ones.
--
-- Example:
--   local fullPath = filepath.join("folder", "subfolder", "file.txt")
--
-- @param first string The first part of the path.
-- @param second string The second part of the path.
-- @param ... string Additional path segments.
-- @return string The joined path string.
function filepath.join(first, second, ...)
  -- actual implementation in Go backend
end

---
-- Splits the given path into directory and file components.
--
-- The function returns two values: the directory and the file. It behaves like
-- Go's filepath.Split.
--
-- @param path string The path string.
-- @return string The directory part of the path.
-- @return string The file part of the path.
function filepath.split(path)
  -- actual implementation in Go backend
end

---
-- Returns the location and local path of a given path.
--
-- It accepts paths like `map:inputdir` and returns "map", "inputdir".
--
-- For paths without a given location specifier such as "map:" it will return "target"
-- as the location.
--
-- @param path string The path string potentially containing a location specifier.
-- @return string The location specifier such as "map", or "target" if none is found.
-- @return string The path part without the location, such as "./inputdir"
function filepath.location(path)
  -- actual implementation in Go backend
end

return filepath
