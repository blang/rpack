#!/bin/bash
# Test: schema-validation
# Verifies rpack rejects invalid author value (number instead of string)
set -e
DEFDIR="$1"
OUTDIR="$2"

# Create a valid users.yaml fixture
cat > "$OUTDIR/users.yaml" <<EOF
- firstname: Test
  lastname: User
EOF

# Run rpack with author as number (schema requires string) - this should fail
if rpack run --def "$DEFDIR" \
  --set author=123 \
  --set-input users.yaml="$OUTDIR/users.yaml" \
  --output-dir "$OUTDIR"; then
  echo "FAIL: rpack should have failed with invalid author type"
  exit 1
fi

# Check that meta.json indicates failure
jq -e '.success == false' "$OUTDIR/meta.json" \
  || { echo "FAIL: meta.json should show success=false"; exit 1; }

# Verify error_phase is schema_validation
jq -e '.error_phase == "schema_validation"' "$OUTDIR/meta.json" \
  || { echo "FAIL: error_phase should be schema_validation, got $(jq -r '.error_phase' "$OUTDIR/meta.json")"; exit 1; }

echo "PASS: schema-validation"
