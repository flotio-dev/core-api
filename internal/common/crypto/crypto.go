// Package crypto provides symmetric encryption for secrets stored at rest.
//
// Secrets are encrypted with AES-256-GCM using a key read from the
// SECRETS_ENCRYPTION_KEY environment variable (base64-encoded 32 bytes).
// Ciphertext is prefixed with "enc:v1:" so that:
//   - Decrypt can transparently pass through legacy plaintext values
//     (those without the prefix), keeping the system working during migration.
//   - Re-encrypting an already-encrypted value can be avoided (idempotent migration).
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
)

const (
	// envKey is the environment variable holding the base64-encoded 32-byte key.
	envKey = "SECRETS_ENCRYPTION_KEY"
	// prefix marks a value as encrypted by this package.
	prefix = "enc:v1:"
)

// ErrNoKey is returned when the encryption key is missing or invalid.
var ErrNoKey = errors.New("crypto: missing or invalid SECRETS_ENCRYPTION_KEY (expected base64-encoded 32 bytes)")

var (
	gcmOnce sync.Once
	gcm     cipher.AEAD
	gcmErr  error
)

// loadGCM builds the AES-256-GCM cipher from the environment key, once.
func loadGCM() (cipher.AEAD, error) {
	gcmOnce.Do(func() {
		raw := os.Getenv(envKey)
		if raw == "" {
			gcmErr = ErrNoKey
			return
		}
		key, err := base64.StdEncoding.DecodeString(raw)
		if err != nil || len(key) != 32 {
			gcmErr = ErrNoKey
			return
		}
		block, err := aes.NewCipher(key)
		if err != nil {
			gcmErr = fmt.Errorf("crypto: %w", err)
			return
		}
		gcm, gcmErr = cipher.NewGCM(block)
	})
	return gcm, gcmErr
}

// Init validates that the encryption key is present and usable.
// Call it at startup to fail closed if the key is missing.
func Init() error {
	_, err := loadGCM()
	return err
}

// Encrypt returns the AES-256-GCM ciphertext of plaintext, prefixed with "enc:v1:".
// An empty input is returned unchanged (nothing to protect).
func Encrypt(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	aead, err := loadGCM()
	if err != nil {
		return "", err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("crypto: nonce: %w", err)
	}
	sealed := aead.Seal(nonce, nonce, []byte(plaintext), nil)
	return prefix + base64.StdEncoding.EncodeToString(sealed), nil
}

// Decrypt reverses Encrypt. Values without the "enc:v1:" prefix are assumed to
// be legacy plaintext and returned as-is, so reads keep working before migration.
func Decrypt(value string) (string, error) {
	if !strings.HasPrefix(value, prefix) {
		return value, nil
	}
	aead, err := loadGCM()
	if err != nil {
		return "", err
	}
	sealed, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(value, prefix))
	if err != nil {
		return "", fmt.Errorf("crypto: decode: %w", err)
	}
	if len(sealed) < aead.NonceSize() {
		return "", errors.New("crypto: ciphertext too short")
	}
	nonce, ciphertext := sealed[:aead.NonceSize()], sealed[aead.NonceSize():]
	plaintext, err := aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("crypto: open: %w", err)
	}
	return string(plaintext), nil
}

// IsEncrypted reports whether value was produced by Encrypt.
func IsEncrypted(value string) bool {
	return strings.HasPrefix(value, prefix)
}
