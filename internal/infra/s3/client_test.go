package s3

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

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

func TestConfigAndEnvFunctions(t *testing.T) {
	// Bucket
	os.Unsetenv("AWS_S3_BUCKET")
	if GetBucket() != "flotio-builds" {
		t.Errorf("expected default bucket 'flotio-builds', got %s", GetBucket())
	}
	os.Setenv("AWS_S3_BUCKET", "custom-bucket")
	if GetBucket() != "custom-bucket" {
		t.Errorf("expected 'custom-bucket', got %s", GetBucket())
	}

	// Prefix
	os.Unsetenv("AWS_S3_PREFIX")
	if GetPrefix() != "builds" {
		t.Errorf("expected default prefix 'builds', got %s", GetPrefix())
	}
	os.Setenv("AWS_S3_PREFIX", "custom-builds")
	if GetPrefix() != "custom-builds" {
		t.Errorf("expected 'custom-builds', got %s", GetPrefix())
	}

	// Cache Prefix
	os.Unsetenv("AWS_S3_CACHE_PREFIX")
	if GetCachePrefix() != "build-cache" {
		t.Errorf("expected 'build-cache', got %s", GetCachePrefix())
	}
	os.Setenv("AWS_S3_CACHE_PREFIX", "/custom-cache/")
	if GetCachePrefix() != "custom-cache" {
		t.Errorf("expected trimmed 'custom-cache', got %s", GetCachePrefix())
	}

	// Namespace prefixes
	if got := BuildCacheNamespacePrefix(""); got != "custom-cache/" {
		t.Errorf("expected custom-cache/, got %s", got)
	}
	if got := BuildCacheNamespacePrefix("proj/main"); got != "custom-cache/proj/main/" {
		t.Errorf("expected custom-cache/proj/main/, got %s", got)
	}
	if got := buildCacheScopePrefix("proj", ""); got != "custom-cache/proj/" {
		t.Errorf("expected custom-cache/proj/, got %s", got)
	}
	if got := buildCacheScopePrefix("proj", "fp1"); got != "custom-cache/proj/fp1/" {
		t.Errorf("expected custom-cache/proj/fp1/, got %s", got)
	}

	// Region
	os.Unsetenv("AWS_REGION")
	if getRegion() != "garage" {
		t.Errorf("expected 'garage', got %s", getRegion())
	}
	os.Setenv("AWS_REGION", "eu-west-1")
	if getRegion() != "eu-west-1" {
		t.Errorf("expected 'eu-west-1', got %s", getRegion())
	}

	// Endpoint & SSL
	os.Unsetenv("AWS_S3_ENDPOINT")
	if getEndpoint() != "s3.eu-west-1.amazonaws.com" {
		t.Errorf("unexpected default endpoint: %s", getEndpoint())
	}

	os.Setenv("AWS_S3_ENDPOINT", "http://my-minio.local:9000")
	if getEndpoint() != "my-minio.local:9000" {
		t.Errorf("expected stripped http endpoint, got %s", getEndpoint())
	}
	if getUseSSL() {
		t.Error("expected useSSL false for http endpoint")
	}

	os.Setenv("AWS_S3_ENDPOINT", "https://s3.example.com")
	if getEndpoint() != "s3.example.com" {
		t.Errorf("expected stripped https endpoint, got %s", getEndpoint())
	}
	if !getUseSSL() {
		t.Error("expected useSSL true for https endpoint")
	}

	os.Setenv("AWS_S3_USE_SSL", "false")
	if getUseSSL() {
		t.Error("expected useSSL false when AWS_S3_USE_SSL is false")
	}
	os.Unsetenv("AWS_S3_USE_SSL")

	// Artifact Key
	os.Setenv("AWS_S3_PREFIX", "builds")
	if key := GetBuildArtifactKey(42, "app.apk"); key != "builds/42/app.apk" {
		t.Errorf("expected 'builds/42/app.apk', got %s", key)
	}

	// Operational Metrics
	metrics := GetCacheOperationalMetrics()
	_ = metrics.PurgeRequests
}

func setupMockS3Server(t *testing.T) (*minio.Client, *httptest.Server) {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")

		// Bucket location probe
		if r.Method == "GET" && r.URL.Query().Has("location") {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<LocationConstraint xmlns="http://s3.amazonaws.com/doc/2006-03-01/">us-east-1</LocationConstraint>`))
			return
		}

		// List objects v1 & v2
		if r.Method == "GET" && (r.URL.Query().Has("list-type") || r.URL.Query().Has("prefix")) {
			xmlResponse := `<?xml version="1.0" encoding="UTF-8"?>
<ListBucketResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/">
    <Name>flotio-builds</Name>
    <Prefix>` + r.URL.Query().Get("prefix") + `</Prefix>
    <KeyCount>3</KeyCount>
    <MaxKeys>1000</MaxKeys>
    <IsTruncated>false</IsTruncated>
    <Contents>
        <Key>builds/42/app-release.apk</Key>
        <LastModified>2026-09-01T12:00:00.000Z</LastModified>
        <ETag>"dummy"</ETag>
        <Size>12345</Size>
        <StorageClass>STANDARD</StorageClass>
    </Contents>
    <Contents>
        <Key>builds/42/app-release.aab</Key>
        <LastModified>2026-09-01T12:05:00.000Z</LastModified>
        <ETag>"dummy"</ETag>
        <Size>23456</Size>
        <StorageClass>STANDARD</StorageClass>
    </Contents>
    <Contents>
        <Key>build-cache/proj/main/fp1/cache.tar.gz</Key>
        <LastModified>2026-09-01T12:10:00.000Z</LastModified>
        <ETag>"dummy"</ETag>
        <Size>34567</Size>
        <StorageClass>STANDARD</StorageClass>
    </Contents>
</ListBucketResult>`
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(xmlResponse))
			return
		}

		// StatObject (HEAD request)
		if r.Method == "HEAD" {
			if r.URL.Path == "/flotio-builds/exists.txt" {
				w.Header().Set("Content-Length", "100")
				w.Header().Set("Last-Modified", time.Now().UTC().Format(http.TimeFormat))
				w.WriteHeader(http.StatusOK)
				return
			}
			w.WriteHeader(http.StatusNotFound)
			return
		}

		// Delete Object
		if r.Method == "DELETE" {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		// Get Object
		if r.Method == "GET" {
			w.Header().Set("Content-Type", "application/octet-stream")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("mock-data"))
			return
		}

		http.NotFound(w, r)
	}))

	c, err := minio.New(ts.Listener.Addr().String(), &minio.Options{
		Creds:  credentials.NewStaticV4("mockAccess", "mockSecret", ""),
		Secure: false,
	})
	if err != nil {
		t.Fatalf("failed to create minio client: %v", err)
	}

	client = c
	return c, ts
}

func TestS3Client_Operations(t *testing.T) {
	_, ts := setupMockS3Server(t)
	defer ts.Close()

	os.Setenv("AWS_S3_BUCKET", "flotio-builds")
	os.Setenv("AWS_S3_PREFIX", "builds")
	os.Setenv("AWS_S3_CACHE_PREFIX", "build-cache")

	// 1. GetClient
	cl, err := GetClient()
	if err != nil || cl == nil {
		t.Fatalf("GetClient failed: %v", err)
	}

	// 2. ListBuildArtifacts
	artifacts, err := ListBuildArtifacts(42)
	if err != nil {
		t.Fatalf("ListBuildArtifacts failed: %v", err)
	}
	if len(artifacts) == 0 {
		t.Error("expected artifacts listed")
	}

	// 3. FindPrimaryArtifactKey platforms & edge cases
	androidArtifact, err := FindPrimaryArtifactKey(42, "android")
	if err != nil || androidArtifact != "builds/42/app-release.aab" {
		t.Errorf("expected .aab priority for android, got: %s (err: %v)", androidArtifact, err)
	}
	iosArtifact, err := FindPrimaryArtifactKey(42, "ios")
	if err != nil {
		t.Errorf("expected primary artifact for ios: %v", err)
	}
	_ = iosArtifact
	webArtifact, err := FindPrimaryArtifactKey(42, "web")
	if err != nil {
		t.Errorf("expected primary artifact for web: %v", err)
	}
	_ = webArtifact
	defaultArtifact, err := FindPrimaryArtifactKey(42, "unknown-platform")
	if err != nil {
		t.Errorf("expected primary artifact for default: %v", err)
	}
	_ = defaultArtifact

	// 4. FindReleaseArtifactKey
	releaseArtifact, err := FindReleaseArtifactKey(42)
	if err != nil || releaseArtifact != "builds/42/app-release.aab" {
		t.Errorf("expected .aab release artifact, got: %s", releaseArtifact)
	}

	// 5. ObjectExists
	exists, err := ObjectExists("exists.txt")
	if err != nil || !exists {
		t.Errorf("expected exists true, got %v, %v", exists, err)
	}
	notExists, err := ObjectExists("missing.txt")
	if err != nil || notExists {
		t.Errorf("expected exists false for missing.txt, got %v, %v", notExists, err)
	}

	// 6. GetObject
	rc, err := GetObject("some-file.txt")
	if err != nil {
		t.Fatalf("GetObject failed: %v", err)
	}
	rc.Close()

	// 7. GetPresignedURL
	urlStr, err := GetPresignedURL("builds/42/app.apk", 60)
	if err != nil || urlStr == "" {
		t.Errorf("GetPresignedURL failed: %v, %s", err, urlStr)
	}

	// 8. DeleteObject
	if err := DeleteObject("builds/42/app.apk"); err != nil {
		t.Errorf("DeleteObject failed: %v", err)
	}

	// 9. DeleteBuildArtifacts
	if err := DeleteBuildArtifacts(42); err != nil {
		t.Errorf("DeleteBuildArtifacts failed: %v", err)
	}

	// 10. Cache Metrics and Entries
	nsMetrics, err := GetCacheNamespaceMetrics("proj/main")
	if err != nil {
		t.Errorf("GetCacheNamespaceMetrics failed: %v", err)
	}
	if nsMetrics.ObjectCount == 0 {
		t.Error("expected >0 objects in cache metrics")
	}

	entries, err := ListCacheEntries("proj/main")
	if err != nil {
		t.Errorf("ListCacheEntries failed: %v", err)
	}
	if len(entries) == 0 {
		t.Error("expected cache entries listed")
	}

	// 11. DeleteCacheNamespace
	count, err := DeleteCacheNamespace("proj/main")
	if err != nil || count == 0 {
		t.Errorf("DeleteCacheNamespace failed: %v, count: %d", err, count)
	}
}
