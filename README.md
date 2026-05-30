# de — Directory-Aware Environment Runner

`de` is a production-ready, local-first CLI that injects per-directory encrypted environment variables into subprocesses at runtime. It achieves **zero-file workspaces** — no `.env`, no `appsettings.json`, no secrets anywhere inside your project tree — by storing all sensitive data in a centralized, AES-GCM encrypted store with the master key locked inside the OS keychain.

Secrets are injected ephemerally: they exist only in the spawned process's memory and vanish when it terminates. No ambient shell pollution. No files for AI code assistants to index.

---

## Table of Contents

- [Installation](#installation)
- [Quick Start](#quick-start)
- [Configuration](#configuration)
- [CLI Reference](#cli-reference)
- [Container Export](#container-export)
- [Shell Completions](#shell-completions)
- [Key Management](#key-management)
- [Storage & Encryption](#storage--encryption)
- [Environment Variables](#environment-variables)
- [Uninstallation](#uninstallation)

---

## Installation

### From Source (Go 1.25+)

```bash
git clone <repository-url>
cd dx-util-env-manager
go build -o de ./cmd/de
```

Place the resulting binary on your `PATH`:

**Linux / macOS:**
```bash
sudo mv de /usr/local/bin/
```

**Windows (PowerShell, run as Administrator):**
```powershell
Move-Item .\de.exe C:\Windows\System32\
```

### Via `go install`

```bash
go install dx-util-env-manager/cmd/de@latest
```

### Release Binaries

Pre-built binaries are available for:

| OS | Architecture |
|----|-------------|
| Linux | amd64, arm64 |
| Windows | amd64 |
| macOS | amd64 (Intel), arm64 (Apple Silicon) |

Download the appropriate archive from the Releases page, extract, and place `de` (or `de.exe`) on your `PATH`.

---

## Quick Start

```bash
# Initialize the store for your current project directory
de init

# Add secrets
de set API_KEY=sk-abc123
de set DB_URL=postgres://localhost/mydb

# Run any command with those secrets injected
de npm run dev
de -- python main.py
de exec -- go run .

# View stored keys (values redacted by default)
de list

# Export for Docker workflows
de export --format compose
```

On first invocation with no existing store, `de` prints a concise onboarding guide and suggests `de init`.

---

## Configuration

### Config File

Optional. Location: `~/.config/de/config.toml` (Linux/macOS) or `%APPDATA%\de\config.toml` (Windows).

```toml
# Show values in plaintext by default (default: false)
reveal = false

# Color output: "auto", "always", "never" (default: "auto")
color = "auto"

# Override the store file path
# store_path = "/custom/path/store.json"

# Allow automatic store re-initialization when decryption fails after a key purge.
# When true: if no master key exists and the store cannot be decrypted, the old
# store is removed and a fresh one is created automatically.
# When false (default): decryption failures produce an error with remediation hints.
# allow_reinit_on_decrypt_fail = true
```

### Environment Variable Overrides

| Variable | Effect |
|----------|--------|
| `DE_STORE_PATH` | Absolute path to the encrypted store file |
| `DE_CONFIG_DIR` | Config directory; store becomes `store.json` inside it |
| `DE_REVEAL` | Set to `1` to show plaintext values in output |

Environment variables take precedence over the config file.

---

## CLI Reference

```
de [--verbose] [--json] <command> [args...]
```

### Global Flags

| Flag | Description |
|------|-------------|
| `--verbose` | Print resolved directory, store path, and variable count to stderr |
| `--json` | Emit machine-readable JSON output where supported |

### Commands

#### `exec` — Run a command with injected environment

```bash
de <command> [args...]        # implicit exec
de exec -- <command> [args...]  # explicit exec
```

Resolves the current directory against the store, merges the stored variables with the process environment, and executes the command. Exit code is forwarded.

Unknown commands are treated as exec targets — `de npm test` is equivalent to `de exec -- npm test`.

#### `set` — Store a variable

```bash
de set KEY=VALUE [--dir PATH]
```

Writes or overwrites a single variable for the given directory context. Keys must be non-empty, valid UTF-8, and must not contain `=`.

#### `get` — Retrieve a variable

```bash
de get KEY [--dir PATH] [--reveal]
```

Prints a single variable. Value is redacted (`****`) unless `--reveal` is passed or `DE_REVEAL=1` is set.

#### `unset` — Remove a variable

```bash
de unset KEY [--dir PATH]
```

Removes the specified key from the directory's variable set.

#### `list` — List all variables

```bash
de list [--dir PATH] [--reveal]
```

Lists all keys for the directory context. Values are redacted by default.

#### `init` — Initialize a directory

```bash
de init [--dir PATH]
```

Creates an empty entry in the encrypted store for the directory. Generates a master key and stores it in the OS keychain if one does not already exist.

#### `migrate` — Import a `.env` file

```bash
de migrate [--file PATH] [--dir PATH]
```

Parses a `.env` file and imports all key-value pairs into the store for the specified directory. The parser supports:

- Single-quoted values (literal, no escape processing)
- Double-quoted values (with `\"`, `\\`, `\n`, `\r`, `\t` escapes)
- Multiline values within quotes
- Inline comments (`# comment` after unquoted values)
- `export` prefix stripping
- Blank lines and `#` comment lines

Default file is `.env` in the current directory.

#### `export` — Emit variables in various formats

```bash
de export [--dir PATH] [--format FORMAT] [--reveal] [--name NAME] [--namespace NS]
```

Formats: `dotenv` (default), `json`, `k8s-secret`, `k8s-configmap`, `compose`, `docker-env`.

Output goes to stdout only. Pipe to a file if persistence is needed.

#### `doctor` — Store integrity check

```bash
de doctor
```

Verifies the store can be decrypted, validates folder ID lengths (HMAC-SHA256 = 64 hex characters), and reports total folder/variable counts and any errors.

#### `key` — Master key operations

```bash
de key rotate                  # Generate new key, re-encrypt store
de key export --file PATH      # Passphrase-protected key backup
de key import --file PATH      # Restore key from backup
de key purge --confirm         # Permanently remove key from keychain
```

#### `completion` — Shell completion scripts

```bash
de completion <bash|zsh|fish|powershell>
```

Outputs a completion script for the specified shell. See [Shell Completions](#shell-completions) below.

#### `version`

```bash
de version
```

Prints version, commit hash, and build date.

---

## Container Export

All container export commands write to stdout and never persist files to disk.

### Kubernetes Secret

```bash
de export --format k8s-secret --name my-app-secrets --namespace production > secret.yaml
```

Produces a valid `apiVersion: v1` / `kind: Secret` manifest with base64-encoded values.

### Kubernetes ConfigMap

```bash
de export --format k8s-configmap --name my-app-config > configmap.yaml
```

### Docker Compose

```bash
de export --format compose
```

Output:
```yaml
environment:
  - API_KEY=sk-abc123
  - DB_URL=postgres://localhost/mydb
```

### Docker Run Flags

```bash
docker run $(de export --format docker-env | tr '\n' ' ') my-image
```

Output:
```
--env API_KEY=sk-abc123
--env DB_URL='postgres://localhost/mydb'
```

Values containing shell metacharacters are automatically single-quote escaped.

---

## Shell Completions

### Bash

```bash
de completion bash >> ~/.bashrc
source ~/.bashrc
```

### Zsh

```bash
de completion zsh >> ~/.zshrc
source ~/.zshrc
```

### Fish

```bash
de completion fish > ~/.config/fish/completions/de.fish
```

### PowerShell

```powershell
de completion powershell >> $PROFILE
. $PROFILE
```

---

## Key Management

The master key is a 32-byte (256-bit) random key stored in:

- **Linux:** Secret Service API (GNOME Keyring / KWallet)
- **Windows:** Windows Credential Manager (via wincred)
- **macOS:** macOS Keychain

Service name: `de-cli-master-key`, account: `master`.

### Backup and Recovery

```bash
# Export: prompts for a passphrase (PBKDF2, 600,000 iterations)
de key export --file ~/de-backup.key

# Import: prompts for the same passphrase
de key import --file ~/de-backup.key
```

The export file format is: 16-byte random salt + AES-GCM ciphertext of the master key, wrapped with a key derived from PBKDF2-SHA256(passphrase, salt).

### Key Rotation

```bash
de key rotate
```

Generates a new 32-byte master key, re-encrypts the entire store, and atomically updates the keychain.

### Key Purge (Uninstall)

```bash
de key purge --confirm
```

Permanently removes the master key from the OS keychain. The `--confirm` flag is required as a safety gate. After purging, the store file becomes unreadable unless you have a backup.

---

## Storage & Encryption

### Store Location

| OS | Default Path |
|----|-------------|
| Linux | `~/.config/de/store.json` |
| macOS | `~/Library/Application Support/de/store.json` |
| Windows | `%APPDATA%\de\store.json` |

The file on disk is a raw AES-GCM ciphertext blob (not JSON). When decrypted, the internal structure is:

```json
{
  "version": 2,
  "folders": {
    "<hmac-sha256-hex>": {
      "KEY": "value"
    }
  }
}
```

### Security Properties

- **Encryption:** AES-256-GCM with a random 12-byte nonce prepended to each ciphertext.
- **Folder Privacy:** Directory paths are never stored in plaintext. Each path is represented as an HMAC-SHA256 hash (keyed with the master key), so the store reveals no information about which directories are tracked.
- **Atomic Writes:** All mutations use a tmp-file + rename strategy to prevent corruption on crash or power loss.
- **File Permissions:** Store and backup files are created with mode `0600` (owner read/write only).
- **Ephemeral Injection:** Variables exist only in the subprocess memory — they never pollute the parent shell or linger after process termination.

### Data Store Versioning

- **v1:** Stored plaintext directory paths as keys. Transparently migrated to v2 on first load.
- **v2 (current):** Stores HMAC-SHA256 hashes of canonical directory paths.

---

## Environment Variables

| Variable | Purpose |
|----------|---------|
| `DE_STORE_PATH` | Override the store file location |
| `DE_CONFIG_DIR` | Override the config directory (store becomes `store.json` inside) |
| `DE_REVEAL` | Set to `1` to show plaintext values in output by default |

---

## Uninstallation

1. Remove the binary from your `PATH`.
2. Remove the config directory:
   - Linux/macOS: `rm -rf ~/.config/de`
   - Windows: `Remove-Item -Recurse "$env:APPDATA\de"`
3. Remove the keychain entry:
   ```bash
   de key purge --confirm
   ```

---

## Documentation

- [docs/PURPOSE.md](docs/PURPOSE.md) — Problem statement and architectural rationale
- [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) — System design and component overview
- [docs/COMMANDS.md](docs/COMMANDS.md) — Complete command specification
- [docs/ROADMAP.md](docs/ROADMAP.md) — Development phases and completion status
