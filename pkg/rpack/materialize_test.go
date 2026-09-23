package rpack

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// residue finds leftover ".rpack-" temporary siblings under dir.
func residue(t *testing.T, dir string) []string {
	t.Helper()
	var found []string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasPrefix(filepath.Base(path), ".rpack-") {
			found = append(found, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", dir, err)
	}
	return found
}

func requireNoResidue(t *testing.T, dir string) {
	t.Helper()
	if found := residue(t, dir); len(found) > 0 {
		t.Fatalf("temporary siblings leaked: %v", found)
	}
}

// TestWriteOutputFile_MaterializesIntentAndCreatesDirs pins the ADR 0002
// publication path: outputs land in the destination's parent with Git-like
// creation bases, nested directories are created, and no temporary sibling
// remains.
func TestWriteOutputFile_MaterializesIntentAndCreatesDirs(t *testing.T) {
	dst := t.TempDir()
	execPath := filepath.Join(dst, "nested", "deep", "deploy.sh")
	if err := writeOutputFile(execPath, []byte("#!/bin/sh\n"), ExecutableMode); err != nil {
		t.Fatalf("writeOutputFile: %v", err)
	}
	if got := CanonicalMode(fileMode(t, execPath)); got != "755" {
		t.Fatalf("deploy.sh intent = %q, want 755", got)
	}
	plainPath := filepath.Join(dst, "nested", "notes.txt")
	if err := writeOutputFile(plainPath, []byte("x"), NonExecutableMode); err != nil {
		t.Fatalf("writeOutputFile: %v", err)
	}
	if got := CanonicalMode(fileMode(t, plainPath)); got != "644" {
		t.Fatalf("notes.txt intent = %q, want 644", got)
	}
	got, err := os.ReadFile(execPath) //nolint:gosec // test
	if err != nil || string(got) != "#!/bin/sh\n" {
		t.Fatalf("deploy.sh content = %q, err %v", got, err)
	}
	requireNoResidue(t, dst)
}

// TestWriteOutputFile_ReplacementDoesNotInheritDestMode pins
// replace-without-rw-preservation: a replacement recreates the inode, so
// a wide pre-existing destination mode never leaks into the output.
func TestWriteOutputFile_ReplacementDoesNotInheritDestMode(t *testing.T) {
	dst := t.TempDir()
	p := filepath.Join(dst, "out.txt")
	preCreateStaged(t, p, 0o777) // owner-executable: wrong for a 644 intent
	if err := writeOutputFile(p, []byte("new"), NonExecutableMode); err != nil {
		t.Fatalf("writeOutputFile: %v", err)
	}
	if got := CanonicalMode(fileMode(t, p)); got != "644" {
		t.Fatalf("replacement intent = %q, want 644 (dest mode must not leak)", got)
	}
	got, err := os.ReadFile(p) //nolint:gosec // test
	if err != nil || string(got) != "new" {
		t.Fatalf("replacement content = %q, err %v", got, err)
	}
	requireNoResidue(t, dst)
}

// TestWriteOutputFile_InvalidModeRefusedExistingPreserved pins the atomic
// contract on the validation failure: an unsupported intent is refused
// before any filesystem mutation — the destination keeps its content and
// mode, no directory is created, and no temporary sibling leaks.
func TestWriteOutputFile_InvalidModeRefusedExistingPreserved(t *testing.T) {
	dst := t.TempDir()
	p := filepath.Join(dst, "out.txt")
	preCreateStaged(t, p, 0o777)
	err := writeOutputFile(p, []byte("new"), 0o600)
	if err == nil || !strings.Contains(err.Error(), "unsupported output mode") {
		t.Fatalf("want unsupported-output-mode refusal, got %v", err)
	}
	if got, rerr := os.ReadFile(p); rerr != nil || string(got) != "old" { //nolint:gosec // test
		t.Fatalf("existing file damaged: content %q, err %v", got, rerr)
	}
	if got := fileMode(t, p); got != 0o777 {
		t.Fatalf("existing mode = %o, want 777 (preserved)", got)
	}
	// A fresh destination path must not even gain its parent directory.
	fresh := filepath.Join(dst, "fresh", "never.txt")
	if err := writeOutputFile(fresh, []byte("x"), 0o700); err == nil {
		t.Fatal("expected refusal for 0700")
	}
	if fileExists(filepath.Join(dst, "fresh")) {
		t.Fatal("invalid mode created the output directory")
	}
	requireNoResidue(t, dst)
}

// TestWriteOutputFile_DestUnwritableExistingPreserved pins the atomic
// contract on the creation failure: when the destination directory refuses
// the temporary sibling, the error surfaces and the existing destination
// file is untouched.
func TestWriteOutputFile_DestUnwritableExistingPreserved(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory permission checks")
	}
	dst := t.TempDir()
	p := filepath.Join(dst, "out.txt")
	preCreateStaged(t, p, 0o755)
	if err := os.Chmod(dst, 0o500); err != nil { //nolint:gosec // test fixture
		t.Fatal(err)
	}
	defer func() {
		if err := os.Chmod(dst, 0o700); err != nil { //nolint:gosec // test fixture
			t.Fatal(err)
		}
	}()
	err := writeOutputFile(p, []byte("new"), NonExecutableMode)
	if err == nil || !strings.Contains(err.Error(), "could not create output") {
		t.Fatalf("want creation refusal, got %v", err)
	}
	if got, rerr := os.ReadFile(p); rerr != nil || string(got) != "old" { //nolint:gosec // test
		t.Fatalf("existing file damaged: content %q, err %v", got, rerr)
	}
	if got := CanonicalMode(fileMode(t, p)); got != "755" {
		t.Fatalf("existing intent = %q, want 755 (preserved)", got)
	}
}

// TestCopyOutputFile_SourceFailurePreservesExisting pins the atomic contract
// on the source failure: an unreadable staged source refuses before the
// destination is touched.
func TestCopyOutputFile_SourceFailurePreservesExisting(t *testing.T) {
	dst := t.TempDir()
	src := filepath.Join(dst, "staged.txt")
	dest := filepath.Join(dst, "out.txt")
	preCreateStaged(t, src, 0o600)
	preCreateStaged(t, dest, 0o755)
	if err := os.Remove(src); err != nil {
		t.Fatal(err)
	}
	modes := map[string]string{src: "644"}
	err := copyOutputFile(src, dest, modes)
	if err == nil || !strings.Contains(err.Error(), "failed to read staged output") {
		t.Fatalf("want staged-read refusal, got %v", err)
	}
	if got, rerr := os.ReadFile(dest); rerr != nil || string(got) != "old" { //nolint:gosec // test
		t.Fatalf("existing destination damaged: content %q, err %v", got, rerr)
	}
	requireNoResidue(t, dst)
}

// TestCopyDir_MissingIntentRefused pins the modes-map contract: every staged
// file must carry a declared intent; a missing entry refuses the copy.
func TestCopyDir_MissingIntentRefused(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "f.txt"), []byte("x"), 0o600); err != nil { //nolint:gosec // test fixture
		t.Fatal(err)
	}
	err := copyDir(src, dst, map[string]string{})
	if err == nil || !strings.Contains(err.Error(), "missing executable intent") {
		t.Fatalf("want missing-intent refusal, got %v", err)
	}
	if fileExists(filepath.Join(dst, "f.txt")) {
		t.Fatal("copy published a file without declared intent")
	}
}

// TestMoveFiles_PublishesDeclaredIntentAndRemovesStaged pins the relocation
// half of the managed path: outputs materialize with their declared intent
// (never derived from staging bits), staged files are removed, and no
// temporary siblings leak.
func TestMoveFiles_PublishesDeclaredIntentAndRemovesStaged(t *testing.T) {
	runDir := t.TempDir()
	execPath := t.TempDir()
	staged := filepath.Join(runDir, "deploy.sh")
	preCreateStaged(t, staged, 0o600)

	// External actor flattens the staged file's physical bits; declared
	// intent in the modes map must win.
	if err := os.Chmod(staged, 0o777); err != nil { //nolint:gosec // test fixture
		t.Fatal(err)
	}
	files := []*ControlledFile{{Path: "deploy.sh", AbsPath: staged}}
	modes := map[string]string{staged: "644"}
	if err := moveFiles(files, execPath, modes); err != nil {
		t.Fatalf("moveFiles: %v", err)
	}
	published := filepath.Join(execPath, "deploy.sh")
	if got := CanonicalMode(fileMode(t, published)); got != "644" {
		t.Fatalf("published intent = %q, want 644 (declared, not staged stat)", got)
	}
	if fileExists(staged) {
		t.Fatal("staged file not removed after move")
	}
	requireNoResidue(t, runDir)
	requireNoResidue(t, execPath)
}

// TestMoveFiles_InvalidIntentExistingPreserved pins the atomic contract on
// the intent-validation failure inside a relocation.
func TestMoveFiles_InvalidIntentExistingPreserved(t *testing.T) {
	runDir := t.TempDir()
	execPath := t.TempDir()
	staged := filepath.Join(runDir, "deploy.sh")
	preCreateStaged(t, staged, 0o600)
	dest := filepath.Join(execPath, "deploy.sh")
	preCreateStaged(t, dest, 0o755)

	files := []*ControlledFile{{Path: "deploy.sh", AbsPath: staged}}
	modes := map[string]string{staged: "invalid"}
	err := moveFiles(files, execPath, modes)
	if err == nil || !strings.Contains(err.Error(), `invalid mode "invalid"`) {
		t.Fatalf("want invalid mode refusal, got %v", err)
	}
	if got, rerr := os.ReadFile(dest); rerr != nil || string(got) != "old" { //nolint:gosec // test
		t.Fatalf("existing destination damaged: content %q, err %v", got, rerr)
	}
	requireNoResidue(t, execPath)
}

// captureStdout swaps os.Stdout for a pipe, runs fn, and returns everything
// fn printed.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = w
	done := make(chan string)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()
	defer func() {
		os.Stdout = old
		_ = w.Close()
		_ = r.Close()
	}()
	fn()
	_ = w.Close()
	return <-done
}

// TestPrintDryRunOutput_ShowsDeclaredIntentNotPhysical pins the dry-run
// header contract: the header prints the declared intent, never the private
// staging permissions of the underlying inode.
func TestPrintDryRunOutput_ShowsDeclaredIntentNotPhysical(t *testing.T) {
	runDir := t.TempDir()
	execPath := filepath.Join(runDir, "deploy.sh")
	plainPath := filepath.Join(runDir, "notes.txt")
	preCreateStaged(t, execPath, 0o600) // private staging on disk
	preCreateStaged(t, plainPath, 0o600)
	modes := map[string]string{
		execPath:  "755",
		plainPath: "644",
	}

	out := captureStdout(t, func() {
		if err := printDryRunOutput(runDir, modes); err != nil {
			t.Errorf("printDryRunOutput: %v", err)
		}
	})

	if !strings.Contains(out, "=== ./deploy.sh (executable) ===") {
		t.Fatalf("dry-run header missing declared executable intent:\n%s", out)
	}
	if !strings.Contains(out, "=== ./notes.txt (non-executable) ===") {
		t.Fatalf("dry-run header missing declared non-executable intent:\n%s", out)
	}
	// Physical staging bits (0600) must never appear in the intent column.
	for _, leaked := range []string{"(600)", "(0600)"} {
		if strings.Contains(out, leaked) {
			t.Fatalf("dry-run header leaked physical staging mode %q:\n%s", leaked, out)
		}
	}
}
