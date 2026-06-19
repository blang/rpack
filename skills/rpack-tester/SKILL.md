---
name: rpack-tester
description: Create tests for rpack definitions that catch real mistakes using simple assertions. Use when you need to validate that an rpack produces correct output, catches schema violations, and handles errors gracefully.
---

# RPack Tester

## Overview

Create tests for rpack definitions that validate correctness and catch common author mistakes. Tests live in `tests/` subdirectories within the rpackdef, and `rpack test --def` discovers and runs them automatically. Every test validates one specific behavior or failure mode.

## Test Structure

Tests are subdirectories inside `tests/` within your rpack definition:

```
rpackdef/
  tests/
    success-basic/          # Verify correct output with valid inputs
      run.sh                # Test script (must be executable)
      users.yaml            # Optional test fixture
    schema-validation/      # Verify schema rejects invalid values
      run.sh
    missing-input/          # Verify graceful handling of missing inputs
      run.sh
```

### Test Script Requirements

- Must be named exactly: `run.sh`, `run.py`, or `run`
- Must be executable (`chmod +x`)
- Receives two positional arguments:
  - `$1` = absolute path to the definition directory (`DEFDIR`)
  - `$2` = absolute path to a temporary output directory (`OUTDIR`)
- Exit code 0 = pass, non-zero = fail
- Each test runs in a fresh output directory (no state leaks between tests)

### Test Naming Conventions

- `success-*` — positive tests verifying correct output
- `schema-*` — tests verifying schema rejects invalid values
- `*error*` or `*missing*` — tests verifying error handling
- Use descriptive, kebab-case names

### Running Tests

```bash
# Run all tests in a definition
rpack test --def path/to/rpackdef

# Run specific tests by name filter
rpack test --def path/to/rpackdef --filter success-basic

# Scaffold a new test directory with template
rpack test --def path/to/rpackdef --init my-new-test
```

## Test Script Template

```bash
#!/bin/bash
set -e
DEFDIR="$1"
OUTDIR="$2"

# Optional: Copy test fixtures to OUTDIR (within sandbox)
# cp "$DEFDIR/tests/<test-name>/fixture.txt" "$OUTDIR/fixture.txt"

# Run rpack with test values
rpack run --def "$DEFDIR" \
  --set key="value" \
  --output-dir "$OUTDIR" \
  --force

# Check success
jq -e '.success' "$OUTDIR/meta.json" \
  || { echo "FAIL: $(jq -r '.error' "$OUTDIR/meta.json")"; exit 1; }

# Verify expected output files exist
test -f "$OUTDIR/expected-file.ext" \
  || { echo "FAIL: expected-file.ext not created"; exit 1; }

# Verify content
grep -q "expected content" "$OUTDIR/output-file.ext" \
  || { echo "FAIL: expected content missing from output-file.ext"; exit 1; }

echo "PASS: test-name"
```

## Test Categories

### 1. Success Path Tests

Verify that rpack produces correct output with valid values and inputs.

```bash
#!/bin/bash
set -e
DEFDIR="$1"
OUTDIR="$2"

# Copy any input fixtures to OUTDIR
cp "$DEFDIR/tests/success-basic/input.yaml" "$OUTDIR/input.yaml"

# Run rpack with valid values
rpack run --def "$DEFDIR" \
  --set value_key="expected value" \
  --set-input input.yaml="$OUTDIR/input.yaml" \
  --output-dir "$OUTDIR" \
  --force

# Verify success
jq -e '.success' "$OUTDIR/meta.json" \
  || { echo "FAIL: $(jq -r '.error' "$OUTDIR/meta.json")"; exit 1; }

# Verify all expected output files
test -f "$OUTDIR/output-file.md" || { echo "FAIL: output-file.md not created"; exit 1; }
test -f "$OUTDIR/.github/workflows/ci.yml" || { echo "FAIL: ci.yml not created"; exit 1; }

# Verify templated values appear in output
grep -q "expected value" "$OUTDIR/output-file.md" \
  || { echo "FAIL: value not templated into output-file.md"; exit 1; }

echo "PASS: success-basic"
```

**What to verify in success path tests:**
- All expected output files exist
- Templated values (`--set`) appear in output files
- Merged files contain both base and custom content
- Files from `files/` directory are present in output
- `meta.json` shows `success: true`
- `meta.json` lists expected files in `files_written`

### 2. Schema Validation Tests

Verify that schema rejects invalid value types, out-of-range values, and malformed patterns.

```bash
#!/bin/bash
set -e
DEFDIR="$1"
OUTDIR="$2"

# Run rpack with invalid value type (e.g., number where string expected)
if rpack run --def "$DEFDIR" \
  --set version=999 \
  --output-dir "$OUTDIR"; then
  echo "FAIL: rpack should have failed with invalid version type"
  exit 1
fi

# Verify failure
jq -e '.success == false' "$OUTDIR/meta.json" \
  || { echo "FAIL: meta.json should show success=false"; exit 1; }

# Verify correct error phase
jq -e '.error_phase == "schema_validation"' "$OUTDIR/meta.json" \
  || { echo "FAIL: error_phase should be schema_validation, got $(jq -r '.error_phase' "$OUTDIR/meta.json")"; exit 1; }

echo "PASS: schema-validation"
```

**Schema validation patterns to test:**
- Wrong types: number instead of string, string instead of int
- Out-of-range: `node_version: 99` when max is 22
- Invalid format: `artifact: "Invalid_Name"` when regex requires `[a-z0-9-]+`
- Missing required values (trigger schema validation error)
- Empty strings when minimum length is required

### 3. Error Handling Tests

Verify that rpack handles missing inputs, bad data, and edge cases gracefully.

```bash
#!/bin/bash
set -e
DEFDIR="$1"
OUTDIR="$2"

# Run rpack WITHOUT required input file
if rpack run --def "$DEFDIR" \
  --set author="Test Author" \
  --output-dir "$OUTDIR"; then
  echo "FAIL: rpack should have failed without required input"
  exit 1
fi

# Verify failure
jq -e '.success == false' "$OUTDIR/meta.json" \
  || { echo "FAIL: meta.json should show success=false"; exit 1; }

# Verify correct error phase
jq -e '.error_phase == "lua_execution"' "$OUTDIR/meta.json" \
  || { echo "FAIL: error_phase should be lua_execution, got $(jq -r '.error_phase' "$OUTDIR/meta.json")"; exit 1; }

# Verify error message mentions what went wrong
jq -e '.error | test("users.yaml")' "$OUTDIR/meta.json" \
  || { echo "FAIL: error should mention users.yaml"; exit 1; }

echo "PASS: missing-input"
```

**Error scenarios to test:**
- Missing required input file
- Missing required value
- Malformed input file (invalid YAML, wrong structure)
- Input file outside sandbox
- Lua script errors (nil references, bad type coercion)

## meta.json Reference

rpack writes a `meta.json` file to the output directory after every run. Use `jq` to query it in tests.

```json
{
  "success": true,
  "error": null,
  "error_phase": null,
  "files_read": [
    "rpack:files/template.md",
    "rpack:files/config.yaml"
  ],
  "files_written": [
    "output.md",
    ".github/workflows/ci.yml"
  ],
  "inputs_used": []
}
```

### meta.json Fields

| Field | Type | Description |
|-------|------|-------------|
| `success` | bool | `true` if execution completed without errors |
| `error` | string or null | Error message if execution failed |
| `error_phase` | string or null | Phase where error occurred (see below) |
| `files_read` | array | Files read during execution (with sandbox prefix) |
| `files_written` | array | Files written to output directory |
| `inputs_used` | array | Input names that were configured and used |

### Error Phases

| Phase | Meaning |
|-------|---------|
| `schema_validation` | Schema validation failed (value type, range, pattern mismatch) |
| `input_validation` | Input validation failed (missing required input) |
| `purity_check` | Purity check failed (output directory already exists without `--force`) |
| `lua_execution` | Lua script encountered an error (nil access, bad operation) |
| `unknown` | Unclassified error |

## Assertion Patterns

Use simple, readable assertions. Every failure must print a clear message.

### File Existence

```bash
test -f "$OUTDIR/.editorconfig" \
  || { echo "FAIL: .editorconfig not created"; exit 1; }

test -d "$OUTDIR/.github/workflows" \
  || { echo "FAIL: workflows directory not created"; exit 1; }
```

### Content Presence

```bash
grep -q "expected text" "$OUTDIR/output.md" \
  || { echo "FAIL: expected text missing from output.md"; exit 1; }

# Multiple content checks
grep -q "first pattern" "$OUTDIR/file.md" \
  || { echo "FAIL: first pattern missing from file.md"; exit 1; }
grep -q "second pattern" "$OUTDIR/file.md" \
  || { echo "FAIL: second pattern missing from file.md"; exit 1; }
```

**Template quoting matters:** If your template uses `"{{ .value }}"` (with quotes), the output will include the quotes. Your grep patterns must match:

```bash
# Template: go-version: "{{ .go_version }}"
# Output:   go-version: "1.22.3"
grep -q 'go-version: "1.22.3"' "$OUTDIR/file.yml"  # ✓ matches

# Template: go-version: {{ .go_version }}
# Output:   go-version: 1.22.3
grep -q 'go-version: 1.22.3' "$OUTDIR/file.yml"    # ✓ matches
```

Always check the template source to construct accurate grep patterns.

### JSON Queries

```bash
# Check success
jq -e '.success' "$OUTDIR/meta.json" \
  || { echo "FAIL: $(jq -r '.error' "$OUTDIR/meta.json")"; exit 1; }

# Check failure
jq -e '.success == false' "$OUTDIR/meta.json" \
  || { echo "FAIL: expected failure but got success"; exit 1; }

# Check error phase
jq -e '.error_phase == "schema_validation"' "$OUTDIR/meta.json" \
  || { echo "FAIL: wrong error phase, got $(jq -r '.error_phase' "$OUTDIR/meta.json")"; exit 1; }

# Check error message contains expected text
jq -e '.error | test("missing input")' "$OUTDIR/meta.json" \
  || { echo "FAIL: error message doesn't mention missing input"; exit 1; }

# Check file tracking
jq -e '(.files_written | index(".editorconfig"))' "$OUTDIR/meta.json" \
  || { echo "FAIL: .editorconfig not in files_written"; exit 1; }
```

### Negative Assertions (test should fail)

```bash
# Run rpack with invalid input — should fail
if rpack run --def "$DEFDIR" \
  --set bad_value="invalid" \
  --output-dir "$OUTDIR"; then
  echo "FAIL: rpack should have failed with invalid input"
  exit 1
fi

# Now check meta.json for expected error details
jq -e '.success == false' "$OUTDIR/meta.json" \
  || { echo "FAIL: meta.json should show success=false"; exit 1; }
```

## What to Test For

### Always Test

1. **All files from `files/` are produced in output** — Check `meta.json` `files_written` array or `test -f` for each expected output file
2. **Values provided via `--set` appear in output** — `grep -q` for each templated value
3. **Schema rejects invalid values** — Run with bad type, bad range, bad pattern and verify failure
4. **Optional inputs are handled gracefully** — Run without optional input, verify success
5. **Template escaping works** — If templates contain literal `{{` (e.g., GitHub Actions), verify they're not parsed as Go templates

### Test When Present

6. **Input file processing** — If the script reads input files, test with valid and malformed inputs
7. **Multiple patterns combined** — If the script mixes copy/template/merge, verify each pattern produces correct output
8. **Directory creation** — Verify nested directories like `.github/workflows/` are created automatically

## Anti-Patterns

### Never Do

**Complex regex pipelines:**
```bash
# BAD — fragile, hard to debug
grep -P '(?<=version:\s)\S+' "$OUTDIR/workflow.yml" | sort | uniq -c
```

**Exact content matching:**
```bash
# BAD — any whitespace or formatting change breaks the test
diff "$DEFDIR/tests/expected-output.txt" "$OUTDIR/actual-output.txt"
```

**Test interdependencies:**
```bash
# BAD — test-b depends on files created by test-a
# Each test gets a FRESH output directory — nothing is shared
```

**Performance or timing tests:**
```bash
# BAD — not the skill's job, rpack tests validate correctness only
time rpack run --def "$DEFDIR" ...
```

**Hardcoded paths outside test directory:**
```bash
# BAD — will fail on other machines
rpack run --def /home/user/projects/rpackdef ...
```

## Common Test Mistakes

1. **Forgetting `--force` when OUTDIR contains fixtures** — If you copy fixtures to OUTDIR before running rpack, use `--force` or rpack will fail with `purity_check` error
2. **Not checking `meta.json` for error phase** — An rpack failure could be from schema, inputs, or Lua. Use `error_phase` to distinguish
3. **Testing implementation details** — Test that files are created and contain expected content. Don't test how many `rpack.copy()` calls were made
4. **Complex assertions that hide the failure reason** — Every `||` should include an `echo "FAIL: ..."` message
5. **Using `-e` for jq boolean checks** — Use `jq -e '.field'` for existence, `jq -e '.field == value'` for comparison. Without `-e`, jq always exits 0 even for `null` results
6. **Not using `set -e`** — Without it, intermediate failures are silently ignored and tests pass incorrectly
7. **Hardcoding values that come from `--set`** — If the test provides values, verify those exact values appear. Don't verify a different hardcoded string

## Scaffolding New Tests

```bash
# Create a new test from template
rpack test --def path/to/rpackdef --init my-test-name

# This creates:
#   rpackdef/tests/my-test-name/run.sh (populated with template)
```

## Complete Example: Full Stack Test Suite

For a definition that distributes `.editorconfig`, `.gitignore` (merged), and `ci.yml` (templated):

### `tests/success-basic/run.sh`

```bash
#!/bin/bash
set -e
DEFDIR="$1"
OUTDIR="$2"

# Create test fixture for custom gitignore
echo "# custom rules" > "$OUTDIR/.gitignore_custom"
echo "node_modules/" >> "$OUTDIR/.gitignore_custom"
echo ".env" >> "$OUTDIR/.gitignore_custom"

# Run rpack with valid values and input
rpack run --def "$DEFDIR" \
  --set go_version="1.22.3" \
  --set repo_name="my-repo" \
  --set-input gitignore_custom="$OUTDIR/.gitignore_custom" \
  --output-dir "$OUTDIR" \
  --force

# Verify success
jq -e '.success' "$OUTDIR/meta.json" \
  || { echo "FAIL: $(jq -r '.error' "$OUTDIR/meta.json")"; exit 1; }

# Check all expected output files
test -f "$OUTDIR/.editorconfig" \
  || { echo "FAIL: .editorconfig not created"; exit 1; }
test -f "$OUTDIR/.gitignore" \
  || { echo "FAIL: .gitignore not created"; exit 1; }
test -f "$OUTDIR/.github/workflows/ci.yml" \
  || { echo "FAIL: ci.yml not created"; exit 1; }
test -f "$OUTDIR/.github/policybot.yml" \
  || { echo "FAIL: policybot.yml not created"; exit 1; }

# Verify static file content
grep -q "end_of_line" "$OUTDIR/.editorconfig" \
  || { echo "FAIL: editorconfig missing expected content"; exit 1; }

# Verify templated values
grep -q "go-version: 1.22.3" "$OUTDIR/.github/workflows/ci.yml" \
  || { echo "FAIL: go_version not templated into ci.yml"; exit 1; }
grep -q "repo: my-repo" "$OUTDIR/.github/policybot.yml" \
  || { echo "FAIL: repo_name not templated into policybot.yml"; exit 1; }

# Verify merged content
grep -q "node_modules" "$OUTDIR/.gitignore" \
  || { echo "FAIL: custom rules not merged into .gitignore"; exit 1; }

# Verify files_read tracks all bundle files
jq -e '(.files_read | index("rpack:files/.editorconfig"))' "$OUTDIR/meta.json" \
  || { echo "FAIL: .editorconfig not in files_read"; exit 1; }
jq -e '(.files_read | index("rpack:files/gitignore.base"))' "$OUTDIR/meta.json" \
  || { echo "FAIL: gitignore.base not in files_read"; exit 1; }

echo "PASS: success-basic"
```

### `tests/schema-validation/run.sh`

```bash
#!/bin/bash
set -e
DEFDIR="$1"
OUTDIR="$2"

# Provide valid custom input (needed to reach schema validation)
echo "# custom" > "$OUTDIR/.gitignore_custom"

# Test 1: Invalid go_version (wrong format)
if rpack run --def "$DEFDIR" \
  --set go_version="v1.22.3" \
  --set repo_name="my-repo" \
  --set-input gitignore_custom="$OUTDIR/.gitignore_custom" \
  --output-dir "$OUTDIR"; then
  echo "FAIL: Test 1 - rpack should have failed with bad go_version format"
  exit 1
fi
jq -e '.error_phase == "schema_validation"' "$OUTDIR/meta.json" \
  || { echo "FAIL: Test 1 - wrong error phase: $(jq -r '.error_phase' "$OUTDIR/meta.json")"; exit 1; }

# Test 2: Invalid repo_name (empty string)
rm -rf "$OUTDIR"/*
if rpack run --def "$DEFDIR" \
  --set go_version="1.22.3" \
  --set repo_name="" \
  --set-input gitignore_custom="$OUTDIR/.gitignore_custom" \
  --output-dir "$OUTDIR"; then
  echo "FAIL: Test 2 - rpack should have failed with empty repo_name"
  exit 1
fi
jq -e '.error_phase == "schema_validation"' "$OUTDIR/meta.json" \
  || { echo "FAIL: Test 2 - wrong error phase: $(jq -r '.error_phase' "$OUTDIR/meta.json")"; exit 1; }

echo "PASS: schema-validation"
```

### `tests/missing-input/run.sh`

```bash
#!/bin/bash
set -e
DEFDIR="$1"
OUTDIR="$2"

# Run WITHOUT optional custom gitignore input — should succeed
rpack run --def "$DEFDIR" \
  --set go_version="1.22.3" \
  --set repo_name="my-repo" \
  --output-dir "$OUTDIR" \
  --force

# Verify it succeeds even without optional input
jq -e '.success' "$OUTDIR/meta.json" \
  || { echo "FAIL: rpack should succeed without optional input: $(jq -r '.error' "$OUTDIR/meta.json")"; exit 1; }

# Verify .gitignore still has base content
test -f "$OUTDIR/.gitignore" \
  || { echo "FAIL: .gitignore not created without custom input"; exit 1; }

# The optional input check pattern uses rpack.inputs() iteration, not has_input()
# See the Lua script for "rpack.inputs()"

echo "PASS: missing-input"
```

## Tips

- Write one test per scenario: success path, each invalid value type gets its own test, each error mode gets its own test
- Use `--force` whenever OUTDIR might contain fixture files (purity check prevents overwriting)
- Always check `meta.json` for execution status before checking output files
- Use descriptive test directory names: `success-basic`, `schema-go-version`, `missing-gitignore-custom`
- Place test fixtures alongside the test script, reference via `$DEFDIR/tests/<test-name>/fixture`
- If you need to test with temporary fixture files, create them in `$OUTDIR` (within the sandbox)
- Run `rpack test --def` after creating tests to verify they all pass

**Run rpack manually first:** Before writing test assertions, run `rpack run --def <path> --output-dir /tmp/test-out` once and inspect the actual output files. This helps you:
- See the exact format of templated values (quoting, spacing)
- Verify file paths are correct
- Construct accurate grep patterns

Template quoting (`"{{ .value }}"` vs `{{ .value }}`) affects grep patterns, so seeing the real output prevents mismatches.

### Passing Array Values

The `--set` flag parses values as YAML and does NOT support JSON array syntax on the command line. To pass array/list values, use repeated `--set` flags:

```bash
# BAD: JSON array syntax (fails with YAML parse error)
--set extra_patterns='["*.log","*.tmp"]'

# GOOD: Repeated --set for each array element
--set 'extra_patterns=*.log' \
--set 'extra_patterns=*.tmp' \
--set 'extra_patterns=coverage/'
```
