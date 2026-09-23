# RPack Definition Test Example

This example demonstrates how to write tests for an rpack definition using the
`rpack test` command.

## Test Structure

Tests live in the `tests/` directory of your rpack definition. Each test is a
subdirectory containing an executable script (`run.sh`, `run.py`, or `run`).

```
rpackdef/
  tests/
    success-basic/
      run.sh              # Test script (executable)
      users.yaml          # Test fixture
    missing-input/
      run.sh              # Test that verifies error handling
    schema-validation/
      run.sh              # Test that verifies schema enforcement
```

## Running Tests

From your project root:

```bash
rpack test --def examples/intro/rpackdef
```

Or run a specific test:

```bash
rpack test --def examples/intro/rpackdef --filter success-basic
```

## Writing Test Scripts

Test scripts receive two arguments:
- `$1` = absolute path to the definition directory
- `$2` = absolute path to a temporary output directory

Exit with code 0 for pass, non-zero for fail.

### Success Path Test

```bash
#!/bin/bash
set -e
DEFDIR="$1"
OUTDIR="$2"

# Copy test fixtures to output directory
cp "$DEFDIR/tests/success-basic/users.yaml" "$OUTDIR/users.yaml"

# Run rpack with test values
rpack run --def "$DEFDIR" \
  --set author="Test Author" \
  --set-input users.yaml="$OUTDIR/users.yaml" \
  --output-dir "$OUTDIR" \
  --force

# Verify execution succeeded
jq -e '.success' "$OUTDIR/meta.json" \
  || { echo "FAIL: $(jq -r '.error' "$OUTDIR/meta.json")"; exit 1; }

# Verify expected output files exist
test -f "$OUTDIR/rpack_intro.md" \
  || { echo "FAIL: rpack_intro.md not created"; exit 1; }

# Verify output content
grep -q "Test Author" "$OUTDIR/rpack_users.md" \
  || { echo "FAIL: author not templated"; exit 1; }
```

### Error Path Test

```bash
#!/bin/bash
set -e
DEFDIR="$1"
OUTDIR="$2"

# Run rpack without required input - should fail
if rpack run --def "$DEFDIR" \
  --set author="Test Author" \
  --output-dir "$OUTDIR"; then
  echo "FAIL: rpack should have failed without users.yaml input"
  exit 1
fi

# Verify failure was from Lua execution
jq -e '.error_phase == "lua_execution"' "$OUTDIR/meta.json" \
  || { echo "FAIL: wrong error phase"; exit 1; }

# Verify error message mentions the missing input
jq -e '.error | test("users.yaml")' "$OUTDIR/meta.json" \
  || { echo "FAIL: error should mention users.yaml"; exit 1; }
```

## Test Fixtures

Place test fixtures (input files, expected outputs) alongside your test script.
Reference them using `$DEFDIR/tests/<test-name>/filename`.

## Tips

- Use `--force` when the output directory contains fixtures
- Check `meta.json` for execution results and error details
- Use `jq` for JSON parsing or fall back to `grep` for simple checks
- Keep tests focused: one scenario per test directory
- Name tests descriptively: `success-basic`, `missing-input`, `schema-validation`
