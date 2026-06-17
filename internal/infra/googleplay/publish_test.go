package googleplay

import "testing"

func TestBuildTrackRelease(t *testing.T) {
	t.Run("full rollout", func(t *testing.T) {
		r := buildTrackRelease(TrackAssignment{Track: "production", VersionCode: 5})
		if r.Status != "completed" {
			t.Fatalf("status = %q want completed", r.Status)
		}
		if r.UserFraction != 0 {
			t.Fatalf("userFraction = %v want 0", r.UserFraction)
		}
		if len(r.VersionCodes) != 1 || r.VersionCodes[0] != 5 {
			t.Fatalf("versionCodes = %v want [5]", r.VersionCodes)
		}
	})

	t.Run("staged rollout", func(t *testing.T) {
		r := buildTrackRelease(TrackAssignment{Track: "production", VersionCode: 5, RolloutFraction: 0.1})
		if r.Status != "inProgress" {
			t.Fatalf("status = %q want inProgress", r.Status)
		}
		if r.UserFraction != 0.1 {
			t.Fatalf("userFraction = %v want 0.1", r.UserFraction)
		}
	})

	t.Run("draft takes precedence over rollout", func(t *testing.T) {
		r := buildTrackRelease(TrackAssignment{Track: "beta", VersionCode: 5, RolloutFraction: 0.5, Draft: true})
		if r.Status != "draft" {
			t.Fatalf("status = %q want draft", r.Status)
		}
		if r.UserFraction != 0 {
			t.Fatalf("userFraction = %v want 0 for draft", r.UserFraction)
		}
	})

	t.Run("fraction >= 1 is full rollout", func(t *testing.T) {
		r := buildTrackRelease(TrackAssignment{Track: "production", VersionCode: 5, RolloutFraction: 1})
		if r.Status != "completed" {
			t.Fatalf("status = %q want completed", r.Status)
		}
	})

	t.Run("release notes default language", func(t *testing.T) {
		r := buildTrackRelease(TrackAssignment{Track: "internal", VersionCode: 5, ReleaseNotes: "first"})
		if len(r.ReleaseNotes) != 1 {
			t.Fatalf("expected 1 localized note, got %d", len(r.ReleaseNotes))
		}
		if r.ReleaseNotes[0].Language != defaultReleaseNotesLang {
			t.Fatalf("language = %q want %q", r.ReleaseNotes[0].Language, defaultReleaseNotesLang)
		}
		if r.ReleaseNotes[0].Text != "first" {
			t.Fatalf("text = %q want first", r.ReleaseNotes[0].Text)
		}
	})

	t.Run("no release notes", func(t *testing.T) {
		r := buildTrackRelease(TrackAssignment{Track: "internal", VersionCode: 5})
		if r.ReleaseNotes != nil {
			t.Fatalf("expected no release notes, got %v", r.ReleaseNotes)
		}
	})
}
