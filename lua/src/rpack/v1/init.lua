------------------------
-- RPack Lua bindings
-- The RPack module contains all necessary functions to manipulate the
-- Use `local rpack = require "rpack.v1"`.
--
-- Output file handles allow `temp:file`, `./dir/myfile` (target location)
-- Input file handles can be `map:mapped-file`, `map:mapped-dir/myfile`, `temp:dir/file`, `rpack:dir/file-from-rpack-source`.
-- Output files carry an executable intent (the owner-execute bit, like git);
-- read/write bits follow the destination's local creation policy (umask), not the script.
-- @module rpack.v1

local rpack = {}

--- User configured inputs.
--- Not all inputs specified in RPackDef must be configured by the user.
--- Can be prefixed with `map:` to use as a file handle, e.g. input: my-file -> map:my-file
--- @return result Array of user supplied inputs names.
function rpack.inputs() end

--- User configured values.
--- The deserialized user supplied config for the RPack.
--- Its data was validated prior with the optional `schema.cue` cuelang schema.
--- @treturn table User supplied configuration values.
function rpack.values() end

--- Copies an file from source to destination
--- @param inputfile string The source file.
--- @param outputfile string Destination file of copy.
--- @param[opt] opts table Options: `executable` — boolean (default `false`) marking
--- the destination as an executable file. `mode` — compatibility alias, not a literal
--- chmod: only `"644"`/`"0644"` (non-executable) or `"755"`/`"0755"` (executable) are
--- accepted. `executable` and `mode` together are rejected. Exact permission modes
--- are not supported — manage private/deployment permissions outside rpack.
function rpack.copy(inputfile, outputfile, opts) end

--- Declare the executable intent of an already-written output file.
--- The `mode` argument is a compatibility alias, not a literal chmod: only
--- `"644"`/`"0644"` (non-executable) and `"755"`/`"0755"` (executable) are
--- accepted; exact modes like `"600"` are rejected. Read/write bits are not
--- managed and follow the destination's local creation policy (umask).
--- Only target paths (`./file`) and `temp:` files can be changed;
--- `rpack:` and `map:` are read-only. Directories are not supported.
--- ORDERING RULE: the declaration applies to the current staged content only —
--- a later `write`/`copy` to the same path resets the intent to non-executable,
--- so declare after your last write.
--- Note: declaring intent on a `temp:` file does not propagate through a
--- later `copy` (copy's own options decide).
--- @param file string The file to declare intent for (must already be written).
--- @param mode string Compatibility alias: `"644"` (non-executable) or `"755"` (executable).
function rpack.chmod(file, mode) end

--- Convert yaml to table
--- @param str string The yaml in string format
--- @return table Deserialized yaml structure.
function rpack.from_yaml(str) end

--- Convert table to yaml
--- @param tbl table The table to convert into yaml str
--- @return string Serialized yaml string.
function rpack.to_yaml(tbl) end

--- Convert json to table
--- @param str string The json in string format
--- @return table Deserialized json structure.
function rpack.from_json(str) end

--- Convert table to json
--- @param tbl table The table to convert into json str
--- @return string Serialized json string.
function rpack.to_json(tbl) end

--- Read lines from file and returns a list of lines.
--- It preserves the information about the line separator used
--- and if the last line is terminated.
--- @param file string The file to read from.
--- @return table {lines: []string, separator: string, finalNewLine: bool}
function rpack.read_lines(file) end

--- List files and directories under a path.
--- @param path string The directory to list.
--- @param[opt=false] recursive boolean Whether to recurse into subdirectories.
--- @return table Array of file paths.
--- @return table Array of directory paths.
function rpack.read_dir(path, recursive) end

--- Write lines to file
--- @param file string The file to write to.
--- @param obj table The lines to write to the file
--- @param[opt="\n"] lineSep string Line separator
--- @param[opt=true] finalNl bool Final new line
function rpack.write_lines(file, obj, lineSep, finalNl) end

--- Read string from file.
--- Reads the files contents into a string.
--- @param file string The file to read from.
--- @return string Contents of file.
function rpack.read(file) end

--- Write a string to a file
--- @param file string The file to write to.
--- @param str string The string to write.
--- @param[opt] opts table Options: `executable` — boolean (default `false`) marking
--- the output as an executable file. `mode` — compatibility alias, not a literal
--- chmod: only `"644"`/`"0644"` (non-executable) or `"755"`/`"0755"` (executable) are
--- accepted. `executable` and `mode` together are rejected. Exact permission modes
--- are not supported — manage private/deployment permissions outside rpack.
function rpack.write(file, str, opts) end

--- Template string contents with data.
--- It uses golangs text/template functionality, see [Go text template](https://pkg.go.dev/text/template).
--- The sprig function rpack.library is also availble [Sprig Functions](https://masterminds.github.io/sprig/).
--- @param tmpl string The template string.
--- @param data table The data to use for templating.
--- @param[opt="{{"] leftDelim Left-side template delimiter.
--- @param[opt="}}"] rightDelim Right-side template delimiter.
--- @return string Templated output.
function rpack.template(tmpl, data, leftDelim, rightDelim) end

--- JQ Query execution.
--- It uses golangs gojq https://github.com/itchyny/gojq library to execute jq like queries
--- on structured data. It results in a similar experience to jq or yq tools if used in conjunction with read_json and
--- write_json.
--- It always returns a slice of results.
--- @param query string The JQ query
--- @param data table The data to execute the query on.
--- @return array Array of results from query execution
function rpack.jq(query, data) end

return rpack
