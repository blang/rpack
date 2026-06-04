#!/bin/bash
# Integration tests for rpack run --def mode
set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
RPACK="go run $PROJECT_DIR/cmd/rpack"

PASS=0; FAIL=0
TMPDIR=$(mktemp -d)
trap 'rm -rf "$TMPDIR"' EXIT

# ── helpers ────────────────────────────────────────────────

run_silent()  { $RPACK run --def "$@" 2>/dev/null; }
run_capture() { $RPACK run --def "$@" 2>&1; }

check() {
    local name="$1"; shift
    if "$@" >/dev/null 2>&1; then echo "PASS  $name"; ((PASS++))
    else echo "FAIL  $name"; echo "      $*"; ((FAIL++)); fi
}
check_contains() {
    local name="$1" text="$2" needle="$3"
    if echo "$text" | grep -qF "$needle"; then echo "PASS  $name"; ((PASS++))
    else echo "FAIL  $name"; echo "      expected: $needle"; echo "      got:      $text"; ((FAIL++)); fi
}
check_not_contains() {
    local name="$1" text="$2" needle="$3"
    if echo "$text" | grep -qF "$needle"; then echo "FAIL  $name"; echo "      unexpected: $needle"; echo "      got:        $text"; ((FAIL++))
    else echo "PASS  $name"; ((PASS++)); fi
}

echo "=== rpack run --def tests ==="
echo ""

# ── dry-run ────────────────────────────────────────────────

OUT=$(run_capture "$PROJECT_DIR/examples/copy_files/rpackdef" \
    --set copy_file1=true --set copy_file2=false --dry-run) || true

check_contains "dry-run: separator"      "$OUT" "=== ./output/file1.txt ==="
check_contains "dry-run: content"        "$OUT" "# File 1"
check_not_contains "dry-run: no extra"   "$OUT" "file2.txt"

# ── --output-dir + meta.json ───────────────────────────────

run_silent "$PROJECT_DIR/examples/copy_files/rpackdef" \
    --set copy_file1=true --set copy_file2=true --output-dir "$TMPDIR/o1"

check "outdir: file1" test -f "$TMPDIR/o1/output/file1.txt"
check "outdir: file2" test -f "$TMPDIR/o1/output/file2.txt"

META=$(<"$TMPDIR/o1/meta.json")
check_contains "meta: success"  "$META" '"success": true'
check_contains "meta: null err" "$META" '"error": null'
check_contains "meta: phase"    "$META" '"error_phase"'
check_contains "meta: read"     "$META" 'rpack:files/file1.txt'
check_contains "meta: written"  "$META" 'output/file1.txt'

# ── error meta.json ────────────────────────────────────────

run_silent "$PROJECT_DIR/examples/intro/rpackdef" \
    --set author=test --output-dir "$TMPDIR/err" || true

ERRMETA=$(<"$TMPDIR/err/meta.json")
check_contains "err: false"   "$ERRMETA" '"success": false'
check_contains "err: lua"     "$ERRMETA" 'lua_execution'

# ── safety checks ──────────────────────────────────────────

mkdir -p "$TMPDIR/existing/sub"; touch "$TMPDIR/existing/sub/x"
OUT=$(run_capture "$PROJECT_DIR/examples/copy_files/rpackdef" \
    --set copy_file1=true --output-dir "$TMPDIR/existing") || true
check_contains "safety: non-empty" "$OUT" "not empty"

run_silent "$PROJECT_DIR/examples/copy_files/rpackdef" \
    --set copy_file1=true --output-dir "$TMPDIR/existing" --force
check "--force: ok" test -f "$TMPDIR/existing/meta.json"

# ── flag conflicts ─────────────────────────────────────────

OUT=$(run_capture "$PROJECT_DIR/examples/copy_files/rpackdef" \
    --set copy_file1=true --output-dir /tmp/x --dry-run) || true
check_contains "conflict: out+run" "$OUT" "mutually exclusive"

OUT=$(run_capture "$PROJECT_DIR/examples/copy_files/rpackdef" \
    "$PROJECT_DIR/examples/copy_files/use/copy.rpack.yaml") || true
check_contains "conflict: def+cfg" "$OUT" "mutually exclusive"

OUT=$($RPACK run --set author=test 2>&1) || true
check_contains "conflict: no-def" "$OUT" "def"

# ── --set: value types ─────────────────────────────────────
# Create a dedicated test definition for value tests

VDEF="$TMPDIR/vdef"
mkdir -p "$VDEF"
cat > "$VDEF/rpack.yaml" <<'EOF'
"@schema_version": "v1"
name: "vals"
EOF
cat > "$VDEF/script.lua" <<'LUA'
local rpack = require("rpack.v1")
local v = rpack.values()
for k, val in pairs(v) do
    rpack.write("./k_" .. k, tostring(val) .. ":" .. type(val))
    if type(val) == "table" then
        local n = 0
        for _, item in ipairs(val) do n = n + 1 end
        rpack.write("./cnt_" .. k, tostring(n))
    end
end
LUA

# Scalars
run_silent "$VDEF" --set name=Alice --set count=42 --set enabled=true \
    --set ratio=2.5 --output-dir "$TMPDIR/types"
check_contains "val: string" "$(<"$TMPDIR/types/k_name")"    "Alice:string"
check_contains "val: int"    "$(<"$TMPDIR/types/k_count")"   "42:number"
check_contains "val: bool"   "$(<"$TMPDIR/types/k_enabled")" "true:boolean"
check_contains "val: float"  "$(<"$TMPDIR/types/k_ratio")"   "2.5:number"

# Nested object
run_silent "$VDEF" --set nested.key=nestedval --output-dir "$TMPDIR/nest"
check "val: nested file" test -f "$TMPDIR/nest/k_nested"

# Duplicate keys → list
run_silent "$VDEF" --set list=first --set list=second --set list=third \
    --output-dir "$TMPDIR/dup"
check_contains "val: dup cnt" "$(<"$TMPDIR/dup/cnt_list")" "3"

# Index notation → list
run_silent "$VDEF" --set idx.0=zero --set idx.1=one --set idx.2=two \
    --output-dir "$TMPDIR/idx"
check_contains "val: idx cnt" "$(<"$TMPDIR/idx/cnt_idx")" "3"

# ── --set-input ────────────────────────────────────────────
# Separate definition for input tests

IDEF="$TMPDIR/idef"
mkdir -p "$IDEF"
echo "hello fixture" > "$TMPDIR/fix.txt"
cat > "$IDEF/rpack.yaml" <<'EOF'
"@schema_version": "v1"
name: "inputs"
inputs:
  - name: testfile
    type: file
EOF
cat > "$IDEF/script.lua" <<'LUA'
local rpack = require("rpack.v1")
rpack.write("./out.txt", rpack.read("map:testfile"))
LUA

run_silent "$IDEF" --set-input testfile="$TMPDIR/fix.txt" --output-dir "$TMPDIR/inp"
check_contains "input: ok" "$(<"$TMPDIR/inp/out.txt")" "hello fixture"

OUT=$(run_capture "$IDEF" --set-input testfile=/nonexistent --output-dir /tmp/x) || true
check_contains "input: err" "$OUT" "does not exist"

# ── normal mode backwards compat ───────────────────────────
# Run from the example's use directory so relative source ../rpackdef resolves
OUT=$(cd "$PROJECT_DIR/examples/copy_files/use" && $RPACK run ./copy.rpack.yaml --dry-run 2>&1) || true
check_contains "compat: normal" "$OUT" "==="

# ── report ─────────────────────────────────────────────────
echo ""
echo "=== $PASS passed, $FAIL failed ==="
(( FAIL > 0 )) && exit 1 || exit 0
