# Stable Definition Contract Versioning Proposal

Date: 2026-07-28
Status: Draft
Scope: rpack definitions (`rpackdef/rpack.yaml` + `script.lua`) and their execution semantics. User config and lockfile schemas keep their own independent versioning.

## 1. Problem

The file-permissions feature exposes a compatibility hole in the current model:

- A definition calling `rpack.chmod(...)` under an older runtime fails because the function does not exist.
- A definition passing `{ mode = "755" }` to `rpack.write`/`rpack.copy` under an older runtime is worse: Lua permits extra arguments, so the old binding ignores the options table, reports success, and produces a non-executable `0644` file.

An old runtime cannot honestly emulate file modes: its filesystem layer has no chmod operation. Silent degradation is therefore not backwards compatibility; it is incorrect output.

This is acceptable while rpack is explicitly unstable (`0.x`): `rpack.v1` is still being formed, so the permissions feature can become part of v1 before `v1.0.0`. It is not acceptable after v1 becomes stable. From `v1.0.0` onward, definitions need both guarantees:

1. **Backward execution compatibility:** a newer runtime executes an older definition with the semantics that definition selected.
2. **Forward rejection:** an older runtime rejects a definition requesting newer semantics before Lua runs; it never guesses, ignores, or partially applies them.

## 2. Decision Summary

Treat the definition's `@schema_version` as the version of the **entire definition contract**, not merely the YAML object shape. Contract `vN` selects all of:

- the accepted `rpackdef/rpack.yaml` fields;
- the matching Lua module `rpack.vN` and its functions/signatures;
- filesystem access, purity, mode, and staging semantics;
- value conversion, defaults, and other observable execution behavior.

A definition uses matching versions:

```yaml
# rpackdef/rpack.yaml
"@schema_version": "v2"
name: "example"
```

```lua
-- script.lua
local rpack = require("rpack.v2")
```

A runtime either implements contract v2 or rejects it before executing `script.lua`. There is no separate `requires.rpack >= X` field: it duplicates the contract version, couples a definition to binary-release chronology, and adds no information needed for compatibility.

Once v1 is frozen at rpack `v1.0.0`, no new Lua function, option, YAML field, default, or observable behavior is added to contract v1. Such behavior is introduced by a new definition contract (v2, then v3, and so on). New runtimes continue to support previous contracts through version-specific adapters.

## 3. Terminology

- **Runtime version** — the binary's SemVer release, e.g. `rpack v1.4.0`.
- **Definition contract** — everything a definition author may observe or rely on while loading and executing a definition.
- **Contract version** — the definition's `@schema_version`, e.g. `v1` or `v2`.
- **Contract adapter** — the runtime implementation that presents exactly one contract version (schema, Lua module, and execution policy) over shared internals.

Contract versions and runtime versions are intentionally different version spaces. A backwards-compatible runtime minor release may add support for contract v2 while retaining v1. Definitions do not need to know which binary release first implemented v2; unsupported runtimes simply reject v2.

## 4. Compatibility Model

Example: runtime `1.0.0` supports definition contract v1. A later runtime adds contract v2 without removing v1.

| Definition | Runtime supporting v1 only | Runtime supporting v1 + v2 |
|---|---|---|
| schema v1 + `rpack.v1` | Runs with v1 semantics | Runs through the frozen v1 adapter |
| schema v2 + `rpack.v2` | Rejected before Lua | Runs with v2 semantics |
| schema v1 + `rpack.v2` | Rejected | Rejected as a contract/module mismatch |
| unknown schema version | Rejected | Rejected |

Required failure message:

```text
unsupported rpack definition contract "v2" (supported: v1)
```

No runtime falls back from v2 to v1, chooses the nearest version, or silently runs a future definition using its latest known behavior.

## 5. One Version Selects Schema and Lua Semantics

### 5.1 Loader flow

The definition loader must:

1. Read only `@schema_version` from the raw YAML header.
2. Look up that exact version in a contract registry.
3. Reject an unknown version immediately.
4. Strictly decode and validate the document against that contract's schema.
5. Carry the selected contract into execution.
6. Preload only the matching Lua module (`rpack.v1` for v1, `rpack.v2` for v2).

The version is selected once at the loader seam. Core code must not grow scattered checks such as `if version >= 2`.

### 5.2 Contract registry

Conceptually:

```go
type DefinitionContract struct {
    Version       string
    Schema        SchemaValidator
    LuaModuleName string
    LuaFunctions  func(*RPackAPI) map[string]lua.LGFunction
    FSPolicy      DefinitionFSPolicy
}
```

The exact Go shape is an implementation detail. The architectural rule is that version-specific behavior is concentrated in contract adapters over shared implementation, rather than copied engines or version conditionals throughout `executor.go`, `filemodel.go`, and Lua bindings.

A v2 adapter may reuse nearly all v1 internals. Reuse must not leak v2 behavior through the v1 interface.

### 5.3 Exact module matching

Only the module matching the selected definition contract is available. A v1 definition cannot import `rpack.v2` merely because the installed runtime happens to implement it. This prevents a definition from declaring v1 while depending on v2 behavior.

This rule is explicit and requires no script-content inspection. rpack never scans Lua to infer which features a definition uses.

## 6. Stability Policy

### Before binary v1.0.0

- Definition contract v1 remains unstable and may gain or change behavior.
- File permissions (`rpack.chmod` and `{ mode = ... }`) become part of v1.
- A `0.x` definition may require a sufficiently recent `0.x` binary; no forward-compatibility guarantee is made between prerelease runtimes.
- The release notes and docs must continue to state that v1 is not frozen until binary v1.0.0.

### At binary v1.0.0

- Snapshot and document the complete v1 definition contract.
- Freeze the v1 YAML schema, Lua surface, options, defaults, filesystem semantics, and output behavior.
- Establish a v1 conformance suite used by every future runtime.

### After binary v1.0.0

| Change | Allowed in frozen v1? | Required path |
|---|---:|---|
| Internal refactor with identical observable behavior | Yes | Keep v1 adapter/conformance green |
| Performance improvement with identical results | Yes | Keep v1 semantics |
| Better diagnostics without changing success/output | Usually | Treat error compatibility conservatively |
| Fix implementation to match documented v1 behavior | Yes | Bug fix + regression test |
| New Lua function | No | New contract version |
| New argument or options-table key | No | New contract version |
| New definition YAML field | No | New contract version |
| Changed default, output bytes, mode, path, purity, or staging behavior | No | New contract version |
| Newly rejected formerly-valid behavior | No, except security fixes | New contract, or documented security exception |
| New CLI command unrelated to definition execution | Yes | No definition-contract change |

The important rule is not "additive changes are always safe." A new function is additive for old definitions but unsafe for a new definition executed by an older runtime. Therefore any newly requestable definition behavior requires a new contract version.

## 7. Stable Contracts Must Reject Unknown Input

Versioning only works if a stable contract fails closed.

### 7.1 Strict YAML decoding

The current typed YAML decode ignores unknown fields before CUE validation sees them. Before v1.0.0, definition loading must become strict: unknown fields in a v1 definition are errors. Otherwise a future field accidentally placed under `@schema_version: v1` could be silently discarded by an older runtime.

The implementation should either use strict YAML decoding or validate the raw YAML map against the closed version-specific CUE schema before decoding to a Go struct.

### 7.2 Strict Lua arity and options

Gopher-lua permits extra arguments, and current bindings generally inspect only the arguments they know. Before v1.0.0, every stable public Lua function must enforce:

- its permitted argument-count range;
- argument types;
- all options-table keys;
- mutually exclusive options.

For example, stable v1 `write(path, content [, opts])` accepts exactly two or three arguments and rejects unknown option keys. A future runtime must never reinterpret an argument that a stable older runtime ignored.

### 7.3 No implicit capability detection

The runtime does not inspect shebangs, Lua source text, output paths, or options to infer a contract. The author explicitly selects the contract in `rpack.yaml` and imports the matching Lua module.

Optional feature detection in Lua is permitted only when both branches are intentionally valid outputs. It is not a substitute for selecting a newer contract when behavior is required.

## 8. Introducing a Future Contract

Suppose a future feature cannot be added to frozen v1.

1. Add definition schema v2.
2. Add the `rpack.v2` contract adapter exposing the new behavior.
3. Keep the v1 schema and `rpack.v1` adapter unchanged.
4. Teach the loader registry that both v1 and v2 are supported.
5. Add compatibility tests proving:
   - v1 definitions produce the same output under the new runtime;
   - v2 definitions run under the new runtime;
   - a v1-only runtime rejects v2 before Lua;
   - schema/module mismatches fail.
6. Update authoring docs and scaffolding to select v2 only when its behavior is needed. Do not rewrite existing definitions automatically.

A runtime release adding v2 is backwards-compatible because it continues to implement v1. Removing v1 support is a runtime SemVer breaking change and is allowed only in a new binary major release, after an explicit deprecation/migration period.

## 9. Why Not `requires.rpack >= X`?

Rejected for four reasons:

1. **Redundant:** schema v2 already means "this definition requires a runtime implementing contract v2."
2. **Wrong abstraction:** definitions depend on behavior, not on release chronology. A fork or backport may implement v2 under a different binary version.
3. **Two sources of truth:** `@schema_version: v2` paired with an incorrect minimum binary version creates contradictory metadata.
4. **Prerelease complexity:** constraints around `1.2.0-rc.1` versus `1.2.0` add machinery without improving contract negotiation.

Documentation may record which binary first introduced each contract, but definitions do not declare that mapping.

## 10. Rejected Alternatives

### Mutate `rpack.v1` forever

Rejected after v1.0.0. Existing definitions keep working, but new definitions can silently misbehave on older v1 runtimes—as `{ mode = "755" }` demonstrates.

### Feature-detect every addition in Lua

Rejected as the primary model. It distributes compatibility policy across every definition, is easy to forget, and permits semantic degradation. Capability checks remain useful only for genuinely optional behavior.

### Always expose the latest Lua module

Rejected. It causes an old definition's behavior to change merely because the runtime was upgraded.

### Fall back to the newest supported contract

Rejected. Running v2 as v1 is not compatibility; it is executing with different semantics.

### Infer required contract from Lua source or output content

Rejected. This is content-sniffing implicit behavior and violates rpack's explicit semantics.

## 11. Implementation Plan Before v1.0.0

1. Introduce the definition-contract registry while it contains only v1.
2. Parse `@schema_version` before decoding the full definition and produce an explicit unsupported-contract error.
3. Make definition YAML decoding strict.
4. Make every v1 Lua binding enforce arity and reject unknown options.
5. Route Lua module registration and filesystem policy through the selected v1 adapter.
6. Build a v1 conformance suite covering API signatures, conversion rules, sandbox access, purity, write defaults/modes, relocation paths, lockfile-visible outputs, and failure behavior.
7. Declare v1 frozen only when binary v1.0.0 is released.

The permissions feature requires no v2 migration now: it lands while v1 is explicitly unstable and becomes part of the contract frozen at v1.0.0.

## 12. Consequences

- New runtimes preserve old definitions through explicit adapters, not best-effort compatibility.
- Old runtimes reject future definitions before side effects instead of silently producing wrong files.
- Definition authors express one compatibility choice, not both schema and minimum binary version.
- New definition behavior has a deliberate cost: a new contract generation and maintained adapter. This pressure is desirable; it prevents casual semantic drift after stability.
- Runtime internals can continue evolving aggressively as long as each supported contract adapter preserves its observable behavior.
- Multiple contract adapters require long-lived conformance tests and cannot be treated as thin aliases to "latest."
