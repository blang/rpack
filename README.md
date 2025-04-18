# RPack

RPack is a package manager for files, a tool for distributing versioned packs of files/configuration with advanced templating and scripting.

[![Go Reference](https://pkg.go.dev/badge/github.com/blang/rpack.svg)](https://pkg.go.dev/github.com/blang/rpack)
[![Go Report Card](https://goreportcard.com/badge/github.com/blang/rpack)](https://goreportcard.com/report/github.com/blang/rpack)

# Overview

Rpack enables you to bundle a set of files, together with configuration and advanced scripting and distribute them in a versioned bundle to users.
The rpack then can be applied on the users side with values and file inputs and write a set of files.

- Think [helm chart](https://helm.sh/) for arbitrary files, with advanced scripting and access to read data from the users repository.
- Think [vendir](https://carvel.dev/vendir/) with templating.
- Think [kustomize](https://github.com/kubernetes-sigs/kustomize) but scriptable and as a versioned bundle.

Use cases include:
- Distribute a common set of files (templated with user config) in a versioned fashion to all your repositories.
- Make your dotfiles configurable and distribute them as a versioned package.
- Package and distribute a set of files with configurable user values (github actions workflows, pre-commit config, ...)
- Migrate any repository from one state to another in a deterministic and templated way.

# Usage

`example.rpack.yaml`:
```yaml
"@schema_version": "v1"
# Can be git, https, s3, and others
source: "git::https://github.com/blang/rpack.git//examples/onlyexample/rpackdef"
config: 
  # Config values determining how files are processed
  values:
    author: "blang"
  # Give rpack access to users files and directories to read from
  inputs:
    "users.yaml": ./myusers.yaml
```

```shell
# Execute the rpack on the directory of the rpack.yaml
rpack run --dry-run ./example.rpack.yaml 
```

An rpack can write to any file or directory in the `rpack` directory, but is not allowed to read files that are not specified as inputs.
You can use a dry-run to fully investigate which files change.

# Features
- Use Lua scripting to manipulate files
- Clean upgrades with side-effect free (pure) templating: A second execution with same inputs will result in same output.
- User-defined values in yaml, verifiable with [cuelang](https://cuelang.org/) schema (optional).
- User-mapped files and directories: Files from the user can be read, copied and parsed (think helm values everywhere).
- Lockfile enables side-effect free updates and detects manual changes.
- Dry-run and transactional updates enables you to test updates and roll back side-effect free.
- Read json and yaml and process them in lua, write them back in json, yaml or templated text.
- Golang [`text/template`](https://pkg.go.dev/text/template) with any values and any source file.

# Full example

In this basic example we:
- copy a file from the bundled rpack to the users repo (`files/intro.md` -> `repo_intro.md`),
- read a list of users from the users repo (`map:users.yaml`)
- read the `author` value from the user supplied config values
- use golang template on a file from the bundled rpack (`files/users.md.tmpl`) with the `author` value as well as the data from the `users.yaml`
- write the output to a new file on the users repo (`./rpack_users.md`).

See [examples/intro](./examples/intro) for the full example.

`./myrpack/rpack.yaml`:
```yaml
"@schema_version": "v1"
name: "intro"
inputs:
  - name: users.yaml
    type: file
```

`./myrpack/script.lua`:
```lua
local rpack = require("rpack.v1")
local values = rpack.values()

-- Copy intro file from rpack to users repo
rpack.copy("rpack:files/intro.md", "./rpack_intro.md")

-- Read the user mapped file from its repo
local users = rpack.from_yaml(rpack.read("map:users.yaml"))
local data = {
    users =  users,
    author = values.author,
}

-- Template the rpacks users.md template with our data
local tmpl_output = rpack.template(rpack.read("rpack:files/users.md.tmpl"), data)

-- Write the template output to the users repo
rpack.write("./rpack_users.md", tmpl_output)
```

The `files/` directory contains the files `rpack_intro.md` and `users.md.tmpl`.
That is all from the rpack bundle side! In the full example we also supplied a `cuelang` schema for validation, but this is optional.

**User side**

`./testing/intro.rpack.yaml`:
```yaml
"@schema_version": "v1"
source: "../myrpack"
config: 
  values:
    author: blang
  inputs:
    "users.yaml": ./myusers.yaml
```

`./testing/myusers.yaml`:
```yaml
- firstname: Alice
  lastname: Johnson
  email: alice.johnson@example.com
- firstname: Bob
  lastname: Smith
```

Now the user can apply the rpack using:
```shell
rpack run ./testing/intro.rpack.yaml
```

This will create the files `rpack_intro.md` and `rpack_users.md` next to `intro.rpack.yaml`, as well as a lockfile.

# More Examples

Have a look at the [examples directory](./examples). They cover all potential aspects you will need as a user or rpack author.

# State

This tool was just released as a beta version and will require further stabilization before becoming production ready. The API might change in this process. Use at your own risk and always execute rpacks only on version controlled directories.

# Creating an RPACK

A `rpack` is a bundle of files, together with an optional schema for inputs and a script to perform file manipulation.

This rpack can then be distributed to users in a versioned fashion (git, s3, https) and applied by the user to their own files.

An rpack is a bundle of files in a directory:
- `rpack.yaml`: The specification of values and inputs available to the user (see its [schema](./pkg/rpack/def_schema.cue))
- `schema.cue`: A [cuelang](https://cuelang.org/) schema to validate the users values.
- `script.lua`: The lua script used to perform file manipulation (see [available lua libaries](./lua/src)).

See the [examples directory](./examples) for more details on the various aspects.

# License

RPack is released under the Apache 2.0 license. See [LICENSE.md](LICENSE.md)
