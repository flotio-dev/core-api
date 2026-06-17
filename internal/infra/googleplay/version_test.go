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
