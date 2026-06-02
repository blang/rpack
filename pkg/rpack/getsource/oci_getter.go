package getsource

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"

	getter "github.com/hashicorp/go-getter"
	ociDigest "github.com/opencontainers/go-digest"
	ociv1 "github.com/opencontainers/image-spec/specs-go/v1"
	orasContent "oras.land/oras-go/v2/content"
	orasRegistry "oras.land/oras-go/v2/registry"
)

// OCIArtifactType is the artifact type expected for RPack module packages
// stored in OCI registries.
const OCIArtifactType = "application/vnd.rpack.modulepkg"

// ociManifestSizeLimitMiB is the maximum size of an OCI manifest we'll accept.
// This matches the OCI Distribution v1.1 spec recommended repository limit.
const ociManifestSizeLimitMiB = 4

// OCIRepositoryStore is the interface for interacting with a single OCI
// Distribution repository. Implementations handle authentication and
// provide reference resolution and blob fetching.
type OCIRepositoryStore interface {
	// Resolve finds the descriptor for the given reference, which may be
	// a tag name or a digest string.
	Resolve(ctx context.Context, reference string) (ociv1.Descriptor, error)

	// Fetch retrieves the blob content for the given descriptor.
	// Callers MUST close the returned reader.
	Fetch(ctx context.Context, target ociv1.Descriptor) (io.ReadCloser, error)
}

// ociDistributionGetter implements getter.Getter for OCI Distribution registries.
// It fetches RPack module packages from OCI registries using the ORAS client.
type ociDistributionGetter struct {
	getOCIRepositoryStore func(ctx context.Context, registryDomain, repositoryName string) (OCIRepositoryStore, error)
	client                *getter.Client
}

var _ getter.Getter = (*ociDistributionGetter)(nil)

// Get implements getter.Getter. Downloads the module package from the OCI registry
// into destDir.
func (g *ociDistributionGetter) Get(destDir string, u *url.URL) error {
	ctx := g.context()

	ref, err := g.resolveRepositoryRef(u)
	if err != nil {
		return err
	}
	store, err := g.getOCIRepositoryStore(ctx, ref.Registry, ref.Repository)
	if err != nil {
		return fmt.Errorf("configuring OCI client for %s: %w", ref, err)
	}
	manifestDesc, err := g.resolveManifestDescriptor(ctx, ref, u.Query(), store)
	if err != nil {
		return err
	}
	manifest, err := g.fetchOCIManifest(ctx, manifestDesc, store)
	if err != nil {
		return err
	}
	pkgDesc, err := selectOCILayer(manifest.Layers)
	if err != nil {
		return err
	}
	decompKey := decompressorMediaTypes[pkgDesc.MediaType]
	decomp := Decompressors[decompKey]
	if decomp == nil {
		return fmt.Errorf("no decompressor available for media type %q", pkgDesc.MediaType)
	}
	tempFile, err := g.fetchOCIBlobToTempFile(ctx, pkgDesc, store)
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tempFile) }()

	var umask os.FileMode
	if g.client != nil {
		umask = g.client.Umask
	}
	return decomp.Decompress(destDir, tempFile, true, umask)
}

// GetFile implements getter.Getter. Not supported for OCI sources because
// the archive format is auto-detected from the image manifest.
func (g *ociDistributionGetter) GetFile(dst string, u *url.URL) error {
	return fmt.Errorf("the \"archive\" argument is not allowed for OCI sources; the archive format is detected automatically from the image manifest")
}

// ClientMode implements getter.Getter. Always returns ClientModeDir.
func (g *ociDistributionGetter) ClientMode(u *url.URL) (getter.ClientMode, error) {
	return getter.ClientModeDir, nil
}

// SetClient implements getter.Getter. Called by the go-getter framework
// to provide the client configuration.
func (g *ociDistributionGetter) SetClient(client *getter.Client) {
	g.client = client
}

func (g *ociDistributionGetter) context() context.Context {
	if g.client != nil {
		return g.client.Ctx
	}
	return context.Background()
}

func (g *ociDistributionGetter) resolveRepositoryRef(u *url.URL) (*orasRegistry.Reference, error) {
	if !u.IsAbs() {
		return nil, fmt.Errorf("OCI source type requires an absolute URL")
	}
	if u.Scheme != "oci" {
		return nil, fmt.Errorf("OCI source type only supports oci:// URL scheme")
	}
	registryDomainName := u.Host
	repositoryName := strings.TrimPrefix(u.Path, "/")
	if repositoryName == "" {
		return nil, fmt.Errorf("OCI source requires a repository path")
	}
	ref := &orasRegistry.Reference{
		Registry:   registryDomainName,
		Repository: repositoryName,
	}
	if err := ref.Validate(); err != nil {
		return nil, fmt.Errorf("invalid OCI reference: %w", err)
	}
	return ref, nil
}

// resolveManifestDescriptor resolves the manifest descriptor from the OCI registry,
// either by tag or by digest, using the query parameters from the source URL.
func (g *ociDistributionGetter) resolveManifestDescriptor(ctx context.Context, ref *orasRegistry.Reference, query url.Values, store OCIRepositoryStore) (ociv1.Descriptor, error) {
	wantTag, wantDigest, err := parseOCIQuery(ref, query)
	if err != nil {
		return ociv1.Descriptor{}, err
	}
	if wantTag == "" && wantDigest == "" {
		wantTag = "latest"
	}

	var desc ociv1.Descriptor
	if wantTag != "" {
		desc, err = store.Resolve(ctx, wantTag)
		if err != nil {
			return ociv1.Descriptor{}, fmt.Errorf("resolving tag %q: %w", wantTag, err)
		}
	} else {
		desc, err = store.Resolve(ctx, wantDigest.String())
		if err != nil {
			return ociv1.Descriptor{}, fmt.Errorf("resolving digest: %w", err)
		}
	}

	if desc.MediaType != ociv1.MediaTypeImageManifest {
		return ociv1.Descriptor{}, fmt.Errorf("selected object is not an OCI image manifest")
	}
	return desc, nil
}

// parseOCIQuery extracts the tag and digest from the query parameters.
//
//nolint:gocognit // straightforward query parameter parsing
func parseOCIQuery(ref *orasRegistry.Reference, query url.Values) (wantTag string, wantDigest ociDigest.Digest, err error) {
	var unsupportedArgs []string
	for name, values := range query {
		if len(values) > 1 {
			return "", "", fmt.Errorf("too many %q arguments", name)
		}
		value := values[0]
		switch name {
		case "tag":
			if value == "" {
				return "", "", fmt.Errorf("tag argument must not be empty")
			}
			tagRef := *ref
			tagRef.Reference = value
			if tagErr := tagRef.ValidateReferenceAsTag(); tagErr != nil {
				return "", "", tagErr
			}
			wantTag = value
		case "digest":
			if value == "" {
				return "", "", fmt.Errorf("digest argument must not be empty")
			}
			d, parseErr := ociDigest.Parse(value)
			if parseErr != nil {
				return "", "", fmt.Errorf("invalid digest: %w", parseErr)
			}
			wantDigest = d
		default:
			unsupportedArgs = append(unsupportedArgs, name)
		}
	}

	switch len(unsupportedArgs) {
	case 1:
		return "", "", fmt.Errorf("unsupported argument %q", unsupportedArgs[0])
	default:
		if len(unsupportedArgs) >= 2 {
			return "", "", fmt.Errorf("unsupported arguments: %s", strings.Join(unsupportedArgs, ", "))
		}
	}

	if wantTag != "" && wantDigest != "" {
		return "", "", fmt.Errorf("cannot set both \"tag\" and \"digest\" arguments")
	}
	return wantTag, wantDigest, nil
}

// fetchOCIManifest retrieves and validates the image manifest from the store.
//
//nolint:gocritic // desc is OCI standard type passed by value
func (g *ociDistributionGetter) fetchOCIManifest(ctx context.Context, desc ociv1.Descriptor, store OCIRepositoryStore) (*ociv1.Manifest, error) {
	if (desc.Size / 1024 / 1024) > ociManifestSizeLimitMiB {
		return nil, fmt.Errorf("manifest size exceeds RPack's limit of %d MiB", ociManifestSizeLimitMiB)
	}

	rc, err := store.Fetch(ctx, desc)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rc.Close() }()

	manifestSrc, err := io.ReadAll(io.LimitReader(rc, desc.Size))
	if err != nil {
		return nil, fmt.Errorf("reading manifest content: %w", err)
	}

	gotDigest := desc.Digest.Algorithm().FromBytes(manifestSrc)
	if gotDigest != desc.Digest {
		return nil, fmt.Errorf("manifest content does not match digest %s", desc.Digest)
	}

	var manifest ociv1.Manifest
	if err := json.Unmarshal(manifestSrc, &manifest); err != nil {
		var idx ociv1.Index
		if err2 := json.Unmarshal(manifestSrc, &idx); err2 == nil && idx.MediaType == ociv1.MediaTypeImageIndex {
			return nil, fmt.Errorf("found index manifest but need image manifest")
		}
		return nil, fmt.Errorf("invalid manifest content: %w", err)
	}

	if manifest.MediaType != desc.MediaType {
		return nil, fmt.Errorf("unexpected manifest media type %q", manifest.MediaType)
	}
	if manifest.ArtifactType != OCIArtifactType {
		return nil, fmt.Errorf("unexpected artifact type %q (expected %q)", manifest.ArtifactType, OCIArtifactType)
	}
	return &manifest, nil
}

func selectOCILayer(descs []ociv1.Descriptor) (ociv1.Descriptor, error) {
	foundBlobs := make(map[string]ociv1.Descriptor, len(decompressorMediaTypes))
	foundWrongMT := 0
	for _, desc := range descs {
		mediaType := desc.MediaType
		if _, ok := decompressorMediaTypes[mediaType]; ok {
			if _, exists := foundBlobs[mediaType]; exists {
				return ociv1.Descriptor{}, fmt.Errorf("multiple layers with media type %q", mediaType)
			}
			foundBlobs[mediaType] = desc
		} else {
			foundWrongMT++
		}
	}
	if len(foundBlobs) == 0 {
		if foundWrongMT > 0 {
			return ociv1.Descriptor{}, fmt.Errorf(
				"image manifest contains no layers of types supported as module packages by RPack, but has other unsupported formats")
		}
		return ociv1.Descriptor{}, fmt.Errorf("image manifest contains no layers of types supported as module packages by RPack")
	}
	for _, mediaType := range ociBlobMediaTypePreference {
		if desc, ok := foundBlobs[mediaType]; ok {
			return desc, nil
		}
	}
	return ociv1.Descriptor{}, fmt.Errorf("image manifest contains no layers of types supported as module packages by RPack")
}

// fetchOCIBlobToTempFile downloads a blob into a temporary file.
//
//nolint:gocritic // desc is OCI standard type passed by value
func (g *ociDistributionGetter) fetchOCIBlobToTempFile(ctx context.Context, desc ociv1.Descriptor, store orasContent.Fetcher) (string, error) {
	f, err := os.CreateTemp("", "rpack-module")
	if err != nil {
		return "", fmt.Errorf("failed to create temporary file: %w", err)
	}
	tempFile := f.Name()

	rc, err := store.Fetch(ctx, desc)
	if err != nil {
		_ = f.Close()
		_ = os.Remove(tempFile)
		return "", err
	}
	defer func() { _ = rc.Close() }()

	v := orasContent.NewVerifyReader(rc, desc)
	_, err = getter.Copy(ctx, f, v)
	_ = f.Close()
	if err != nil {
		_ = os.Remove(tempFile)
		return "", err
	}
	if err := v.Verify(); err != nil {
		_ = os.Remove(tempFile)
		return "", fmt.Errorf("invalid blob returned from registry: %w", err)
	}

	return tempFile, nil
}
