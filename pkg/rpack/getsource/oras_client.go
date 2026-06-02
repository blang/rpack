package getsource

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"

	ociDigest "github.com/opencontainers/go-digest"
	ociv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"
)

// ORASStore is a real OCI registry client using oras-go/v2 remote.
// It implements both OCIRepositoryStore (for reading) and OCIPublisher (for writing).
type ORASStore struct {
	repo *remote.Repository
}

// NewORASStore creates a new ORASStore for the given registry and repository.
// Credentials are read from OCI_USERNAME and OCI_PASSWORD environment variables.
// If credentials are not set, anonymous access is attempted.
func NewORASStore(registryDomain, repositoryName string) (*ORASStore, error) {
	ref := registryDomain + "/" + repositoryName
	repo, err := remote.NewRepository(ref)
	if err != nil {
		return nil, fmt.Errorf("creating remote repository: %w", err)
	}
	repo.Client = &auth.Client{
		Credential: func(ctx context.Context, registry string) (auth.Credential, error) {
			username := os.Getenv("OCI_USERNAME")
			password := os.Getenv("OCI_PASSWORD")
			if username != "" && password != "" {
				return auth.Credential{
					Username: username,
					Password: password,
				}, nil
			}
			return auth.Credential{}, nil // anonymous
		},
	}
	return &ORASStore{repo: repo}, nil
}

// Resolve resolves a tag or digest to a descriptor.
func (s *ORASStore) Resolve(ctx context.Context, reference string) (ociv1.Descriptor, error) {
	return s.repo.Resolve(ctx, reference)
}

// Fetch retrieves the content of a blob identified by the descriptor.
//
//nolint:gocritic // target is OCI standard type passed by value
func (s *ORASStore) Fetch(ctx context.Context, target ociv1.Descriptor) (io.ReadCloser, error) {
	return s.repo.Fetch(ctx, target)
}

var _ OCIRepositoryStore = (*ORASStore)(nil)

// OCIPublisher is the interface for pushing artifacts to an OCI registry.
type OCIPublisher interface {
	// PushBlob pushes a blob to the registry and returns its descriptor.
	PushBlob(ctx context.Context, mediaType string, content io.Reader) (ociv1.Descriptor, error)
	// PushManifest pushes a manifest referencing the given layers.
	PushManifest(ctx context.Context, artifactType string, layers []ociv1.Descriptor) (ociv1.Descriptor, error)
	// Tag creates a tag pointing to a manifest descriptor.
	Tag(ctx context.Context, desc ociv1.Descriptor, tagName string) error
}

// PushBlob implements OCIPublisher.
func (s *ORASStore) PushBlob(ctx context.Context, mediaType string, content io.Reader) (ociv1.Descriptor, error) {
	data, err := io.ReadAll(content)
	if err != nil {
		return ociv1.Descriptor{}, fmt.Errorf("reading blob content: %w", err)
	}
	desc := ociv1.Descriptor{
		MediaType: mediaType,
		Digest:    ociDigest.FromBytes(data),
		Size:      int64(len(data)),
	}
	if err := s.repo.Push(ctx, desc, bytes.NewReader(data)); err != nil {
		return ociv1.Descriptor{}, fmt.Errorf("pushing blob: %w", err)
	}
	return desc, nil
}

// PushManifest implements OCIPublisher.
func (s *ORASStore) PushManifest(ctx context.Context, artifactType string, layers []ociv1.Descriptor) (ociv1.Descriptor, error) {
	desc, err := oras.PackManifest(ctx, s.repo, oras.PackManifestVersion1_1, artifactType, oras.PackManifestOptions{
		Layers: layers,
	})
	if err != nil {
		return ociv1.Descriptor{}, fmt.Errorf("pushing manifest: %w", err)
	}
	return desc, nil
}

// Tag implements OCIPublisher.
//
//nolint:gocritic // desc is OCI standard type passed by value
func (s *ORASStore) Tag(ctx context.Context, desc ociv1.Descriptor, tagName string) error {
	return s.repo.Tag(ctx, desc, tagName)
}

var _ OCIPublisher = (*ORASStore)(nil)
