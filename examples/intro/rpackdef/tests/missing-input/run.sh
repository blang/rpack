#!/bin/bash
# Test: missing-input
# Verifies rpack fails gracefully when required input users.yaml is not provided
set -e
DEFDIR="$1"
OUTDIR="$2"

# Run rpack WITHOUT providing users.yaml input - this should fail
# users.yaml is declared optional: false, so rpack rejects the run before Lua
if rpack run --def "$DEFDIR" \
  --set author="Test Author" \
  --output-dir "$OUTDIR"; then
  echo "FAIL: rpack should have failed without users.yaml input"
  exit 1
fi

# Check that meta.json indicates failure
jq -e '.success == false' "$OUTDIR/meta.json" \
  || { echo "FAIL: meta.json should show success=false"; exit 1; }

# Verify error_phase is input_validation (required input rejected before Lua)
jq -e '.error_phase == "input_validation"' "$OUTDIR/meta.json" \
  || { echo "FAIL: error_phase should be input_validation, got $(jq -r '.error_phase' "$OUTDIR/meta.json")"; exit 1; }

# Verify error message mentions users.yaml
jq -e '.error | test("users.yaml")' "$OUTDIR/meta.json" \
  || { echo "FAIL: error should mention users.yaml"; exit 1; }

echo "PASS: missing-input"
