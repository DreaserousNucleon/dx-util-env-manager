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

The encrypted store (v2) uses HMAC-SHA256 hashes as folder identifiers:

```json
{
  "version": 2,
  "folders": {
    "<hmac-sha256-hex-64-chars>": {
      "API_KEY": "secret-value",
      "DB_URL": "postgres://..."
    }
  }
}
```

On disk, this JSON is encrypted with AES-256-GCM. The file is a raw ciphertext blob:

```
[12-byte nonce][GCM ciphertext + authentication tag]
```

The master key (32 bytes) is stored in the OS keychain, never on the filesystem.

## Execution Flow

1. Load configuration (defaults → env overrides → config.toml).
2. Parse global flags (`--verbose`, `--json`) and route to the target command.
3. Resolve the directory context via `paths.CanonicalDir()` (absolute, clean, symlinks resolved, case-folded on Windows).
4. Retrieve master key from OS keychain (or generate on first use).
5. Decrypt the store file with AES-GCM.
6. Compute `HMAC-SHA256(canonical_path, master_key)` to find the folder entry.
7. Merge stored variables with the current process environment.
8. Execute the target command with the merged environment via `os/exec`.
9. Forward the subprocess exit code to the caller.

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

- **v1 (removed):** Stored plaintext directory paths as JSON keys. Transparent migration was available in early releases but has been removed. Stores at v1 are now rejected with an explicit error.
- **v2 (current):** Folder keys are HMAC-SHA256 hex digests keyed with the master key.
- **Future versions:** Increment `CurrentVersion` in `store.go` and add a version check in `loadAndMigrate()`.

## Cross-Platform Behavior

| Concern | Linux/macOS | Windows |
|---------|-------------|---------|
| Key storage | D-Bus Secret Service / Keychain | Windows Credential Manager |
| Path case | Preserved as-is | Lowercased (`strings.ToLower`) |
| Env key matching | Case-sensitive | Case-insensitive (`strings.ToUpper`) |
| Atomic rename | `os.Rename` overwrites target | `os.Remove` then `os.Rename` |
| Config dir | `~/.config/de/` | `%APPDATA%\de\` |
