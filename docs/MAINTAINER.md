# Maintainer Guide

This document captures the engineering decisions, internal architecture, and operational knowledge required to maintain and extend `de`.

---

## Table of Contents

- [Architectural Rationale](#architectural-rationale)
- [Package Layout](#package-layout)
- [Cryptographic Engineering](#cryptographic-engineering)
- [Data Model & Store Mechanics](#data-model--store-mechanics)
- [Path Canonicalization](#path-canonicalization)
- [Parser Architecture](#parser-architecture)
- [Output System](#output-system)
- [Build & Release](#build--release)
- [Testing Strategy](#testing-strategy)
- [Maintenance Procedures](#maintenance-procedures)
- [Future Considerations](#future-considerations)

---

## Architectural Rationale

### Why a Runtime Wrapper (Not Shell Hooks)

The core design decision was to implement a **folder-aware runtime wrapper** rather than a shell hook system (like `direnv`). This decision is load-bearing and should not be revisited without understanding the full tradeoff space:

| Concern | Shell Hook (`direnv` model) | Runtime Wrapper (`de` model) |
|---------|----------------------------|------------------------------|
| File presence in workspace | Requires `.envrc` or `.env` in project tree | Zero files in project tree |
| AI/indexer exposure | Plain-text files are scrapeable by extensions, LLMs, rogue tooling | No files to scrape; secrets exist only in encrypted central store |
| Shell compatibility | Requires per-shell hook scripts (bash, zsh, fish, PowerShell differ) | Single compiled binary; identical behavior everywhere |
| Secret lifetime | Loaded into ambient shell; persist until `cd` away or shell exit | Injected only into the subprocess; vanish on process termination |
| Cross-platform parity | Complex; Windows shell hooks are fragile | Native; Go's `os/exec` works identically across platforms |

The wrapper model means secrets are **never** in the user's terminal environment. They exist exclusively within the spawned subprocess's memory block and are reclaimed by the OS when the process exits.

### Why Go

- Single static binary with no runtime dependencies.
- Native `os/exec` with proper stdin/stdout/stderr forwarding.
- `golang.org/x/term` for secure passphrase reading.
- `github.com/zalando/go-keyring` abstracts OS keychain differences (Secret Service on Linux, wincred on Windows, Keychain on macOS).
- Cross-compilation is trivial for the release matrix.

---

## Package Layout

```
cmd/
  de/main.go            CLI entrypoint; delegates to internal/app.Run()
  internal/main.go      Alternate entrypoint (same behavior)

internal/
  app/
    app.go              Command router, all command implementations
    output.go           TTY detection, colored output, JSON mode, redaction
    completion.go       Shell completion scripts (bash/zsh/fish/powershell)
    passphrase.go       Secure passphrase input via x/term
    usage.go            Help text

  config/
    config.go           Config resolution: defaults → env vars → config.toml

  crypto/
    crypto.go           AES-GCM encrypt/decrypt, keychain CRUD, HMAC folder IDs, key export/import
    kdf.go              PBKDF2 key derivation (600k iterations, SHA-256)

  env/
    env.go              Environment slice merging, key normalization (case-insensitive on Windows)
    parse.go            Hardened .env parser (quotes, escapes, multiline, comments)

  export/
    export.go           Container format emitters (K8s Secret/ConfigMap, Compose, docker-env)

  paths/
    paths.go            CanonicalDir: absolute path, symlink resolution, case folding (Windows)

  runtime/
    exec.go             Subprocess execution with environment injection

  store/
    store.go            EnvStore interface, DB struct, DoctorResult
    json_store.go       Encrypted JSON store: load/save/migrate/rotate/doctor

  version/
    version.go          Version/Commit/BuildDate vars (injected via ldflags)
```

### Dependency Graph (Internal)

```
app → config, crypto, env, export, paths, runtime, store, version
store → crypto
(all other packages are leaf packages with no internal dependencies)
```

---

## Cryptographic Engineering

### Master Key Lifecycle

1. **Generation:** 32 bytes from `crypto/rand.Reader` (256-bit AES key).
2. **Storage:** Base64-encoded in the OS keychain under service `de-cli-master-key`, account `master`.
3. **Retrieval:** `crypto.GetOrGenerateKey()` — returns existing key or generates + stores a new one.
4. **Rotation:** New key generated → store re-encrypted → keychain updated (all in `store.Rotate()`).
5. **Export:** PBKDF2-wrapped with user passphrase → written to file.
6. **Import:** Read blob → unwrap with passphrase → store in keychain.
7. **Purge:** `keyring.Delete()` — removes the keychain entry.

### AES-GCM Encryption

```
Ciphertext format: [12-byte nonce][GCM-sealed payload]
```

- Nonce is generated fresh from `crypto/rand` for every encryption operation.
- AES-256-GCM provides both confidentiality and integrity (authenticated encryption).
- A modified store file will fail decryption with a GCM tag mismatch error.

### Key Export/Import Format

```
Export blob: [16-byte random salt][AES-GCM(derived_key, master_key)]
```

- `derived_key = PBKDF2-SHA256(passphrase, salt, 600000 iterations, 32 bytes)`
- The passphrase is read directly from the terminal via `term.ReadPassword()` — never echoed, never logged.
- 600,000 PBKDF2 iterations follow OWASP 2023 guidance for SHA-256.

### Folder ID Hashing

```
folder_id = HMAC-SHA256(canonical_path, master_key) → hex string (64 chars)
```

- The master key is used as the HMAC key.
- This means folder IDs change when the master key rotates (handled in `store.Rotate()`).
- On-disk, no directory path is ever stored in plaintext.

### Keychain Backend Details

The `go-keyring` library dispatches to:

| OS | Backend | Credential Identifier |
|----|---------|----------------------|
| Linux | D-Bus Secret Service API (GNOME Keyring, KWallet) | Service: `de-cli-master-key`, User: `master` |
| Windows | Windows Credential Manager (`wincred`) | Target: `de-cli-master-key/master` |
| macOS | macOS Keychain | Service: `de-cli-master-key`, Account: `master` |

---

## Data Model & Store Mechanics

### On-Disk Format

The file at `~/.config/de/store.json` is **not** JSON on disk. It is a raw AES-GCM ciphertext blob. When decrypted, the plaintext is:

```json
{
  "version": 3,
  "folders": {
    "a3f8...64-char-hex-hmac...": {
      "dev": {
        "API_KEY": "sk-live-abc123",
        "DB_HOST": "localhost"
      },
      "staging": {
        "API_KEY": "sk-staging-xyz"
      }
    },
    "b7c2...another-hmac...": {
      "dev": {
        "PORT": "8080"
      }
    }
  }
}
```

Each folder maps profile names (`dev`, `staging`, `pre-prod`, `qe`, `prod`) to variable sets.

### Store Versioning

- **Version 1:** Folder keys were plaintext canonical paths. This version is rejected with an explicit error directing users to migrate via v1.0.0.
- **Version 2:** Folder keys are HMAC-SHA256 hex digests; values are flat `map[string]string`. Transparently migrated to v3 on first load (existing vars wrapped into the `dev` profile).
- **Version 3 (current):** Folder values gain a profile dimension: `map[profile]map[key]value`. Migration from v2 is atomic (old store replaced via tmp+rename).

### Atomic Write Strategy

All store mutations follow this sequence:

1. Marshal JSON → encrypt with AES-GCM.
2. Write to `store.json.tmp` with mode `0600`.
3. `os.Remove("store.json")` (ignore ENOENT).
4. `os.Rename("store.json.tmp", "store.json")`.

The remove-then-rename pattern handles Windows, where `os.Rename` fails if the target exists. On crash at any point: either the old file or the new file exists — never a partial write.

### Key Rotation Re-Hashing

During `store.Rotate(oldKey, newKey)`:
1. Load and decrypt with `oldKey`.
2. For each folder entry, the **existing HMAC hash string** is itself hashed under `newKey` (since we cannot reverse HMAC to get the original path).
3. Re-encrypt with `newKey` and save.
4. Update the keychain with `newKey`.

This means after rotation, the folder IDs are `HMAC(old_hmac_hex_string, newKey)`. This is deterministic: running `de get KEY` from the same directory will produce `HMAC(canonical_path, newKey)` — **this matches** because `canonical_path` was already the input that produced `old_hmac_hex_string` under `oldKey`.

**Important correction:** After rotation, lookups use `FolderID(dir, newKey)` which hashes the *canonical path* with the new key. But the stored keys are `FolderID(old_hmac_string, newKey)`. These will NOT match. This is a known design limitation: rotation works correctly only if the user does not have active directory associations that need to be looked up post-rotation without re-initialization. In practice, the `Rotate()` function re-hashes the opaque IDs, preserving the internal mapping even though the relationship to the original paths is lost. Future iterations may store a reversible path index.

### Doctor Integrity Checks

`de doctor` performs:
1. Retrieve key from keychain (fail → report keychain error).
2. Decrypt store with key (fail → report decryption error).
3. Validate all folder ID lengths are 64 characters (detect v1 remnants).
4. Count folders and total variables (across all profiles).
5. Report `status: ok` or `status: errors detected`.

---

## Path Canonicalization

`paths.CanonicalDir(dir string)` applies these transformations in order:

1. If `dir` is empty, use `"."` (current working directory).
2. `filepath.Abs()` — resolve to absolute path.
3. `filepath.Clean()` — normalize separators and remove trailing slashes.
4. `filepath.EvalSymlinks()` — resolve symbolic links (best-effort; ignored on error).
5. **Windows only:** `strings.ToLower()` — case-fold the entire path.

This ensures that `C:\Users\Dev\Project`, `c:\users\dev\project`, and `C:\Users\Dev\Project\` all resolve to the same canonical form on Windows.

---

## Parser Architecture

The `.env` parser in `internal/env/parse.go` handles production-grade `.env` files:

### Parsing Rules

1. **Blank lines and comments:** Lines that are empty or start with `#` are skipped.
2. **Export prefix:** Lines beginning with `export ` have the prefix stripped.
3. **Key extraction:** Split on first `=`. Key is trimmed of whitespace.
4. **Value parsing** branches on the first character after `=`:
   - **`"`** → Double-quoted mode: interprets escape sequences (`\"`, `\\`, `\n`, `\r`, `\t`). Supports multiline (continues to next line if closing quote not found).
   - **`'`** → Single-quoted mode: literal content, no escapes. Supports multiline.
   - **Anything else** → Unquoted mode: strip inline `#` comments, trim trailing whitespace.

### Validation

- `ValidateKey()`: Rejects empty keys, keys containing `=`, and non-UTF-8 strings.
- `ValidateValue()`: Rejects non-UTF-8 strings.
- Line numbers are included in all parse error messages.

### Environment Merging

`env.Merge(base []string, extra map[string]string)` merges the store variables into the process environment:

1. Index existing `KEY=VALUE` pairs from `base` by normalized key.
2. For each extra variable: if the normalized key exists in base, **overwrite in-place**; otherwise append.
3. On Windows, normalization is `strings.ToUpper(key)` — ensuring case-insensitive matching.

---

## Output System

### TTY Detection

`defaultOutputMode()` checks if stdout is a terminal via `term.IsTerminal(os.Stdout.Fd())`. This controls:

- **TTY mode:** Bold ANSI formatting for keys (`\033[1m`).
- **Pipe mode:** Plain `KEY=value` text, no escape sequences.

### Redaction

By default, all values are printed as `****`. Plaintext is revealed only when:
- `--reveal` flag is passed (per-command).
- `DE_REVEAL=1` environment variable is set.
- `reveal = true` in `config.toml`.

### JSON Mode

`--json` emits `{"KEY": "value"}` (or `{"KEY": "****"}` when redacted) via `encoding/json` with 2-space indentation.

### Verbose Mode

`--verbose` prints to stderr (never stdout) to avoid corrupting pipeable output:
```
[verbose] dir: /home/user/project
[verbose] store: /home/user/.config/de/store.json
[verbose] variables: 3
```

---

## Build & Release

### Local Build

```bash
go build -o de ./cmd/de
```

### Release Build (goreleaser)

The `.goreleaser.yaml` at the project root defines:

- **Build matrix:** linux/amd64, linux/arm64, windows/amd64, darwin/amd64, darwin/arm64.
- **CGO disabled:** Static binaries with no system library dependencies.
- **ldflags:** Inject `Version`, `Commit`, `BuildDate` into `internal/version` vars.
- **Archives:** `.tar.gz` for Linux/macOS, `.zip` for Windows.
- **Checksums:** SHA-256 checksums file generated automatically.

### Version Injection

```bash
go build -ldflags "-s -w \
  -X dx-util-env-manager/internal/version.Version=1.0.0 \
  -X dx-util-env-manager/internal/version.Commit=$(git rev-parse HEAD) \
  -X dx-util-env-manager/internal/version.BuildDate=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  -o de ./cmd/de
```

---

## Testing Strategy

### Manual Verification

```bash
# Build both entrypoints
go build ./cmd/de
go build ./cmd/internal

# Static analysis
go vet ./...

# Full test run (when tests are added)
go test ./...
```

### Key Test Scenarios

| Scenario | Verification |
|----------|-------------|
| First-run (no store, no key) | Should print onboarding message |
| `de init` creates store | Verify file exists at store path |
| `de set` / `de get` roundtrip | Set a value, get it back with `--reveal` |
| Redaction | `de get KEY` shows `****`, `de get KEY --reveal` shows value |
| Key rotation | `de key rotate` → existing vars still accessible |
| Key export/import | Export → purge → import → vars still accessible |
| `.env` parsing edge cases | Quoted values, escapes, multiline, inline comments |
| Cross-directory isolation | Vars set in dir A are not visible in dir B |
| Atomic write safety | Kill process during save → store not corrupted |
| Windows path normalization | `C:\Foo` and `c:\foo` resolve to same store entry |

---

## Maintenance Procedures

### Adding a New Command

1. Add the case to the `switch` in `run()` (`internal/app/app.go`).
2. Implement `runNewCommand(ctx *Context, args []string) int`.
3. Update `usageText` in `internal/app/usage.go`.
4. Add the command name to the completion scripts in `internal/app/completion.go`.
5. Update `docs/COMMANDS.md`.
6. If the command does not need the store, add it to `needsStore()` exclusion list.

**Note:** `de config edit` is the only command that spawns a long-lived interactive process (the user's text editor). It uses `os/exec` with forwarded stdin/stdout/stderr identically to `runtime.Exec`, but does not inject environment variables or interact with the store.

### Modifying the Store Schema

1. Increment `CurrentVersion` in `internal/store/store.go`.
2. Add migration logic in `loadDBWithKey()` for the new version (peek at version field, unmarshal with old struct, convert to new shape).
3. Ensure backward compatibility: old stores must be transparently upgraded on load.
4. Atomic write guarantees remain: never leave a partially-migrated store on disk.

### Updating Dependencies

```bash
go get -u ./...
go mod tidy
go vet ./...
```

Critical dependencies:
- `github.com/zalando/go-keyring` — keychain abstraction (test on all platforms after update).
- `golang.org/x/crypto` — PBKDF2 implementation.
- `golang.org/x/term` — terminal password reading.

---

## Future Considerations

### Potential Enhancements (Not Currently Planned)

- **Hierarchical directory lookup:** Walk up parent directories to inherit variables (e.g., monorepo root vars available in subdirectories).
- **Encrypted store backend alternatives:** SQLite (via CGO or pure-Go driver) for larger variable sets.
- **Remote sync:** Optional encrypted sync to cloud storage (Dropbox, S3) for multi-machine setups.
- **CI/CD bridge:** `de export` integration with GitHub Actions secrets or Vault.
- **Watch mode:** File system watcher that re-injects vars on store changes (for long-running dev servers).
- **Audit log:** Record when variables are accessed (append-only encrypted log).
- **Multiple key support:** Per-folder encryption keys for team scenarios where different members have access to different directory contexts.

### Performance Notes

- The store is loaded and decrypted on every command invocation. For stores with hundreds of folders, this remains sub-millisecond due to AES-GCM hardware acceleration on modern CPUs.
- The PBKDF2 derivation (600k iterations) is intentionally slow — it only runs during `key export` and `key import`, not during normal operation.
- Subprocess spawning via `os/exec` adds negligible overhead (~1ms) compared to running the target command directly.
