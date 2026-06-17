package crypto

import (
	"encoding/base64"
	"os"
	"sync"
	"testing"
)

// resetGCM lets tests re-evaluate the env key (loadGCM is otherwise once-only).
func resetGCM() {
	gcmOnce = sync.Once{}
	gcm = nil
	gcmErr = nil
}

func setKey(t *testing.T) {
	t.Helper()
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	os.Setenv(envKey, base64.StdEncoding.EncodeToString(key))
	resetGCM()
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	setKey(t)

	secret := "p@ssw0rd-très-secret"
	enc, err := Encrypt(secret)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if !IsEncrypted(enc) {
		t.Fatalf("expected encrypted prefix, got %q", enc)
	}
	if enc == secret {
		t.Fatal("ciphertext equals plaintext")
	}

	dec, err := Decrypt(enc)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if dec != secret {
		t.Fatalf("round trip mismatch: got %q want %q", dec, secret)
	}
}

func TestDecryptLegacyPlaintextPassThrough(t *testing.T) {
	setKey(t)

	legacy := "not-encrypted-yet"
	dec, err := Decrypt(legacy)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if dec != legacy {
		t.Fatalf("expected pass-through, got %q", dec)
	}
}

func TestEncryptEmptyIsNoop(t *testing.T) {
	setKey(t)

	enc, err := Encrypt("")
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if enc != "" {
		t.Fatalf("expected empty, got %q", enc)
	}
}

func TestNonceIsRandom(t *testing.T) {
	setKey(t)

	a, _ := Encrypt("same")
	b, _ := Encrypt("same")
	if a == b {
		t.Fatal("expected different ciphertext for same plaintext (random nonce)")
	}
}

func TestMissingKeyFailsClosed(t *testing.T) {
	os.Unsetenv(envKey)
	resetGCM()

	if err := Init(); err != ErrNoKey {
		t.Fatalf("expected ErrNoKey, got %v", err)
	}
	if _, err := Encrypt("x"); err != ErrNoKey {
		t.Fatalf("expected ErrNoKey from Encrypt, got %v", err)
	}
}
