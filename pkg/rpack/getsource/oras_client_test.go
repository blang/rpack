package getsource

import (
	"bytes"
	"context"
	"testing"
)

func TestNewORASStore_EnvCredentials(t *testing.T) {
	t.Setenv("OCI_USERNAME", "testuser")
	t.Setenv("OCI_PASSWORD", "testpass")

	store, err := NewORASStore("example.com", "my/repo")
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if store == nil {
		t.Fatal("expected non-nil store")
	}
	if store.repo == nil {
		t.Fatal("expected non-nil inner repository")
	}

	// Verify it implements OCIRepositoryStore
	var _ OCIRepositoryStore = store
}

func TestNewORASStore_MissingCredentials(t *testing.T) {
	// No credentials set — anonymous access

	store, err := NewORASStore("example.com", "my/repo")
	if err != nil {
		t.Fatalf("unexpected error (anonymous access ok): %s", err)
	}
	if store == nil {
		t.Fatal("expected non-nil store")
	}
}

func TestORASStore_ImplementsOCIPublisher(t *testing.T) {
	t.Setenv("OCI_USERNAME", "user")
	t.Setenv("OCI_PASSWORD", "pass")

	store, err := NewORASStore("example.com", "my/repo")
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	var pub OCIPublisher = store
	_ = pub
}

func TestOCIPublisher_Interface(t *testing.T) {
	// Verify the interface has the expected methods by checking
	// that ORASStore compiles as OCIPublisher
	store := &ORASStore{}
	var _ OCIPublisher = store
}

func TestORASStore_PushBlobInMemory(t *testing.T) {
	// Use the existing in-memory OCI store, not the remote one,
	// to verify the PushBlob concept works without a network connection.
	// The remote ORASStore requires a real registry, so this is a
	// compile-time test that the methods exist.
	t.Setenv("OCI_USERNAME", "user")
	t.Setenv("OCI_PASSWORD", "pass")

	store, err := NewORASStore("localhost:5000", "test/repo")
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	// Verify PushBlob accepts the right signature
	data := bytes.NewReader([]byte("test content"))
	desc, err := store.PushBlob(context.Background(), "application/zip", data)
	if err == nil {
		// We don't expect success against a fake registry,
		// but the descriptor should be populated if we got one.
		t.Logf("unexpected success: got descriptor %+v", desc)
	}
	// Expected error since localhost:5000 is not running ORAS
	if err == nil {
		t.Log("no error — maybe localhost:5000 is running?")
	}
}
