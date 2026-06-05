#!/bin/bash
set -uo pipefail
cd "$(dirname "$0")/.."

TMPDIR=$(mktemp -d)
trap 'rm -rf $TMPDIR' EXIT
cp -r examples/copy_files/rpackdef "$TMPDIR/"

RPACK=$(pwd)/dist/rpack-linux-amd64
if [ ! -f "$RPACK" ]; then
    just build 2>&1 | tail -1
fi

# --init scaffold
$RPACK test --def "$TMPDIR/rpackdef" --init smoke

# Write real test
cat > "$TMPDIR/rpackdef/tests/smoke/run.sh" <<EOF
#!/bin/bash
set -e; D="\$1"; O="\$2"
$RPACK run --def "\$D" --set copy_file1=true --set copy_file2=false --output-dir "\$O"
jq -e '.success' "\$O/meta.json"
test -f "\$O/output/file1.txt"
test ! -f "\$O/output/file2.txt"
EOF
chmod +x "$TMPDIR/rpackdef/tests/smoke/run.sh"

# Run tests
$RPACK test --def "$TMPDIR/rpackdef"
echo "exit: $?"
