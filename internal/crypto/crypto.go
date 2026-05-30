package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"

	"github.com/zalando/go-keyring"
)

const (
	keyringService = "de-cli-master-key"
	keyringUser    = "master"
	KeyLength      = 32
)

// GetOrGenerateKey returns the master key, creating one if absent.
func GetOrGenerateKey() ([]byte, error) {
	key, err := GetKey()
	if err == nil {
		return key, nil
	}
	if !errors.Is(err, ErrKeyNotFound) {
		return nil, err
	}
	return generateAndStoreKey()
}

// ErrKeyNotFound is returned when the keychain has no master key.
var ErrKeyNotFound = errors.New("master key not found in keychain")

// GetKey returns the master key or ErrKeyNotFound.
func GetKey() ([]byte, error) {
	stored, err := keyring.Get(keyringService, keyringUser)
	if err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			return nil, ErrKeyNotFound
		}
		return nil, fmt.Errorf("keyring get: %w", err)
	}
	return decodeKey(stored)
}

// SetKey stores a raw key in the keychain.
func SetKey(key []byte) error {
	if len(key) != KeyLength {
		return fmt.Errorf("invalid key length %d", len(key))
	}
	encoded := base64.StdEncoding.EncodeToString(key)
	if err := keyring.Set(keyringService, keyringUser, encoded); err != nil {
		return fmt.Errorf("keyring set: %w", err)
	}
	return nil
}

// DeleteKey removes the master key from the keychain.
func DeleteKey() error {
	err := keyring.Delete(keyringService, keyringUser)
	if err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return fmt.Errorf("keyring delete: %w", err)
	}
	return nil
}

func generateAndStoreKey() ([]byte, error) {
	key := make([]byte, KeyLength)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, fmt.Errorf("generate key: %w", err)
	}
	if err := SetKey(key); err != nil {
		return nil, err
	}
	return key, nil
}

// FolderID produces an HMAC-SHA256 hex digest of dir keyed by the master key.
func FolderID(dir string, key []byte) string {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(dir))
	return fmt.Sprintf("%x", mac.Sum(nil))
}

// --- AES-GCM encrypt / decrypt ---

func Encrypt(plaintext []byte, key []byte) ([]byte, error) {
	if len(key) != KeyLength {
		return nil, fmt.Errorf("invalid key length %d", len(key))
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("cipher init: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("gcm init: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("nonce: %w", err)
	}

	return gcm.Seal(nonce, nonce, plaintext, nil), nil
}

func Decrypt(ciphertext []byte, key []byte) ([]byte, error) {
	if len(key) != KeyLength {
		return nil, fmt.Errorf("invalid key length %d", len(key))
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("cipher init: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("gcm init: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}

	plaintext, err := gcm.Open(nil, ciphertext[:nonceSize], ciphertext[nonceSize:], nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt: %w", err)
	}

	return plaintext, nil
}

// --- Key export / import (PBKDF2 + AES-GCM wrap) ---

// ExportKey encrypts the master key with a passphrase and returns the blob.
// Format: 16-byte salt || AES-GCM(deriveKey(passphrase, salt), masterKey)
func ExportKey(masterKey []byte, passphrase []byte) ([]byte, error) {
	salt := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, fmt.Errorf("salt: %w", err)
	}

	wrapKey := deriveKey(passphrase, salt)
	wrapped, err := Encrypt(masterKey, wrapKey)
	if err != nil {
		return nil, fmt.Errorf("wrap key: %w", err)
	}

	return append(salt, wrapped...), nil
}

// ImportKey decrypts a master key from an export blob using the passphrase.
func ImportKey(blob []byte, passphrase []byte) ([]byte, error) {
	if len(blob) < 16 {
		return nil, fmt.Errorf("backup file too short")
	}
	salt := blob[:16]
	wrapKey := deriveKey(passphrase, salt)

	masterKey, err := Decrypt(blob[16:], wrapKey)
	if err != nil {
		return nil, fmt.Errorf("unwrap key (wrong passphrase?): %w", err)
	}
	if len(masterKey) != KeyLength {
		return nil, fmt.Errorf("recovered key has invalid length %d", len(masterKey))
	}
	return masterKey, nil
}

func decodeKey(encoded string) ([]byte, error) {
	if encoded == "" {
		return nil, fmt.Errorf("keyring returned empty key")
	}
	key, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("decode key: %w", err)
	}
	if len(key) != KeyLength {
		return nil, fmt.Errorf("invalid key length %d", len(key))
	}
	return key, nil
}
