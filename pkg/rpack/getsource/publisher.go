package getsource

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	ociv1 "github.com/opencontainers/image-spec/specs-go/v1"
)

// PublishRPack publishes an rpack definition directory to an OCI registry.
//
// defDir is the local path to the rpack definition directory (containing
// rpack.yaml, script.lua, schema.cue, and any other files).
// storeFactory creates an OCIPublisher for the given registry and repository.
// ociRef is the target OCI URL in the format: oci://registry/repo/path?tag=name
func PublishRPack(ctx context.Context, defDir string,
	storeFactory func(registry, repo string) (OCIPublisher, error),
	ociRef string,
) error {
	// Validate the definition directory contains rpack.yaml
	if _, err := os.Stat(filepath.Join(defDir, "rpack.yaml")); err != nil {
		return fmt.Errorf("definition directory must contain rpack.yaml: %w", err)
	}

	// Parse OCI reference
	reg, repo, tag, err := parseOCIRef(ociRef)
	if err != nil {
		return fmt.Errorf("invalid OCI reference: %w", err)
	}

	// Create the OCI publisher
	store, err := storeFactory(reg, repo)
	if err != nil {
		return fmt.Errorf("creating OCI publisher for %s/%s: %w", reg, repo, err)
	}

	// Create zip of the definition directory
	zipData, err := zipDirectory(defDir)
	if err != nil {
		return fmt.Errorf("creating zip: %w", err)
	}

	// Push the blob
	blobDesc, err := store.PushBlob(ctx, "archive/zip", bytes.NewReader(zipData))
	if err != nil {
		return fmt.Errorf("pushing blob: %w", err)
	}

	// Push the manifest
	manifestDesc, err := store.PushManifest(ctx, OCIArtifactType, []ociv1.Descriptor{blobDesc})
	if err != nil {
		return fmt.Errorf("pushing manifest: %w", err)
	}

	// Tag the manifest
	if err := store.Tag(ctx, manifestDesc, tag); err != nil {
		return fmt.Errorf("tagging manifest: %w", err)
	}

	return nil
}

// parseOCIRef parses an OCI reference URL into registry, repository, and tag.
// Input: oci://registry.example.com/repo/path?tag=v1
func parseOCIRef(ref string) (registry, repository, tag string, err error) {
	u, err := url.Parse(ref)
	if err != nil {
		return "", "", "", fmt.Errorf("parsing URL: %w", err)
	}
	if u.Scheme != "oci" {
		return "", "", "", fmt.Errorf("expected oci:// scheme, got %q", u.Scheme)
	}
	registry = u.Host
	if registry == "" {
		return "", "", "", fmt.Errorf("missing registry host")
	}
	repository = strings.TrimPrefix(u.Path, "/")
	if repository == "" {
		return "", "", "", fmt.Errorf("missing repository path")
	}
	tag = u.Query().Get("tag")
	if tag == "" {
		tag = "latest"
	}
	return registry, repository, tag, nil
}

// zipDirectory creates a zip archive of all regular files in dir,
// using relative paths as entry names.
func zipDirectory(dir string) ([]byte, error) {
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)

	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		relPath, relErr := filepath.Rel(dir, path)
		if relErr != nil {
			return relErr
		}
		f, createErr := w.Create(relPath)
		if createErr != nil {
			return createErr
		}
		data, readErr := os.ReadFile(path) //nolint:gosec // path comes from WalkDir
		if readErr != nil {
			return readErr
		}
		_, writeErr := f.Write(data)
		return writeErr
	})

	if closeErr := w.Close(); closeErr != nil && err == nil {
		err = closeErr
	}
	if err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}
