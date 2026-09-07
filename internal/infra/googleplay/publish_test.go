package googleplay

import (
	"context"
	"testing"
)

func TestResolveReleaseStatus(t *testing.T) {
	cases := []struct {
		name     string
		draft    bool
		fraction float64
		want     string
	}{
		{"full rollout", false, 0, releaseStatusCompleted},
		{"staged rollout", false, 0.1, releaseStatusInProgress},
		{"fraction >= 1 is full", false, 1, releaseStatusCompleted},
		{"draft wins over rollout", true, 0.5, releaseStatusDraft},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := resolveReleaseStatus(tc.draft, tc.fraction); got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}

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

func TestBuildTrackRelease_CustomLang(t *testing.T) {
	r := buildTrackRelease(TrackAssignment{
		Track:            "internal",
		VersionCode:      5,
		ReleaseNotes:     "note",
		ReleaseNotesLang: "fr-FR",
	})
	if len(r.ReleaseNotes) != 1 || r.ReleaseNotes[0].Language != "fr-FR" {
		t.Errorf("expected language fr-FR, got %+v", r.ReleaseNotes)
	}
}

func TestPublish_ValidationErrors(t *testing.T) {
	c := &Client{}

	// Empty track
	_, err := c.Publish(nil, PublishInput{Track: ""})
	if err == nil || err.Error() != "googleplay: empty track" {
		t.Errorf("expected empty track error, got %v", err)
	}

	// Empty package name on insertEdit
	_, err = c.insertEdit(nil, "")
	if err == nil || err.Error() != "googleplay: empty package name" {
		t.Errorf("expected empty package name error, got %v", err)
	}

	// Nil AAB on uploadBundle
	_, err = c.uploadBundle(nil, "pkg", "edit", nil)
	if err == nil || err.Error() != "googleplay: nil AAB reader" {
		t.Errorf("expected nil AAB reader error, got %v", err)
	}

	// Empty track on assignTrack
	err = c.assignTrack(nil, "pkg", "edit", TrackAssignment{Track: ""})
	if err == nil || err.Error() != "googleplay: empty track" {
		t.Errorf("expected empty track error, got %v", err)
	}

	// Service getter
	if c.Service() != nil {
		t.Errorf("expected nil service from empty client")
	}

	// NewClient empty
	_, err = NewClient(nil, nil)
	if err == nil {
		t.Error("expected error for empty credentials")
	}

	// NewClientFromCredentials invalid/empty
	_, err = NewClientFromCredentials(nil, "")
	if err == nil {
		t.Error("expected error for empty encrypted credentials")
	}
}

func TestClient_EmptyPackageMethods(t *testing.T) {
	c := &Client{}
	ctx := context.Background()

	// CheckAccess
	if err := c.CheckAccess(ctx, ""); err == nil {
		t.Error("expected error for empty package in CheckAccess")
	}

	// LatestVersionCode
	if _, err := c.LatestVersionCode(ctx, ""); err == nil {
		t.Error("expected error for empty package in LatestVersionCode")
	}

	// NextVersionCode
	if _, err := c.NextVersionCode(ctx, "", 0); err == nil {
		t.Error("expected error for empty package in NextVersionCode")
	}
}
