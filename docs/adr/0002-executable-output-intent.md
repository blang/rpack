# Executable output intent: owner-execute classification, umask-aware creation

Status: implemented — supersedes [ADR 0001](0001-output-file-permissions.md) while rpack is `0.x` ([issue #15](https://github.com/blang/rpack/issues/15)).

ADR 0001 shipped an exact-mode contract: `rpack.chmod` and `{ mode = "755" }` accepted any owner-readable octal mode, every staged write was normalized to 0644 umask-independently, and the lockfile recorded and strictly compared full rwx modes. That contract repeatedly conflicts with ordinary repository workflows: Git preserves only the executable bit, so even the canonical `644`/`755` outputs drifted after a fresh clone under a different umask. The mismatch was documented during the 2026-09 portable-permissions investigation; [issue #15](https://github.com/blang/rpack/issues/15) preserves the original evidence and chose the executable-only semantics this ADR records. The earlier hybrid recommendation (executable intent by default plus an exact-mode opt-in) was **not** adopted.

## Decision

rpack manages the **executable intent** of each output file and nothing else. Read/write bits belong to the destination environment.

### Lua API

- `rpack.write(path, content [, opts])` and `rpack.copy(src, dst [, opts])` take `opts.executable` (boolean, default `false`).
- `opts.mode` is a **compatibility alias**: only `"644"`/`"0644"` (= `executable = false`) and `"755"`/`"0755"` (= `executable = true`) are accepted. Everything else (`"600"`, `"640"`, …) is rejected — there is no exact-mode feature.
- `rpack.chmod(path, mode)` is the same compatibility alias for amending intent after a write.
- `executable` and `mode` in one options table are contradictory and rejected.
- Ordering rule unchanged: a write establishes its own intent (default non-executable); chmod amends until the next write to the same path.
- Chmod on `temp:` files remains permitted and is never implicitly copied — a later `copy` decides intent through its own options.
- No content sniffing, no implicit source-mode inheritance in `copy`.

### Classification and lockfile

- Executability is classified by **owner execute** (`mode & 0o100`), matching Git — not equality of all three execute bits.
- The lockfile `mode` string field and format are unchanged; values are now always canonical `"644"` (non-executable) or `"755"` (executable). Absence of the field still means "pre-feature lockfile, mode unknown".
- Drift is a change in owner-execute classification (recorded `"755"` but no owner-execute on disk, or vice versa). Read/write bit differences are not drift and never require `--force`.
- No lockfile schema version bump: a pre-`v1.0.0` contract change; `Validate()` hard-rejects unknown versions and the field was already additive.

### Materialization

- New and replaced files are created with Git-like bases — `0666` (non-executable) / `0777` (executable) — restricted by the local creation policy (process umask, default ACLs).
- Replacement recreates the file even on overwrite; rpack makes **no promise to preserve local read/write permissions** on replacement.
- Staged content stays private at `0600`, independent of intent. Final publication creates the file in the destination's parent directory and renames it into place, so the destination's local creation policy applies to the published inode; the staged `0600` is never implicitly copied to the output.
- The reproducible artifact is **bytes plus executable intent**. Purity is refined accordingly: same inputs always produce the same output bytes and the same executable intent; the materialized rwx bits are local policy, not contract.

### Publication validation

Before publishing, each created file is stat'ed: if it lacks owner-read, or its owner-execute classification no longer matches the declared intent, the file is refused — removed before the rename, so it is never published, and rpack never silently records success with an intent the filesystem disallows. The guarantee is the **absence of publication, not a step-specific message**: under a mask that denies directory search or write (for example umask `0100`), staging or setup can fail first with a less specific error. Concrete helper case: under umask `0100`, publishing a `755`-intent output into a pre-existing destination directory lands `0677` and is refused before the rename, with no destination file or temporary residue.

### Scope

- Directories use the normal creation policy and are not tracked.
- Ownership, ACLs, extended attributes, and special mode bits are not managed.

## Git and umask: the round trip this fixes

For regular files, Git records two canonical modes — `100644` (non-executable) and `100755` (executable) — and classifies executability by owner execute. Checkout creation requests `0666`/`0777` and lets the OS apply local policy:

| Git regular-file mode | umask `0022` | umask `0002` | umask `0077` |
|---|---|---|---|
| `100644` | `0644` | `0664` | `0600` |
| `100755` | `0755` | `0775` | `0700` |

Verified manually on Linux with Git `2.55.0` against rpack `dev` built from `c4383a6`; issue #15 preserves the full matrix, reproduction commands, and the drift messages observed at the exact-mode baseline. Every row is one Git-tracked state, but under ADR 0001 each cell was a different lockfile mode, and all of them — including the defaults `644`/`755` — reported drift after a Git round trip under a non-`022` umask. Under this ADR all cells in a row compare equal: executable intent survives Git checkouts, clones, and editor replacements; read/write bits belong to the destination machine.

## Migration: `rpack migrate-modes`

- `rpack migrate-modes --acknowledge-permission-change <config>` (optional local `--working-dir`/`-w`); there is deliberately no `--force`.
- It rewrites **only non-canonical legacy mode metadata** in the lockfile, using the recorded owner-execute bit (`"600"` → `"644"`, `"700"` → `"755"`, `"664"` → `"644"`, …). Common old `"644"`/`"755"` entries are reinterpreted automatically and need no migration.
- It validates all recorded content, executable state, and file existence first; any conflict aborts without rewriting. It performs no chmod and makes no target-file changes.
- Entries without a recorded mode stay unknown until the next generation rewrites them.
- Definitions still requesting rejected modes (`"600"`, …) fail and must change deliberately — private or deployment permissions belong to external permission management applied outside rpack.
- Downgrade caveat: an old binary still strict-compares full rwx modes against the `mode` strings, so a migrated lockfile can report mode drift under an old runtime, and an old runtime's successful next run rewrites entries with its own full modes, undoing the migration. Do not mix binaries across a migration; accepted for `0.x` (issue #15).

## Considered options

- **Exact modes with an explicit opt-in (the investigation's hybrid recommendation)** — rejected: two permission models, per-file policy metadata, provenance tracking in the lockfile, and doubled migration surface; exact modes recreate the Git/umask conflict for every definition that uses them.
- **Keep the ADR 0001 exact-mode contract** — rejected: documented caveats and `--force` did not resolve the competing policies; even the defaults drifted under normal umasks.
- **Stop tracking permissions entirely, keep chmod on generation** — rejected: a lost executable flag becomes invisible.
- **Ignore individual bits (e.g., group-write)** — rejected: an arbitrary exception list is not a permission model.
- **Blanket `--force` or `core.fileMode=false` as the remedy** — rejected as the general solution; both widen far beyond permissions.

## Consequences

- Git round trips under umasks `0022`/`0002`/`0077` no longer create permission conflicts or machine-specific lockfile modes.
- Adding or removing owner-execute externally remains detected; group/other execute differences alone do not change classification.
- rpack no longer guarantees — or overwrites — read/write bits on outputs. Replacement recreates the file, so its read/write bits are recomputed from the current creation policy (umask, default ACLs), not preserved: a `0600` checkout does not stay `0600` across regeneration. Only executable intent is tracked and verified.
- Documentation, Lua stubs, the executable example, and the author/tester skills move to `executable = true`, with `mode` described as an intent alias rather than a literal chmod.
- **Acceptance coverage** (implemented for issue #15): mode-alias parser and mutual-exclusion tests in `filemode_test.go`/`lualib_rpack_test.go` (accept `644`/`755` and leading zeros; reject other modes and the `mode`+`executable` combination); owner-execute classification and drift tests; creation-base and destination-parent publication tests in `materialize_test.go` (including replace-without-inheritance, refusal paths that preserve existing files, and dry-run intent display); reset-on-write ordering and temp non-propagation tests in `chmod_test.go`; umask coverage in `permission_umask_test.go` (child-process umask matrix across output paths, staging privacy under zero umask, fail-before-publish refusal under `0100`/`0400`, Git round-trip drift acceptance); `migrate-modes` tests in `modemigration_test.go` (rewrite, validation-abort, unknown-mode passthrough). The default-ACL flow is covered by an optional test that skips when `setfacl` is unavailable or the filesystem rejects ACLs — it is not claimed to run everywhere.
