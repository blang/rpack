package getsource

import (
	"context"
	"maps"
	"net/http"

	getter "github.com/hashicorp/go-getter"
)

// Fetcher downloads sources using go-getter with curated configuration.
type Fetcher struct {
	// NewOCIRepositoryStore is a factory for creating OCI repository stores.
	// When nil, OCI sources are unavailable.
	// DefaultFetcher sets this to use the standard credential chain
	// (Podman, Docker config, env vars, credential helpers).
	NewOCIRepositoryStore func(ctx context.Context, registryDomain, repositoryName string) (OCIRepositoryStore, error)

	httpClient *http.Client
}

// DefaultFetcher creates a Fetcher with standard OCI credential support
// (reading from Podman, Docker config, env vars, and credential helpers)
// and a default HTTP client.
func DefaultFetcher() *Fetcher {
	return &Fetcher{
		httpClient: http.DefaultClient,
		NewOCIRepositoryStore: func(ctx context.Context, registryDomain, repositoryName string) (OCIRepositoryStore, error) {
			return NewORASStore(registryDomain, repositoryName)
		},
	}
}

// Fetch downloads the source at the given normalized address into destDir.
// The sourceAddr must already be normalized (e.g. via NormalizeSource).
func (f *Fetcher) Fetch(ctx context.Context, destDir, sourceAddr string) error {
	// Build the complete getter map, adding dynamic entries
	getters := make(map[string]getter.Getter, len(Getters)+3)
	maps.Copy(getters, Getters)

	// HTTP/HTTPS getter
	httpGetter := &getter.HttpGetter{
		Client: f.httpClient,
		Netrc:  true,
	}
	getters["http"] = httpGetter
	getters["https"] = httpGetter

	// OCI getter (only if NewOCIRepositoryStore is configured)
	if f.NewOCIRepositoryStore != nil {
		getters["oci"] = &ociDistributionGetter{
			getOCIRepositoryStore: f.NewOCIRepositoryStore,
		}
	}

	client := &getter.Client{
		Src: sourceAddr,
		Dst: destDir,
		Pwd: destDir,

		Mode: getter.ClientModeDir,

		Detectors:     Detectors,
		Decompressors: Decompressors,
		Getters:       getters,
		Ctx:           ctx,
	}

	return client.Get()
}
