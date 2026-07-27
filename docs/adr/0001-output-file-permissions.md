# Output file permissions: chmod primitive, mode option, lockfile mode tracking

Status: accepted

rpack could not produce executable output files: every write staged at 0644 and `rpack.copy` discarded source modes. We add an explicit `rpack.chmod(path, mode)` primitive, an optional `{mode = "755"}` options table on `rpack.write`/`rpack.copy`, and record each managed file's mode in the lockfile so out-of-band chmod is detected as drift. Everything is explicit — no content sniffing, no declared outputs, no implicit mode inheritance.

## Terminology

- **staged output** — a target-resolver file written into the run dir before relocation.
- **declared mode** — the mode a script requests (`chmod` argument or `opts.mode`); parsed once at the Lua boundary.
- **canonical default mode** — `0644`, explicitly re-asserted by every staged target write (normalization).
- **effective mode** — the staged file's actual `Mode().Perm()` at capture time; what the lockfile records.
- **recorded mode** — the lockfile `mode` field (string, e.g. `mode: "755"`).
- **mode drift** — on-disk target mode ≠ recorded mode at check/guard time.
- **reset-on-write** — any write re-establishes 0644, discarding earlier chmods; chmod only amends the current staged content until the next write.

## Decision

**Lua API.** `rpack.chmod(path, mode)`; `rpack.write(path, content [, {mode = "755"}])`; `rpack.copy(src, dst [, {mode = "755"}])`. The options table is the only new calling convention — no file-spec objects, no string shorthand. `write_lines` gets no options (positional args 3/4 are taken); chmod after it covers the case.

**Mode grammar.** Octal string matching `^0?[0-7]{3}$` (`"755"`, `"0755"`). Numbers are rejected (493 is unreadable — the reason the grammar exists). Special bits (>0o777) are rejected. Modes without owner-read (`mode & 0o400 == 0`) are rejected because rpack hashes its own outputs: a `"000"` staged file makes `computeFilesToMove` fail after successful execution, and if ever recorded, `CheckIntegrity` would hard-error before `--force` is consulted — a self-DoS. Unknown options-table keys are hard errors listing the known keys. Parse errors attribute to the offending argument (`L.ArgError`); FS failures raise with the friendly path. If `preserve_mode` is ever added, `mode` + `preserve_mode` together are a hard error ("contradictory").

**Chmod semantics.** Allowed on unprefixed target paths and `temp:` (the resolvers writable today); refused on `rpack:`/`map:`. Directories are refused (relocation recreates parents at 0755 — a staged dir mode would be silently dropped, so accepting it would be a lie). Chmod never creates: `BaseFS.Chmod` runs the shared writability check first, then a pre-hook existence check (`cannot chmod <path>: file does not exist (write it first)`), then fires the normal Write hook chain, then `handle.Chmod`. The pre-hook existence check guarantees a pcall-swallowed chmod leaves zero recorder/purity residue; running writability *before* existence keeps the access-refusal the canonical error for read-only resolvers. Chmod on `temp:` is allowed and purity-invisible, and its mode does **not** propagate through `copy` (copy = read bytes + write 0644) — documented, since users will expect otherwise. Last chmod wins; chmod to the current mode is a no-op success.

**Recording and purity.** Chmod reuses the `Write` hook path — no fifth `FSAccessHook` method, no new `FSAccessType`. Access control, `EnsurePure` (read `map:x` + chmod `./x` conflicts in both orders), and the recorder all treat chmod as a write. Dedup collapses the double record when write+chmod hit the same path.

**Staging normalization.** Every staged *target* write is normalized: `os.WriteFile(..., 0644)` followed by an explicit `os.Chmod(0644)` inside `FileBackedFSHandle.Write`, applied only when `handle.Resolver() == TargetResolver` (temp writes are excluded so umask-077 users keep 0600 scratch files). This kills umask nondeterminism — otherwise two developers with different umasks record different modes and the lockfile is uncommittable — and yields the reset-on-write rule: *a write establishes mode 0644; chmod amends until the next write*. The options-table mode is applied as a separate `fs.Chmod` through the full hook path after the write; `Write` itself never takes a mode parameter (no privileged second mutation path).

**Relocation.** The normal path (`moveFiles`, `os.Rename`, same filesystem) already preserves staged modes. `copyDir` (`--output-dir`, `--dry-run`+output, direct-to-CWD) must propagate the staged mode with an explicit post-write `os.Chmod(srcInfo.Mode().Perm())` — passing perm to `os.WriteFile` is creation-only and would leak stale modes on `--force` overwrites. Direct mode never renames; keep it that way (EXDEV). Dry-run output shows the mode (`=== ./deploy.sh (mode 755) ===`).

**Lockfile (G).** Additive `mode` string field per entry — **no schema version bump** (`Validate()` rejects unknown versions, so bumping would hard-break old binaries; the additive field is silently ignored by them). String type with quoted YAML values, because unquoted `755`/`0o755` is an octal-integer trap under YAML. The field is always emitted (no `omitempty`; capture asserts non-empty) including `"644"` — absence means exactly one thing: "pre-feature lockfile, mode unknown". Effective mode is captured via staging stat in `computeFilesToMove` (already touches every file; deterministic thanks to normalization).

**Integrity.** `RPackLockFileIntegrity` gains `ModeModified []string` — a distinct category, because "file was modified" on a content-identical file sends users diff-hunting. All files' modes are checked (not only explicitly-chmod'ed ones); entries lacking a recorded mode are skipped with an `slog.Warn` (upgrade window). Comparison lives in `CheckIntegrity` (detection choke point); both enforcement call sites — `guardLockfileIntegrity` and `checker.go` — gain a `ModeModified` branch with want/got pairs (`deploy.sh (recorded 755, on disk 644)`), refused without `--force`, same severity as content modification. A file that cannot be read for hashing is classified as drift, not a hard error, so `--force` can always heal (rename-replace + lockfile rewrite). Removed files win over mode comparison.

**Scope.** Mode *survival* applies on all four output paths; mode *tracking* applies only to the lockfile-managed path (`--output-dir`/direct/dry-run write no lockfile). `meta.json` stays a path list. No CUE schema changes — modes are script-level, never config-level. POSIX-only (builds target linux/darwin).

## Considered Options

- **File-spec objects** (`rpack.write(path, rpack.file(content, {mode=...}))`) — rejected: new concept, union-typed signatures, table-vs-string dispatch, all to serve a composition pattern short linear rpack scripts never have. The options table is the same expressiveness with zero new concepts.
- **Shebang auto-detection** (content starting `#!` stages 0755) — rejected: content-sniffing implicit behavior destroys the semantics of rpack; every behavior must be explicit at the call site.
- **Mode-preserving copy by default** — rejected as implicit (mode would depend on hidden source state); deferred as an explicit `{preserve_mode = true}` option on `copy` if a real use case appears.
- **Declarative permissions manifest in the rpackdef** — rejected: outputs are never declared in the definition and never will be.
- **Lockfile schema version bump** — rejected: `Validate()` hard-rejects unknown versions; the additive field degrades gracefully instead.
- **Folding mode drift into `Modified`** — rejected: the error message must say *permissions* changed, or users hunt for content diffs that don't exist.
- **Recording only explicitly-set modes** — rejected: recording the effective mode of every file closes the invisibility hole completely and costs nothing after normalization.
- **`os.WriteFile` perm bits for relocation/normalization** — rejected: perm applies only at file creation; explicit chmod is deterministic for both create and overwrite.

## Consequences

- **Behavior change:** staged target outputs are now always 0644 regardless of umask (previously umask-077 produced 0600). The old behavior was accidental, not contractual; users who want 0600 can declare `mode = "600"` — which was previously impossible.
- **Git interaction (document prominently):** git preserves only the executable bit, so recorded modes other than 0644/0755 will report drift after a fresh clone until `rpack run --force` re-converges. Full 000–777 remains allowed for non-committed outputs.
- **Upgrade window:** the first run against a pre-feature lockfile cannot detect pre-existing mode drift (no recorded modes); it is skipped with a warning and the lockfile is rewritten with modes. Content drift in the same run is still refused.
- **Downgrade:** an old binary reading a new lockfile ignores `mode`, and its next run rewrites the lockfile without modes — silent loss of mode tracking (accepted; beta).
- **Ordering rule users must learn:** chmod after your last write; a write resets the mode. Enforced by reset-on-write and taught by the stubs' ldoc.
- **Cosmetic:** a chmod is logged as `write` in the FS-interaction slog records; `meta.json` is unaffected (deduped path set).
- **Follow-through surface:** Lua stubs/ldoc (with the ordering rule and temp non-propagation note), README, `rpack-author`/`rpack-tester` skills (`test -x` assertion pattern), one example, `InMemoryFS` gains a mode field, and the test matrix: mode parser table tests, Lua-level API tests, hook/purity tests, four-path relocation tests, lockfile record/check/heal/upgrade tests.
