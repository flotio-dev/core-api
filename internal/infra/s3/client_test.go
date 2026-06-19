package s3

import "testing"

func TestHasExtension(t *testing.T) {
	cases := []struct {
		name string
		key  string
		ext  string
		want bool
	}{
		{"aab match", "builds/12/app-release.aab", ".aab", true},
		{"apk not aab", "builds/12/app-release.apk", ".aab", false},
		{"exact", ".aab", ".aab", true},
		{"shorter than ext", "ab", ".aab", false},
		{"empty key", "", ".aab", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := hasExtension(tc.key, tc.ext); got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}
