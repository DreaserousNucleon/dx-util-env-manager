package paths

import (
	"path/filepath"
	"runtime"
	"strings"
)

func CanonicalDir(dir string) (string, error) {
	if dir == "" {
		dir = "."
	}

	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	abs = filepath.Clean(abs)

	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		abs = resolved
	}

	if runtime.GOOS == "windows" {
		abs = strings.ToLower(abs)
	}

	return abs, nil
}
