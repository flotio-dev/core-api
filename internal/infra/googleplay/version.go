package googleplay

import (
	"context"
	"fmt"

	"google.golang.org/api/androidpublisher/v3"
)

// LatestVersionCode returns the highest versionCode already uploaded for the
// app, across all of its App Bundles. It returns 0 when the app has no bundle
// yet (first release).
//
// Bundles (not just track releases) are the source of truth: Google rejects an
// upload whose versionCode is not strictly greater than every previously
// uploaded bundle, even ones not assigned to a track.
func (c *Client) LatestVersionCode(ctx context.Context, packageName string) (int64, error) {
	if packageName == "" {
		return 0, fmt.Errorf("googleplay: empty package name")
	}

	edit, err := c.service.Edits.Insert(packageName, &androidpublisher.AppEdit{}).Context(ctx).Do()
	if err != nil {
		return 0, fmt.Errorf("googleplay: open edit for %s: %w", packageName, err)
	}
	// Read-only: discard the edit instead of committing it. Best-effort cleanup;
	// abandoned edits also expire on their own.
	defer func() { _ = c.service.Edits.Delete(packageName, edit.Id).Context(ctx).Do() }()

	list, err := c.service.Edits.Bundles.List(packageName, edit.Id).Context(ctx).Do()
	if err != nil {
		return 0, fmt.Errorf("googleplay: list bundles for %s: %w", packageName, err)
	}

	return maxBundleVersionCode(list.Bundles), nil
}

// maxBundleVersionCode returns the highest versionCode in the bundle list, or 0
// when the list is empty.
func maxBundleVersionCode(bundles []*androidpublisher.Bundle) int64 {
	var max int64
	for _, b := range bundles {
		if b != nil && b.VersionCode > max {
			max = b.VersionCode
		}
	}
	return max
}

// NextVersionCode determines the versionCode to bake into the next build.
// When override is provided (> 0) it must be strictly greater than the latest
// published versionCode; otherwise the next code is latest + 1.
func (c *Client) NextVersionCode(ctx context.Context, packageName string, override int64) (int64, error) {
	latest, err := c.LatestVersionCode(ctx, packageName)
	if err != nil {
		return 0, err
	}
	return resolveVersionCode(latest, override)
}

// resolveVersionCode applies the increment/override rules against the latest
// published versionCode. It is pure so the rules can be unit-tested.
func resolveVersionCode(latest, override int64) (int64, error) {
	if override > 0 {
		if override <= latest {
			return 0, fmt.Errorf("googleplay: override versionCode %d must be greater than latest published %d", override, latest)
		}
		return override, nil
	}
	return latest + 1, nil
}
