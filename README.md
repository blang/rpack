# RPack

A package manager for files — distribute versioned bundles of templated files with Lua scripting.

Think [Helm](https://helm.sh/) for arbitrary files, [vendir](https://carvel.dev/vendir/) with templating, or [kustomize](https://github.com/kubernetes-sigs/kustomize) but scriptable.

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

## Worked example

**RPack bundle** (`rpackdef/`):

`rpack.yaml` — declares inputs:
```yaml
"@schema_version": "v1"
name: "intro"
inputs:
  - name: users.yaml
    type: file
```

`script.lua` — processes files:
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

`schema.cue` (optional) — validates user values:
```cue
#Schema: {
    author!: string
}
```

`files/` — bundled templates and static files.

**User side**:

`intro.rpack.yaml`:
```yaml
"@schema_version": "v1"
source: "../rpackdef"
config:
  values:
    author: blang
  inputs:
    "users.yaml": ./myusers.yaml
```

```shell
rpack run ./intro.rpack.yaml
```

Creates `rpack_intro.md`, `rpack_users.md`, and a lockfile alongside the config.

## Creating an rpack

An rpack bundle is a directory containing:

| File | Required | Description |
|------|----------|-------------|
| `rpack.yaml` | Yes | Name and input declarations. See [def_schema.cue](./pkg/rpack/def_schema.cue). |
| `script.lua` | Yes | Lua script using `rpack.v1` API. |
| `schema.cue` | No | CUE schema to validate user `values`. |
| `files/` | No | Static files accessible via `rpack:` prefix. |

Distribute via git, https, or s3. See [examples/](./examples) for complete examples.

## CLI reference

### `rpack run <config>`

Execute an rpack.

| Flag | Short | Description |
|------|-------|-------------|
| `--dry-run` | | Preview changes without writing files |
| `--force` | `-f` | Overwrite files, ignore lockfile integrity warnings |
| `--working-dir` | `-w` | Override working directory (default: config file location) |
| `--debug` | | Enable verbose logging |

### `rpack check <config>`

Verify lockfile integrity — checks that all managed files exist and haven't been modified externally.

| Flag | Short | Description |
|------|-------|-------------|
| `--working-dir` | `-w` | Override working directory |
| `--debug` | | Enable verbose logging |

## State

Beta. API may change. Always run rpacks on version-controlled directories.
