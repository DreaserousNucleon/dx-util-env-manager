# Commands

Complete reference for all `de` commands. All commands default to the current working directory unless `--dir PATH` is specified.

## Global Flags

| Flag | Description |
|------|-------------|
| `--verbose` | Print resolved directory, store path, and variable count to stderr |
| `--json` | Emit machine-readable JSON output where supported |

---

## exec (default)

Run a command with environment variables injected from the store.

```
de <command> [args...]
de exec -- <command> [args...]
```

Unknown subcommands are treated as exec targets. The `--` separator is optional with explicit `exec`.

**Behavior:**
- Resolves the canonical CWD against the store.
- Merges stored variables into the current process environment (store values override existing keys).
- Spawns the command with stdin/stdout/stderr forwarded.
- Returns the subprocess exit code.

---

## init

Create an empty directory entry in the store.

```
de init [--dir PATH]
```

If no master key exists in the OS keychain, one is generated automatically (32 bytes from `crypto/rand`) and stored in the keychain.

---

## set

Write or overwrite a single variable.

```
de set KEY=VALUE [--dir PATH]
```

**Validation:**
- Key must be non-empty, valid UTF-8, and must not contain `=`.
- Value must be valid UTF-8.

---

## get

Retrieve and print a single variable.

```
de get KEY [--dir PATH] [--reveal]
```

Value is redacted (`****`) by default. Use `--reveal` or set `DE_REVEAL=1` to show plaintext.

---

## unset

Remove a single variable from a directory context.

```
de unset KEY [--dir PATH]
```

---

## list

List all variable keys for a directory context.

```
de list [--dir PATH] [--reveal]
```

Values are redacted by default. Output is sorted alphabetically by key. If no variables exist, prints `no variables set`.

---

## migrate

Import a `.env` file into the store.

```
de migrate [--file PATH] [--dir PATH]
```

Default file: `.env` in the current directory.

**Parser capabilities:**
- Single-quoted values (literal, no escape processing)
- Double-quoted values with escape sequences: `\"`, `\\`, `\n`, `\r`, `\t`
- Multiline values within quotes
- Inline comments (`# comment` after unquoted values)
- `export` prefix stripping
- Blank lines and `#` full-line comments

---

## export

Emit variables to stdout in various formats.

```
de export [--dir PATH] [--format FORMAT] [--reveal] [--name NAME] [--namespace NS]
```

| Format | Output |
|--------|--------|
| `dotenv` (default) | `KEY=value` lines |
| `json` | JSON object with 2-space indent |
| `k8s-secret` | Kubernetes Secret manifest (YAML, base64 values) |
| `k8s-configmap` | Kubernetes ConfigMap manifest (YAML) |
| `compose` | Docker Compose `environment:` block |
| `docker-env` | `--env KEY=value` flags for `docker run` |

`--name` and `--namespace` apply to Kubernetes formats only (default name: `app-env`).

For `dotenv` and `json` formats, values are redacted unless `--reveal` is passed. Container formats (`k8s-secret`, `k8s-configmap`, `compose`, `docker-env`) always include plaintext values since they are the payload.

---

## key

Master key management subcommands.

### key rotate

```
de key rotate
```

Generates a new 32-byte master key, re-encrypts the entire store, and updates the keychain atomically.

### key export

```
de key export --file PATH
```

Prompts for a passphrase (never echoed) and writes a PBKDF2-wrapped backup of the master key to the specified file (mode 0600).

### key import

```
de key import --file PATH
```

Prompts for the passphrase used during export, unwraps the master key, and stores it in the OS keychain.

### key purge

```
de key purge --confirm
```

Permanently removes the master key from the OS keychain. The `--confirm` flag is mandatory. After purging, the store becomes unreadable without a backup.

---

## doctor

Verify store integrity.

```
de doctor
```

Checks:
- Keychain access (master key retrievable).
- Store decryption succeeds.
- All folder IDs are valid 64-character hex strings (HMAC-SHA256).
- Reports folder count, total variable count, and any errors.

---

## completion

Output shell completion scripts.

```
de completion <bash|zsh|fish|powershell>
```

Prints the completion script to stdout. Pipe or append to your shell's config file.

---

## version

Print version metadata.

```
de version
```

Output format: `de <version> (commit <hash>, built <date>)`

---

## help

Print usage information.

```
de help
de -h
de --help
```