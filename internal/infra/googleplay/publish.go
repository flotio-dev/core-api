package googleplay

import (
	"context"
	"errors"
	"fmt"
	"io"

	"google.golang.org/api/androidpublisher/v3"
)

// insertEdit opens a new edit transaction for the app and returns its id.
// Every change to a Play listing happens inside such a transaction.
func (c *Client) insertEdit(ctx context.Context, packageName string) (string, error) {
	if packageName == "" {
		return "", errors.New("googleplay: empty package name")
	}
	edit, err := c.service.Edits.Insert(packageName, &androidpublisher.AppEdit{}).Context(ctx).Do()
	if err != nil {
		return "", fmt.Errorf("googleplay: open edit for %s: %w", packageName, err)
	}
	return edit.Id, nil
}

// abortEdit discards an uncommitted edit. Best-effort: abandoned edits also
// expire on their own, so a failure here is ignored.
func (c *Client) abortEdit(ctx context.Context, packageName, editID string) {
	_ = c.service.Edits.Delete(packageName, editID).Context(ctx).Do()
}

// uploadBundle uploads a signed AAB into an open edit and returns the created
// bundle, whose VersionCode is confirmed by Google.
func (c *Client) uploadBundle(ctx context.Context, packageName, editID string, aab io.Reader) (*androidpublisher.Bundle, error) {
	if aab == nil {
		return nil, errors.New("googleplay: nil AAB reader")
	}
	bundle, err := c.service.Edits.Bundles.Upload(packageName, editID).Media(aab).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("googleplay: upload bundle for %s: %w", packageName, err)
	}
	return bundle, nil
}
