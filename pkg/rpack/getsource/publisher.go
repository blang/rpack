package getsource

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	bz2 "github.com/dsnet/compress/bzip2"
	"github.com/ulikunitz/xz"

	ociv1 "github.com/opencontainers/image-spec/specs-go/v1"
)

// PublishRPack publishes an rpack definition directory.
//
// defDir is the local path to the rpack definition directory (containing
// rpack.yaml, script.lua, schema.cue, and any other files).
// storeFactory creates an OCIPublisher for the given registry and repository.
// ociRef is the target OCI URL in the format: oci://registry/repo/path?tag=name
func PublishRPack(ctx context.Context, defDir string,
	storeFactory func(registry, repo string) (OCIPublisher, error),
	ociRef string,
) error {
	// Validate the definition directory contains required files
	if err := validateDefDir(defDir); err != nil {
		return fmt.Errorf("definition validation failed: %w", err)
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
		relPath, relErr := filepath.Rel(dir, path)
		if relErr != nil {
			return relErr
		}
		// Skip tests/ directory before processing other directories
		if strings.HasPrefix(relPath, "tests/") || strings.HasPrefix(relPath, "tests"+string(filepath.Separator)) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
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

// validateDefDir performs basic sanity checks on a definition directory.
// It verifies that rpack.yaml and script.lua exist and are readable.
// For full schema validation, use rpack.ValidateRPackDef from the command layer.
func validateDefDir(defDir string) error {
	if _, err := os.Stat(filepath.Join(defDir, "rpack.yaml")); err != nil {
		return fmt.Errorf("rpack.yaml not found: %w", err)
	}
	if _, err := os.Stat(filepath.Join(defDir, "script.lua")); err != nil {
		return fmt.Errorf("script.lua not found: %w", err)
	}
	return nil
}

// PublishArchive creates a tar.xz archive of the definition directory.
// defDir is validated before archiving (uses the same checks as publish and validate).
// archivePath must end in .tar.xz.
func PublishArchive(defDir, archivePath string) error {
	if err := validateDefDir(defDir); err != nil {
		return fmt.Errorf("definition validation failed: %w", err)
	}
	if !strings.HasSuffix(archivePath, ".tar.xz") {
		return fmt.Errorf("archive target must end in .tar.xz: %s", archivePath)
	}
	if err := os.MkdirAll(filepath.Dir(archivePath), 0o755); err != nil { //nolint:gosec // intentional: standard directory permissions
		return fmt.Errorf("creating output directory: %w", err)
	}
	return createTarXZ(defDir, archivePath)
}

// BundleZip creates a zip archive of the definition directory.
// defDir is validated before archiving.
func BundleZip(defDir, archivePath string) error {
	if err := validateDefDir(defDir); err != nil {
		return fmt.Errorf("definition validation failed: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(archivePath), 0o755); err != nil { //nolint:gosec // intentional: standard directory permissions
		return fmt.Errorf("creating output directory: %w", err)
	}
	zipData, err := zipDirectory(defDir)
	if err != nil {
		return fmt.Errorf("creating zip: %w", err)
	}
	if err := os.WriteFile(archivePath, zipData, 0o644); err != nil { //nolint:gosec // archive output permissions
		return fmt.Errorf("writing zip file: %w", err)
	}
	return nil
}

// BundleTarXZ creates a tar.xz archive of the definition directory.
// defDir is validated before archiving.
func BundleTarXZ(defDir, archivePath string) error {
	if err := validateDefDir(defDir); err != nil {
		return fmt.Errorf("definition validation failed: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(archivePath), 0o755); err != nil { //nolint:gosec // intentional: standard directory permissions
		return fmt.Errorf("creating output directory: %w", err)
	}
	return createTarXZ(defDir, archivePath)
}

// BundleTarBZ2 creates a tar.bz2 archive of the definition directory.
// defDir is validated before archiving.
func BundleTarBZ2(defDir, archivePath string) error {
	if err := validateDefDir(defDir); err != nil {
		return fmt.Errorf("definition validation failed: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(archivePath), 0o755); err != nil { //nolint:gosec // intentional: standard directory permissions
		return fmt.Errorf("creating output directory: %w", err)
	}
	return createTarBZ2(defDir, archivePath)
}

// createTarXZ creates a tar.xz archive of the source directory at destPath.
//
//nolint:gocognit,gocyclo // file system walk + writer chain is inherently detailed
func createTarXZ(srcDir, destPath string) error {
	f, err := os.Create(destPath) //nolint:gosec // destPath is user-specified output path
	if err != nil {
		return fmt.Errorf("creating archive file: %w", err)
	}

	xzWriter, err := xz.NewWriter(f)
	if err != nil {
		_ = f.Close()
		return fmt.Errorf("creating xz writer: %w", err)
	}

	tw := tar.NewWriter(xzWriter)

	walkErr := filepath.WalkDir(srcDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relPath, relErr := filepath.Rel(srcDir, path)
		if relErr != nil {
			return relErr
		}
		if relPath == "tests" || strings.HasPrefix(relPath, "tests/") || strings.HasPrefix(relPath, "tests"+string(filepath.Separator)) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		info, statErr := d.Info()
		if statErr != nil {
			return statErr
		}
		header, headerErr := tar.FileInfoHeader(info, "")
		if headerErr != nil {
			return headerErr
		}
		header.Name = relPath
		if err := tw.WriteHeader(header); err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		src, openErr := os.Open(path) //nolint:gosec // path from WalkDir
		if openErr != nil {
			return openErr
		}
		_, copyErr := io.Copy(tw, src)
		_ = src.Close() // close immediately after copy, don't defer in WalkDir callback
		if copyErr != nil {
			return copyErr
		}
		return nil
	})

	// Close in order: tar → xz → file. Preserve first error (walkErr takes precedence).
	if closeErr := tw.Close(); closeErr != nil && walkErr == nil {
		walkErr = closeErr
	}
	if closeErr := xzWriter.Close(); closeErr != nil && walkErr == nil {
		walkErr = closeErr
	}
	if closeErr := f.Close(); closeErr != nil && walkErr == nil {
		walkErr = closeErr
	}
	return walkErr
}

// createTarBZ2 creates a tar.bz2 archive of the source directory at destPath.
//
//nolint:gocognit,gocyclo // file system walk + writer chain is inherently detailed
func createTarBZ2(srcDir, destPath string) error {
	f, err := os.Create(destPath) //nolint:gosec // destPath is user-specified output path
	if err != nil {
		return fmt.Errorf("creating archive file: %w", err)
	}

	bz2Writer, err := bz2.NewWriter(f, nil)
	if err != nil {
		_ = f.Close()
		return fmt.Errorf("creating bz2 writer: %w", err)
	}

	tw := tar.NewWriter(bz2Writer)

	walkErr := filepath.WalkDir(srcDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relPath, relErr := filepath.Rel(srcDir, path)
		if relErr != nil {
			return relErr
		}
		if relPath == "tests" || strings.HasPrefix(relPath, "tests/") || strings.HasPrefix(relPath, "tests"+string(filepath.Separator)) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		info, statErr := d.Info()
		if statErr != nil {
			return statErr
		}
		header, headerErr := tar.FileInfoHeader(info, "")
		if headerErr != nil {
			return headerErr
		}
		header.Name = relPath
		if err := tw.WriteHeader(header); err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		src, openErr := os.Open(path) //nolint:gosec // path from WalkDir
		if openErr != nil {
			return openErr
		}
		_, copyErr := io.Copy(tw, src)
		_ = src.Close() // close immediately after copy, don't defer in WalkDir callback
		if copyErr != nil {
			return copyErr
		}
		return nil
	})

	// Close in order: tar → bz2 → file. Preserve first error (walkErr takes precedence).
	if closeErr := tw.Close(); closeErr != nil && walkErr == nil {
		walkErr = closeErr
	}
	if closeErr := bz2Writer.Close(); closeErr != nil && walkErr == nil {
		walkErr = closeErr
	}
	if closeErr := f.Close(); closeErr != nil && walkErr == nil {
		walkErr = closeErr
	}
	return walkErr
}
