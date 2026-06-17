package kubernetes

import "testing"

func TestVersionBuildFlags(t *testing.T) {
	cases := []struct {
		name        string
		versionName string
		versionCode int64
		want        string
	}{
		{"none falls back to pubspec", "", 0, ""},
		{"name only", "1.3.0", 0, " --build-name=1.3.0"},
		{"code only", "", 42, " --build-number=42"},
		{"both", "1.3.0", 42, " --build-name=1.3.0 --build-number=42"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := versionBuildFlags(tc.versionName, tc.versionCode); got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}
