package getsource

import "testing"

func TestNormalizeSource_GitHub(t *testing.T) {
	result, err := NormalizeSource("github.com/user/repo")
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if result == "" {
		t.Fatal("expected non-empty result")
	}
	// GitHub detector should normalize to git::https://...
	t.Logf("normalized GitHub: %s", result)
}

func TestNormalizeSource_GitHubWithQuery(t *testing.T) {
	result, err := NormalizeSource("github.com/user/repo?ref=main")
	if err != nil {
		t.Fatalf("unexpected error with query params: %s", err)
	}
	if result == "" {
		t.Fatal("expected non-empty result")
	}
	t.Logf("normalized GitHub with query: %s", result)
}

func TestNormalizeSource_LocalDotSlash(t *testing.T) {
	result, err := NormalizeSource("./local/module")
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if result == "" {
		t.Fatal("expected non-empty result")
	}
	t.Logf("normalized local ./: %s", result)
}

func TestNormalizeSource_RelativePathError(t *testing.T) {
	_, err := NormalizeSource("some/module")
	if err == nil {
		t.Fatal("expected error for bare relative path")
	}
	t.Logf("error: %s", err)
}

func TestNormalizeSource_S3(t *testing.T) {
	result, err := NormalizeSource("s3::https://s3.amazonaws.com/bucket/path")
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if result == "" {
		t.Fatal("expected non-empty result")
	}
	t.Logf("normalized S3: %s", result)
}

func TestNormalizeSource_Git(t *testing.T) {
	result, err := NormalizeSource("git::https://example.com/repo.git")
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if result == "" {
		t.Fatal("expected non-empty result")
	}
	t.Logf("normalized Git: %s", result)
}

func TestNormalizeSource_GCS(t *testing.T) {
	result, err := NormalizeSource("gs://my-bucket/path/to/module")
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if result == "" {
		t.Fatal("expected non-empty result")
	}
	t.Logf("normalized GCS: %s", result)
}

func TestNormalizeSource_AbsolutePath(t *testing.T) {
	result, err := NormalizeSource("/absolute/path/to/module")
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if result != "file:///absolute/path/to/module" {
		t.Fatalf("unexpected result: %s", result)
	}
}

func TestSplitSourceSubdir_NoSubdir(t *testing.T) {
	addr, sub := SplitSourceSubdir("git::https://example.com/repo.git")
	if addr != "git::https://example.com/repo.git" {
		t.Fatalf("unexpected addr: %s", addr)
	}
	if sub != "" {
		t.Fatalf("unexpected subdir: %s", sub)
	}
}

func TestSplitSourceSubdir_WithSubdir(t *testing.T) {
	addr, sub := SplitSourceSubdir("git::https://example.com/repo.git//sub/dir")
	if addr != "git::https://example.com/repo.git" {
		t.Fatalf("unexpected addr: %s", addr)
	}
	if sub != "sub/dir" {
		t.Fatalf("unexpected subdir: %s", sub)
	}
}

func TestSplitSourceSubdir_GitHubWithSubdir(t *testing.T) {
	addr, sub := SplitSourceSubdir("github.com/user/repo//subdir")
	if sub != "subdir" {
		t.Fatalf("unexpected subdir: %s", sub)
	}
	t.Logf("addr=%s sub=%s", addr, sub)
}
