package githubEngine

import (
	"bytes"
	"fmt"
	"net/http"
	"os"

	"github.com/bradleyfalzon/ghinstallation/v2"
	"github.com/google/go-github/v79/github"
)

type GitHubClientManager struct {
	AppID          int64
	PrivateKeyPath string
	appTransport   *ghinstallation.AppsTransport
}

func NewGitHubClientManager() (*GitHubClientManager, error) {
	appID := os.Getenv("GITHUB_APP_ID")
	privateKeyPath := os.Getenv("GITHUB_APP_PRIVATE_KEY_PATH")

	appIDInt, err := parseInt64(appID)
	if err != nil {
		return nil, fmt.Errorf("invalid GITHUB_APP_ID: %w", err)
	}

	if _, err := os.Stat(privateKeyPath); err != nil {
		return nil, fmt.Errorf("private key file not found at %s: %w", privateKeyPath, err)
	}

	keyBytes, err := os.ReadFile(privateKeyPath)
	if err != nil {
		return nil, fmt.Errorf("reading private key: %w", err)
	}
	fmt.Printf("Key file size: %d bytes\n", len(keyBytes))
	fmt.Printf("Key contains literal backslash-n: %v\n", bytes.Contains(keyBytes, []byte("\\n")))

	appTransport, err := ghinstallation.NewAppsTransportKeyFromFile(
		http.DefaultTransport,
		appIDInt,
		privateKeyPath,
	)
	if err != nil {
		return nil, fmt.Errorf("creating apps transport: %w", err)
	}

	return &GitHubClientManager{
		AppID:          appIDInt,
		PrivateKeyPath: privateKeyPath,
		appTransport:   appTransport,
	}, nil
}

func (m *GitHubClientManager) ClientForInstallation(installationID int64) (*github.Client, error) {
	if m.appTransport == nil {
		return nil, fmt.Errorf("appTransport not initialized")
	}

	tr := ghinstallation.NewFromAppsTransport(m.appTransport, installationID)
	client := github.NewClient(&http.Client{Transport: tr})
	return client, nil
}

func (m *GitHubClientManager) ClientForApp() (*github.Client, error) {
	if m.appTransport == nil {
		return nil, fmt.Errorf("appTransport not initialized")
	}
	client := github.NewClient(&http.Client{Transport: m.appTransport})
	return client, nil
}

func parseInt64(s string) (int64, error) {
	var i int64
	_, err := fmt.Sscan(s, &i)
	return i, err
}
