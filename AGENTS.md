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
just lint-lua     # selene + stylua --check on lua/src/
just lint-lua-fix # stylua auto-format on lua/src/
just lint-cue     # cue fmt --check on all .cue files
just lint-cue-fix # cue fmt auto-format on all .cue files
just lint-yaml    # yamllint -s .
just lint-all     # run all linters (Go, Lua, CUE, YAML)
just fix          # goimports + stylua + cue fmt + go mod tidy
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

**All tools must be installed exclusively via `mise`.** Do not use brew, apt, pip, or other package managers. Run `mise install` to set up all tools.

| Tool | Version | Notes |
|------|---------|-------|
| Go | 1.24 | go.mod; mise.toml |
| golangci-lint | 2 | `.golangci.yml` |
| goimports | latest | `go:golang.org/x/tools/cmd/goimports` |
| prek | 0.3 | `prek.toml` |
| selene | latest | `aqua:Kampfkarren/selene`; `selene.toml` |
| stylua | latest | `.stylua.toml` |
| cue | latest | CUE schema formatter/validator |
| yamllint | latest | `.yamllint.yml` |

`mise activate` auto-installs pre-commit hooks via the `enter` hook.

## Linting

**Go**: golangci-lint v2 config (`.golangci.yml`): 30 linters including `modernize`, `gocritic`, `revive`, `gosec`. Excludes `dist/` and `lua/` directories. Complexity thresholds: gocyclo 15, gocognit 20.

**Lua**: selene for linting (`selene.toml`, std `lua51`, excludes stub files), stylua for formatting (`.stylua.toml`, 4-space indent, 120-column width).

**CUE**: `cue fmt --check` validates formatting of all `.cue` files. Schema validation is done by Go code at runtime.

**YAML**: yamllint (`.yamllint.yml`, strict mode, 120-column width, excludes `.serena/` and `dist/`).

Pre-commit hooks (prek): builtin file checks + local hooks for Go (gofmt, goimports, golangci-lint, go-mod-tidy), Lua (selene, stylua --check), CUE (cue fmt --check), and YAML (yamllint).

## Build

Version info injected via linker flags (`BuildVersion`, `BuildCommit`, `BuildTime`). Cross-compilation uses `CGO_ENABLED=0`. Output binary: `dist/rpack-{os}-{arch}`.

## Testing

Tests are co-located (`*_test.go` in same package). Run all: `just test`. Run Lua tests only: `just testlua`.

## Conventions

- Logging: `slog` with `devslog` for colored output
- Task runner: `just` (not make)
- Git hooks: `prek` (not pre-commit)
- Tool installation: `mise` only (not brew, apt, pip, or other package managers)
- Scripting: Lua (not JS); `lua/` dir excluded from Go linting
- `.serena/` and `cue.mod/` excluded via `.git/info/exclude` (not `.gitignore`)
