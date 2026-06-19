# RPack

A package manager for files — bundle files with Lua scripting and CUE validation, distribute them via git/https/s3, and apply them with user-specific values and inputs.

An **rpack author** creates a bundle containing templates, a Lua script, and an optional CUE schema. An **rpack user** references the bundle in a config file, provides values and input files, and runs `rpack run` to generate output files.

Think [Helm](https://helm.sh/) for arbitrary files, [vendir](https://carvel.dev/vendir/) with templating, or [kustomize](https://github.com/kubernetes-sigs/kustomize) but scriptable.

Use cases: distribute templated config files across repositories, package dotfiles as versioned bundles, share GitHub Actions workflows or pre-commit configs, migrate repositories deterministically.

[![Go Reference](https://pkg.go.dev/badge/github.com/blang/rpack.svg)](https://pkg.go.dev/github.com/blang/rpack)
[![Go Report Card](https://goreportcard.com/badge/github.com/blang/rpack)](https://goreportcard.com/report/github.com/blang/rpack)

## Install

```
go install github.com/blang/rpack/cmd/rpack@latest
```

## Quick start

```shell
# Create a config pointing to an rpack
cat > example.rpack.yaml <<'EOF'
"@schema_version": "v1"
source: "git::https://github.com/blang/rpack.git//examples/intro/rpackdef"
config:
  values:
    author: "blang"
  inputs:
    "users.yaml": ./myusers.yaml
EOF

# Dry-run to preview changes
rpack run --dry-run ./example.rpack.yaml

# Apply
rpack run ./example.rpack.yaml
```

## Worked example

This example shows the full flow: an rpack bundle that copies a static file, reads user data, and templates output.

### RPack bundle (`rpackdef/`)

The author creates a directory with these files:

**`rpack.yaml`** — declares the bundle name and what inputs it expects:
```yaml
"@schema_version": "v1"
name: "intro"
inputs:
  - name: users.yaml
    type: file
```

**`script.lua`** — the Lua script that processes files:
```lua
local rpack = require("rpack.v1")
local values = rpack.values()

-- Copy a bundled file to the target directory
rpack.copy("rpack:files/intro.md", "./rpack_intro.md")

-- Read user input, template, and write output
local users = rpack.from_yaml(rpack.read("map:users.yaml"))
local output = rpack.template(rpack.read("rpack:files/users.md.tmpl"), {
    users = users,
    author = values.author,
})
rpack.write("./rpack_users.md", output)
```

**`schema.cue`** (optional) — validates the user's values:
```cue
#Schema: {
    author!: string
}
```

**`files/intro.md`** — a static file bundled with the rpack:
```markdown
# RPack Intro

This file was created because you applied the intro rpack.
```

**`files/users.md.tmpl`** — a Go text/template that uses the user's data:
```markdown
# Users

Author: {{ .author }}
{{ range .users -}}
- {{ .firstname }} {{ .lastname }}{{ if .email }} <{{ .email }}>{{ end }}
{{ end -}}
```

### User side

The user creates a config and provides input files:

**`intro.rpack.yaml`**:
```yaml
"@schema_version": "v1"
source: "../rpackdef"
config:
  values:
    author: blang
  inputs:
    "users.yaml": ./myusers.yaml
```

**`myusers.yaml`** — the input file mapped to `users.yaml`:
```yaml
- firstname: Alice
  lastname: Johnson
  email: alice.johnson@example.com
- firstname: Bob
  lastname: Smith
```

### Result

Running `rpack run ./intro.rpack.yaml` produces:

**`rpack_intro.md`** — copied from the bundle:
```markdown
# RPack Intro

This file was created because you applied the intro rpack.
```

**`rpack_users.md`** — generated from the template:
```markdown
# Users

Author: blang
- Alice Johnson <alice.johnson@example.com>
- Bob Smith
```

A lockfile is also written alongside the config, tracking all managed files with SHA256 checksums.

## Concepts

### Execution model

`rpack run` loads your config, fetches the rpack source, validates values/inputs against the definition's schema, executes the Lua script in a sandboxed filesystem, then writes output files and a lockfile.

### Filesystem sandbox

Scripts access files through four prefixes:

| Prefix | Access | Description |
|--------|--------|-------------|
| `rpack:files/x` | Read-only | Files bundled in the rpack definition |
| `map:name` | Read-only | User-mapped input files/dirs |
| `temp:name` | Read/Write | Temporary files during execution |
| `./path` | Write-only | Target directory (alongside the rpack.yaml) |

Writes to `rpack:` or `map:` are blocked. Reads from the target directory are blocked (ensures purity — scripts can't read files they're about to overwrite).

### Purity

Scripts are pure: same inputs always produce same outputs. The executor detects read-after-write conflicts and fails if a script reads a file it previously wrote. This guarantees idempotent execution.

### Lockfiles

After execution, rpack writes a lockfile tracking all output files with SHA256 checksums. On subsequent runs, rpack verifies that managed files haven't been modified externally. Use `--force` to override. Files removed from the lockfile are cleaned up automatically.

## Configuration

User configs are `*.rpack.yaml` files:

```yaml
"@schema_version": "v1"
source: "git::https://github.com/user/repo//path/to/rpackdef"  # git, https, s3
config:
  values:          # Arbitrary data passed to the Lua script
    author: "blang"
  inputs:          # Map input names to local paths
    "users.yaml": ./myusers.yaml
```

## Lua API

The `rpack.v1` module is the scripting interface:

### File operations

| Function | Signature | Description |
|----------|-----------|-------------|
| `read` | `read(path) → string` | Read file contents. Path uses sandbox prefixes. |
| `write` | `write(path, content)` | Write string to target file. |
| `copy` | `copy(src, dst)` | Copy file. Both paths use sandbox prefixes. |
| `read_dir` | `read_dir(path, recursive?) → files, dirs` | List directory contents. Returns two tables. |

### Data parsing

| Function | Signature | Description |
|----------|-----------|-------------|
| `from_yaml` | `from_yaml(str) → table` | Parse YAML string to Lua table. |
| `to_yaml` | `to_yaml(table) → string` | Serialize Lua table as YAML. |
| `from_json` | `from_json(str) → table` | Parse JSON string to Lua table. |
| `to_json` | `to_json(table) → string` | Serialize Lua table as JSON. |

### Templating & queries

| Function | Signature | Description |
|----------|-----------|-------------|
| `template` | `template(tmpl, data, leftDelim?, rightDelim?) → string` | Execute Go [`text/template`](https://pkg.go.dev/text/template) with data. Optional custom delimiters. |
| `jq` | `jq(query, data) → table` | Execute [gojq](https://github.com/itchyny/gojq) query on data. |

### External data

| Function | Signature | Description |
|----------|-----------|-------------|
| `values` | `values() → table` | User-supplied config values. |
| `inputs` | `inputs() → table` | List of user-supplied input names. |

## Creating an rpack

An rpack bundle is a directory containing:

| File | Required | Description |
|------|----------|-------------|
| `rpack.yaml` | Yes | Name and input declarations. See [def_schema.cue](./pkg/rpack/def_schema.cue). |
| `script.lua` | Yes | Lua script using `rpack.v1` API. |
| `schema.cue` | No | CUE schema to validate user `values`. |
| `files/` | No | Static files accessible via `rpack:` prefix. |

Validate the bundle with `rpack validate --def ./your-rpack` before publishing.
Distribute via git, https, or s3. Publish to an OCI registry with `rpack publish -T oci`.

See [examples/](./examples) for complete examples.

## Agentic Usage

The [skills/](./skills) directory contains AI agent skills for guided rpack development. Install them in your agent harness to let LLMs create and test rpack definitions:

- **rpack-author** — creates rpack definitions with correct structure, Lua scripting, and CUE schemas
- **rpack-tester** — generates tests that validate rpack output and catch common mistakes

## CLI reference

### `rpack run [--def <dir>] [flags] [<config-file>]`

Execute an rpack from a user config file or a local definition directory.

**Normal mode** — full pipeline (source download, validation, execution, lockfile):
```
rpack run ./app.rpack.yaml
rpack run ./app.rpack.yaml --dry-run
```

**`--def` mode** — run directly against a local definition (skips source download, config loading, lockfile):
```
rpack run --def ./my-rpack --set author=test --output-dir /tmp/out
rpack run --def ./my-rpack --set author=test --dry-run
```

| Flag | Short | Description |
|------|-------|-------------|
| `--def` | `-d` | Use a local definition directory. Mutually exclusive with `<config-file>`. |
| `--set key=value` | | Set a config value (`--def` only, repeatable). Dot notation for nesting, auto-detects int/bool/float/string. |
| `--set-input name=path` | | Map an input name to a local file or directory (`--def` only, repeatable). |
| `--output-dir` | | Write output files to this directory. Creates `meta.json` alongside. Mutually exclusive with `--dry-run`. |
| `--dry-run` | | Preview changes. In `--def` mode, prints each file's path and content to stdout. |
| `--force` | `-f` | Overwrite files, ignore lockfile integrity warnings. With `--output-dir`, allow overwriting non-empty directories. |
| `--working-dir` | `-w` | Override working directory (default: config file location) |
| `--debug` | | Enable verbose logging |

### `rpack check <config>`

Verify lockfile integrity — checks that all managed files exist and haven't been modified externally.

| Flag | Short | Description |
|------|-------|-------------|
| `--working-dir` | `-w` | Override working directory |
| `--debug` | | Enable verbose logging |

### `rpack test --def <dir> [--filter <name>] [--init <name>]`

Discover and run test scripts in a definition's `tests/` directory.

Each test is a subdirectory of `tests/` containing an executable script
(`run.sh`, `run.py`, or `run`). The script receives two positional arguments:
`$1` = definition directory, `$2` = temp output directory. Exit 0 = pass,
non-zero = fail.

| Flag | Short | Description |
|------|-------|-------------|
| `--def` | `-d` | Path to rpack definition directory (required) |
| `--filter` | | Run only tests whose directory name contains this substring |
| `--init <name>` | | Scaffold a new test directory `tests/<name>/` with a template `run.sh` |

### `rpack validate --def <dir>`

Validate an rpack definition directory. Checks that rpack.yaml is schema-valid,
script.lua exists, and schema.cue (if present) is syntactically correct.

| Flag | Short | Description |
|------|-------|-------------|
| `--def` | `-d` | Path to rpack definition directory (required) |
| `--debug` | | Enable verbose logging |

### `rpack publish --def <dir> --type <type> --target <target>`

Publish an rpack definition to a registry or create a local archive.

| Flag | Short | Description |
|------|-------|-------------|
| `--def` | `-d` | Path to rpack definition directory (required) |
| `--type` | `-T` | Publish type: `oci` or `archive` (required) |
| `--target` | `-t` | OCI URL (`oci://`) or archive path (`.tar.xz`) (required) |
| `--debug` | | Enable verbose logging |

**OCI:** `rpack publish -d ./myrpack -T oci -t oci://docker.io/user/pack?tag=v1`
**Archive:** `rpack publish -d ./myrpack -T archive -t ./dist/pack.tar.xz`

OCI credentials are resolved automatically from Podman login, Docker login,
credential helpers, or the `OCI_USERNAME`/`OCI_PASSWORD` environment variables.

## State

Beta. API may change. Always run rpacks on version-controlled directories.
