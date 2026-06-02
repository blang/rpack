package getsource

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	getter "github.com/hashicorp/go-getter"
	ociDigest "github.com/opencontainers/go-digest"
	ociSpecs "github.com/opencontainers/image-spec/specs-go"
	ociv1 "github.com/opencontainers/image-spec/specs-go/v1"
	orasContent "oras.land/oras-go/v2/content"
	orasMemory "oras.land/oras-go/v2/content/memory"
)

func TestDecompressorMediaTypesIntegration(t *testing.T) {
	for k, v := range decompressorMediaTypes {
		if _, ok := Decompressors[v]; !ok {
			t.Errorf("decompressorMediaTypes[%q] refers to %q, which is not defined in Decompressors", k, v)
		}
	}
	if len(decompressorMediaTypes) != len(ociBlobMediaTypePreference) {
		t.Errorf("decompressorMediaTypes has %d elements, ociBlobMediaTypePreference has %d",
			len(decompressorMediaTypes), len(ociBlobMediaTypePreference))
	}
}

//nolint:gocognit // test setup is naturally complex
func TestOCIDistributionGetter(t *testing.T) {
	mainStore := &digestResolvingInMemoryOCIStore{
		Store: orasMemory.New(),
	}

	latestBlobDesc := ociPushFakeModulePackageBlob(t, "content of latest", mainStore.Store)
	latestManifestDesc := ociPushFakeImageManifest(t, latestBlobDesc, OCIArtifactType, mainStore.Store)
	ociCreateTag(t, "latest", latestManifestDesc, mainStore.Store)
	fooBlobDesc := ociPushFakeModulePackageBlob(t, "content of foo", mainStore.Store)
	fooManifestDesc := ociPushFakeImageManifest(t, fooBlobDesc, OCIArtifactType, mainStore.Store)
	ociCreateTag(t, "foo", fooManifestDesc, mainStore.Store)

	g := &ociDistributionGetter{
		getOCIRepositoryStore: func(ctx context.Context, registryDomain, repositoryName string) (OCIRepositoryStore, error) {
			return mainStore, nil
		},
	}

	t.Run("default tag (latest)", func(t *testing.T) {
		destDir := t.TempDir()
		u, err := parseOCIURL("oci://example.com/test/module")
		if err != nil {
			t.Fatal(err)
		}
		err = g.Get(destDir, u)
		if err != nil {
			t.Fatalf("unexpected error: %s", err)
		}
		//nolint:gosec // test path is controlled
		content, err := os.ReadFile(filepath.Join(destDir, "module.txt"))
		if err != nil {
			t.Fatal(err)
		}
		if string(content) != "content of latest" {
			t.Fatalf("unexpected content: %s", content)
		}
	})

	t.Run("explicit tag", func(t *testing.T) {
		destDir := t.TempDir()
		u, err := parseOCIURL("oci://example.com/test/module?tag=foo")
		if err != nil {
			t.Fatal(err)
		}
		err = g.Get(destDir, u)
		if err != nil {
			t.Fatalf("unexpected error: %s", err)
		}
		//nolint:gosec // test path is controlled
		content, err := os.ReadFile(filepath.Join(destDir, "module.txt"))
		if err != nil {
			t.Fatal(err)
		}
		if string(content) != "content of foo" {
			t.Fatalf("unexpected content: %s", content)
		}
	})

	t.Run("ClientMode returns Dir", func(t *testing.T) {
		u, _ := parseOCIURL("oci://example.com/test/module")
		mode, err := g.ClientMode(u)
		if err != nil {
			t.Fatalf("unexpected error: %s", err)
		}
		if mode != getter.ClientModeDir {
			t.Fatalf("expected ClientModeDir (%d), got %d", getter.ClientModeDir, mode)
		}
	})

	t.Run("GetFile rejects archive param", func(t *testing.T) {
		u, _ := parseOCIURL("oci://example.com/test/module")
		err := g.GetFile("", u)
		if err == nil {
			t.Fatal("expected error")
		}
		t.Logf("expected error: %s", err)
	})

	t.Run("SetClient does not panic", func(t *testing.T) {
		g.SetClient(nil)
	})
}

func TestOCIDistributionGetter_Errors(t *testing.T) {
	mainStore := &digestResolvingInMemoryOCIStore{
		Store: orasMemory.New(),
	}

	g := &ociDistributionGetter{
		getOCIRepositoryStore: func(ctx context.Context, registryDomain, repositoryName string) (OCIRepositoryStore, error) {
			return mainStore, nil
		},
	}

	t.Run("nonexistent tag", func(t *testing.T) {
		destDir := t.TempDir()
		u, _ := parseOCIURL("oci://example.com/test/nonexistent?tag=nope")
		err := g.Get(destDir, u)
		if err == nil {
			t.Fatal("expected error for nonexistent tag")
		}
		t.Logf("expected error: %s", err)
	})

	t.Run("tag and digest together", func(t *testing.T) {
		destDir := t.TempDir()
		u, _ := parseOCIURL("oci://example.com/test/module?tag=foo&digest=sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
		err := g.Get(destDir, u)
		if err == nil {
			t.Fatal("expected error for both tag and digest")
		}
	})

	t.Run("invalid digest format", func(t *testing.T) {
		destDir := t.TempDir()
		u, _ := parseOCIURL("oci://example.com/test/module?digest=not-a-digest")
		err := g.Get(destDir, u)
		if err == nil {
			t.Fatal("expected error for invalid digest")
		}
	})

	t.Run("wrong artifact type", func(t *testing.T) {
		blobDesc := ociPushFakeModulePackageBlob(t, "wrong type", mainStore.Store)
		manifestDesc := ociPushFakeImageManifest(t, blobDesc, "application/vnd.oci.image.manifest.v1+json", mainStore.Store)
		ociCreateTag(t, "wrongtype", manifestDesc, mainStore.Store)

		destDir := t.TempDir()
		u, _ := parseOCIURL("oci://example.com/test/module?tag=wrongtype")
		err := g.Get(destDir, u)
		if err == nil {
			t.Fatal("expected error for wrong artifact type")
		}
		t.Logf("expected error: %s", err)
	})

	t.Run("unsupported media type layer", func(t *testing.T) {
		unsupportedBlob := ociPushFakeBlob(t, "unsupported layer", "application/octet-stream", mainStore.Store)
		manifestDesc := ociPushFakeImageManifestWithLayers(t, []ociv1.Descriptor{unsupportedBlob}, OCIArtifactType, mainStore.Store)
		ociCreateTag(t, "unsupported", manifestDesc, mainStore.Store)

		destDir := t.TempDir()
		u, _ := parseOCIURL("oci://example.com/test/module?tag=unsupported")
		err := g.Get(destDir, u)
		if err == nil {
			t.Fatal("expected error for unsupported layer media type")
		}
		t.Logf("expected error: %s", err)
	})
}

// ---- Test helpers ----

// parseOCIURL parses an OCI URL string for testing.
func parseOCIURL(raw string) (*url.URL, error) {
	before, query, hasQuery := strings.Cut(raw, "?")
	if !hasQuery {
		return url.Parse(raw)
	}
	// url.Parse doesn't handle 'oci://' well, so do it manually.
	// The path is everything between '://' and '?'.
	schemeEnd := strings.Index(before, "://")
	if schemeEnd == -1 {
		schemeEnd = strings.Index(before, ":")
	}
	rest := before[schemeEnd+3:]
	path := "/" + rest
	host := ""
	if slash := strings.IndexByte(path[1:], '/'); slash != -1 {
		host = path[1 : slash+1]
		path = path[slash+1:]
	} else {
		host = path[1:]
		path = "/"
	}
	return &url.URL{
		Scheme:   "oci",
		Host:     host,
		Path:     path,
		RawQuery: query,
	}, nil
}

// ociPushFakeModulePackageBlob creates a fake zip blob containing module.txt.
func ociPushFakeModulePackageBlob(t *testing.T, content string, pusher orasContent.Pusher) ociv1.Descriptor {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	fw, err := w.Create("module.txt")
	if err != nil {
		t.Fatal(err)
	}
	_, err = fw.Write([]byte(content))
	if err != nil {
		t.Fatal(err)
	}
	_ = w.Close()

	desc := ociv1.Descriptor{
		MediaType: "archive/zip",
		Digest:    ociDigest.FromBytes(buf.Bytes()),
		Size:      int64(buf.Len()),
	}
	err = pusher.Push(context.Background(), desc, bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	return desc
}

// ociPushFakeBlob creates any arbitrary blob.
func ociPushFakeBlob(t *testing.T, content, mediaType string, pusher orasContent.Pusher) ociv1.Descriptor {
	t.Helper()
	data := []byte(content)
	desc := ociv1.Descriptor{
		MediaType: mediaType,
		Digest:    ociDigest.FromBytes(data),
		Size:      int64(len(data)),
	}
	err := pusher.Push(context.Background(), desc, bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	return desc
}

// ociPushFakeImageManifest creates a manifest with the given layers.
//
//nolint:gocritic // desc is OCI standard type passed by value
func ociPushFakeImageManifest(t *testing.T, desc ociv1.Descriptor, artifactType string, store orasContent.Storage) ociv1.Descriptor {
	t.Helper()
	return ociPushFakeImageManifestWithLayers(t, []ociv1.Descriptor{desc}, artifactType, store)
}

func ociPushFakeImageManifestWithLayers(t *testing.T, layers []ociv1.Descriptor, artifactType string, store orasContent.Storage) ociv1.Descriptor {
	t.Helper()
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
		t.Fatal(err)
	}
	manifestDesc := ociv1.Descriptor{
		MediaType:    ociv1.MediaTypeImageManifest,
		ArtifactType: artifactType,
		Digest:       ociDigest.FromBytes(manifestBytes),
		Size:         int64(len(manifestBytes)),
	}
	err = store.Push(context.Background(), manifestDesc, bytes.NewReader(manifestBytes))
	if err != nil {
		t.Fatal(err)
	}
	return manifestDesc
}

// ociCreateTag creates a tag pointing to a manifest descriptor.
//
//nolint:gocritic // desc is OCI standard type passed by value
func ociCreateTag(t *testing.T, tagName string, desc ociv1.Descriptor, tagger orasContent.Tagger) {
	t.Helper()
	err := tagger.Tag(context.Background(), desc, tagName)
	if err != nil {
		t.Fatal(err)
	}
}

// digestResolvingInMemoryOCIStore wraps orasMemory.Store to implement
// our OCIRepositoryStore interface. It embeds the concrete *orasMemory.Store
// type because the memory store implements Resolver and Tagger interfaces
// that are not part of the orasContent.Storage interface.
type digestResolvingInMemoryOCIStore struct {
	*orasMemory.Store
}

func (s *digestResolvingInMemoryOCIStore) Resolve(ctx context.Context, reference string) (ociv1.Descriptor, error) {
	return s.Store.Resolve(ctx, reference)
}

//nolint:gocritic // target is OCI standard type passed by value
func (s *digestResolvingInMemoryOCIStore) Fetch(ctx context.Context, target ociv1.Descriptor) (io.ReadCloser, error) {
	return s.Store.Fetch(ctx, target)
}

var _ OCIRepositoryStore = (*digestResolvingInMemoryOCIStore)(nil)
