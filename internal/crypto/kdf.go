package crypto

import (
	"crypto/sha256"

	"golang.org/x/crypto/pbkdf2"
)

const (
	pbkdf2Iterations = 600_000
	pbkdf2KeyLen     = 32
)

func deriveKey(passphrase, salt []byte) []byte {
	return pbkdf2.Key(passphrase, salt, pbkdf2Iterations, pbkdf2KeyLen, sha256.New)
}
