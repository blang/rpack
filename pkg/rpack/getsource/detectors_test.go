package getsource

import (
	"errors"
	"testing"
)

func TestFileDetector_AbsolutePath(t *testing.T) {
	d := new(fileDetector)
	result, ok, err := d.Detect("/absolute/path/to/module", "")
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if !ok {
		t.Fatal("expected detector to match absolute path")
	}
	if result != "file:///absolute/path/to/module" {
		t.Fatalf("unexpected result: %s", result)
	}
}

func TestFileDetector_RelativePath(t *testing.T) {
	d := new(fileDetector)
	_, ok, err := d.Detect("some/module", "")
	if !ok {
		t.Fatal("expected detector to match relative path")
	}
	if err == nil {
		t.Fatal("expected error for relative path without ./ or ../ prefix")
	}
	var relErr *MaybeRelativePathError
	if !errors.As(err, &relErr) {
		t.Fatalf("expected *MaybeRelativePathError, got %T: %s", err, err)
	}
}

func TestFileDetector_DotSlashPrefix(t *testing.T) {
	d := new(fileDetector)
	result, ok, err := d.Detect("./local/module", "/some/pwd")
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if !ok {
		t.Fatal("expected detector to match ./ path")
	}
	if result == "" {
		t.Fatal("expected non-empty result")
	}
}

func TestFileDetector_DotDotSlashPrefix(t *testing.T) {
	d := new(fileDetector)
	result, ok, err := d.Detect("../other/module", "/some/pwd")
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if !ok {
		t.Fatal("expected detector to match ../ path")
	}
	if result == "" {
		t.Fatal("expected non-empty result")
	}
}

func TestFileDetector_EmptyString(t *testing.T) {
	d := new(fileDetector)
	_, ok, err := d.Detect("", "")
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if ok {
		t.Fatal("expected detector to not match empty string")
	}
}

func TestWithoutQueryParams_Passthrough(t *testing.T) {
	d := &withoutQueryParams{d: &mockDetector{}}
	result, ok, err := d.Detect("github.com/user/repo", "")
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if !ok {
		t.Fatal("expected ok=true")
	}
	if result != "mock:github.com/user/repo" {
		t.Fatalf("unexpected result: %s", result)
	}
}

func TestWithoutQueryParams_StripsQuery(t *testing.T) {
	mock := &mockDetector{}
	d := &withoutQueryParams{d: mock}
	_, _, _ = d.Detect("github.com/user/repo?ref=main", "")
	if mock.gotSrc != "github.com/user/repo" {
		t.Fatalf("expected query-stripped src, got %q", mock.gotSrc)
	}
}

func TestWithoutQueryParams_ReattachesQuery(t *testing.T) {
	mock := &mockDetector{}
	d := &withoutQueryParams{d: mock}
	result, _, _ := d.Detect("github.com/user/repo?ref=main", "")
	if result != "mock:github.com/user/repo?ref=main" {
		t.Fatalf("expected query params reattached, got %q", result)
	}
}

func TestWithoutQueryParams_MultipleQueryParams(t *testing.T) {
	mock := &mockDetector{}
	d := &withoutQueryParams{d: mock}
	result, _, _ := d.Detect("github.com/user/repo?ref=main&type=module", "")
	if mock.gotSrc != "github.com/user/repo" {
		t.Fatalf("expected query-stripped src, got %q", mock.gotSrc)
	}
	if result != "mock:github.com/user/repo?ref=main&type=module" {
		t.Fatalf("expected query params reattached, got %q", result)
	}
}

func TestDetectorsList(t *testing.T) {
	if len(Detectors) == 0 {
		t.Fatal("expected non-empty detectors list")
	}
}

// mockDetector is a fake detector for testing withoutQueryParams.
type mockDetector struct {
	gotSrc string
}

func (m *mockDetector) Detect(src, pwd string) (result string, ok bool, err error) {
	m.gotSrc = src
	return "mock:" + src, true, nil
}
