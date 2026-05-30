# Roadmap

> Phases 1–9 are complete and frozen (v1.0.0 through v1.2.0). This document defines the evolution of `de`.

---

## Completed (v1.0.0 Baseline)

All original development phases have been delivered and are production-ready:

- ✓ Phase 1: Core CLI Completion (set, get, unset, list, hardened parser, validation)
- ✓ Phase 2: Security & Data Hardening (key rotation, export/import, HMAC folder IDs, atomic writes)
- ✓ Phase 3: Local Developer Experience (redaction, completions, color, JSON, verbose, export)
- ✓ Phase 4: Local Container Export Helpers (k8s-secret, k8s-configmap, compose, docker-env)
- ✓ Phase 5: Distribution & Onboarding (goreleaser, first-run wizard, config.toml, key purge)

## Completed (v1.0.0 Patch)

- ✓ Phase 6: Clean Uninstall & Re-Initialization Resilience (guarded auto-reinit after key purge)
- ✓ Phase 7: Codebase De-bloat & Release Freezing (v1 removal, git init, v1.0.0 branch/tag)

## Completed (v1.1.0)

- ✓ Phase 8: Environment Profiles (store v3, profile dimension, --profile flag, DE_PROFILE, profile command, v2→v3 migration)

## Completed (v1.2.0)

- ✓ Phase 9: Inline Config Edit (config path/edit/show, editor resolution, default template, documentation)

---

## Phase 9: Inline Config Edit (Target: v1.2.0)

**Goal:** Provide a native CLI method for users to view, modify, and open their configuration file without requiring them to know or remember the OS-specific config path.

**Problem Statement:**
Currently, modifying `de`'s configuration requires users to manually locate and edit a TOML file at a platform-dependent path (`~/.config/de/config.toml` on Linux/macOS, `%APPDATA%\de\config.toml` on Windows). This creates unnecessary friction:
- New users must read documentation to find the path.
- The path differs across operating systems, complicating team runbooks.
- There is no CLI-native way to inspect or modify settings beyond the `profile set` command added in v1.1.0.

**Solution:** Introduce a `de config` command family that provides first-class config file interaction directly from the terminal.

**Design Constraint:** The feature must not introduce new dependencies. Editor detection must use only standard environment variables and OS conventions. The existing `de profile set` command remains functional and unchanged — `de config edit` is the general-purpose superset.

### New Commands

| Command | Behavior |
|---------|----------|
| `de config path` | Print the absolute path to the active `config.toml` to stdout. Useful for scripting (`cat $(de config path)`). |
| `de config edit` | Open `config.toml` in the user's preferred terminal editor. Creates the file with commented defaults if it does not exist. |
| `de config show` | Print the current contents of `config.toml` to stdout (raw file contents). If the file does not exist, prints the default template with all keys commented out. |

### Editor Resolution (for `config edit`)

Resolution order (first match wins):

1. `DE_EDITOR` environment variable (de-specific override).
2. `VISUAL` environment variable.
3. `EDITOR` environment variable.
4. Platform fallback:
   - **Linux/macOS:** `nano` (widely pre-installed), then `vi`.
   - **Windows:** `notepad.exe` (always available).

If no editor is found and no fallback is available, print an actionable error: `"no editor found; set EDITOR or DE_EDITOR environment variable"` and exit with code 1.

### Default Config Template

When `config edit` is invoked and no `config.toml` exists, the file must be created with the following commented template before opening the editor:

```toml
# de configuration file
# See: de help | de config show

# Show values in plaintext by default (default: false)
# reveal = false

# Color output: "auto", "always", "never" (default: "auto")
# color = "auto"

# Active environment profile (default: "dev")
# Allowed: dev, staging, pre-prod, qe, prod
# profile = "dev"

# Override the store file path
# store_path = "/custom/path/store.json"

# Allow automatic store re-initialization when decryption fails after a key purge
# allow_reinit_on_decrypt_fail = false
```

This ensures users always see the full option set with documentation inline.

### Implementation Tasks

| # | Task | Detail | Status |
|---|------|--------|--------|
| 1 | Add `config` case to command router | In `app.go` `run()` switch, add `case "config"` routing to `runConfig(ctx, args[2:])`. Add `"config"` to `needsStore()` exclusion list (config commands do not require the store). | ✓ |
| 2 | Implement `runConfig()` dispatcher | Parse subcommands: no args or `"help"` → print config usage, `"path"` → `runConfigPath()`, `"edit"` → `runConfigEdit()`, `"show"` → `runConfigShow()`. Unknown subcommand → error with usage hint. | ✓ |
| 3 | Implement `de config path` | `fmt.Fprintln(ctx.Out, config.ConfigFilePath(ctx.Config))`. Trivial — prints the resolved path and exits 0. | ✓ |
| 4 | Implement `de config show` | Read `config.toml` from `config.ConfigFilePath(ctx.Config)`. If file exists, print contents to stdout. If file does not exist, print the default commented template to stdout (do NOT create the file on disk for `show`). | ✓ |
| 5 | Define default config template | Create a `const defaultConfigTemplate` in `internal/config/config.go` containing the full commented TOML template. This constant is reused by both `config show` (stdout) and `config edit` (file creation). | ✓ |
| 6 | Implement editor resolution | Create `resolveEditor() (string, error)` in `internal/app/app.go` (or a small helper). Check `DE_EDITOR` → `VISUAL` → `EDITOR` → platform fallback. On Windows, use `notepad.exe`. On Unix, try `nano` then `vi` existence via `exec.LookPath`. Return error if nothing found. | ✓ |
| 7 | Implement `de config edit` | (a) Resolve config file path. (b) If file does not exist, create it with the default template (mode 0600, parent dir 0700). (c) Resolve editor. (d) Execute `editor <config-path>` via `os/exec` with stdin/stdout/stderr forwarded (same pattern as `runtime.Exec`). (e) Wait for editor to exit. Return editor's exit code. | ✓ |
| 8 | Handle editor exit codes | If the editor process fails to start (not found, permission denied), print an actionable error and return exit code 1. If the editor exits non-zero, forward that exit code without additional messaging. | ✓ |
| 9 | Add `config` to shell completions | Add `"config"` to all four shell completion command lists (bash, zsh, fish, powershell). Add subcommand completions for `de config`: `path`, `edit`, `show`. | ✓ |
| 10 | Update usage text | Add `de config path`, `de config edit`, `de config show` to `usageText` in `usage.go`. Document `DE_EDITOR` in the environment variables section. | ✓ |
| 11 | Build and vet | `go build ./cmd/de ; go build ./cmd/internal ; go vet ./...` must pass cleanly. | ✓ |

### 9B: Documentation Update

**Strict dependency:** The release CANNOT be finalized until all documentation updates below are committed. This ensures the published v1.2.0 tag always ships with accurate, self-consistent documentation.

| # | Task | Detail | Status |
|---|------|--------|--------|
| 12 | Update `README.md` — CLI Reference | Add `de config path`, `de config edit`, `de config show` to the Commands section. Replace the manual "Config File" section's OS path instructions with a reference to `de config path` and `de config edit` as the primary workflow. Keep the OS paths as a "Manual alternative" note. | ✓ |
| 13 | Update `README.md` — Environment Variables | Add `DE_EDITOR` to the environment variables table with description: "Override the editor used by `de config edit`". | ✓ |
| 14 | Update `COMMANDS.md` | Add a full `## config` section documenting `config path`, `config edit`, `config show`, editor resolution order, and default template behavior. | ✓ |
| 15 | Update `ARCHITECTURE.md` | Add a note to the Execution Flow or Components section explaining that `de config edit` spawns an external editor process with forwarded I/O. No store or keychain interaction occurs. | ✓ |
| 16 | Update `MAINTAINER.md` | Add `config` to the "Adding a New Command" checklist example. Note that `config edit` is the only command that spawns a long-lived interactive process (the editor). | ✓ |
| 17 | Verify documentation accuracy | Re-read all five docs files and confirm no stale references to "manually edit config.toml" remain without also mentioning `de config edit`. | ✓ |

### 9C: Git Workflow & Release Freeze

**Strict ordering:** All implementation tasks (9A #1–#11) and all documentation tasks (9B #12–#17) MUST be committed to `master` before any release operations. The release branch and tag are created from the fully-complete `master` HEAD.

| # | Task | Detail | Status |
|---|------|--------|--------|
| 18 | Commit feature code | Stage all modified Go source files. Commit message: `feat: de config edit/path/show — inline config management (v1.2.0)`. | ✓ |
| 19 | Commit documentation | Stage all modified `.md` files. Commit message: `docs: update all documentation for v1.2.0 config commands`. May be combined with #18 into a single commit if preferred. | ✓ |
| 20 | Create `v1.2.0` release branch | `git branch v1.2.0` from the `master` HEAD containing both commits. This branch is frozen — no further commits unless a critical security patch is required. | ✓ |
| 21 | Tag `v1.2.0` | `git tag -a v1.2.0 -m "Release v1.2.0: inline config editing, config path/show commands"` | ✓ |
| 22 | Verify `master` is active | Confirm `git branch` shows `* master`. Confirm `git tag -l` shows `v1.0.0`, `v1.1.0`, `v1.2.0`. | ✓ |

### UX Examples

```bash
# Find where your config lives (useful in scripts or unfamiliar machines)
de config path
# Output: /home/user/.config/de/config.toml

# Open config in your preferred editor
de config edit
# → Opens nano/vim/$EDITOR with config.toml
# → File is created with commented defaults if it didn't exist

# Quick inspection without opening an editor
de config show
# → Prints config.toml contents to stdout (pipe-friendly)

# Override editor for a single invocation
DE_EDITOR=code de config edit
# → Opens config.toml in VS Code

# Use in scripts to check config path
cat "$(de config path)"
```

### Security Considerations

- `de config edit` spawns a child process (the editor). The editor inherits the current process environment but does NOT receive any secrets from the encrypted store — this command never loads the store.
- The config file itself contains no secrets (only preferences like `reveal`, `color`, `profile`). The `store_path` value is a filesystem path, not a credential.
- File creation uses mode `0600` (owner read/write only) and parent directory `0700`.
- No shell expansion or interpolation is applied to the editor command — it is invoked directly via `os/exec`, preventing command injection.

**Exit Criteria:** A user on any supported platform can run `de config edit` to immediately open their configuration file in an editor without needing to know the OS-specific path. `de config path` and `de config show` provide non-interactive access for scripting and inspection. All documentation reflects the new commands as the primary config workflow. The v1.2.0 tag is created from a `master` commit that includes both the implementation and the documentation update.
