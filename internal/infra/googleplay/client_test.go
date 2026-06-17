package googleplay

import (
	"encoding/base64"
	"os"
	"testing"

	"github.com/flotio-dev/core-api/internal/common/crypto"
)

func setKey(t *testing.T) {
	t.Helper()
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	os.Setenv("SECRETS_ENCRYPTION_KEY", base64.StdEncoding.EncodeToString(key))
}

func TestDecodeCredentialsBase64JSON(t *testing.T) {
	setKey(t)

	saJSON := `{"type":"service_account","project_id":"x"}`
	stored, err := crypto.Encrypt(base64.StdEncoding.EncodeToString([]byte(saJSON)))
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	got, err := decodeCredentials(stored)
	if err != nil {
		t.Fatalf("decodeCredentials: %v", err)
	}
	if string(got) != saJSON {
		t.Fatalf("got %q want %q", got, saJSON)
	}
}

func TestDecodeCredentialsRawJSONFallback(t *testing.T) {
	setKey(t)

	saJSON := `{"type":"service_account"}`
	stored, err := crypto.Encrypt(saJSON) // not base64-encoded
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	got, err := decodeCredentials(stored)
	if err != nil {
		t.Fatalf("decodeCredentials: %v", err)
	}
	if string(got) != saJSON {
		t.Fatalf("got %q want %q", got, saJSON)
	}
}

func TestDecodeCredentialsEmpty(t *testing.T) {
	setKey(t)

	if _, err := decodeCredentials(""); err == nil {
		t.Fatal("expected error for empty credentials")
	}
}
