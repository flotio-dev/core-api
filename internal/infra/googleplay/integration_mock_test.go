package googleplay

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/androidpublisher/v3"
	"google.golang.org/api/option"
)

func setupMockPlayServer(t *testing.T) (*Client, *httptest.Server) {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// 1. Insert Edit: POST .../edits
		if r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/edits") {
			_ = json.NewEncoder(w).Encode(map[string]string{"id": "edit-123"})
			return
		}

		// 2. Abort Edit: DELETE .../edits/edit-123
		if r.Method == "DELETE" && strings.Contains(r.URL.Path, "/edits/edit-123") {
			w.WriteHeader(http.StatusOK)
			return
		}

		// 3. Upload Bundle: POST .../bundles
		if r.Method == "POST" && strings.Contains(r.URL.Path, "/bundles") {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"versionCode": 42})
			return
		}

		// 4. Assign Track: PUT .../tracks/production
		if r.Method == "PUT" && strings.Contains(r.URL.Path, "/tracks/") {
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"track": "production"})
			return
		}

		// 5. Commit Edit: POST .../edits/edit-123:commit
		if strings.Contains(r.URL.Path, ":commit") {
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]string{"id": "edit-123"})
			return
		}

		// 6. List Bundles: GET .../bundles
		if r.Method == "GET" && strings.Contains(r.URL.Path, "/bundles") {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"bundles": []map[string]interface{}{
					{"versionCode": 10},
					{"versionCode": 42},
				},
			})
			return
		}

		http.NotFound(w, r)
	}))

	ctx := context.Background()
	svc, err := androidpublisher.NewService(ctx, option.WithEndpoint(ts.URL), option.WithoutAuthentication())
	if err != nil {
		t.Fatalf("failed to create mock androidpublisher service: %v", err)
	}

	return &Client{service: svc}, ts
}

func TestMockPlay_Publish(t *testing.T) {
	client, ts := setupMockPlayServer(t)
	defer ts.Close()

	ctx := context.Background()
	input := PublishInput{
		PackageName:     "com.example.app",
		AAB:             strings.NewReader("fake aab content"),
		Track:           "production",
		RolloutFraction: 1.0,
		Name:            "v1.0.0",
		ReleaseNotes:    "initial release",
	}

	res, err := client.Publish(ctx, input)
	if err != nil {
		t.Fatalf("Publish failed: %v", err)
	}
	if res.VersionCode != 42 || res.Status != "completed" || res.Track != "production" {
		t.Errorf("unexpected publish result: %+v", res)
	}
}

func TestMockPlay_CheckAccess(t *testing.T) {
	client, ts := setupMockPlayServer(t)
	defer ts.Close()

	ctx := context.Background()
	err := client.CheckAccess(ctx, "com.example.app")
	if err != nil {
		t.Fatalf("CheckAccess failed: %v", err)
	}
}

func TestMockPlay_VersionCodes(t *testing.T) {
	client, ts := setupMockPlayServer(t)
	defer ts.Close()

	ctx := context.Background()
	latest, err := client.LatestVersionCode(ctx, "com.example.app")
	if err != nil {
		t.Fatalf("LatestVersionCode failed: %v", err)
	}
	if latest != 42 {
		t.Errorf("expected latest 42, got %d", latest)
	}

	next, err := client.NextVersionCode(ctx, "com.example.app", 0)
	if err != nil {
		t.Fatalf("NextVersionCode failed: %v", err)
	}
	if next != 43 {
		t.Errorf("expected next 43, got %d", next)
	}
}
