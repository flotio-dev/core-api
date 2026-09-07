package github

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"os"
	"testing"
)

func generateTestKeyFile(t *testing.T) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey failed: %v", err)
	}
	keyBytes := x509.MarshalPKCS1PrivateKey(key)
	block := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: keyBytes,
	}
	f, err := os.CreateTemp("", "mock_gh_key_*.pem")
	if err != nil {
		t.Fatalf("CreateTemp failed: %v", err)
	}
	defer f.Close()
	if err := pem.Encode(f, block); err != nil {
		t.Fatalf("pem.Encode failed: %v", err)
	}
	return f.Name()
}

func TestGitHubClientManager(t *testing.T) {
	// 1. Nil receiver checks
	var nilManager *GitHubClientManager
	if _, err := nilManager.ClientForInstallation(123); err == nil {
		t.Errorf("expected error for nil manager ClientForInstallation")
	}
	if _, err := nilManager.ClientForApp(); err == nil {
		t.Errorf("expected error for nil manager ClientForApp")
	}

	// 2. Uninitialized appTransport
	emptyManager := &GitHubClientManager{}
	if _, err := emptyManager.ClientForInstallation(123); err == nil {
		t.Errorf("expected error for uninitialized appTransport")
	}
	if _, err := emptyManager.ClientForApp(); err == nil {
		t.Errorf("expected error for uninitialized appTransport")
	}

	// 3. NewGitHubClientManager without GITHUB_APP_PRIVATE_KEY_PATH
	t.Setenv("GITHUB_APP_PRIVATE_KEY_PATH", "")
	mgr, err := NewGitHubClientManager()
	if err != nil || mgr == nil || mgr.appTransport != nil {
		t.Errorf("expected empty manager when private key path is empty, got %v, err: %v", mgr, err)
	}

	// 4. Invalid GITHUB_APP_ID
	t.Setenv("GITHUB_APP_PRIVATE_KEY_PATH", "/non/existent/key.pem")
	t.Setenv("GITHUB_APP_ID", "invalid-id")
	if _, err := NewGitHubClientManager(); err == nil {
		t.Errorf("expected error for invalid app id")
	}

	// 5. File not found
	t.Setenv("GITHUB_APP_ID", "12345")
	if _, err := NewGitHubClientManager(); err == nil {
		t.Errorf("expected error for non-existent key file")
	}

	// 6. Valid setup with generated key file
	keyPath := generateTestKeyFile(t)
	defer os.Remove(keyPath)

	t.Setenv("GITHUB_APP_PRIVATE_KEY_PATH", keyPath)
	t.Setenv("GITHUB_APP_ID", "12345")

	mgr, err = NewGitHubClientManager()
	if err != nil || mgr == nil || mgr.appTransport == nil {
		t.Fatalf("NewGitHubClientManager failed: %v", err)
	}

	clientApp, err := mgr.ClientForApp()
	if err != nil || clientApp == nil {
		t.Errorf("ClientForApp failed: %v", err)
	}

	clientInst, err := mgr.ClientForInstallation(999)
	if err != nil || clientInst == nil {
		t.Errorf("ClientForInstallation failed: %v", err)
	}
}
