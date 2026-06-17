package googleplay

import (
	"testing"

	"google.golang.org/api/androidpublisher/v3"
)

func TestMaxBundleVersionCode(t *testing.T) {
	cases := []struct {
		name    string
		bundles []*androidpublisher.Bundle
		want    int64
	}{
		{"empty", nil, 0},
		{"single", []*androidpublisher.Bundle{{VersionCode: 7}}, 7},
		{"multiple", []*androidpublisher.Bundle{{VersionCode: 3}, {VersionCode: 42}, {VersionCode: 12}}, 42},
		{"nil entry ignored", []*androidpublisher.Bundle{nil, {VersionCode: 5}}, 5},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := maxBundleVersionCode(tc.bundles); got != tc.want {
				t.Fatalf("got %d want %d", got, tc.want)
			}
		})
	}
}

func TestResolveVersionCode(t *testing.T) {
	cases := []struct {
		name     string
		latest   int64
		override int64
		want     int64
		wantErr  bool
	}{
		{"first release", 0, 0, 1, false},
		{"auto increment", 41, 0, 42, false},
		{"valid override", 41, 50, 50, false},
		{"override equals latest", 41, 41, 0, true},
		{"override below latest", 41, 10, 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveVersionCode(tc.latest, tc.override)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %d", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %d want %d", got, tc.want)
			}
		})
	}
}
