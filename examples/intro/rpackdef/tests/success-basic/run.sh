#!/bin/bash
# Test: success-basic
# Verifies the intro rpackdef produces correct output with valid values and input
set -e
DEFDIR="$1"
OUTDIR="$2"

# Copy fixture to output directory (within sandbox)
cp "$DEFDIR/tests/success-basic/users.yaml" "$OUTDIR/users.yaml"

# Run rpack with valid values and input
rpack run --def "$DEFDIR" \
  --set author="Test Author" \
  --set-input users.yaml="$OUTDIR/users.yaml" \
  --output-dir "$OUTDIR" \
  --force

# Check execution succeeded
jq -e '.success' "$OUTDIR/meta.json" \
  || { echo "FAIL: $(jq -r '.error' "$OUTDIR/meta.json")"; exit 1; }

# Check rpack_intro.md was created and contains intro content
test -f "$OUTDIR/rpack_intro.md" \
  || { echo "FAIL: rpack_intro.md not created"; exit 1; }
grep -q "intro" "$OUTDIR/rpack_intro.md" \
  || { echo "FAIL: rpack_intro.md missing expected content"; exit 1; }

# Check rpack_users.md was created and contains templated content
test -f "$OUTDIR/rpack_users.md" \
  || { echo "FAIL: rpack_users.md not created"; exit 1; }

# Verify author value was templated
grep -q "Test Author" "$OUTDIR/rpack_users.md" \
  || { echo "FAIL: rpack_users.md missing author"; exit 1; }

# Verify users from input were templated
grep -q "Alice Johnson" "$OUTDIR/rpack_users.md" \
  || { echo "FAIL: rpack_users.md missing Alice Johnson"; exit 1; }
grep -q "Bob Smith" "$OUTDIR/rpack_users.md" \
  || { echo "FAIL: rpack_users.md missing Bob Smith"; exit 1; }
grep -q "alice.johnson@example.com" "$OUTDIR/rpack_users.md" \
  || { echo "FAIL: rpack_users.md missing Alice's email"; exit 1; }

echo "PASS: success-basic"
