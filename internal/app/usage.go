package app

import (
	"fmt"
	"io"
)

const usageText = `de - directory-aware env runner

Usage:
  de [--verbose] [--json] [--profile NAME] <command> [args...]
  de exec -- <command> [args...]
  de set KEY=VALUE [--dir PATH]
  de get KEY [--dir PATH] [--reveal]
  de unset KEY [--dir PATH]
  de list [--dir PATH] [--reveal]
  de export [--dir PATH] [--format dotenv|json|k8s-secret|k8s-configmap|compose|docker-env] [--reveal]
         [--name NAME] [--namespace NS]
  de init [--dir PATH]
  de migrate [--file PATH] [--dir PATH]
  de key rotate
  de key export --file PATH
  de key import --file PATH
  de key purge --confirm
  de profile
  de profile list
  de profile set <name>
  de doctor
  de completion <bash|zsh|fish|powershell>
  de version
  de help

Global flags:
  --verbose   Print resolved directory, store path, and variable count to stderr.
  --json      Emit machine-readable JSON output where supported.
  --profile   Override the active profile for this invocation.

Profile commands:
  de profile           Print the active profile.
  de profile list      List all allowed profiles (marks active with *).
  de profile set NAME  Set the default profile in config.toml.

  Profiles: dev, staging, pre-prod, qe, prod (default: dev).
  Resolution: --profile flag > DE_PROFILE env var > config.toml > default.

Notes:
  - Unknown commands are treated as exec targets.
  - Store data is encrypted (AES-GCM); master key lives in the OS keychain.
  - Folder paths are stored as HMAC-SHA256 hashes; no plaintext paths on disk.
  - Values are redacted by default; use --reveal or DE_REVEAL=1 to show plaintext.
`

func printUsage(w io.Writer) {
	fmt.Fprint(w, usageText)
}
