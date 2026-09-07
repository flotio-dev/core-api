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
	if _, err := Decrypt("enc:v1:someciphertext"); err != ErrNoKey {
		t.Fatalf("expected ErrNoKey from Decrypt, got %v", err)
	}
}

func TestInvalidKeyConfigurations(t *testing.T) {
	// Not base64
	os.Setenv(envKey, "not-valid-base64!!!")
	resetGCM()
	if err := Init(); err != ErrNoKey {
		t.Fatalf("expected ErrNoKey for invalid base64, got %v", err)
	}

	// Base64 but wrong length (16 bytes instead of 32)
	shortKey := make([]byte, 16)
	os.Setenv(envKey, base64.StdEncoding.EncodeToString(shortKey))
	resetGCM()
	if err := Init(); err != ErrNoKey {
		t.Fatalf("expected ErrNoKey for 16-byte key, got %v", err)
	}
}

func TestDecryptErrors(t *testing.T) {
	setKey(t)

	// Invalid base64 after prefix
	if _, err := Decrypt("enc:v1:???not-base-64???"); err == nil {
		t.Fatal("expected error for invalid base64")
	}

	// Ciphertext too short (< NonceSize)
	shortCipher := base64.StdEncoding.EncodeToString([]byte("short"))
	if _, err := Decrypt("enc:v1:" + shortCipher); err == nil {
		t.Fatal("expected error for too short ciphertext")
	}

	// Tampered ciphertext (fails authentication)
	enc, err := Encrypt("secret")
	if err != nil {
		t.Fatal(err)
	}
	rawCipher, _ := base64.StdEncoding.DecodeString(enc[len(prefix):])
	rawCipher[len(rawCipher)-1] ^= 0xFF // tamper with last byte
	tampered := prefix + base64.StdEncoding.EncodeToString(rawCipher)
	if _, err := Decrypt(tampered); err == nil {
		t.Fatal("expected error for tampered ciphertext")
	}
}
