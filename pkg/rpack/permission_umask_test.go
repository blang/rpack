//go:build linux || darwin

package rpack

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"

	"github.com/blang/rpack/pkg/rpack/util"
)

// Subprocess umask matrix (ADR 0002, issue #15). The umask is process-global
// and Go tests run in parallel, so the parent never mutates its own umask:
// every umask-dependent scenario re-executes the test binary as a child
// (forkUmaskChild), the child applies syscall.Umask before any filesystem
// work (TestUmaskChildProcess), and observations return as one JSON report.

// Child-process environment contract.
const (
	umaskChildEnv    = "RPACK_TEST_UMASK_CHILD_MASK"   // octal mask; presence triggers child mode
	umaskActionEnv   = "RPACK_TEST_UMASK_CHILD_ACTION" // stage|stagefail|run|publish|git
	umaskScenarioEnv = "RPACK_TEST_UMASK_CHILD_SCENARIO"
	umaskTargetEnv   = "RPACK_TEST_UMASK_CHILD_TARGET"
	umaskDefEnv      = "RPACK_TEST_UMASK_CHILD_DEF"
	umaskOutEnv      = "RPACK_TEST_UMASK_CHILD_OUT"
	umaskForceEnv    = "RPACK_TEST_UMASK_CHILD_FORCE"
	umaskResultEnv   = "RPACK_TEST_UMASK_CHILD_RESULT"
	umaskRunEnv      = "RPACK_TEST_UMASK_CHILD_RUN" // known run dir for staged observation
)

// umaskGitScrubEnvVars mirrors the repository-locator variables scrubbed by
// the go-getter wrapper (pkg/rpack/getsource/git_getter.go).
var umaskGitScrubEnvVars = []string{
	"GIT_DIR", "GIT_WORK_TREE", "GIT_COMMON_DIR", "GIT_INDEX_FILE",
	"GIT_OBJECT_DIRECTORY", "GIT_ALTERNATE_OBJECT_DIRECTORIES",
	"GIT_NAMESPACE", "GIT_QUARANTINE_PATH",
}

const twoFileScript = `local rpack = require("rpack.v1")
rpack.write("./deploy.sh", "#!/bin/sh\n", {executable = true})
rpack.write("./notes.txt", "x")
`

// umaskReport is the child's whole observation set; "" means "no error",
// nil maps mean "nothing observed".
type umaskReport struct {
	Probe      string            `json:"probe"` // mode of a 0666-created file: proves the mask is in effect
	Files      map[string]string `json:"files"`
	Locks      map[string]string `json:"locks"`
	Staged     map[string]string `json:"staged"`
	StagedMode string            `json:"staged_mode"`
	Intent     string            `json:"intent"`
	WriteErr   string            `json:"write_err"`
	RunErr     string            `json:"run_err"`
	GitErr     string            `json:"git_err"`
}

// TestUmaskChildProcess is the re-exec entry point: absent the trigger env it
// returns immediately; in a forked child it applies the umask, runs the
// action, and exits.
func TestUmaskChildProcess(t *testing.T) {
	maskStr := os.Getenv(umaskChildEnv)
	if maskStr == "" {
		return
	}
	mask, err := strconv.ParseInt(maskStr, 8, 32)
	if err != nil {
		fmt.Fprintf(os.Stderr, "bad umask %q: %v\n", maskStr, err)
		os.Exit(1)
	}
	_ = syscall.Umask(int(mask)) // the ONLY place tests may change a umask
	if err := umaskChildAction(); err != nil {
		fmt.Fprintln(os.Stderr, "umask child:", err)
		os.Exit(1)
	}
	os.Exit(0)
}

// umaskChildAction dispatches one action and writes the JSON report. Scenario
// outcomes (expected failures, observed modes) go into the report; structural
// harness failures return an error and exit 1.
func umaskChildAction() error {
	rep := &umaskReport{}
	// Probe: the mode of a freshly 0666-created file proves the child's
	// umask is really applied, guarding the harness against silent no-ops.
	if err := os.WriteFile(filepath.Join(filepath.Dir(os.Getenv(umaskResultEnv)), "probe"), []byte("p"), 0o666); err != nil { //nolint:gosec // probe file
		return err
	}
	if info, err := os.Stat(filepath.Join(filepath.Dir(os.Getenv(umaskResultEnv)), "probe")); err == nil { //nolint:gosec // probe file
		rep.Probe = FormatMode(info.Mode().Perm())
	}
	var err error
	switch os.Getenv(umaskActionEnv) {
	case "stage":
		err = umaskChildStage(rep)
	case "stagefail":
		err = umaskChildStageFail(rep)
	case "run":
		err = umaskChildRun(rep)
	case "publish":
		umaskChildPublish(rep)
	case "git":
		umaskChildGit(rep)
	default:
		return fmt.Errorf("unknown umask child action %q", os.Getenv(umaskActionEnv))
	}
	if err != nil {
		return err
	}
	return writeReport(rep)
}

// writeReport encodes the report and makes it parent-readable (the child's
// umask may deny owner-read on creation; chmod is umask-independent).
func writeReport(rep *umaskReport) error {
	resultPath := os.Getenv(umaskResultEnv)
	f, err := os.Create(resultPath) //nolint:gosec // test-owned path
	if err != nil {
		return err
	}
	encErr := json.NewEncoder(f).Encode(rep)
	if cerr := f.Close(); encErr == nil {
		encErr = cerr
	}
	if encErr != nil {
		return encErr
	}
	return os.Chmod(resultPath, 0o644) //nolint:gosec // test-owned report file
}

// umaskChildStage proves staging is private and intent survives: write+chmod
// must stage 0600 and retain declared intent 755 regardless of the umask.
func umaskChildStage(rep *umaskReport) error {
	target := os.Getenv(umaskTargetEnv)
	if err := os.MkdirAll(filepath.Join(target, "run"), 0o750); err != nil { //nolint:gosec // test fixture dirs
		return err
	}
	fs := NewRPackFS(true,
		filepath.Join(target, "def"), filepath.Join(target, "run"), filepath.Join(target, "tmp"), "", nil)
	if err := fs.Write("./s.sh", []byte("x")); err != nil {
		rep.WriteErr = err.Error()
		return nil //nolint:nilerr // expected scenario failures are returned in the JSON report
	}
	if err := fs.Chmod("./s.sh", ExecutableMode); err != nil {
		return err
	}
	info, err := os.Stat(filepath.Join(target, "run", "s.sh")) //nolint:gosec // test-owned staging path
	if err != nil {
		return err
	}
	rep.StagedMode = FormatMode(info.Mode().Perm())
	h, err := fs.resolve("./s.sh")
	if err != nil {
		return err
	}
	rep.Intent = CanonicalMode(h.OutputMode())
	return nil
}

// umaskChildStageFail records the staging write error under a read-denying umask.
func umaskChildStageFail(rep *umaskReport) error {
	target := os.Getenv(umaskTargetEnv)
	if err := os.MkdirAll(filepath.Join(target, "run"), 0o750); err != nil { //nolint:gosec // test fixture dirs
		return err
	}
	fs := NewRPackFS(true,
		filepath.Join(target, "def"), filepath.Join(target, "run"), filepath.Join(target, "tmp"), "", nil)
	rep.WriteErr = errString(fs.Write("./s.sh", []byte("x")))
	return nil
}

// umaskChildRun executes one output-path scenario end to end and records the
// physical modes of every output, the lockfile records (managed), and the
// staged files kept in the run dir (managed dry run, via the known run path).
func umaskChildRun(rep *umaskReport) error {
	target := os.Getenv(umaskTargetEnv)
	if err := os.Chdir(target); err != nil {
		return err
	}
	scenario := os.Getenv(umaskScenarioEnv)
	e := &Executor{Force: os.Getenv(umaskForceEnv) == "1"}
	ctx := context.Background()
	switch scenario {
	case "direct", "outputdir", "dryrunout":
		e.OutputDir = os.Getenv(umaskOutEnv) // empty for direct
		e.DryRun = scenario == "dryrunout"
		rep.RunErr = errString(e.ExecRPackDirect(ctx, os.Getenv(umaskDefEnv), nil, nil))
	case "managed", "manageddry":
		e.DryRun = scenario == "manageddry"
		if e.DryRun {
			e.OutputDir = os.Getenv(umaskOutEnv)
		}
		rep.RunErr = errString(e.ExecRPack(ctx, filepath.Join(target, "app.rpack.yaml")))
	default:
		return fmt.Errorf("unknown umask child scenario %q", scenario)
	}

	landing := target
	if e.OutputDir != "" {
		landing = e.OutputDir
	}
	var err error
	if rep.Files, err = dirModes(landing); err != nil {
		return fmt.Errorf("observe outputs: %w", err)
	}
	if scenario == "managed" && rep.RunErr == "" {
		lf, lerr := loadRPackLockFile(filepath.Join(target, "app.rpack.lock.yaml"))
		if lerr != nil {
			return fmt.Errorf("lockfile: %w", lerr)
		}
		rep.Locks = map[string]string{}
		for _, fe := range lf.Files {
			rep.Locks[fe.Path] = fe.Mode
		}
	}
	if scenario == "manageddry" {
		// Staged files remain in the run dir after a dry run (loader.go
		// layout .rpack.d/<sha(source)>/<sha(config)>/run, passed by parent).
		if rep.Staged, err = dirModes(os.Getenv(umaskRunEnv)); err != nil {
			return fmt.Errorf("observe staged: %w", err)
		}
	}
	return nil
}

// umaskChildPublish exercises writeOutputFile's creation-policy validation
// against a pre-existing destination directory, which a dir-execute-denying
// umask cannot break (unlike rpack-created staging directories).
func umaskChildPublish(rep *umaskReport) {
	dest := filepath.Join(os.Getenv(umaskTargetEnv), "out", "deploy.sh")
	rep.RunErr = errString(writeOutputFile(dest, []byte("#!/bin/sh\n"), ExecutableMode))
}

// umaskChildGit re-checkouts all indexed files under the child's umask with
// the repository-locator environment scrubbed.
func umaskChildGit(rep *umaskReport) {
	scrubbed := umaskEnvWithout(os.Environ(), umaskGitScrubEnvVars)
	scrubbed = append(scrubbed, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull)
	cmd := exec.Command("git", "checkout-index", "-a", "-f") //nolint:gosec // test helper with fixed args
	cmd.Dir = os.Getenv(umaskTargetEnv)
	cmd.Env = scrubbed
	if out, err := cmd.CombinedOutput(); err != nil {
		rep.GitErr = fmt.Sprintf("%v: %s", err, out)
	}
}

// dirModes returns name -> physical mode for every regular file directly in dir.
func dirModes(dir string) (map[string]string, error) {
	entries, err := os.ReadDir(dir) //nolint:gosec // test-owned path
	if err != nil {
		return nil, err
	}
	modes := map[string]string{}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, ierr := e.Info()
		if ierr != nil {
			return nil, ierr
		}
		modes[e.Name()] = FormatMode(info.Mode().Perm())
	}
	return modes, nil
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// --- parent-side harness -----------------------------------------------------

// forkUmaskChild re-executes the test binary with the given umask and action,
// decodes the JSON report, and verifies the umask probe.
func forkUmaskChild(t *testing.T, mask int, action string, extraEnv map[string]string) *umaskReport {
	t.Helper()
	resultDir := t.TempDir()
	resultPath := filepath.Join(resultDir, "result.txt")

	owned := []string{
		umaskChildEnv, umaskActionEnv, umaskScenarioEnv, umaskTargetEnv,
		umaskDefEnv, umaskOutEnv, umaskForceEnv, umaskResultEnv, umaskRunEnv,
	}
	owned = append(owned, umaskGitScrubEnvVars...)
	for k := range extraEnv {
		owned = append(owned, k)
	}
	cmdEnv := umaskEnvWithout(os.Environ(), owned)
	cmdEnv = append(cmdEnv,
		fmt.Sprintf("%s=%04o", umaskChildEnv, mask),
		umaskActionEnv+"="+action,
		umaskResultEnv+"="+resultPath,
	)
	for k, v := range extraEnv {
		cmdEnv = append(cmdEnv, k+"="+v)
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestUmaskChildProcess$") //nolint:gosec // re-executes the compiled test binary
	cmd.Env, cmd.Dir = cmdEnv, resultDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("umask child (mask %04o, action %q) failed: %v\n%s", mask, action, err, out)
	}

	b, err := os.ReadFile(resultPath) //nolint:gosec // test-owned path
	if err != nil {
		t.Fatalf("read umask child report: %v", err)
	}
	var rep umaskReport
	if err := json.Unmarshal(b, &rep); err != nil {
		t.Fatalf("decode umask child report: %v", err)
	}
	if want := fmt.Sprintf("%03o", 0o666&^mask); rep.Probe != want {
		t.Fatalf("umask child probe = %q, want %q (mask %04o not in effect?)", rep.Probe, want, mask)
	}
	return &rep
}

// umaskEnvWithout drops entries whose key is in the exclude set, preventing
// duplicate-key environments when the caller re-sets a variable.
func umaskEnvWithout(environ, exclude []string) []string {
	excluded := make(map[string]struct{}, len(exclude))
	for _, ex := range exclude {
		excluded[ex] = struct{}{}
	}
	out := make([]string, 0, len(environ))
	for _, kv := range environ {
		name, _, _ := strings.Cut(kv, "=")
		if _, skip := excluded[name]; !skip {
			out = append(out, kv)
		}
	}
	return out
}

// expectedMode returns the physical mode Git-style creation must produce
// under mask for the given canonical intent (0666/0777 &^ umask).
func expectedMode(intent string, mask int) string {
	base := 0o666
	if intent == "755" {
		base = 0o777
	}
	return fmt.Sprintf("%03o", base&^mask)
}

// assertMatrixModes checks every matrix output's exact physical mode.
func assertMatrixModes(t *testing.T, rep *umaskReport, mask int) {
	t.Helper()
	for name, intent := range matrixIntent {
		got, ok := rep.Files[name]
		if !ok {
			t.Errorf("child reported no mode for %s (mask %04o)", name, mask)
			continue
		}
		if want := expectedMode(intent, mask); got != want {
			t.Errorf("%s mode = %s, want %s (intent %s &^ umask %04o)", name, got, want, intent, mask)
		}
	}
}

// assertCanonicalLocks checks lockfile records stay canonical, never
// umask-derived, for every matrix output.
func assertCanonicalLocks(t *testing.T, rep *umaskReport, mask int) {
	t.Helper()
	for name, want := range matrixIntent {
		if got := rep.Locks[name]; got != want {
			t.Errorf("lockfile mode for %s = %q, want %q (mask %04o)", name, got, want, mask)
		}
	}
}

// requireStagedPrivate requires staged files were observed and all are 0600.
func requireStagedPrivate(t *testing.T, rep *umaskReport, mask int) {
	t.Helper()
	if len(rep.Staged) == 0 {
		t.Fatalf("no staged files observed in run dir (mask %04o)", mask)
	}
	for name, mode := range rep.Staged {
		if mode != "600" {
			t.Errorf("staged %s mode = %s, want 600 (mask %04o)", name, mode, mask)
		}
	}
}

// requireRunOK fails when the child recorded a run or git error.
func requireRunOK(t *testing.T, rep *umaskReport, mask int) {
	t.Helper()
	if rep.RunErr != "" || rep.GitErr != "" {
		t.Fatalf("child run failed (mask %04o): %s%s", mask, rep.RunErr, rep.GitErr)
	}
}

// chmodForTest changes a file's mode without touching its content, so
// pollution between runs cannot masquerade as content drift.
func chmodForTest(t *testing.T, path string, mode os.FileMode) {
	t.Helper()
	if err := os.Chmod(path, mode); err != nil { //nolint:gosec // test fixture
		t.Fatal(err)
	}
}

// umaskStageTarget pre-creates the def/run/tmp staging layout in the parent,
// so a child umask stripping directory execute bits (0100) cannot break
// staging before the scenario under test runs.
func umaskStageTarget(t *testing.T) string {
	t.Helper()
	target := t.TempDir()
	for _, sub := range []string{"def", "run", "tmp"} {
		if err := os.MkdirAll(filepath.Join(target, sub), 0o750); err != nil { //nolint:gosec // test fixture dirs
			t.Fatal(err)
		}
	}
	return target
}

// restoreDirPerms makes a tree listable again after a child run created
// directories under a read-denying umask (0755 &^ 0400 = 0355); each directory
// is chmod'd top-down before enumerating (chmod is umask-independent).
func restoreDirPerms(t *testing.T, dir string) {
	t.Helper()
	if err := os.Chmod(dir, 0o755); err != nil { //nolint:gosec // test cleanup
		t.Fatalf("restore %s: %v", dir, err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}
	for _, e := range entries {
		if e.IsDir() {
			restoreDirPerms(t, filepath.Join(dir, e.Name()))
		}
	}
}

// --- output-path matrix ------------------------------------------------------

// umaskScenario table-drives the matrix: every scenario runs matrixScript in
// a umask child, on create and on replacement.
type umaskScenario struct {
	name      string
	child     string
	managed   bool // managed run via ExecRPack (config+lock), not ExecRPackDirect
	hasOut    bool // --output-dir (or dry-run+--output-dir)
	force2    bool // replacement run needs --force (non-empty output dir)
	dryStaged bool // require observing staged files kept in the run dir
}

var umaskScenarios = []umaskScenario{
	{name: "direct to CWD", child: "direct"},
	{name: "output dir", child: "outputdir", hasOut: true, force2: true},
	{name: "dry run with output dir", child: "dryrunout", hasOut: true},
	{name: "managed config and lock", child: "managed", managed: true},
	{name: "managed dry run keeps private staging", child: "manageddry", managed: true, hasOut: true, dryStaged: true},
}

func (sc umaskScenario) setup(t *testing.T) (env map[string]string, landing string) {
	t.Helper()
	if sc.managed {
		_, useDir, _ := setupExecRPackEnv(t, matrixScript)
		env = map[string]string{umaskScenarioEnv: sc.child, umaskTargetEnv: useDir}
		landing = useDir
	} else {
		defDir := writeDef(t, map[string]string{
			"rpack.yaml": minimalDefYAML("umatrix"),
			"script.lua": matrixScript,
		})
		env = map[string]string{
			umaskScenarioEnv: sc.child,
			umaskTargetEnv:   t.TempDir(),
			umaskDefEnv:      defDir,
		}
		landing = env[umaskTargetEnv]
	}
	if sc.hasOut {
		env[umaskOutEnv] = filepath.Join(t.TempDir(), "out")
		landing = env[umaskOutEnv]
	}
	if sc.dryStaged {
		env[umaskRunEnv] = filepath.Join(env[umaskTargetEnv], ".rpack.d",
			util.Sha256String("../rpackdef"), util.Sha256String(env[umaskTargetEnv]), "run")
	}
	return env, landing
}

func (sc umaskScenario) assertReport(t *testing.T, report *umaskReport, mask int) {
	t.Helper()
	requireRunOK(t, report, mask)
	assertMatrixModes(t, report, mask)
	if sc.child == "managed" { // dry runs never write a lockfile
		assertCanonicalLocks(t, report, mask)
	}
	if sc.dryStaged {
		requireStagedPrivate(t, report, mask)
	}
}

// TestUmaskMatrix_OutputPaths proves Git-like creation bases (0666/0777 &^
// umask) on create AND replacement for every output path, canonical lockfile
// records across masks, and private staging — covering the boolean and mode
// aliases, resets, and the chmod API (matrixScript).
func TestUmaskMatrix_OutputPaths(t *testing.T) {
	for _, mask := range []int{0o022, 0o002, 0o077} {
		for _, sc := range umaskScenarios {
			t.Run(fmt.Sprintf("mask_%04o/%s", mask, sc.name), func(t *testing.T) {
				env, landing := sc.setup(t)
				r1 := forkUmaskChild(t, mask, "run", env)
				sc.assertReport(t, r1, mask)

				// Replacement: out-of-band mode pollution must not leak into
				// recreated outputs. Managed reruns have no force, so their
				// pollution stays intent-preserving (R/W-only drift).
				execPollute, plainPollute := os.FileMode(0o644), os.FileMode(0o777)
				if sc.managed {
					execPollute, plainPollute = 0o711, 0o600
				}
				chmodForTest(t, filepath.Join(landing, "exec_bool.sh"), execPollute)
				chmodForTest(t, filepath.Join(landing, "default.txt"), plainPollute)
				if sc.force2 {
					env[umaskForceEnv] = "1"
				}
				r2 := forkUmaskChild(t, mask, "run", env)
				sc.assertReport(t, r2, mask)
			})
		}
	}
}

// TestUmaskStagingPrivateUnderZeroUmask pins that private staging is
// explicit, not a umask artifact: even under umask 000 the staged inode is
// 0600, never 0666, and intent is retained.
func TestUmaskStagingPrivateUnderZeroUmask(t *testing.T) {
	for _, mask := range []int{0o000, 0o022, 0o077} {
		t.Run(fmt.Sprintf("mask_%04o", mask), func(t *testing.T) {
			r := forkUmaskChild(t, mask, "stage", map[string]string{umaskTargetEnv: umaskStageTarget(t)})
			if r.WriteErr != "" {
				t.Fatalf("stage write failed (mask %04o): %s", mask, r.WriteErr)
			}
			if r.StagedMode != "600" {
				t.Errorf("staged mode = %s, want 600 (mask %04o)", r.StagedMode, mask)
			}
			if r.Intent != "755" {
				t.Errorf("declared intent = %s, want 755", r.Intent)
			}
		})
	}
}

// --- denied creation policies ------------------------------------------------

// TestUmaskPublication_OwnerExecDenied pins the fail-before-publish contract
// under umask 0100: staging and the declared 755 intent survive, publication
// of the executable output refuses before any rename, and a managed run
// leaves the lockfile and existing outputs untouched with no residue.
//
// Note: under 0100 a managed run cannot reach publication — rpack-created
// staging directories (MkdirAll 0755 / MkdirTemp 0700) lose their
// owner-execute bit and file creation inside them fails with a raw EACCES —
// so the publication validation is exercised against a pre-existing
// destination directory (publish action).
func TestUmaskPublication_OwnerExecDenied(t *testing.T) {
	const mask = 0o100 // owner-execute denied

	// Staging and declared intent are creation-policy independent.
	rStage := forkUmaskChild(t, mask, "stage", map[string]string{umaskTargetEnv: umaskStageTarget(t)})
	if rStage.WriteErr != "" {
		t.Fatalf("stage write failed: %s", rStage.WriteErr)
	}
	if rStage.StagedMode != "600" || rStage.Intent != "755" {
		t.Fatalf("stage under 0100: staged=%s intent=%s, want 600/755 (declared intent retained)",
			rStage.StagedMode, rStage.Intent)
	}

	// Publication lands 0677 under 0100: refused before the rename.
	pub := t.TempDir()
	if err := os.MkdirAll(filepath.Join(pub, "out"), 0o750); err != nil { //nolint:gosec // test fixture dirs
		t.Fatal(err)
	}
	rPub := forkUmaskChild(t, mask, "publish", map[string]string{umaskTargetEnv: pub})
	if rPub.RunErr == "" || !strings.Contains(rPub.RunErr, "creation policy prevents executable output") {
		t.Fatalf("want creation-policy refusal, got %q", rPub.RunErr)
	}
	if fileExists(filepath.Join(pub, "out", "deploy.sh")) {
		t.Fatal("refused publication created the destination file")
	}
	requireNoResidue(t, pub)

	// A managed end-to-end run under 0100 refuses without side effects.
	_, useDir, cfg := setupExecRPackEnv(t, twoFileScript)
	if err := runExecRPack(t, false, cfg, useDir); err != nil {
		t.Fatalf("run1: %v", err)
	}
	lockPath := filepath.Join(useDir, "app.rpack.lock.yaml")
	lockBefore := string(mustReadFile(t, lockPath))
	deploy := filepath.Join(useDir, "deploy.sh")
	modeBefore := fileMode(t, deploy)

	r := forkUmaskChild(t, mask, "run", map[string]string{umaskScenarioEnv: "managed", umaskTargetEnv: useDir})
	if r.RunErr == "" {
		t.Fatal("managed run under 0100 must refuse")
	}
	if after := string(mustReadFile(t, lockPath)); after != lockBefore {
		t.Fatal("lockfile rewritten by refused run")
	}
	if got := fileMode(t, deploy); got != modeBefore {
		t.Fatalf("deploy.sh mode changed by refused run: %o", got)
	}
	requireNoResidue(t, useDir)
}

// TestUmaskPublication_OwnerReadDenied pins that umask 0400 fails a staging
// write that lands owner-unreadable, during execution and before anything is
// published.
func TestUmaskPublication_OwnerReadDenied(t *testing.T) {
	const mask = 0o400 // owner-read denied

	rStage := forkUmaskChild(t, mask, "stagefail", map[string]string{umaskTargetEnv: t.TempDir()})
	if rStage.WriteErr == "" || !strings.Contains(rStage.WriteErr, "owner-unreadable") {
		t.Fatalf("want owner-unreadable staging error, got %q", rStage.WriteErr)
	}

	_, useDir, _ := setupExecRPackEnv(t, twoFileScript)
	r := forkUmaskChild(t, mask, "run", map[string]string{umaskScenarioEnv: "managed", umaskTargetEnv: useDir})
	if r.RunErr == "" || !strings.Contains(r.RunErr, "owner-unreadable") {
		t.Fatalf("want owner-unreadable error before publish, got %q", r.RunErr)
	}
	if fileExists(filepath.Join(useDir, "deploy.sh")) {
		t.Fatal("owner-unreadable run published deploy.sh")
	}
	if fileExists(filepath.Join(useDir, "app.rpack.lock.yaml")) {
		t.Fatal("owner-unreadable run wrote a lockfile")
	}
	// The child created its cache dirs owner-unreadable (0755 &^ 0400 = 0355);
	// restore readability for residue checks and TempDir cleanup.
	restoreDirPerms(t, filepath.Join(useDir, ".rpack.d"))
	requireNoResidue(t, useDir)
}

// --- Git roundtrip -----------------------------------------------------------

// umaskTestGit runs a git command with the repository-locator environment
// scrubbed and no user/system gitconfig.
func umaskTestGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	cmdEnv := umaskEnvWithout(os.Environ(), umaskGitScrubEnvVars)
	cmdEnv = append(cmdEnv, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull)
	cmd := exec.Command("git", args...) //nolint:gosec // test helper with fixed args
	cmd.Dir = dir
	cmd.Env = cmdEnv
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// TestGitRoundtrip_UmaskDriftAccepted_ExecDriftRefused is the issue #15
// regression: a real Git checkout under a different umask changes only
// read/write bits, which check and a normal (non-force) run accept with
// content hashes unchanged; a flipped owner-execute bit is refused and healed
// by force; content and unmanaged-file guards remain. The roundtrip uses git
// init/add/checkout-index — no commits, no user/global config writes.
//
//nolint:gocognit,gocyclo // test: multi-phase roundtrip scenario
func TestGitRoundtrip_UmaskDriftAccepted_ExecDriftRefused(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	defDir, useDir, cfg := setupExecRPackEnv(t, twoFileScript)
	if err := runExecRPack(t, false, cfg, useDir); err != nil {
		t.Fatalf("run1: %v", err)
	}
	deploy := filepath.Join(useDir, "deploy.sh")
	notes := filepath.Join(useDir, "notes.txt")

	// Poison the parent environment with a bogus repository locator: the git
	// commands below succeed only because locator variables are scrubbed,
	// like the go-getter wrapper does for source fetches.
	t.Setenv("GIT_DIR", filepath.Join(t.TempDir(), "bogus-rpack-test.git"))

	// Index the outputs without any commit: git records the canonical
	// 100755/100644 modes from the index.
	umaskTestGit(t, useDir, "init", "-q")
	umaskTestGit(t, useDir, "add", "deploy.sh", "notes.txt")
	shaDeploy, shaNotes := mustSha(t, deploy), mustSha(t, notes)

	for _, mask := range []int{0o022, 0o002, 0o077} {
		t.Run(fmt.Sprintf("checkout_umask_%04o", mask), func(t *testing.T) {
			// Remove the files so checkout-index recreates fresh inodes,
			// exactly like a fresh clone under this umask would.
			for _, p := range []string{deploy, notes} {
				if err := os.Remove(p); err != nil {
					t.Fatal(err)
				}
			}
			// The child's git process runs under a different umask and a
			// poisoned repo-locator env it must scrub.
			r := forkUmaskChild(t, mask, "git", map[string]string{
				umaskTargetEnv: useDir,
				"GIT_DIR":      filepath.Join(t.TempDir(), "poison-rpack-test.git"),
			})
			if r.GitErr != "" {
				t.Fatalf("child git checkout-index failed: %s", r.GitErr)
			}
			// Git's checkout matrix: 100755 -> 0777&^umask, 100644 -> 0666&^umask.
			if got := FormatMode(fileMode(t, deploy)); got != expectedMode("755", mask) {
				t.Fatalf("deploy.sh after checkout = %s, want %s", got, expectedMode("755", mask))
			}
			if got := FormatMode(fileMode(t, notes)); got != expectedMode("644", mask) {
				t.Fatalf("notes.txt after checkout = %s, want %s", got, expectedMode("644", mask))
			}
			if mustSha(t, deploy) != shaDeploy || mustSha(t, notes) != shaNotes {
				t.Fatal("checkout changed content hashes")
			}
			// Read/write drift is not drift: check and a normal run accept it
			// without force, and the lockfile stays canonical.
			t.Chdir(useDir)
			if err := (&Checker{}).CheckIntegrity(t.Context(), cfg); err != nil {
				t.Fatalf("check must accept R/W drift after checkout: %v", err)
			}
			if err := runExecRPack(t, false, cfg, useDir); err != nil {
				t.Fatalf("normal run must accept R/W drift after checkout: %v", err)
			}
			modes := lockfileModes(t, useDir)
			if modes["deploy.sh"] != "755" || modes["notes.txt"] != "644" {
				t.Fatalf("lockfile modes = %v, want canonical 755/644", modes)
			}
		})
	}

	t.Run("owner-exec drift refused then healed", func(t *testing.T) {
		chmodForTest(t, deploy, 0o644)
		t.Chdir(useDir)
		err := (&Checker{}).CheckIntegrity(t.Context(), cfg)
		if err == nil || !strings.Contains(err.Error(), "deploy.sh (expected executable (755), found non-executable (644))") {
			t.Fatalf("expected executable-intent drift diagnostic, got %v", err)
		}
		if err := runExecRPack(t, false, cfg, useDir); err == nil ||
			!strings.Contains(err.Error(), "permissions were changed outside of rpack") {
			t.Fatalf("expected run refusal for owner-exec drift, got %v", err)
		}
		if err := runExecRPack(t, true, cfg, useDir); err != nil {
			t.Fatalf("force must heal owner-exec drift: %v", err)
		}
		if got := canonicalFileMode(t, deploy); got != "755" {
			t.Fatalf("deploy.sh intent after force heal = %q, want 755", got)
		}
	})

	t.Run("content guard remains", func(t *testing.T) {
		if err := os.WriteFile(notes, []byte("tampered"), 0o644); err != nil { //nolint:gosec // test fixture
			t.Fatal(err)
		}
		if err := runExecRPack(t, false, cfg, useDir); err == nil ||
			!strings.Contains(err.Error(), "modified outside of rpack") {
			t.Fatalf("content guard must still refuse, got %v", err)
		}
		if err := runExecRPack(t, true, cfg, useDir); err != nil {
			t.Fatalf("force must heal content drift: %v", err)
		}
	})

	t.Run("unmanaged file guard remains", func(t *testing.T) {
		writeFile(t, filepath.Join(defDir, "script.lua"), twoFileScript+`rpack.write("./new.txt", "n")
`)
		preCreateStaged(t, filepath.Join(useDir, "new.txt"), 0o644) // unmanaged on disk
		if err := runExecRPack(t, false, cfg, useDir); err == nil ||
			!strings.Contains(err.Error(), "use force flag to ignore") {
			t.Fatalf("unmanaged file guard must still refuse, got %v", err)
		}
	})
}

// TestUmaskDefaultACL_FlowsToOutput pins that publication honors the
// destination's default ACLs: a directory granting group read+write sees that
// access on new outputs even under umask 077 (POSIX ignores the umask when a
// default ACL applies), while executable intent stays canonical. Optional:
// skipped when setfacl is unavailable or the filesystem rejects ACLs (never
// installed on demand).
func TestUmaskDefaultACL_FlowsToOutput(t *testing.T) {
	setfacl, err := exec.LookPath("setfacl")
	if err != nil {
		t.Skip("setfacl not available; skipping default ACL scenario")
	}
	target := t.TempDir()
	if out, aclErr := exec.Command(setfacl, "-d", "-m", "g::rw-", target).CombinedOutput(); aclErr != nil { //nolint:gosec // test fixture
		t.Skipf("could not apply default ACL (filesystem without ACL support?): %v\n%s", aclErr, out)
	}

	defDir := writeDef(t, map[string]string{
		"rpack.yaml": minimalDefYAML("acl"),
		"script.lua": twoFileScript,
	})
	r := forkUmaskChild(t, 0o077, "run", map[string]string{
		umaskScenarioEnv: "direct",
		umaskTargetEnv:   target,
		umaskDefEnv:      defDir,
	})
	requireRunOK(t, r, 0o077)

	for name, wantIntent := range map[string]string{"deploy.sh": "755", "notes.txt": "644"} {
		mode := fileMode(t, filepath.Join(target, name))
		if got := mode.Perm() & 0o060; got != 0o060 {
			t.Errorf("%s group rw = %o, want rw (default ACL must flow despite umask 077)", name, got)
		}
		if got := CanonicalMode(mode); got != wantIntent {
			t.Errorf("%s intent = %q, want %q", name, got, wantIntent)
		}
	}
}

// mustReadFile reads path or fails the test.
func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path) //nolint:gosec // test
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return b
}

// mustSha returns the hex sha256 of the file at path.
func mustSha(t *testing.T, path string) string {
	t.Helper()
	sha, err := util.Sha256File(path)
	if err != nil {
		t.Fatalf("sha256 %s: %v", path, err)
	}
	return sha
}
