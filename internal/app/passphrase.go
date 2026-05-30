package app

import (
	"os"

	"golang.org/x/term"
)

func readPassphrase() ([]byte, error) {
	fd := int(os.Stdin.Fd())
	return term.ReadPassword(fd)
}
