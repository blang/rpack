package rpack

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blang/rpack/pkg/rpack/util"
)

// Repeated string literals asserted across tests.
const (
	happyPayload     = "hello"
	phaseLuaExec     = "lua_execution"
	phasePurityCheck = "purity_check"
)

// --- Fixtures ---------------------------------------------------------------

// writeDef creates a minimal rpack definition directory under t.TempDir() and
// returns its absolute path. files maps relative paths to content; "rpack.yaml"
// and "script.lua" are required keys supplied by callers.
func writeDef(t *testing.T, files map[string]string) string {
	t.Helper()
	defDir := t.TempDir()
	for rel, content := range files {
		p := filepath.Join(defDir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil { //nolint:gosec // test fixture
			t.Fatalf("mkdir %s: %v", filepath.Dir(p), err)
		}
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil { //nolint:gosec // test fixture
			t.Fatalf("write %s: %v", p, err)
		}
	}
	return defDir
}

// minimalDefYAML returns a valid rpack definition body for a name (no inputs).
func minimalDefYAML(name string) string {
	return "\"@schema_version\": \"v1\"\nname: \"" + name + "\"\n"
}

// runDirect runs ExecRPackDirect against defDir with cwd set to target, so the
// terminal copyDir(runDir, ".") lands inside the test scratch dir.
func runDirect(t *testing.T, e *Executor, defDir string, inputs map[string]string, target string) error {
	t.Helper()
	t.Chdir(target)
	return e.ExecRPackDirect(context.Background(), defDir, nil, inputs)
}

// loadMeta reads and parses meta.json from dir.
func loadMeta(t *testing.T, dir string) map[string]any {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, "meta.json")) //nolint:gosec // test
	if err != nil {
		t.Fatalf("read meta.json: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal meta.json: %v", err)
	}
	return m
}

// fileExists reports whether path exists (file or dir).
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil { //nolint:gosec // test fixture
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil { //nolint:gosec // test fixture
		t.Fatal(err)
	}
}

// --- ExecRPackDirect: end-to-end -------------------------------------------

func TestExecRPackDirect_HappyPath_WritesToCWD(t *testing.T) {
	defDir := writeDef(t, map[string]string{
		"rpack.yaml": minimalDefYAML("happy"),
		"script.lua": `local rpack = require("rpack.v1"); rpack.write("./out.txt", "` + happyPayload + `")`,
	})
	target := t.TempDir()
	if err := runDirect(t, &Executor{}, defDir, nil, target); err != nil {
		t.Fatalf("ExecRPackDirect: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(target, "out.txt")) //nolint:gosec // test
	if err != nil {
		t.Fatalf("out.txt missing: %v", err)
	}
	if string(got) != happyPayload {
		t.Fatalf("out.txt = %q, want %q", got, happyPayload)
	}
}

func TestExecRPackDirect_DryRun_DoesNotWriteToCWD(t *testing.T) {
	defDir := writeDef(t, map[string]string{
		"rpack.yaml": minimalDefYAML("dry"),
		"script.lua": `local rpack = require("rpack.v1"); rpack.write("./out.txt", "dry-content")`,
	})
	target := t.TempDir()
	e := &Executor{DryRun: true}
	if err := runDirect(t, e, defDir, nil, target); err != nil {
		t.Fatalf("ExecRPackDirect dry-run: %v", err)
	}
	if fileExists(filepath.Join(target, "out.txt")) {
		t.Fatal("dry-run wrote out.txt into cwd; it must only print")
	}
}

func TestExecRPackDirect_DryRun_OutputDirCopiesFilesAndMeta(t *testing.T) {
	defDir := writeDef(t, map[string]string{
		"rpack.yaml": minimalDefYAML("dryout"),
		"script.lua": `local rpack = require("rpack.v1"); rpack.write("./out.txt", "x")`,
	})
	target := t.TempDir()
	out := filepath.Join(target, "out")
	e := &Executor{DryRun: true, OutputDir: out}
	if err := runDirect(t, e, defDir, nil, target); err != nil {
		t.Fatalf("ExecRPackDirect dry-run+output-dir: %v", err)
	}
	if !fileExists(filepath.Join(out, "out.txt")) {
		t.Fatal("dry-run+output-dir did not copy out.txt")
	}
	meta := loadMeta(t, out)
	if meta["success"] != true {
		t.Fatalf("meta success = %v, want true", meta["success"])
	}
}

func TestExecRPackDirect_OutputDirNonEmpty_RefusedThenForce(t *testing.T) {
	defDir := writeDef(t, map[string]string{
		"rpack.yaml": minimalDefYAML("odir"),
		"script.lua": `local rpack = require("rpack.v1"); rpack.write("./out.txt", "x")`,
	})
	target := t.TempDir()
	out := filepath.Join(target, "out")
	writeFile(t, filepath.Join(out, "stale.txt"), "keep")

	// Without force: refused.
	if err := runDirect(t, &Executor{OutputDir: out}, defDir, nil, target); err == nil ||
		!strings.Contains(err.Error(), "not empty") {
		t.Fatalf("expected non-empty refusal, got %v", err)
	}

	// With force: succeeds and writes meta.json.
	if err := runDirect(t, &Executor{OutputDir: out, Force: true}, defDir, nil, target); err != nil {
		t.Fatalf("force should succeed, got %v", err)
	}
	if !fileExists(filepath.Join(out, "meta.json")) {
		t.Fatal("force did not write meta.json")
	}
}

func TestExecRPackDirect_LuaError_WritesMetaAndReturnsErr(t *testing.T) {
	defDir := writeDef(t, map[string]string{
		"rpack.yaml": minimalDefYAML("errpath"),
		"script.lua": `local rpack = require("rpack.v1"); error("boom")`,
	})
	target := t.TempDir()
	out := filepath.Join(target, "out")
	err := runDirect(t, &Executor{OutputDir: out}, defDir, nil, target)
	if err == nil {
		t.Fatal("expected error from failed script")
	}
	if !errors.Is(err, ErrLuaExecution) {
		t.Fatalf("error not ErrLuaExecution: %v", err)
	}
	meta := loadMeta(t, out)
	if meta["success"] != false {
		t.Fatalf("meta success = %v, want false", meta["success"])
	}
	if meta["error_phase"] != phaseLuaExec {
		t.Fatalf("meta error_phase = %v, want %s", meta["error_phase"], phaseLuaExec)
	}
}

func TestExecRPackDirect_LuaError_NoOutputDir_NoMeta(t *testing.T) {
	defDir := writeDef(t, map[string]string{
		"rpack.yaml": minimalDefYAML("errnometa"),
		"script.lua": `local rpack = require("rpack.v1"); error("boom")`,
	})
	target := t.TempDir()
	err := runDirect(t, &Executor{}, defDir, nil, target)
	if err == nil || !errors.Is(err, ErrLuaExecution) {
		t.Fatalf("expected ErrLuaExecution, got %v", err)
	}
	// No OutputDir → writeErrorMeta is a no-op; nothing should be created in target.
	entries, _ := os.ReadDir(target)
	if len(entries) != 0 {
		t.Fatalf("error path without OutputDir created files in cwd: %v", entries)
	}
}

func TestExecRPackDirect_PurityConflict_ReadAndWriteSameFile(t *testing.T) {
	// The purity checker only tracks map: reads against target writes (the
	// Read/Write hooks filter on MapResolver/TargetResolver). Reading an input
	// and writing the target at the same IndirectTargetPath breaks idempotency
	// and fs.Check() must reject it with ErrPurityCheck.
	defDir := writeDef(t, map[string]string{
		"rpack.yaml": "\"@schema_version\": \"v1\"\nname: \"purity\"\ninputs:\n  - name: data\n    type: file\n",
		"script.lua": `local rpack = require("rpack.v1"); local _ = rpack.read("map:data"); rpack.write("./data", "clobber")`,
	})
	target := t.TempDir()
	// The map resolver's IndirectTargetPath is the input's UserPath, so map the
	// input to a relative path "data" that equals the target write "./data".
	writeFile(t, filepath.Join(target, "data"), "payload")

	if err := runDirect(t, &Executor{}, defDir, map[string]string{"data": "data"}, target); err == nil ||
		!errors.Is(err, ErrPurityCheck) {
		t.Fatalf("expected ErrPurityCheck, got %v", err)
	}

	// The error phase must be classified as purity_check in meta.json.
	out := filepath.Join(target, "out")
	if err := runDirect(t, &Executor{OutputDir: out}, defDir, map[string]string{"data": "data"}, target); err == nil ||
		!errors.Is(err, ErrPurityCheck) {
		t.Fatalf("expected ErrPurityCheck with OutputDir, got %v", err)
	}
	if m := loadMeta(t, out); m["error_phase"] != phasePurityCheck {
		t.Fatalf("meta error_phase = %v, want %s", m["error_phase"], phasePurityCheck)
	}
}

func TestExecRPackDirect_Inputs_ResolvedAndRead(t *testing.T) {
	defDir := writeDef(t, map[string]string{
		"rpack.yaml": "\"@schema_version\": \"v1\"\nname: \"inp\"\ninputs:\n  - name: data\n    type: file\n",
		"script.lua": `local rpack = require("rpack.v1"); rpack.write("./out.txt", rpack.read("map:data"))`,
	})
	target := t.TempDir()
	inFile := filepath.Join(target, "data.txt")
	writeFile(t, inFile, "payload")
	err := runDirect(t, &Executor{}, defDir, map[string]string{"data": inFile}, target)
	if err != nil {
		t.Fatalf("ExecRPackDirect with input: %v", err)
	}
	got, _ := os.ReadFile(filepath.Join(target, "out.txt")) //nolint:gosec // test
	if string(got) != "payload" {
		t.Fatalf("out.txt = %q, want payload", got)
	}
}

func TestExecRPackDirect_InputMissingAbsolutePath_Error(t *testing.T) {
	defDir := writeDef(t, map[string]string{
		"rpack.yaml": "\"@schema_version\": \"v1\"\nname: \"inp2\"\ninputs:\n  - name: data\n    type: file\n",
		"script.lua": `local rpack = require("rpack.v1"); rpack.write("./out.txt", rpack.read("map:data"))`,
	})
	target := t.TempDir()
	err := runDirect(t, &Executor{}, defDir, map[string]string{"data": "/nonexistent/path-xyz"}, target)
	if err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("expected does-not-exist error, got %v", err)
	}
}

// --- ExecRPack: closed-loop with lockfile -----------------------------------

// setupExecRPackEnv builds a sibling rpackdef/ + use/ layout under a temp root
// and returns (defDir, useDir, configPath). The config references
// `source: "../rpackdef"` so local-source fetch resolves when cwd is useDir.
func setupExecRPackEnv(t *testing.T, scriptBody string) (defDir, useDir, configPath string) {
	t.Helper()
	root := t.TempDir()
	defDir = filepath.Join(root, "rpackdef")
	writeFile(t, filepath.Join(defDir, "rpack.yaml"), minimalDefYAML("execrpcl"))
	writeFile(t, filepath.Join(defDir, "script.lua"), scriptBody)
	useDir = filepath.Join(root, "use")
	if err := os.MkdirAll(useDir, 0o750); err != nil { //nolint:gosec // test fixture
		t.Fatal(err)
	}
	configPath = filepath.Join(useDir, "app.rpack.yaml")
	writeFile(t, configPath, "\"@schema_version\": \"v1\"\nsource: \"../rpackdef\"\nconfig:\n  values: {}\n")
	return defDir, useDir, configPath
}

func runExecRPack(t *testing.T, force bool, configPath, useDir string) error {
	t.Helper()
	t.Chdir(useDir)
	e := &Executor{Force: force}
	return e.ExecRPack(context.Background(), configPath)
}

func TestExecRPack_NormalMode_NoLockfile_ProducesFilesAndLockfile(t *testing.T) {
	_, useDir, cfg := setupExecRPackEnv(t,
		"local rpack = require(\"rpack.v1\"); rpack.write('./a.txt', 'A')")

	if err := runExecRPack(t, false, cfg, useDir); err != nil {
		t.Fatalf("ExecRPack: %v", err)
	}
	got, _ := os.ReadFile(filepath.Join(useDir, "a.txt")) //nolint:gosec // test
	if string(got) != "A" {
		t.Fatalf("a.txt = %q, want A", got)
	}
	if !fileExists(filepath.Join(useDir, "app.rpack.lock.yaml")) {
		t.Fatal("lockfile not written")
	}
}

func TestExecRPack_NormalMode_ModifiedLockfileRefusedThenForce(t *testing.T) {
	_, useDir, cfg := setupExecRPackEnv(t,
		"local rpack = require(\"rpack.v1\"); rpack.write('./a.txt', 'A')")

	// Run 1: produces a.txt and lockfile.
	if err := runExecRPack(t, false, cfg, useDir); err != nil {
		t.Fatalf("run1: %v", err)
	}

	// Tamper with the managed file out-of-band.
	if err := os.WriteFile(filepath.Join(useDir, "a.txt"), []byte("TAMPERED"), 0o600); err != nil { //nolint:gosec // test fixture
		t.Fatal(err)
	}

	// Run 2 without force: integrity guard must refuse.
	err := runExecRPack(t, false, cfg, useDir)
	if err == nil || !strings.Contains(err.Error(), "modified outside of rpack") {
		t.Fatalf("expected modified-lockfile refusal, got %v", err)
	}

	// Run 2 with force: proceeds and reproduces a.txt.
	if err := runExecRPack(t, true, cfg, useDir); err != nil {
		t.Fatalf("run2 force: %v", err)
	}
	got, _ := os.ReadFile(filepath.Join(useDir, "a.txt")) //nolint:gosec // test
	if string(got) != "A" {
		t.Fatalf("a.txt = %q, want A after force", got)
	}
}

func TestExecRPack_NormalMode_RemovedFileCleanup(t *testing.T) {
	defDir, useDir, cfg := setupExecRPackEnv(t,
		"local rpack = require(\"rpack.v1\"); rpack.write('./a.txt', 'A')")

	// Run 1: produces a.txt.
	if err := runExecRPack(t, false, cfg, useDir); err != nil {
		t.Fatalf("run1: %v", err)
	}
	if !fileExists(filepath.Join(useDir, "a.txt")) {
		t.Fatal("a.txt not produced in run1")
	}

	// Edit the source script to drop a.txt and emit b.txt instead. Local-source
	// fetch resolves to this directory live (symlink), so run2 sees the new script.
	if err := os.WriteFile(filepath.Join(defDir, "script.lua"),
		[]byte(`local rpack = require("rpack.v1"); rpack.write("./b.txt", "B")`), 0o600); err != nil { //nolint:gosec // test fixture
		t.Fatal(err)
	}

	if err := runExecRPack(t, false, cfg, useDir); err != nil {
		t.Fatalf("run2: %v", err)
	}

	// a.txt (no longer maintained) must be cleaned up; b.txt must be present.
	if fileExists(filepath.Join(useDir, "a.txt")) {
		t.Error("a.txt was not removed by lockfile cleanup")
	}
	if !fileExists(filepath.Join(useDir, "b.txt")) {
		t.Fatal("b.txt not produced in run2")
	}
}

// --- extract helpers --------------------------------------------------------

func TestComputeFilesToMove_DedupesDuplicateWrites(t *testing.T) {
	runDir := t.TempDir()
	// defDir/tempDir are not accessed by this scenario; reuse runDir's parent.
	defDir := t.TempDir()
	fs := NewRPackFS(true, defDir, runDir, defDir, "", nil)

	if err := fs.Write("./dup.txt", []byte("content")); err != nil {
		t.Fatalf("write1: %v", err)
	}
	if err := fs.Write("./dup.txt", []byte("content2")); err != nil {
		t.Fatalf("write2: %v", err)
	}

	files, checksums, err := computeFilesToMove(fs, runDir)
	if err != nil {
		t.Fatalf("computeFilesToMove: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("got %d files, want 1 (dedup)", len(files))
	}
	absPath := filepath.Join(runDir, "dup.txt")
	if _, ok := checksums[absPath]; !ok {
		t.Fatalf("checksum for %s missing", absPath)
	}
	want, _ := util.Sha256File(absPath)
	if checksums[absPath] != want {
		t.Fatalf("checksum mismatch: got %q want %q", checksums[absPath], want)
	}
	// The deduped file keeps the content of the last write.
	got, _ := os.ReadFile(absPath) //nolint:gosec // test
	if string(got) != "content2" {
		t.Fatalf("dup.txt = %q, want content2", got)
	}
}

func TestComputeFilesToMove_MissingFileOnDisk_ReturnsChecksumError(t *testing.T) {
	// A target handle whose backing file was never written should surface a
	// checksum error rather than silently producing an empty lockfile entry.
	runDir := t.TempDir()
	defDir := t.TempDir()
	fs := NewRPackFS(true, defDir, runDir, defDir, "", nil)

	// Resolve a handle without writing its backing file by calling the recorder
	// directly via a controlled Write that fails to persist is not possible; use
	// a real write then delete the file to simulate the missing-file edge.
	if err := fs.Write("./gone.txt", []byte("x")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(runDir, "gone.txt")); err != nil {
		t.Fatal(err)
	}
	_, _, err := computeFilesToMove(fs, runDir)
	if err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Fatalf("expected checksum error, got %v", err)
	}
}

func TestBuildNewLockfile_RecordsPathsAndChecksums(t *testing.T) {
	dir := t.TempDir()
	p1 := filepath.Join(dir, "a.txt")
	p2 := filepath.Join(dir, "sub", "b.txt")
	writeFile(t, p1, "alpha")
	writeFile(t, p2, "beta")
	c1, _ := util.Sha256File(p1)
	c2, _ := util.Sha256File(p2)

	files := []*ControlledFile{
		{Path: "a.txt", AbsPath: p1},
		{Path: "sub/b.txt", AbsPath: p2},
	}
	checksums := map[string]string{p1: c1, p2: c2}

	lf := buildNewLockfile(files, checksums)
	if len(lf.Files) != 2 {
		t.Fatalf("lockfile has %d files, want 2", len(lf.Files))
	}
	byPath := map[string]string{}
	for _, f := range lf.Files {
		byPath[f.Path] = f.Sha
	}
	if byPath["a.txt"] != c1 || byPath["sub/b.txt"] != c2 {
		t.Fatalf("lockfile entries mismatch: %+v", byPath)
	}
}

func TestBuildNewLockfile_MissingChecksum_Panics(t *testing.T) {
	// Invariant guard: a produced file without a paired checksum is a caller bug
	// and must fail loudly, not silently.
	dir := t.TempDir()
	p1 := filepath.Join(dir, "a.txt")
	writeFile(t, p1, "alpha")
	c1, _ := util.Sha256File(p1)

	files := []*ControlledFile{
		{Path: "a.txt", AbsPath: p1},
		{Path: "ghost.txt", AbsPath: filepath.Join(dir, "ghost.txt")}, // no checksum
	}
	checksums := map[string]string{p1: c1}

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for missing checksum")
		}
	}()
	_ = buildNewLockfile(files, checksums)
}

func TestGuardAddedFiles_RefusesExistingWithoutForce(t *testing.T) {
	execPath := t.TempDir()
	writeFile(t, filepath.Join(execPath, "existing.txt"), "keep")

	added := []string{"existing.txt"}

	if err := (&Executor{Force: false}).guardAddedFiles(execPath, added); err == nil ||
		!strings.Contains(err.Error(), "use force flag to ignore") {
		t.Fatalf("expected force refusal, got %v", err)
	}
	if err := (&Executor{Force: true}).guardAddedFiles(execPath, added); err != nil {
		t.Fatalf("force should permit overwrite, got %v", err)
	}
}

func TestGuardAddedFiles_NewFileNoError(t *testing.T) {
	execPath := t.TempDir()
	added := []string{"brandnew.txt"}
	if err := (&Executor{Force: false}).guardAddedFiles(execPath, added); err != nil {
		t.Fatalf("new file should not error, got %v", err)
	}
}

func TestAssertOutputDirEmpty(t *testing.T) {
	t.Run("missing dir is empty", func(t *testing.T) {
		d := filepath.Join(t.TempDir(), "nope")
		if err := assertOutputDirEmpty(d, false); err != nil {
			t.Fatalf("missing dir should be treated as empty: %v", err)
		}
	})
	t.Run("empty dir ok", func(t *testing.T) {
		d := t.TempDir()
		if err := assertOutputDirEmpty(d, false); err != nil {
			t.Fatalf("empty dir: %v", err)
		}
	})
	t.Run("non-empty refused without force", func(t *testing.T) {
		d := t.TempDir()
		writeFile(t, filepath.Join(d, "x.txt"), "x")
		if err := assertOutputDirEmpty(d, false); err == nil ||
			!strings.Contains(err.Error(), "not empty") {
			t.Fatalf("expected non-empty refusal, got %v", err)
		}
	})
	t.Run("non-empty allowed with force", func(t *testing.T) {
		d := t.TempDir()
		writeFile(t, filepath.Join(d, "x.txt"), "x")
		if err := assertOutputDirEmpty(d, true); err != nil {
			t.Fatalf("force should allow non-empty: %v", err)
		}
	})
}

func TestResolveDirectInputs_RelativeResolvedToCWD(t *testing.T) {
	cwd := t.TempDir()
	t.Chdir(cwd)
	writeFile(t, filepath.Join(cwd, "rel.txt"), "rel")
	in, err := resolveDirectInputs(map[string]string{"a": "rel.txt"})
	if err != nil {
		t.Fatalf("resolveDirectInputs: %v", err)
	}
	if len(in) != 1 || in[0].Name != "a" {
		t.Fatalf("got %+v", in)
	}
	if in[0].ResolvedPath != filepath.Join(cwd, "rel.txt") {
		t.Fatalf("ResolvedPath = %q, want %q", in[0].ResolvedPath, filepath.Join(cwd, "rel.txt"))
	}
	if in[0].Type != RPackInputTypeFile {
		t.Fatalf("Type = %q, want file", in[0].Type)
	}
}

func TestResolveDirectInputs_DirectoryTypeDetected(t *testing.T) {
	d := t.TempDir()
	sub := filepath.Join(d, "subdir")
	if err := os.MkdirAll(sub, 0o750); err != nil { //nolint:gosec // test fixture
		t.Fatal(err)
	}
	in, err := resolveDirectInputs(map[string]string{"d": sub})
	if err != nil {
		t.Fatalf("resolveDirectInputs: %v", err)
	}
	if in[0].Type != RPackInputTypeDirectory {
		t.Fatalf("Type = %q, want dir", in[0].Type)
	}
}

func TestResolveDirectInputs_MissingPathErrors(t *testing.T) {
	_, err := resolveDirectInputs(map[string]string{"x": "/nonexistent/zzz-abc"})
	if err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("expected does-not-exist error, got %v", err)
	}
}

// --- classifyError ----------------------------------------------------------

func TestClassifyError(t *testing.T) {
	if got := classifyError(nil); got != "" {
		t.Fatalf("nil -> %q, want empty", got)
	}
	if got := classifyError(ErrLuaExecution); got != "lua_execution" {
		t.Fatalf("lua -> %q", got)
	}
	if got := classifyError(ErrPurityCheck); got != "purity_check" {
		t.Fatalf("purity -> %q", got)
	}
	if got := classifyError(ErrSchemaValidation); got != "schema_validation" {
		t.Fatalf("schema -> %q", got)
	}
	if got := classifyError(ErrInputValidation); got != "input_validation" {
		t.Fatalf("input -> %q", got)
	}
	if got := classifyError(errors.New("misc")); got != "unknown" {
		t.Fatalf("misc -> %q, want unknown", got)
	}
}
