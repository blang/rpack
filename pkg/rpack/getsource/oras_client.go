package getsource

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	ociDigest "github.com/opencontainers/go-digest"
	ociv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"
	"oras.land/oras-go/v2/registry/remote/credentials"
)

// ORASStore is a real OCI registry client using oras-go/v2 remote.
// It implements both OCIRepositoryStore (for reading) and OCIPublisher (for writing).
type ORASStore struct {
	repo *remote.Repository
}

// NewORASStore creates a new ORASStore for the given registry and repository.
// Credentials are resolved using ORAS's built-in credential chain:
//  1. OCI_USERNAME / OCI_PASSWORD environment variables
//  2. Docker config file (~/.docker/config.json) — including credential helpers
//  3. Podman config file (~/.config/containers/auth.json)
//  4. Native credential stores (wincred, osxkeychain, pass, secretservice)
//  5. Anonymous (no authentication)
func NewORASStore(registryDomain, repositoryName string) (*ORASStore, error) {
	ref := registryDomain + "/" + repositoryName
	repo, err := remote.NewRepository(ref)
	if err != nil {
		return nil, fmt.Errorf("creating remote repository: %w", err)
	}

	store := newCredentialStore()

	repo.Client = &auth.Client{
		Credential: credentials.Credential(store),
	}
	return &ORASStore{repo: repo}, nil
}

// newCredentialStore creates a credential store reading from all standard
// OCI config files (Docker and Podman), with fallback to env vars.
func newCredentialStore() credentials.Store {
	stores := []credentials.Store{}

	// Podman config first (~/.config/containers/auth.json and $XDG_RUNTIME_DIR/containers/auth.json)
	if podmanStore, err := newPodmanStore(); err == nil {
		stores = append(stores, podmanStore)
	}

	// Docker config (handles ~/.docker/config.json, credHelpers, credsStore, native stores)
	if dockerStore, err := credentials.NewStoreFromDocker(credentials.StoreOptions{
		DetectDefaultNativeStore: true,
	}); err == nil {
		stores = append(stores, dockerStore)
	}

	// Environment variable fallback
	stores = append(stores, envCredentialStore{})

	if len(stores) == 1 {
		return stores[0]
	}
	return credentials.NewStoreWithFallbacks(stores[0], stores[1:]...)
}

// newPodmanStore creates a credential store from Podman's auth.json.
// Podman stores credentials under "docker.io" while ORAS expects
// "https://index.docker.io/v1/", so we use a simple file reader
// with address aliasing instead of ORAS's DynamicStore.
func newPodmanStore() (credentials.Store, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	candidates := []string{
		filepath.Join(home, ".config", "containers", "auth.json"),
	}
	if runtimeDir := os.Getenv("XDG_RUNTIME_DIR"); runtimeDir != "" {
		candidates = append([]string{filepath.Join(runtimeDir, "containers", "auth.json")}, candidates...)
	}

	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil { //nolint:gosec // path is from known config locations
			return newSimpleConfigStore(path)
		}
	}
	return nil, fmt.Errorf("no podman auth file found")
}

// ociConfig represents a Docker/Podman config.json or auth.json.
type ociConfig struct {
	Auths map[string]ociAuthEntry `json:"auths"`
}

type ociAuthEntry struct {
	Auth     string `json:"auth"`
	Username string `json:"username"`
	Password string `json:"password"`
}

// simpleConfigStore reads credentials from a Docker/Podman-style config JSON file.
// It handles Docker Hub address aliasing (e.g., registry-1.docker.io ↔ docker.io).
type simpleConfigStore struct {
	auths map[string]ociAuthEntry
}

func newSimpleConfigStore(path string) (*simpleConfigStore, error) {
	data, err := os.ReadFile(path) //nolint:gosec // path is from known config locations
	if err != nil {
		return nil, err
	}
	var cfg ociConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &simpleConfigStore{auths: cfg.Auths}, nil
}

func (s *simpleConfigStore) Get(_ context.Context, serverAddress string) (auth.Credential, error) {
	for _, key := range serverAddressAliases(serverAddress) {
		if cred, ok := s.lookup(key); ok {
			return cred, nil
		}
	}
	return auth.EmptyCredential, nil
}

func (s *simpleConfigStore) Put(_ context.Context, _ string, _ auth.Credential) error {
	return fmt.Errorf("simple config store does not support Put")
}

func (s *simpleConfigStore) Delete(_ context.Context, _ string) error {
	return fmt.Errorf("simple config store does not support Delete")
}

func (s *simpleConfigStore) lookup(key string) (auth.Credential, bool) {
	entry, ok := s.auths[key]
	if !ok {
		return auth.Credential{}, false
	}
	if entry.Username != "" && entry.Password != "" {
		return auth.Credential{Username: entry.Username, Password: entry.Password}, true
	}
	if entry.Auth != "" {
		decoded, err := base64.StdEncoding.DecodeString(entry.Auth)
		if err != nil {
			return auth.Credential{}, false
		}
		user, pass, ok := strings.Cut(string(decoded), ":")
		if ok {
			return auth.Credential{Username: user, Password: pass}, true
		}
	}
	return auth.Credential{}, false
}

// serverAddressAliases returns all possible keys for a server address
// that might appear in Docker/Podman config files.
// The ORAS credentials layer uses ServerAddressFromHostname which maps
// registry-1.docker.io → https://index.docker.io/v1/, so we need aliases
// for all three forms to match what podman/docker login stores.
func serverAddressAliases(serverAddress string) []string {
	aliases := []string{serverAddress}
	switch serverAddress {
	case "registry-1.docker.io", "index.docker.io":
		aliases = append(aliases, "docker.io", "https://index.docker.io/v1/")
	case "docker.io":
		aliases = append(aliases, "registry-1.docker.io", "https://index.docker.io/v1/")
	case "https://index.docker.io/v1/":
		aliases = append(aliases, "docker.io", "registry-1.docker.io")
	default:
	}
	return aliases
}

// envCredentialStore is a simple store that reads from OCI_USERNAME/OCI_PASSWORD env vars.
type envCredentialStore struct{}

func (envCredentialStore) Get(_ context.Context, _ string) (auth.Credential, error) {
	user := os.Getenv("OCI_USERNAME")
	pass := os.Getenv("OCI_PASSWORD")
	if user != "" && pass != "" {
		return auth.Credential{Username: user, Password: pass}, nil
	}
	return auth.EmptyCredential, nil
}

func (envCredentialStore) Put(_ context.Context, _ string, _ auth.Credential) error {
	return fmt.Errorf("env credential store does not support Put")
}

func (envCredentialStore) Delete(_ context.Context, _ string) error {
	return fmt.Errorf("env credential store does not support Delete")
}

var _ credentials.Store = envCredentialStore{}

// --- OCIRepositoryStore implementation ---

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

// --- OCIPublisher interface ---

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
