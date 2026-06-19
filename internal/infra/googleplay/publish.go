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

// commitEdit validates the edit transaction, making the changes effective.
func (c *Client) commitEdit(ctx context.Context, packageName, editID string) error {
	if _, err := c.service.Edits.Commit(packageName, editID).Context(ctx).Do(); err != nil {
		return fmt.Errorf("googleplay: commit edit for %s: %w", packageName, err)
	}
	return nil
}

// PublishInput describes a full Google Play publication request.
type PublishInput struct {
	PackageName      string
	AAB              io.Reader // signed .aab content
	Track            string    // internal, alpha, beta, production
	RolloutFraction  float64   // 0..1 for a staged rollout; <=0 or >=1 means full
	Draft            bool      // publish as draft instead of rolling out
	Name             string    // optional release name (e.g. versionName)
	ReleaseNotes     string    // optional changelog
	ReleaseNotesLang string    // BCP-47 language for the notes; defaults to en-US
}

// PublishResult reports the outcome of a publication.
type PublishResult struct {
	VersionCode int64
	Track       string
	Status      string
}

// Publish runs the full edits transaction: open, upload the AAB, assign it to a
// track and commit. The edit is aborted if any step fails so no dangling
// transaction is left behind.
func (c *Client) Publish(ctx context.Context, in PublishInput) (*PublishResult, error) {
	if in.Track == "" {
		return nil, errors.New("googleplay: empty track")
	}

	var editID string
	if err := withRetry(ctx, maxPublishAttempts, func() error {
		var e error
		editID, e = c.insertEdit(ctx, in.PackageName)
		return e
	}); err != nil {
		return nil, classifyError(err)
	}
	committed := false
	defer func() {
		if !committed {
			c.abortEdit(ctx, in.PackageName, editID)
		}
	}()

	// The upload streams the AAB and is not retried (the reader cannot rewind).
	bundle, err := c.uploadBundle(ctx, in.PackageName, editID, in.AAB)
	if err != nil {
		return nil, classifyError(err)
	}

	assignment := TrackAssignment{
		Track:            in.Track,
		VersionCode:      bundle.VersionCode,
		RolloutFraction:  in.RolloutFraction,
		Draft:            in.Draft,
		Name:             in.Name,
		ReleaseNotes:     in.ReleaseNotes,
		ReleaseNotesLang: in.ReleaseNotesLang,
	}
	if err := withRetry(ctx, maxPublishAttempts, func() error {
		return c.assignTrack(ctx, in.PackageName, editID, assignment)
	}); err != nil {
		return nil, classifyError(err)
	}

	// Commit is retried only on transient errors. A retry after a committed but
	// lost response fails non-transiently (the edit is gone), so it never
	// double-publishes.
	if err := withRetry(ctx, maxPublishAttempts, func() error {
		return c.commitEdit(ctx, in.PackageName, editID)
	}); err != nil {
		return nil, classifyError(err)
	}
	committed = true

	return &PublishResult{
		VersionCode: bundle.VersionCode,
		Track:       in.Track,
		Status:      resolveReleaseStatus(in.Draft, in.RolloutFraction),
	}, nil
}

// defaultReleaseNotesLang is used when release notes are provided without a
// specific BCP-47 language tag.
const defaultReleaseNotesLang = "en-US"

// Google Play release statuses.
const (
	releaseStatusDraft      = "draft"
	releaseStatusInProgress = "inProgress"
	releaseStatusCompleted  = "completed"
)

// resolveReleaseStatus maps the draft/rollout intent to a Play release status.
// It is pure for testing.
func resolveReleaseStatus(draft bool, rolloutFraction float64) string {
	switch {
	case draft:
		return releaseStatusDraft
	case rolloutFraction > 0 && rolloutFraction < 1:
		return releaseStatusInProgress
	default:
		return releaseStatusCompleted
	}
}

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

	release.Status = resolveReleaseStatus(a.Draft, a.RolloutFraction)
	if release.Status == releaseStatusInProgress {
		release.UserFraction = a.RolloutFraction
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
