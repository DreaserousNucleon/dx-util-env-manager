package app

import (
	"fmt"
	"io"
)

const usageText = `de - directory-aware env runner

Usage:
  de [--verbose] [--json] <command> [args...]
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
  de doctor
  de completion <bash|zsh|fish|powershell>
  de version
  de help

Global flags:
  --verbose   Print resolved directory, store path, and variable count to stderr.
  --json      Emit machine-readable JSON output where supported.

Notes:
  - Unknown commands are treated as exec targets.
  - Store data is encrypted (AES-GCM); master key lives in the OS keychain.
  - Folder paths are stored as HMAC-SHA256 hashes; no plaintext paths on disk.
  - Values are redacted by default; use --reveal or DE_REVEAL=1 to show plaintext.
`

func printUsage(w io.Writer) {
	fmt.Fprint(w, usageText)
}
