# AGENTS.md

## Project

RPack — a file package manager with Lua scripting, CUE schema validation, and lockfiles. Module: `github.com/blang/rpack`. Beta: API may change.

## Commands

```
just dev          # goimports → build → run
just test         # go test -v ./...
just testlua      # go test -v -test.run '.*Lua.*' ./...
just lint         # golangci-lint run (alias: just check)
just lint-fix     # golangci-lint run --fix
just lint-ci      # golangci-lint run --out-format=line-number
just fix          # goimports -w ./ + go mod tidy
just build        # cross-compile to dist/ (default linux/amd64)
just build-all    # all platforms (linux/darwin, amd64/arm64)
just example      # build + run examples/basic/root/basic.rpack.yaml
just ldoc         # generate Lua API docs (requires ldoc)
just prek-install # install git pre-commit hooks
just prek-run     # run all pre-commit hooks on all files
```

## Architecture

- **CLI**: `cmd/rpack/main.go` → `pkg/cmd/` (Cobra commands: `run`, `check`)
- **Core**: `pkg/rpack/` — executor, loader, file resolver, Lua bindings
- **Lua libs**: `lua/src/` — embedded via gopher-lua; `rpack` and `filepath` libraries exposed to scripts
- **CUE schemas**: two separate files — `schema.cue` validates rpack.yaml configs (source, inputs, values); `def_schema.cue` validates rpack definitions (name, inputs declaration)
- **Sources**: git, https, s3 via go-getter

## Tooling

Tools managed by mise (run `mise install` to set up):

| Tool | Version | Notes |
|------|---------|-------|
| Go | 1.24.1 | go.mod; mise.toml uses `"1.24"` |
| golangci-lint | 2 | `.golangci.yml` |
| goimports | latest | `go:golang.org/x/tools/cmd/goimports` |
| prek | 0.3 | `prek.toml` |

`mise activate` auto-installs pre-commit hooks via the `enter` hook.

## Linting

golangci-lint v2 config (`.golangci.yml`): 30 linters including `modernize`, `gocritic`, `revive`, `gosec`. Excludes `dist/` and `lua/` directories. Complexity thresholds: gocyclo 15, gocognit 20.

Pre-commit hooks (prek): builtin file checks + local Go hooks (gofmt, goimports, golangci-lint, go-mod-tidy).

## Build

Version info injected via linker flags (`BuildVersion`, `BuildCommit`, `BuildTime`). Cross-compilation uses `CGO_ENABLED=0`. Output binary: `dist/rpack-{os}-{arch}`.

## Testing

Tests are co-located (`*_test.go` in same package). Run all: `just test`. Run Lua tests only: `just testlua`.

## Conventions

- Logging: `slog` with `devslog` for colored output
- Task runner: `just` (not make)
- Git hooks: `prek` (not pre-commit)
- Scripting: Lua (not JS); `lua/` dir excluded from Go linting
- `.serena/` excluded via `.git/info/exclude` (not `.gitignore`)
