package getsource

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	ociDigest "github.com/opencontainers/go-digest"
	ociSpecs "github.com/opencontainers/image-spec/specs-go"
	ociv1 "github.com/opencontainers/image-spec/specs-go/v1"
	orasMemory "oras.land/oras-go/v2/content/memory"
)

func TestPublishRPack_Success(t *testing.T) {
	defDir := t.TempDir()
	writeFile(t, filepath.Join(defDir, "rpack.yaml"), "name: test-pack\ninputs: {}\n")
	writeFile(t, filepath.Join(defDir, "script.lua"), "-- test script\n")
	writeFile(t, filepath.Join(defDir, "schema.cue"), "#Schema: {}\n")

	store := newInMemoryPublisherStore()

	err := PublishRPack(context.Background(), defDir,
		func(registry, repo string) (OCIPublisher, error) {
			return store, nil
		},
		"oci://example.com/test/pack?tag=v1",
	)
	if err != nil {
		t.Fatalf("PublishRPack failed: %s", err)
	}

	// Verify the tag resolves
	desc, err := store.Resolve(context.Background(), "v1")
	if err != nil {
		t.Fatalf("tag 'v1' not found after publish: %s", err)
	}
	t.Logf("published manifest: digest=%s size=%d", desc.Digest, desc.Size)

	// Verify we can fetch and decompress the blob (simulate the download path)
	rc, err := store.Fetch(context.Background(), desc)
	if err != nil {
		t.Fatalf("fetch manifest: %s", err)
	}
	defer func() { _ = rc.Close() }()

	manifestBytes, err := io.ReadAll(rc)
	if err != nil {
		t.Fatal(err)
	}
	var manifest ociv1.Manifest
	if unmarshalErr := json.Unmarshal(manifestBytes, &manifest); unmarshalErr != nil {
		t.Fatal(unmarshalErr)
	}
	if manifest.ArtifactType != OCIArtifactType {
		t.Fatalf("expected artifact type %q, got %q", OCIArtifactType, manifest.ArtifactType)
	}
	if len(manifest.Layers) == 0 {
		t.Fatal("expected at least one layer")
	}
	layer0 := manifest.Layers[0]
	if layer0.MediaType != "archive/zip" {
		t.Fatalf("expected layer media type archive/zip, got %q", layer0.MediaType)
	}

	// Fetch the blob itself and verify it's a valid zip containing our files
	blobRC, err := store.Fetch(context.Background(), layer0)
	if err != nil {
		t.Fatalf("fetch blob: %s", err)
	}
	defer func() { _ = blobRC.Close() }()
	blobBytes, err := io.ReadAll(blobRC)
	if err != nil {
		t.Fatal(err)
	}
	// Verify it's a valid zip with expected entries
	zipReader, err := zip.NewReader(bytes.NewReader(blobBytes), int64(len(blobBytes)))
	if err != nil {
		t.Fatalf("not a valid zip: %s", err)
	}
	entries := make(map[string]bool)
	for _, f := range zipReader.File {
		entries[f.Name] = true
	}
	for _, want := range []string{"rpack.yaml", "script.lua", "schema.cue"} {
		if !entries[want] {
			t.Errorf("expected entry %q not found in zip", want)
		}
	}
}

func TestPublishRPack_MissingRPackYAML(t *testing.T) {
	defDir := t.TempDir()
	// No rpack.yaml
	err := PublishRPack(context.Background(), defDir,
		func(registry, repo string) (OCIPublisher, error) {
			return newInMemoryPublisherStore(), nil
		},
		"oci://example.com/test/pack?tag=v1",
	)
	if err == nil {
		t.Fatal("expected error for missing rpack.yaml")
	}
	t.Logf("expected error: %s", err)
}

func TestPublishRPack_InvalidOCIRef(t *testing.T) {
	defDir := t.TempDir()
	writeFile(t, filepath.Join(defDir, "rpack.yaml"), "name: test\ninputs: {}\n")
	writeFile(t, filepath.Join(defDir, "script.lua"), "-- test\n")

	err := PublishRPack(context.Background(), defDir,
		func(registry, repo string) (OCIPublisher, error) {
			return newInMemoryPublisherStore(), nil
		},
		"not-a-valid-oci-ref",
	)
	if err == nil {
		t.Fatal("expected error for invalid OCI ref")
	}
	t.Logf("expected error: %s", err)
}

func TestPublishRPack_DefaultTag(t *testing.T) {
	defDir := t.TempDir()
	writeFile(t, filepath.Join(defDir, "rpack.yaml"), "name: test\ninputs: {}\n")
	writeFile(t, filepath.Join(defDir, "script.lua"), "-- test\n")

	store := newInMemoryPublisherStore()

	err := PublishRPack(context.Background(), defDir,
		func(registry, repo string) (OCIPublisher, error) {
			return store, nil
		},
		"oci://example.com/test/pack", // no tag → defaults to "latest"
	)
	if err != nil {
		t.Fatalf("PublishRPack failed: %s", err)
	}

	desc, err := store.Resolve(context.Background(), "latest")
	if err != nil {
		t.Fatalf("tag 'latest' not found: %s", err)
	}
	t.Logf("published with default tag: digest=%s", desc.Digest)
}

// --- in-memory publisher store for testing ---

// inMemoryPublisherStore wraps orasMemory.Store and implements both
// OCIRepositoryStore and OCIPublisher for unit testing.
type inMemoryPublisherStore struct {
	*orasMemory.Store
}

func newInMemoryPublisherStore() *inMemoryPublisherStore {
	return &inMemoryPublisherStore{Store: orasMemory.New()}
}

// --- OCIRepositoryStore implementation ---

func (s *inMemoryPublisherStore) Resolve(ctx context.Context, reference string) (ociv1.Descriptor, error) {
	return s.Store.Resolve(ctx, reference)
}

//nolint:gocritic // target is OCI standard type passed by value
func (s *inMemoryPublisherStore) Fetch(ctx context.Context, target ociv1.Descriptor) (io.ReadCloser, error) {
	return s.Store.Fetch(ctx, target)
}

// --- OCIPublisher implementation ---

func (s *inMemoryPublisherStore) PushBlob(ctx context.Context, mediaType string, content io.Reader) (ociv1.Descriptor, error) {
	data, err := io.ReadAll(content)
	if err != nil {
		return ociv1.Descriptor{}, err
	}
	desc := ociv1.Descriptor{
		MediaType: mediaType,
		Digest:    ociDigest.FromBytes(data),
		Size:      int64(len(data)),
	}
	err = s.Push(ctx, desc, bytes.NewReader(data))
	if err != nil {
		return ociv1.Descriptor{}, err
	}
	return desc, nil
}

func (s *inMemoryPublisherStore) PushManifest(ctx context.Context, artifactType string, layers []ociv1.Descriptor) (ociv1.Descriptor, error) {
	manifest := ociv1.Manifest{
		MediaType:    ociv1.MediaTypeImageManifest,
		ArtifactType: artifactType,
		Layers:       layers,
		Versioned: ociSpecs.Versioned{
			SchemaVersion: 2,
		},
	}
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		return ociv1.Descriptor{}, err
	}
	desc := ociv1.Descriptor{
		MediaType:    ociv1.MediaTypeImageManifest,
		ArtifactType: artifactType,
		Digest:       ociDigest.FromBytes(manifestBytes),
		Size:         int64(len(manifestBytes)),
	}
	err = s.Push(ctx, desc, bytes.NewReader(manifestBytes))
	if err != nil {
		return ociv1.Descriptor{}, err
	}
	return desc, nil
}

//nolint:gocritic // desc is OCI standard type passed by value
func (s *inMemoryPublisherStore) Tag(ctx context.Context, desc ociv1.Descriptor, tag string) error {
	return s.Store.Tag(ctx, desc, tag)
}

// Compile-time interface checks
var (
	_ OCIRepositoryStore = (*inMemoryPublisherStore)(nil)
	_ OCIPublisher       = (*inMemoryPublisherStore)(nil)
)

// --- helpers ---

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	err := os.WriteFile(path, []byte(content), 0o600)
	if err != nil {
		t.Fatal(err)
	}
}

func writeSampleDef(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "rpack.yaml"), "\"@schema_version\": \"v1\"\nname: \"test\"\n")
	writeFile(t, filepath.Join(dir, "script.lua"), "-- test script\n")
	return dir
}

func TestPublishArchive(t *testing.T) {
	t.Run("valid definition", func(t *testing.T) {
		defDir := writeSampleDef(t)
		dest := filepath.Join(t.TempDir(), "out.tar.xz")
		err := PublishArchive(defDir, dest)
		if err != nil {
			t.Fatalf("PublishArchive failed: %v", err)
		}
		// Verify file exists and has content
		info, err := os.Stat(dest)
		if err != nil {
			t.Fatalf("archive not found: %v", err)
		}
		if info.Size() == 0 {
			t.Error("archive is empty")
		}
	})

	t.Run("invalid suffix", func(t *testing.T) {
		defDir := writeSampleDef(t)
		err := PublishArchive(defDir, filepath.Join(t.TempDir(), "out.zip"))
		if err == nil {
			t.Error("expected error for non-.tar.xz suffix")
		}
	})

	t.Run("invalid definition", func(t *testing.T) {
		dir := t.TempDir()
		// Missing script.lua — only rpack.yaml
		if err := os.WriteFile(filepath.Join(dir, "rpack.yaml"), []byte("\"@schema_version\": \"v1\"\nname: \"test\"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		err := PublishArchive(dir, filepath.Join(t.TempDir(), "out.tar.xz"))
		if err == nil {
			t.Error("expected error for invalid definition")
		}
	})
}

func TestPublishRPack_ValidatesDef(t *testing.T) {
	t.Run("rejects invalid definition", func(t *testing.T) {
		dir := t.TempDir()
		// Write only rpack.yaml (missing script.lua)
		if err := os.WriteFile(filepath.Join(dir, "rpack.yaml"), []byte("\"@schema_version\": \"v1\"\nname: \"test\"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		store := &inMemoryPublisherStore{orasMemory.New()}
		err := PublishRPack(context.Background(), dir,
			func(_, _ string) (OCIPublisher, error) { return store, nil },
			"oci://example.com/test?tag=v1",
		)
		if err == nil {
			t.Error("expected validation error, got nil")
		}
		if !strings.Contains(err.Error(), "validation") && !strings.Contains(err.Error(), "script") {
			t.Errorf("expected validation/script error, got: %v", err)
		}
	})
}
