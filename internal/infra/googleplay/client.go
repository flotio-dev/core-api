// Package googleplay builds authenticated clients for the Google Play
// Android Publisher API.
//
// In the BYO (bring-your-own) model, each client uploads its own service
// account JSON key, stored encrypted at rest. This package decrypts that key
// and constructs an androidpublisher.Service that acts on the client's own
// Play Console account.
package googleplay

import (
	"context"
	"errors"
	"fmt"

	"google.golang.org/api/androidpublisher/v3"
	"google.golang.org/api/option"

	"github.com/flotio-dev/core-api/internal/common/crypto"
)

// Client wraps the authenticated Android Publisher service.
type Client struct {
	service *androidpublisher.Service
}

// Service exposes the underlying Android Publisher service for edits operations
// (insert, bundles.upload, tracks.update, commit...).
func (c *Client) Service() *androidpublisher.Service {
	return c.service
}

// NewClient builds an authenticated client from a service account JSON key
// (decrypted, raw JSON bytes).
func NewClient(ctx context.Context, saJSON []byte) (*Client, error) {
	if len(saJSON) == 0 {
		return nil, errors.New("googleplay: empty service account credentials")
	}
	service, err := androidpublisher.NewService(ctx, option.WithCredentialsJSON(saJSON))
	if err != nil {
		return nil, fmt.Errorf("googleplay: build service: %w", err)
	}
	return &Client{service: service}, nil
}

// NewClientFromCredentials decrypts a stored GooglePlayCredentials value and
// builds an authenticated client. The stored value is the service account JSON
// (optionally base64-encoded) encrypted at rest.
func NewClientFromCredentials(ctx context.Context, encryptedCredentials string) (*Client, error) {
	saJSON, err := decodeCredentials(encryptedCredentials)
	if err != nil {
		return nil, err
	}
	return NewClient(ctx, saJSON)
}

// decodeCredentials reverses how credentials are stored: encrypted at rest, and
// the plaintext may itself be base64-encoded JSON. It returns the raw JSON bytes.
func decodeCredentials(encryptedCredentials string) ([]byte, error) {
	decrypted, err := crypto.Decrypt(encryptedCredentials)
	if err != nil {
		return nil, fmt.Errorf("googleplay: decrypt credentials: %w", err)
	}
	if decrypted == "" {
		return nil, errors.New("googleplay: empty service account credentials")
	}
	return DecodeServiceAccount(decrypted), nil
}
