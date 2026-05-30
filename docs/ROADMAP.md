# Roadmap

> Phases 1–5 of the initial release are complete and frozen. This document defines the next evolution of `de` beyond the v1.0.0 baseline.

---

## Completed (v1.0.0 Baseline)

All original development phases have been delivered and are production-ready:

- ✓ Phase 1: Core CLI Completion (set, get, unset, list, hardened parser, validation)
- ✓ Phase 2: Security & Data Hardening (key rotation, export/import, HMAC folder IDs, atomic writes)
- ✓ Phase 3: Local Developer Experience (redaction, completions, color, JSON, verbose, export)
- ✓ Phase 4: Local Container Export Helpers (k8s-secret, k8s-configmap, compose, docker-env)
- ✓ Phase 5: Distribution & Onboarding (goreleaser, first-run wizard, config.toml, key purge)

---

## Phase 6: Clean Uninstall & Re-Initialization Resilience

**Goal:** Allow seamless recovery after a key purge without requiring manual filesystem intervention.

**Problem Statement:**
After `de key purge --confirm` removes the master key from the OS keychain, the encrypted store file remains on disk. Any subsequent command that touches the store (e.g., `de init`, `de set`) attempts to decrypt it with a newly generated key, fails with a GCM authentication error, and blocks the user. The only recovery path today is manually deleting `~/.config/de/store.json` — an unacceptable UX gap for a tool designed for zero-friction operation.

**Solution:** Introduce a guarded automatic re-initialization pathway triggered by decryption failure, controlled by an explicit opt-in configuration flag.

| Task | Detail | Status |
|------|--------|--------|
| Add `allow_reinit_on_decrypt_fail` to config schema | New boolean field in `internal/config/config.go` `Config` struct. Parsed from `config.toml` key `allow_reinit_on_decrypt_fail`. | ☐ |
| Default to `false` | The field must default to `false`. When false, decryption failures produce the existing error message with remediation hints (no behavior change). | ☐ |
| Implement guarded re-init in store load path | In `json_store.go` `loadAndMigrate()`: when decryption fails AND no key exists in the keychain AND config flag is `true` → remove the unreadable store file, generate a fresh master key, and return an empty DB. | ☐ |
| Safety logging | When automatic re-init fires, emit a clear warning to stderr: `"warning: store was unreadable (no master key); purged and re-initialized a fresh store."` | ☐ |
| Guard against accidental data loss | Re-init MUST only trigger when ALL of the following are true simultaneously: (1) the config flag is `true`, (2) the decryption error is an authentication/integrity failure (not a filesystem permission error), (3) no master key exists in the keychain (confirming a prior purge rather than a corrupted key). If a key IS present in the keychain but decryption fails, this indicates a mismatched key — the existing error path with the `de key import` hint must be preserved. | ☐ |
| Update `config.toml` documentation | Add commented example to README and inline comment in config loading explaining the flag's purpose and risk. | ☐ |
| End-to-end validation | Verify the following sequence completes without manual intervention when the flag is enabled: `de init` → `de set FOO=bar` → `de key purge --confirm` → `de init` → `de set FOO=baz` → `de get FOO --reveal` returns `baz`. | ☐ |

**Exit Criteria:** A user who has previously run `de key purge --confirm` can immediately run `de init` in any directory and resume normal operation without touching the filesystem, provided `allow_reinit_on_decrypt_fail = true` is set in their config. Users who have not opted in experience zero behavior change.

---

## Phase 7: Codebase De-bloat & Release Freezing

**Goal:** Remove legacy code paths, establish version control hygiene, and freeze the stable v1.0.0 release.

### 7A: v1 Store Migration Removal

**Rationale:** The v1 store format stored plaintext directory paths as JSON keys. The transparent v1→v2 migration in `loadAndMigrate()` was a transition mechanism. With v1.0.0 shipped and all stores upgraded on first load, retaining this code:
- Increases attack surface (plaintext path handling logic remains reachable).
- Adds cognitive load for maintainers reading the store layer.
- Provides no value to any user who has run any command since v2 was introduced.

| Task | Detail | Status |
|------|--------|--------|
| Remove v1 migration branch | In `json_store.go` `loadAndMigrate()`: delete the `if db.Version < 2 && len(db.Folders) > 0` block that iterates folders and re-hashes plaintext paths. | ☐ |
| Fail explicitly on v1 stores | If a decrypted store has `version < 2`, return an error: `"store version %d is no longer supported; this store must be migrated with de v1.0.0 before upgrading"`. This prevents silent data loss if an ancient store is encountered. | ☐ |
| Remove `CurrentVersion` write in migration path | The remaining `db.Version = CurrentVersion` assignment after the deleted block can stay (it normalizes version on fresh loads) but verify no dead code references remain. | ☐ |
| Audit for orphaned v1 references | Grep codebase for any comments, test stubs, or documentation referencing "v1", "plaintext path", or "migration" and remove or update them. | ☐ |
| Verify build and vet | `go build ./cmd/de ; go build ./cmd/internal ; go vet ./...` must pass cleanly. | ☐ |

### 7B: Git Repository Initialization & Release Branch

| Task | Detail | Status |
|------|--------|--------|
| `git init` | Initialize the repository in the project root. | ☐ |
| Create `.gitignore` | Ignore: `de`, `de.exe`, `*.tmp`, `dist/`, and any IDE-specific directories (`.idea/`, `.vscode/` settings). Do NOT ignore `docs/` or `.goreleaser.yaml`. | ☐ |
| Initial commit on `master` | Stage all files. Commit message: `feat: de v1.0.0 — production-ready local env runner`. This commit represents the complete, de-bloated codebase after Phase 7A. | ☐ |
| Create `v1.0.0` release branch | `git branch v1.0.0` from the initial commit. This branch is frozen — no further commits unless a critical security patch is required. | ☐ |
| Tag `v1.0.0` | `git tag -a v1.0.0 -m "Release v1.0.0: full local secret management CLI"` | ☐ |
| Verify `master` is active | Confirm `git branch` shows `* master` as the working branch for future development. | ☐ |

**Exit Criteria:** The repository has clean version control with a frozen release branch, the codebase contains zero v1 legacy code, and the master branch is ready for Phase 8 feature development targeting v1.1.0.

---

## Phase 8: Environment Profiles (Target: v1.1.0)

**Goal:** Allow developers to maintain multiple isolated environment configurations (e.g., dev, staging, prod) within the same directory, switchable with a single command.

**Design Constraint:** The profile system must not degrade the current zero-friction UX. A user who never touches profiles experiences identical behavior to v1.0.0 — the `dev` profile is implicit and invisible.

### Data Model Extension

The encrypted store's folder structure gains a profile dimension:

```json
{
  "version": 3,
  "folders": {
    "<hmac-folder-id>": {
      "dev": {
        "API_KEY": "dev-key",
        "DB_URL": "localhost:5432/myapp"
      },
      "staging": {
        "API_KEY": "staging-key",
        "DB_URL": "staging-db.internal:5432/myapp"
      }
    }
  }
}
```

- **Version bump:** Store version becomes `3`. A transparent v2→v3 migration wraps existing flat `map[string]string` folder entries into `{"dev": <existing_map>}`.
- **Profile names:** Restricted to the predefined set: `dev`, `staging`, `pre-prod`, `qe`, `prod`. Custom profile names are explicitly out of scope for v1.1.0.

### Active Profile Resolution

Resolution order (first match wins):

1. `--profile` flag on the command (e.g., `de set KEY=VALUE --profile staging`).
2. `DE_PROFILE` environment variable.
3. `profile` key in `~/.config/de/config.toml`.
4. Default: `dev`.

The active profile applies to ALL commands that interact with the store: `set`, `get`, `unset`, `list`, `exec`, `export`, `migrate`.

### New Commands

| Command | Behavior |
|---------|----------|
| `de profile` | Print the currently active profile name to stdout. |
| `de profile set <name>` | Write `profile = "<name>"` to `config.toml`, making it the persistent default. Validates against the allowed set. |
| `de profile list` | Print all allowed profile names, marking the active one with `*`. |

### Implementation Tasks

| Task | Detail | Status |
|------|--------|--------|
| Extend `Config` struct | Add `Profile string` field. Load from `config.toml` key `profile`, env var `DE_PROFILE`, and `--profile` global flag. Default `"dev"`. | ☐ |
| Define allowed profiles | Constant slice: `[]string{"dev", "staging", "pre-prod", "qe", "prod"}`. Validation function rejects anything outside this set. | ☐ |
| Update `DB` struct | `Folders` type changes from `map[string]map[string]string` to `map[string]map[string]map[string]string` (folder ID → profile → vars). | ☐ |
| Store version bump to 3 | Update `CurrentVersion` to `3`. | ☐ |
| v2→v3 transparent migration | In `loadAndMigrate()`: if `db.Version == 2`, wrap each folder's `map[string]string` into `{"dev": existing_map}` and set version to 3. Atomic save. | ☐ |
| Thread profile through store interface | All `EnvStore` methods gain awareness of the active profile. The profile is determined at the `Context` level and passed into store calls. | ☐ |
| Update `EnvStore` interface | Methods become profile-aware: `EnvForDir(dir, profile string)`, `SetVar(dir, profile, key, value string)`, etc. Alternatively, embed profile in the store constructor. Choose the approach that minimizes interface churn. | ☐ |
| Implement `de profile` command | Route in `run()` switch. Subcommands: (none) → print active, `set <name>` → persist to config, `list` → enumerate with marker. | ☐ |
| Add `--profile` global flag | Parse alongside `--verbose` and `--json` in `parseGlobalFlags()`. Override `ctx.Config.Profile` if present. | ☐ |
| Update `exec` to use active profile | `runExec()` reads `ctx.Config.Profile` and passes it to `EnvForDir()`. | ☐ |
| Update `set`, `get`, `unset`, `list` | All CRUD commands use `ctx.Config.Profile` for store operations. | ☐ |
| Update `export` | Export emits variables from the active profile. | ☐ |
| Update `migrate` | Imports into the active profile for the directory. | ☐ |
| Update `doctor` | Validate the v3 store structure. Report per-profile variable counts. | ☐ |
| Update `key rotate` | Re-hash logic must handle the nested profile structure. | ☐ |
| Update shell completions | Add `profile` to command lists. Add `--profile` completion for all relevant commands. Add profile name completions for `de profile set`. | ☐ |
| Update usage text | Add `de profile`, `de profile set <name>`, `de profile list` to help output. Document `--profile` global flag. | ☐ |
| Update documentation | README, COMMANDS.md, ARCHITECTURE.md — document profile behavior, resolution order, and data model change. | ☐ |
| Backward-compatible guarantee | A user who never sets a profile, never passes `--profile`, and has no `DE_PROFILE` env var experiences identical behavior to v1.0.0. The `dev` profile is implicit. Verify with existing workflow: `de init` → `de set KEY=VALUE` → `de get KEY --reveal` → returns VALUE. | ☐ |
| Build and vet | `go build ./cmd/de ; go vet ./...` clean. | ☐ |

### UX Examples

```bash
# Default behavior (implicit "dev" profile, zero-friction)
de set API_KEY=dev-key-123
de npm run dev                  # injects dev-key-123

# Switch to staging
de profile set staging
de set API_KEY=staging-key-456
de npm run dev                  # injects staging-key-456

# One-off execution with a different profile
de --profile prod exec -- node server.js

# Check active profile
de profile                      # prints: staging

# See all profiles
de profile list
#   dev
# * staging
#   pre-prod
#   qe
#   prod
```

**Exit Criteria:** A developer can manage isolated variable sets per profile within the same directory, switch profiles persistently or per-command, and experience zero UX regression when profiles are not explicitly used. The store transparently upgrades from v2 to v3 on first access after the update.

---

## Summary

| Phase | Target | Focus |
|-------|--------|-------|
| 6 | Post-v1.0.0 patch | Re-initialization resilience after key purge |
| 7 | v1.0.0 freeze | Legacy removal + git workflow establishment |
| 8 | v1.1.0 feature | Environment profiles for multi-context workflows |
