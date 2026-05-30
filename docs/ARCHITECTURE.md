# Architecture

## Design Goals

- **Zero-file workspaces:** No secret files (`.env`, credentials) ever exist inside a project directory.
- **Directory-aware lookup:** Each directory maps to an isolated set of encrypted environment variables.
- **Ephemeral subprocess injection:** Secrets exist only in the spawned process's memory and are reclaimed on exit.
- **Cross-platform parity:** Identical behavior on Windows, Linux, and macOS from a single compiled binary.
- **Defense against AI indexers:** Centralized encrypted storage prevents local LLM tooling and rogue extensions from scraping credentials.

## Components

| Package | Responsibility |
|---------|---------------|
| `cmd/de` | CLI entrypoint; delegates to `internal/app.Run()` |
| `internal/app` | Command routing, flag parsing, output formatting, shell completions |
| `internal/config` | Config resolution: OS defaults → environment variables → `config.toml` |
| `internal/crypto` | AES-GCM encryption, OS keychain CRUD, HMAC folder IDs, PBKDF2 key wrapping |
| `internal/store` | `EnvStore` interface and encrypted JSON store implementation |
| `internal/env` | Hardened `.env` parser, environment merging, key validation |
| `internal/export` | Container format emitters (K8s, Compose, Docker flags) |
| `internal/paths` | Path canonicalization: absolute, symlinks, case folding |
| `internal/runtime` | Subprocess execution with full I/O forwarding |
| `internal/version` | Build metadata (version, commit, date via ldflags) |

## Data Model

The encrypted store (v3) uses HMAC-SHA256 hashes as folder identifiers with a profile dimension:

```json
{
  "version": 3,
  "folders": {
    "<hmac-sha256-hex-64-chars>": {
      "dev": {
        "API_KEY": "dev-secret",
        "DB_URL": "postgres://localhost/mydb"
      },
      "staging": {
        "API_KEY": "staging-secret"
      }
    }
  }
}
```

Each folder entry maps profile names to variable sets. Allowed profiles: `dev`, `staging`, `pre-prod`, `qe`, `prod`.

On disk, this JSON is encrypted with AES-256-GCM. The file is a raw ciphertext blob:

```
[12-byte nonce][GCM ciphertext + authentication tag]
```

The master key (32 bytes) is stored in the OS keychain, never on the filesystem.

## Execution Flow

1. Load configuration (defaults → env overrides → config.toml).
2. Parse global flags (`--verbose`, `--json`, `--profile`) and route to the target command.
3. Resolve the active profile (`--profile` flag > `DE_PROFILE` env var > config.toml > default `dev`).
4. Resolve the directory context via `paths.CanonicalDir()` (absolute, clean, symlinks resolved, case-folded on Windows).
5. Retrieve master key from OS keychain (or generate on first use).
6. Decrypt the store file with AES-GCM.
7. Compute `HMAC-SHA256(canonical_path, master_key)` to find the folder entry.
8. Select the active profile's variable set from the folder entry.
9. Merge stored variables with the current process environment.
10. Execute the target command with the merged environment via `os/exec`.
11. Forward the subprocess exit code to the caller.

**Exception:** Commands that do not interact with the store (`config`, `profile`, `completion`, `version`, `help`) skip steps 4–9. Notably, `de config edit` spawns an external editor process with forwarded stdin/stdout/stderr — no keychain or store access occurs.

## Security Architecture

| Layer | Mechanism |
|-------|-----------|
| At-rest encryption | AES-256-GCM; nonce generated per write from `crypto/rand` |
| Key storage | OS keychain (Secret Service / wincred / macOS Keychain) |
| Key derivation (backup) | PBKDF2-SHA256, 600,000 iterations, 16-byte random salt |
| Folder privacy | HMAC-SHA256 keyed hashes; no plaintext paths on disk |
| Atomic persistence | tmp-file + remove + rename; prevents partial writes |
| File permissions | Store written with mode `0600`; config dir with `0700` |
| Passphrase input | `term.ReadPassword()`; never echoed or logged |
| Output redaction | All values masked (`****`) unless explicitly revealed |

## Store Versioning

- **v1 (removed):** Stored plaintext directory paths as JSON keys. Rejected with an explicit error directing users to migrate via v1.0.0 first.
- **v2 (legacy):** Folder keys are HMAC-SHA256 hex digests; folder values are flat `map[string]string`. Transparently migrated to v3 on first load (existing vars wrapped into the `dev` profile).
- **v3 (current):** Folder values gain a profile dimension: `map[profile]map[key]value`. Introduced in v1.1.0.
- **Future versions:** Increment `CurrentVersion` in `store.go` and add a version check in `loadDBWithKey()`.

## Cross-Platform Behavior

| Concern | Linux/macOS | Windows |
|---------|-------------|---------|
| Key storage | D-Bus Secret Service / Keychain | Windows Credential Manager |
| Path case | Preserved as-is | Lowercased (`strings.ToLower`) |
| Env key matching | Case-sensitive | Case-insensitive (`strings.ToUpper`) |
| Atomic rename | `os.Rename` overwrites target | `os.Remove` then `os.Rename` |
| Config dir | `~/.config/de/` | `%APPDATA%\de\` |
