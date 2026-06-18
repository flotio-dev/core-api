package googleplay

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
)

// serviceAccountKey is the subset of a Google service account JSON we validate.
type serviceAccountKey struct {
	Type        string `json:"type"`
	ProjectID   string `json:"project_id"`
	ClientEmail string `json:"client_email"`
	PrivateKey  string `json:"private_key"`
}

// DecodeServiceAccount returns the raw JSON bytes from a stored credentials
// value, accepting either base64-encoded JSON or raw JSON.
func DecodeServiceAccount(value string) []byte {
	if raw, err := base64.StdEncoding.DecodeString(value); err == nil {
		return raw
	}
	return []byte(value)
}

// ValidateServiceAccountJSON checks that raw is a well-formed Google service
// account key, before it is stored.
func ValidateServiceAccountJSON(raw []byte) error {
	var k serviceAccountKey
	if err := json.Unmarshal(raw, &k); err != nil {
		return fmt.Errorf("googleplay: invalid service account JSON: %w", err)
	}
	if k.Type != "service_account" {
		return errors.New(`googleplay: not a service account key (expected "type":"service_account")`)
	}
	if k.ClientEmail == "" || k.PrivateKey == "" {
		return errors.New("googleplay: service account JSON missing client_email or private_key")
	}
	return nil
}
