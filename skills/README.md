# RPack Skills

Install the skills in this directory in your agent harness skills directory.

Each subdirectory contains a separate `SKILL.md`.

## Skills vailable

### rpack-author
**Purpose:** Guide LLMs to create correct, working rpack definitions

**Key Features:**
- Complete rpack anatomy (rpack.yaml, script.lua, schema.cue, files/)
- Three core Lua scripting patterns:
  - Static file copy (explicit `rpack.copy()` per file)
  - Templated files (`rpack.template()` + `rpack.write()`)
  - File merging (read base, concatenate custom, write merged)
- CUE schema design patterns (required vs optional, constraints)
- File sandbox prefixes (`rpack:`, `map:`, `temp:`, `./`)
- Best practices and common mistakes


### rpack-tester
**Purpose:** Guide LLMs to create tests that catch real mistakes

**Key Features:**
- Test directory structure (`tests/<name>/run.sh`)
- Test script signature (DEFDIR, OUTDIR parameters)
- Three test categories: success path, schema validation, error handling
- Simple assertion patterns (`test -f`, `grep -q`, `jq -e`)
- meta.json structure and error phases reference
- Anti-patterns and common test mistakes

## Usage Examples

### Using rpack-author
Ask an LLM to create an rpack definition:
> "Using the rpack-author skill, create an rpack definition that distributes GitHub Actions workflows with templated Go and Node versions."

The skill guides the LLM through:
1. Defining rpack.yaml with schema version and inputs
2. Writing script.lua with explicit operations
3. Creating schema.cue with appropriate constraints
4. Bundling files in the files/ directory

### Using rpack-tester
Ask an LLM to create tests for an rpack definition:
> "Using the rpack-tester skill, create tests for this rpack definition that verify all files are created and schema validation works."

The skill guides the LLM through:
1. Creating test directory structure
2. Writing success path tests (file existence, templated values)
3. Writing schema validation tests (reject invalid inputs)
4. Writing error handling tests (optional inputs, edge cases)
5. Using simple, readable assertions

## Key Learnings

1. **Explicit beats clever:** One `rpack.copy()` per line is clearer than loops
2. **Test the actual API:** `rpack.inputs()` exists, `rpack.has_input()` doesn't
3. **Schema must include inputs:** All schemas need `inputs: #Inputs` block
4. **Template quoting matters:** `"{{ .value }}"` produces quoted output
5. **Array values need repeated flags:** `--set key=val1 --set key=val2`, not JSON
6. **Inspect before testing:** Run rpack manually to see actual output format
7. **Simple assertions win:** `test -f`, `grep -q`, `jq -e` are sufficient
