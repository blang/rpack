# Stable Definition Contract Versioning Proposal

Date: 2026-07-28
Validated: 2026-07-29
Status: Implemented for v1; validated with an unshipped v2 experiment
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
- **Contract adapter** — the runtime implementation that presents exactly one contract version (schema, Lua libraries, config validation, and execution policy) over shared internals.
- **Contract wire document** — the version-specific Go representation used for strict YAML decoding and presence tracking.
- **Runtime definition model** — the common fields needed by shared execution code, plus the retained version-specific wire document for its adapter.

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

## 5. One Version Selects the Complete Adapter

### 5.1 Loader flow

The definition loader must:

1. Read only `@schema_version` from the raw YAML header.
2. Look up that exact version in a contract registry.
3. Reject an unknown version immediately.
4. Invoke that contract's strict wire decoder; different contracts may accept different YAML fields.
5. Validate the retained wire document as a concrete value against that contract's definition schema.
6. Normalize common fields into the runtime definition model without discarding the retained wire document.
7. Carry the selected contract and definition model into config validation and execution.
8. Open that contract's Lua libraries and preload only its matching module (`rpack.v1` for v1, `rpack.v2` for v2).

The version is selected once at the loader seam. Core code must not grow scattered checks such as `if version >= 2`.

### 5.2 Contract registry

Conceptually (names abbreviated):

```go
type DefinitionContract struct {
    Version            string
    DefinitionSchema   SchemaValidator
    DecodeDefinition   func([]byte) (*RPackDef, error)
    NewConfigValidator func([]byte, string) (SchemaValidator, error)
    OpenLuaLibraries   func(*lua.LState) error
    LuaModuleName      string
    LuaFunctions       func(*LuaModel) map[string]lua.LGFunction
    NewFS              func(*definitionFSConfig) *RPackFS
}
```

The runtime model retains the contract-specific decoded document. This lets a v2 Lua function consume a v2-only YAML field without adding that field to v1's strict wire shape or leaking it into shared code.

The exact Go shape remains an implementation detail. The architectural rule is that version-specific behavior is concentrated in contract adapters over shared implementation, rather than copied engines or version conditionals throughout `executor.go`, `filemodel.go`, and Lua bindings.

A v2 adapter may reuse nearly all v1 internals. Reuse must not leak v2 behavior through the v1 interface.

### 5.3 Exact module matching

Only the rpack module matching the selected definition contract is available. A v1 definition cannot import `rpack.v2` merely because the installed runtime happens to implement it. This prevents a definition from declaring v1 while depending on v2 behavior.

The unversioned import name `filepath` is also installed by the selected adapter. Its v1 function set therefore remains frozen for v1 definitions even if a future contract supplies different filepath behavior. Injected data functions such as `values()` and `inputs()` are likewise built by the selected adapter rather than overlaid globally.

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

### 7.1 Strict, version-specific YAML decoding

The old typed YAML decode ignored unknown fields before CUE validation saw them. Definition loading now reads the version header first and delegates full strict decoding to that contract. Unknown v1 fields are errors even under a runtime that also supports v2; a v2-only field cannot be smuggled into v1.

A single shared strict Go struct is insufficient: as soon as v2 adds a field, either the shared struct rejects valid v2 or accepts the v2 field under v1. Each contract therefore owns its wire document and decoder, while shared execution receives only normalized common fields.

Definition CUE validation must use `cue.Concrete(true)`. Version-specific wire fields that are required but have a valid zero value must preserve presence (for example with pointer fields); otherwise decoding an absent string as `""` makes absence indistinguishable from an explicitly empty value before CUE validation.

### 7.2 Strict Lua arity and options

Gopher-lua permits extra arguments, and the old bindings inspected only the arguments they knew. Every public v1 Lua function now enforces:

- its permitted argument-count range;
- argument types;
- all options-table keys;
- mutually exclusive options.

For example, v1 `write(path, content [, opts])` accepts exactly two or three arguments and rejects unknown option keys. A future runtime must never reinterpret an argument that a stable older runtime ignored. The same exact-arity rule applies to injected data functions and fixed-arity `filepath` functions; intentionally variadic `filepath.join` enforces only its minimum of two arguments.

### 7.3 No implicit capability detection

The runtime does not inspect shebangs, Lua source text, output paths, or options to infer a contract. The author explicitly selects the contract in `rpack.yaml` and imports the matching Lua module.

Optional feature detection in Lua is permitted only when both branches are intentionally valid outputs. It is not a substitute for selecting a newer contract when behavior is required.

## 8. Introducing a Future Contract

Suppose a future feature cannot be added to frozen v1.

1. Add the v2 definition schema, strict wire document, decoder, and common-field normalization.
2. Add the `rpack.v2` adapter: config-schema validator, Lua libraries/functions, and filesystem policy.
3. Keep every v1 adapter component unchanged.
4. Teach the loader registry that both v1 and v2 are supported.
5. Add compatibility tests proving:
   - v1 definitions produce the same output under the new runtime;
   - v2 definitions run under the new runtime;
   - a v1-only runtime rejects v2 before Lua;
   - schema/module mismatches fail;
   - v2-only YAML fields, Lua functions, and library functions remain unavailable under v1;
   - strict v1 arity and fields remain strict under the new runtime.
6. Use a permanently unsupported sentinel such as `v999` for generic unknown-contract tests; do not encode the assumption that `v2` will always be unsupported.
7. Update authoring docs and scaffolding to select v2 only when its behavior is needed. Do not rewrite existing definitions automatically.

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

## 11. Implemented v1 Baseline

The v1 infrastructure was implemented in commits `2785d3d` and `8dcf814`:

- raw-header contract selection with explicit, sorted unsupported-version errors;
- a fail-fast contract registry;
- contract-owned strict definition decoding and retained wire documents;
- concrete CUE validation, including required-field enforcement;
- contract-owned `schema.cue` validator construction;
- contract-owned Lua library opening, module function maps, and injected data functions;
- exact module preloading—only `rpack.v1` is available to a v1 definition;
- contract-owned filesystem construction;
- strict arity for all v1 rpack functions, injected data functions, and filepath functions;
- strict options-table keys;
- v1 conformance coverage for module matching, output bytes/modes, unknown fields, unknown contracts, and pre-Lua rejection.

The audit also reconciled a pre-stability documentation mismatch: `read_dir(path, recursive?)` was documented as optional but implemented as required. V1 now accepts one or two arguments and defaults `recursive` to false. An isolated execution of all five shipped examples found and corrected a separate stale call to nonexistent `rpack.read_yaml`; all five examples then passed.

Remaining v1.0.0 release gates are process rather than architecture:

1. Snapshot the final v1 API and behavior documentation.
2. Keep expanding the v1 conformance corpus as bugs or ambiguities are found.
3. Declare v1 frozen only when binary v1.0.0 is released.

The permissions feature requires no v2 migration now: it lands while v1 is explicitly unstable and becomes part of the contract frozen at v1.0.0.

## 12. Validation Experiment: Adding Contract v2

### 12.1 Method

A real second contract was implemented on local branch `test/definition-contract-v2-experiment` rather than merely mocked. The original experiment is commit `fb5b7aa`; after merging the corrected adapter seams, the validated branch head is `10d7a07`. The dummy v2 contract deliberately changed five dimensions:

1. A required v2-only `dummy_message` YAML field with its own strict wire type and CUE schema.
2. `rpack.v2.dummy_feature()`, which returned the decoded `dummy_message` value.
3. A v2-only `filepath.contract_marker()` function under the same unversioned `filepath` import name.
4. A v2-specific `schema.cue` validator factory requiring a test marker.
5. A v2 filesystem policy with purity checking disabled, while v1 retained its purity policy.

The dummy contract is experimental evidence only and must not be merged or shipped as the real v2 design.

Two binaries were built independently:

- v1-only baseline from `2785d3d`;
- v1+v2 experiment from the test branch.

The experiment branch passed 413 Go tests plus all configured Go, Lua, CUE, and YAML linters. A separate black-box script then executed 15 compatibility cases against the two binaries:

| Case | Result |
|---|---|
| v1-only runtime + v1 definition | Succeeded |
| v1-only runtime + v2 definition containing v2 fields | Rejected from the version header before Lua |
| v1+v2 runtime + v1 definition | Succeeded through v1 adapter |
| v1+v2 runtime + v2 definition | Succeeded; Lua returned the v2-only YAML value |
| v1 schema + `rpack.v2` under v1+v2 runtime | Rejected; `rpack.v2` was not preloaded |
| v2 schema + `rpack.v1` under v1+v2 runtime | Rejected; `rpack.v1` was not preloaded |
| v2-only YAML field declared under v1, on both runtimes | Rejected as an unknown v1 field |
| v2-only Lua function requested through `rpack.v1` | Rejected; function absent |
| Extra future argument passed to v1 `write`, on both runtimes | Rejected with identical strict-arity error |
| Required v2 field omitted | Rejected during schema validation before Lua |
| Unknown `v999` contract, on both runtimes | Rejected from header; supported-version list was correct and sorted |
| Same v1 definition under both runtimes | Output bytes were identical and mode remained `0755` |

Rejected cases produced no managed result file. In-process tests separately proved that the v2-only filepath function and config-validator behavior did not leak into v1, and that the filesystem factory could select a different policy.

### 12.2 Assumptions Falsified or Refined

The experiment changed the design in several important ways:

1. **A schema validator alone is not enough.** A shared strict Go struct cannot both reject v2 fields under v1 and accept them under v2. The decoder and wire document must be version-specific too.
2. **Required-field presence must survive decoding.** `cue.Concrete(true)` catches incomplete required fields, but a plain Go scalar can already have collapsed absence into a valid zero value. Version-specific wire documents must use presence-preserving representations where absence matters.
3. **The adapter is broader than `rpack.vN`.** `filepath`, injected data functions, config-schema compilation, and filesystem construction are observable contract surfaces and must be selected by the same adapter.
4. **Unknown-version tests must not reserve the next number forever.** Generic tests now use `v999`; the two-binary matrix tests the concrete v1-to-v2 transition.
5. **A dormant filesystem path was broken.** Selecting a v2 `enforcePure=false` policy exposed that `NewRPackFS` inserted a typed-nil purity hook, causing a panic on access. The constructor now omits that hook when purity is disabled, with a regression test.
6. **Conformance work sharpens v1 before freeze.** The `read_dir` documentation/implementation mismatch and stale example call to nonexistent `read_yaml` were found only because signatures and shipped examples were exercised end to end.

These findings strengthen rather than overturn the core proposal: the schema version is sufficient negotiation, provided it selects a complete adapter and every known input surface fails closed.

## 13. Consequences

- New runtimes preserve old definitions through explicit adapters, not best-effort compatibility.
- Old runtimes reject future definitions before side effects instead of silently producing wrong files.
- Definition authors express one compatibility choice, not both schema and minimum binary version.
- New definition behavior has a deliberate cost: a new contract generation, wire decoder, and maintained adapter. This pressure is desirable; it prevents casual semantic drift after stability.
- Runtime internals can continue evolving aggressively as long as each supported contract adapter preserves its observable behavior.
- Multiple contract adapters require long-lived conformance tests and cannot be treated as thin aliases to "latest."
