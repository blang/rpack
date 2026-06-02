# Progress

## Status
**COMPLETE** — Implementation done. Awaiting Docker Hub integration testing.

## Tasks

| # | Task | Status |
|---|------|--------|
| 1.1 | detectors.go + tests | ✅ |
| 1.2 | getters.go + decompressors.go + tests | ✅ |
| 2.1 | source.go + tests | ✅ |
| 2.2 | oci_getter.go + tests | ✅ |
| 3.1 | fetcher.go + tests | ✅ |
| 4.1 | loader.go wiring + regression tests | ✅ |
| 5.1 | oras_client.go + tests | ✅ |
| 5.2 | publisher.go + tests | ✅ |
| 5.3 | CLI publish command | ✅ |
| FIX | Review findings (1 round) | ✅ |

## Files Changed

### New: `pkg/rpack/getsource/` (15 files)
| File | Lines | Purpose |
|------|-------|---------|
| `detectors.go` | 94 | Curated detectors + fileDetector + withoutQueryParams |
| `detectors_test.go` | 134 | 9 detector tests |
| `getters.go` | 13 | Curated static getter map |
| `decompressors.go` | 31 | Curated decompressors + OCI media-type mapping |
| `decompressors_test.go` | 45 | Consistency tests |
| `source.go` | 28 | NormalizeSource + SplitSourceSubdir |
| `source_test.go` | 116 | 11 normalization tests |
| `oci_getter.go` | ~310 | OCI Distribution getter (implementing getter.Getter) |
| `oci_getter_test.go` | ~320 | 10 OCI tests with in-memory store |
| `fetcher.go` | 63 | Fetcher struct + DefaultFetcher + Fetch |
| `fetcher_test.go` | 65 | 3 fetcher tests |
| `oras_client.go` | 105 | Real ORAS remote client (OCIRepositoryStore + OCIPublisher) |
| `oras_client_test.go` | 80 | 5 ORAS client tests |
| `publisher.go` | 127 | PublishRPack + zipDirectory + parseOCIRef |
| `publisher_test.go` | 235 | 4 publisher tests (in-memory end-to-end) |

### Modified
| File | Change |
|------|--------|
| `pkg/rpack/loader.go` | Replaced raw go-getter with getsource calls |
| `pkg/rpack/loader_test.go` | +4 regression tests |
| `pkg/cmd/publish.go` | New CLI command |
| `go.mod` | go-getter v1.7.8→v1.8.5, +oras-go, +opencontainers |

## Test Summary
- **getsource package:** 49 tests (all pass)
- **rpack package (total):** 126 tests (all pass)
- **Lua tests:** 4/4 pass
- **Lint:** 0 issues
- **Build:** succeeds

## Notes

### What user needs for integration testing:
```bash
# Set Docker Hub credentials
export OCI_USERNAME="your-username"
export OCI_PASSWORD="your-token"

# Publish an rpack definition
rpack publish --def ./examples/basic/rpackdef/basic --target oci://registry-1.docker.io/username/my-pack?tag=v1

# Use the published pack as a source
# In rpack.yaml:
#   source: "oci://registry-1.docker.io/username/my-pack?tag=v1"

# For local testing without Docker Hub:
# Start a local OCI registry (e.g., docker run -d -p 5000:5000 registry:2)
# Then publish to oci://localhost:5000/repo/pack?tag=v1
```
