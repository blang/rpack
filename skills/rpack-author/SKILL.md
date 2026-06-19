---
name: rpack-author
description: Create rpack definitions that bundle files with Lua scripting and CUE validation for tracked distribution across repositories. Use when you need to distribute config files, CI workflows, bot configs, or any tracked files that need templating or values.
---

# RPack Author

## Overview

An rpack definition (rpackdef) is a bundle of files that is distributed via git, HTTPS, or OCI registries. Users reference your bundle in their `*.rpack.yaml` config, provide values and input files, and run `rpack run` to generate output files. Your job as the author is to create the bundle: the metadata, the Lua script that processes files, an optional CUE schema that validates user inputs, and the static files to distribute.

rpack is designed for **tracked files that get updated over time** — config files (`.editorconfig`, `.gitignore`), CI/CD workflows (GitHub Actions), bot configs, and similar. It is NOT for one-shot file delivery or binary distribution.

## Anatomy

An rpack definition is a directory containing:

```
rpackdef/
├── rpack.yaml       # REQUIRED: Name and input declarations
├── script.lua       # REQUIRED: Lua script that processes files
├── schema.cue       # OPTIONAL: CUE schema to validate user values
└── files/           # OPTIONAL: Static files accessible via rpack: prefix
```

### rpack.yaml

Declares the bundle name and what inputs users can provide.

```yaml
"@schema_version": "v1"
name: "my-rpack"
inputs:
  - name: users.yaml
    type: file
  - name: configs
    type: dir
```

- `@schema_version` — always `"v1"` (quoted, because CUE requires it)
- `name` — alphanumeric, dashes, and underscores, 1-64 characters
- `inputs` — optional list of `{name, type}` where type is `"file"` or `"dir"`

Inputs declared here are NOT automatically provided. The user must map them in their config. Inputs not configured by the user are simply not available — scripts should handle missing optional inputs gracefully.

### script.lua

The Lua script runs in a sandboxed filesystem. It uses the `rpack.v1` module. Every script starts with:

```lua
local rpack = require("rpack.v1")
local values = rpack.values()
```

The script runs top-to-bottom: read inputs → process → write outputs. Scripts are **pure** — same inputs always produce same outputs. The executor guarantees this by blocking reads from the output directory.

### schema.cue

Validates user-provided `values` using CUE. Optional but recommended — it catches misconfiguration early and gives users clear error messages.

```cue
#Schema: {
    values: #Values
    inputs: #Inputs
}

#Values: {
    author: string
    repo_name?: string  // optional
}

#Inputs: [string]: string
```

The schema checks `values` (arbitrary data the user sets in their config), not the content of input files. Common constraints:
- `string` — any string value
- `int & >=0 & <=100` — integer range
- `string & =~"^[a-z0-9-]+$"` — regex pattern match
- `field?` — optional field
- `field!` — required field (redundant with no `?`, but explicit)

### files/

Static files bundled with your rpack. Accessed in scripts via the `rpack:` prefix:

```lua
rpack.copy("rpack:files/my-config.yml", "./my-config.yml")
rpack.read("rpack:files/template.yml.tmpl")
```

All files in `files/` should be referenced in `script.lua`. Orphaned files (present in `files/` but never copied/read) are a common author mistake.

## Lua Scripting Patterns

### Pattern 1: Static File Copy

For files that don't need templating — copy them as-is from your bundle to the user's repo. **One `rpack.copy()` per file, each on its own line.**

```lua
local rpack = require("rpack.v1")

rpack.copy("rpack:files/.editorconfig", "./.editorconfig")
rpack.copy("rpack:files/.gitattributes", "./.gitattributes")
rpack.copy("rpack:files/.github/CODEOWNERS", "./.github/CODEOWNERS")
rpack.copy("rpack:files/.prettierrc.json", "./.prettierrc.json")
```

**Key points:**
- Directories are created automatically — you don't need to `mkdir` for `.github/`
- Always use the full destination path (`./.github/CODEOWNERS` not just `./CODEOWNERS`)
- Keep it explicit — one line per file, no loops

### Pattern 2: Templated Files

For files with placeholders — read a template, fill in values, write the result. Uses Go's `text/template` syntax, NOT Lua string interpolation.

**Template file** (`files/ci.yml.tmpl`):
```yaml
name: CI
on: [push, pull_request]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: "{{ .go_version }}"
```

**Lua script:**
```lua
local rpack = require("rpack.v1")
local values = rpack.values()

-- Read template, execute with values, write result
local ci = rpack.template(rpack.read("rpack:files/ci.yml.tmpl"), {
    go_version = values.go_version,
    node_version = values.node_version,
})
rpack.write("./.github/workflows/ci.yml", ci)
```

**Template syntax reference:**
- `{{ .field }}` — simple value insertion
- `{{ if .field }}...{{ end }}` — conditionals
- `{{ range .list }}...{{ end }}` — iteration
- Delimiters default to `{{` and `}}`, but can be customized: `rpack.template(tmpl, data, "[[", "]]")`

Sprig template functions are available: `{{ .name | upper }}`, `{{ .list | join "," }}`, etc.

#### Escaping literal braces

When template files contain literal `{{` (e.g., GitHub Actions `${{ secrets.GITHUB_TOKEN }}`), you must escape them to prevent Go template parsing:

```yaml
# In your template file
${{ "{{" }} secrets.GITHUB_TOKEN {{ "}}" }}
```

This outputs `${{ secrets.GITHUB_TOKEN }}` in the final file.

**Key points:**
- Templates use Go `text/template` syntax, NOT Lua
- Values come from `rpack.values()` (user's config values)
- Always use `rpack.read()` + `rpack.template()` + `rpack.write()` — never string concatenation with user values (loses purity guarantees)
- If you need optional values, use template conditionals: `{{ if .optional_field }}...{{ end }}`
- Escape literal `{{` with `{{ "{{" }}` and `}}` with `{{ "}}" }}` in template files

### Pattern 3: File Merging

For combining a base file from your bundle with repo-local content. Common for `.gitignore` files where users have their own rules.

```lua
local rpack = require("rpack.v1")
local values = rpack.values()

-- Read the base file from the bundle
local base = rpack.read("rpack:files/gitignore.base")

-- Try to read the user's custom file (if mapped)
local custom = ""
local inputs = rpack.inputs()
local has_custom = false
for _, name in ipairs(inputs) do
    if name == "gitignore_custom" then
        has_custom = true
        break
    end
end
if has_custom then
    custom = "\n# Custom rules\n" .. rpack.read("map:gitignore_custom")
end

-- Append extra patterns from values
local extra = ""
if values.extra_patterns then
    for _, pattern in ipairs(values.extra_patterns) do
        extra = extra .. pattern .. "\n"
    end
end

-- Write the merged result
rpack.write("./.gitignore", base .. custom .. extra)
```

**Key points:**
- Check for optional inputs with `rpack.inputs()` — iterate the returned list and check if the input name is present before reading
- Handle missing optional values with `if values.field then`
- String concatenation with Lua `..` operator is fine for building output
- The merged result is still pure — same base + same custom = same output

### Pattern 4: Read-Transform-Write

For processing structured data — read YAML/YAML input, transform it, write the result.

```lua
local rpack = require("rpack.v1")
local values = rpack.values()

-- Read and parse user input
local users = rpack.from_yaml(rpack.read("map:users.yaml"))

-- Template with structured data
local output = rpack.template(rpack.read("rpack:files/users.md.tmpl"), {
    users = users,
    author = values.author,
})
rpack.write("./USERS.md", output)
```

## File Sandbox Prefixes

Scripts access files through four prefixes. Understanding these is critical:

| Prefix | Access | Use case |
|--------|--------|----------|
| `rpack:files/path` | Read-only | Bundled files from `files/` directory |
| `map:input-name` | Read-only | User-mapped input files |
| `temp:filename` | Read/Write | Temporary files during execution |
| `./relative/path` | Write-only | Target output files |

**Rules that will cause errors if broken:**
- Cannot write to `rpack:` or `map:`
- Cannot read from `./` (output directory is write-only)
- Must use `rpack:` prefix for bundled files, not relative paths

## CUE Schema Design

### Simple validation

```cue
#Schema: {
    values: #Values
    inputs: #Inputs
}

#Values: {
    author: string
}

#Inputs: [string]: string
```

### With optional fields and constraints

```cue
#Schema: {
    values: #Values
    inputs: #Inputs
}

#Values: {
    org:            string
    repo_name:      string & =~"^[a-z0-9-]+$"
    go_version?:    string & =~"^[0-9]+\\.[0-9]+(\\.[0-9]+)?$"
    node_version?:  int & >=16 & <=22
}

#Inputs: [string]: string
```

### Schema design guidelines

1. **Validate what the script uses.** If `script.lua` reads `values.org`, add `org` to the schema.
2. **Mark truly optional as optional.** Use `field?` for values that have sensible defaults in the script.
3. **Constrain string formats.** Use regex to validate versions, slugs, identifiers.
4. **Constrain numeric ranges.** Use `& >=min & <=max` for version numbers.
5. **Keep it simple.** The schema should validate user config, not enforce business logic. Business logic goes in the Lua script.

## Best Practices

1. **Explicit over clever.** One `rpack.copy()` per line. Don't loop over file lists — it's harder to read, harder to debug, and unnecessary for the typical 4-20 files in an rpack.

2. **Simple schemas.** Validate what matters — required values, format constraints. Don't over-constrain. The Lua script handles business logic.

3. **Handle missing inputs gracefully.** Not all declared inputs will be configured. Use `rpack.inputs()` to get the list of configured inputs and check for your input name before reading.

4. **Use Go template syntax, not Lua.** `{{ .value }}` in template files, never `.. value ..` or string.find hacks. Go templates are the right tool for text generation.

5. **Bundle real content in `files/`.** Don't generate config content in Lua — put it in template files. Lua should wire things together, not produce content.

6. **Test locally before distributing.** Use `rpack validate --def` and `rpack run --def --output-dir /tmp/test` to verify your definition works before publishing.

7. **One rpack per purpose.** Don't build a monster rpack that does 20 different things. Split into focused bundles: `repo-configs`, `github-actions`, `security-policies`, etc.

8. **Use meaningful file names for templates.** `ci.yml.tmpl` not `templates/file1.tmpl`. The name communicates intent.

## Common Mistakes

1. **Forgot `rpack.copy()` for a file in `files/`.** Every file in `files/` must be explicitly referenced in `script.lua`. A file sitting in `files/` does nothing on its own.

2. **Missing schema field for a value used in `script.lua`.** If your script reads `values.org`, add `org` to the schema. Without it, invalid values slip through to runtime.

3. **Not handling optional inputs.** If you declare an input but the user doesn't configure it, `rpack.read("map:name")` will fail. Always guard by checking `rpack.inputs()` for the input name before reading.

4. **Using Lua string interpolation for templates.** `"Hello " .. values.name` works but loses template features. Use `rpack.template()` with proper template files instead.

5. **Reading from the output directory.** Scripts cannot read from `./` (write-only). Use `temp:` for intermediate reads or restructure to write in one pass.

6. **Complex abstractions.** Wrapping `rpack.copy()` in a loop or helper function makes the script harder to understand. The verbosity of explicit copies IS the feature — it tells readers exactly what files are produced.

7. **Wrong `@schema_version` format.** Must be `"@schema_version": "v1"` — quoted, because CUE requires quoted identifiers for `@`-prefixed fields.

8. **Schema validates the wrong thing.** The schema validates `values` (user config), not the content of template files or output files. Don't try to validate generated content in CUE.

## Validation Checklist

Before distributing your rpack definition, verify:

1. **`rpack validate --def ./rpackdef` exits 0** — catches schema errors, missing script.lua, invalid rpack.yaml

2. **`rpack run --def ./rpackdef --set key=value --output-dir /tmp/test` produces all expected files** — catches runtime errors, missing values, file path mistakes

3. **All files in `files/` are referenced in `script.lua`** — check for orphaned files. Every file in the bundle should have a purpose.

4. **Schema validates all user-provided values** — every value read from `rpack.values()` in the script has a corresponding field in schema.cue. Optional values use `?`.

5. **`rpack check` passes on the lockfile** — (when running from a config, not `--def` mode) verifies output files match the lockfile
