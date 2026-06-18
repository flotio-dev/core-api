package googleplay

import (
	"encoding/base64"
	"testing"
)

func TestValidateServiceAccountJSON(t *testing.T) {
	valid := `{"type":"service_account","project_id":"p","client_email":"a@b.iam.gserviceaccount.com","private_key":"-----BEGIN PRIVATE KEY-----"}`

	cases := []struct {
		name    string
		raw     string
		wantErr bool
	}{
		{"valid", valid, false},
		{"wrong type", `{"type":"user","client_email":"a","private_key":"k"}`, true},
		{"missing private key", `{"type":"service_account","client_email":"a"}`, true},
		{"missing client email", `{"type":"service_account","private_key":"k"}`, true},
		{"not json", `not-json`, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateServiceAccountJSON([]byte(tc.raw))
			if tc.wantErr != (err != nil) {
				t.Fatalf("wantErr=%v got err=%v", tc.wantErr, err)
			}
		})
	}
}

func TestDecodeServiceAccount(t *testing.T) {
	json := `{"type":"service_account"}`

	// base64-encoded JSON decodes back to JSON.
	if got := string(DecodeServiceAccount(base64.StdEncoding.EncodeToString([]byte(json)))); got != json {
		t.Fatalf("base64 path: got %q want %q", got, json)
	}
	// raw JSON (not base64) passes through.
	if got := string(DecodeServiceAccount(json)); got != json {
		t.Fatalf("raw path: got %q want %q", got, json)
	}
}
