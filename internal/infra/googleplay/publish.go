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

// defaultReleaseNotesLang is used when release notes are provided without a
// specific BCP-47 language tag.
const defaultReleaseNotesLang = "en-US"

// TrackAssignment describes how a versionCode is released on a track.
type TrackAssignment struct {
	Track            string  // internal, alpha, beta, production
	VersionCode      int64   // the bundle to release
	RolloutFraction  float64 // 0..1 for a staged rollout; <=0 or >=1 means full rollout
	Draft            bool    // prepare the release as a draft instead of rolling it out
	Name             string  // optional release name shown in the Console (e.g. versionName)
	ReleaseNotes     string  // optional changelog
	ReleaseNotesLang string  // BCP-47 language for the notes; defaults to en-US
}

// assignTrack assigns the bundle to a track within an open edit, applying the
// rollout/draft status and optional release notes.
func (c *Client) assignTrack(ctx context.Context, packageName, editID string, a TrackAssignment) error {
	if a.Track == "" {
		return errors.New("googleplay: empty track")
	}
	track := &androidpublisher.Track{
		Track:    a.Track,
		Releases: []*androidpublisher.TrackRelease{buildTrackRelease(a)},
	}
	if _, err := c.service.Edits.Tracks.Update(packageName, editID, a.Track, track).Context(ctx).Do(); err != nil {
		return fmt.Errorf("googleplay: update track %s for %s: %w", a.Track, packageName, err)
	}
	return nil
}

// buildTrackRelease maps a TrackAssignment to an androidpublisher.TrackRelease,
// resolving the release status and rollout fraction. It is pure for testing.
func buildTrackRelease(a TrackAssignment) *androidpublisher.TrackRelease {
	release := &androidpublisher.TrackRelease{
		Name:         a.Name,
		VersionCodes: []int64{a.VersionCode},
	}

	switch {
	case a.Draft:
		release.Status = "draft"
	case a.RolloutFraction > 0 && a.RolloutFraction < 1:
		release.Status = "inProgress"
		release.UserFraction = a.RolloutFraction
	default:
		release.Status = "completed"
	}

	if a.ReleaseNotes != "" {
		lang := a.ReleaseNotesLang
		if lang == "" {
			lang = defaultReleaseNotesLang
		}
		release.ReleaseNotes = []*androidpublisher.LocalizedText{{
			Language: lang,
			Text:     a.ReleaseNotes,
		}}
	}

	return release
}
