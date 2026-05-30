# Purpose

## Problem Statement

Modern development workflows introduce significant friction and security risks when managing environment variables and application secrets across multiple codebases, languages, and operating systems. This problem is compounded by two core challenges:

* **The AI Exposure Risk (Primary Conflict):** The rise of codebase AI assistants and workspace indexers makes storing plain-text credentials in localized files (such as `.env`, `appsettings.json`, or `.envrc`) a severe security liability. Rogue extensions, careless git commits, or automated LLM prompt scaffolding can easily expose these local secret files.
* **Environment Disconnect & Friction (Secondary Conflict):** Switching between disparate configuration loading methods when transitioning an application from local development to production container environments introduces unnecessary cognitive overhead and manual engineering overhead.

---

## Solution: The `de` Runtime Wrapper

`de` is a compiled Go binary that acts as a **folder-aware runtime wrapper**. It maps each project directory to an encrypted set of environment variables stored centrally, injecting them into subprocesses at execution time without ever placing secret files in the workspace.

### Why the Wrapper Approach Wins Over Shell Hooks

| Concern | Shell Hook (`direnv`) | Runtime Wrapper (`de`) |
|---------|----------------------|------------------------|
| Files in workspace | Requires `.envrc` in project tree | Zero files — completely decoupled |
| AI/indexer safety | Plain-text files are scrapeable | Encrypted central store; no local files to index |
| Shell compatibility | Per-shell hook scripts required | Single binary; identical on all shells and OSes |
| Secret lifetime | Loaded into ambient shell session | Exists only in subprocess memory; vanishes on exit |
| Windows support | Fragile, often unsupported | Native `os/exec`; first-class Windows citizen |

---

## Delivered Capabilities

The following production-ready features are fully implemented:

### Core Secret Management
- `de set KEY=VALUE` / `de get KEY` / `de unset KEY` / `de list` — Full CRUD for per-directory encrypted variables.
- `de exec` / `de <command>` — Transparent environment injection into any subprocess.
- `de init` — Initialize a directory context in the encrypted store.
- `de migrate` — Import existing `.env` files with a hardened parser (quotes, escapes, multiline).

### Security & Cryptography
- **AES-256-GCM** encryption with per-write random nonces.
- **OS keychain integration** (Linux Secret Service, Windows Credential Manager, macOS Keychain).
- **HMAC-SHA256 folder hashing** — directory paths are never stored in plaintext.
- **Key rotation** — atomic re-encryption of the entire store.
- **Key export/import** — PBKDF2-wrapped (600k iterations) passphrase-protected backups.
- **Atomic writes** — tmp-file + rename prevents corruption on crash.

### Developer Experience
- **Configurable redaction** — values masked by default; `--reveal` or `DE_REVEAL=1` for plaintext.
- **Shell completions** — bash, zsh, fish, PowerShell.
- **Colored output** — ANSI bold keys on TTY; plain text for pipes.
- **JSON mode** — `--json` for machine-consumable output.
- **Verbose mode** — `--verbose` prints diagnostics to stderr without leaking secrets.
- **Config file** — optional `~/.config/de/config.toml` for persistent preferences.
- **First-run wizard** — onboarding message when no store or key exists.
- **Store doctor** — integrity checker for the encrypted store.

### Container Export
- `de export --format k8s-secret` — Kubernetes Secret manifest (YAML).
- `de export --format k8s-configmap` — Kubernetes ConfigMap manifest (YAML).
- `de export --format compose` — Docker Compose `environment:` block.
- `de export --format docker-env` — `docker run --env` flags.

### Distribution
- **Cross-platform binaries** via goreleaser (Linux, Windows, macOS × amd64/arm64).
- **Version injection** via ldflags.
- **Clean uninstall** path: binary + config dir + `de key purge --confirm`.

---

## Competitive Positioning

| Solution | Local-first | Zero-file | Directory-aware | Cross-platform | Free |
|----------|:-----------:|:---------:|:---------------:|:--------------:|:----:|
| **de** | ✓ | ✓ | ✓ | ✓ | ✓ |
| Doppler | ✗ (cloud) | ✓ | ✗ (project-based) | ✓ | ✗ |
| direnv | ✓ | ✗ (.envrc required) | ✓ | ✗ (Unix only) | ✓ |
| envio | ✓ | ✓ | ✗ (manual profiles) | Partial | ✓ |
| 1Password CLI | ✗ (cloud) | ✓ | ✗ | ✓ | ✗ |

`de` occupies a unique position: local-first, zero-file, directory-mapped, and cross-platform with no external service dependencies.