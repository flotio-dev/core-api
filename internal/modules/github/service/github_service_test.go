package service

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	dbEngine "github.com/flotio-dev/core-api/internal/common/database"
	githubEngine "github.com/flotio-dev/core-api/internal/infra/github"
	repositories "github.com/flotio-dev/core-api/internal/modules/github/repository"
	"github.com/glebarez/sqlite"
	"github.com/google/go-github/v79/github"
	"gorm.io/gorm"
)

var counter int64

func randInt() int64 {
	counter++
	return counter
}

func setupServiceTest(t *testing.T) (*GithubService, *httptest.Server) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:gh_svc_memdb_%d?mode=memory&cache=shared", randInt())), &gorm.Config{})
	if err != nil {
		t.Fatalf("sqlite open error: %v", err)
	}
	_ = db.AutoMigrate(&dbEngine.GithubInstallation{}, &dbEngine.User{})

	repo := repositories.NewGithubRepository(db)

	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	keyBytes := x509.MarshalPKCS1PrivateKey(key)
	block := &pem.Block{Type: "RSA PRIVATE KEY", Bytes: keyBytes}
	tmpDir := t.TempDir()
	keyFile := filepath.Join(tmpDir, "gh.pem")
	f, _ := os.Create(keyFile)
	_ = pem.Encode(f, block)
	f.Close()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/app/installations/123/access_tokens":
			fmt.Fprintf(w, `{"token":"token-123","expires_at":"2030-01-01T00:00:00Z"}`)
		case "/user/installations":
			fmt.Fprintf(w, `{"total_count":1,"installations":[{"id":123}]}`)
		case "/installation/repositories":
			fmt.Fprintf(w, `{"total_count":1,"repositories":[{"id":100,"name":"flutter_app","full_name":"org/flutter_app","language":"Dart","default_branch":"main"}]}`)
		case "/app/installations/123":
			if r.Method == http.MethodDelete {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			fmt.Fprintf(w, `{"id":123,"account":{"login":"org","type":"Organization","id":999,"avatar_url":"http://avatar"}}`)
		case "/app/installations/404":
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprintf(w, `{"message":"Not Found"}`)
		case "/user":
			fmt.Fprintf(w, `{"login":"mockuser","id":555}`)
		case "/repos/org/flutter_app":
			fmt.Fprintf(w, `{"id":100,"name":"flutter_app","full_name":"org/flutter_app"}`)
		case "/repos/org/sub_app/contents/":
			fmt.Fprintf(w, `[{"type":"dir","name":"subfolder","path":"subfolder"}]`)
		case "/repos/org/sub_app/contents/subfolder":
			fmt.Fprintf(w, `[{"type":"file","name":"pubspec.yaml","path":"subfolder/pubspec.yaml"}]`)
		case "/repos/org/flutter_app/contents/.fvm/fvm_config.json":
			content := base64.StdEncoding.EncodeToString([]byte(`{"flutter":"3.19.0"}`))
			fmt.Fprintf(w, `{"type":"file","name":"fvm_config.json","path":".fvm/fvm_config.json","content":"%s","encoding":"base64"}`, content)
		case "/repos/org/flutter_app/contents/pubspec.yaml":
			content := base64.StdEncoding.EncodeToString([]byte("name: flutter_app\nflutter:\n  sdk: flutter\n  version: '3.16.0'\n"))
			fmt.Fprintf(w, `{"type":"file","name":"pubspec.yaml","path":"pubspec.yaml","content":"%s","encoding":"base64"}`, content)
		case "/repos/org/flutter_app/contents/":
			fmt.Fprintf(w, `[{"type":"file","name":"pubspec.yaml","path":"pubspec.yaml"},{"type":"dir","name":"android","path":"android"}]`)
		case "/repos/org/flutter_app/contents/android":
			fmt.Fprintf(w, `[{"type":"file","name":"build.gradle","path":"android/build.gradle"}]`)
		case "/repos/org/flutter_app/contents/android/app/google-services.json":
			content := base64.StdEncoding.EncodeToString([]byte(`{}`))
			fmt.Fprintf(w, `{"type":"file","name":"google-services.json","path":"android/app/google-services.json","content":"%s","encoding":"base64"}`, content)
		case "/repos/org/sub_app/contents/subfolder/.fvm/fvm_config.json":
			content := base64.StdEncoding.EncodeToString([]byte(`{"flutterSdkVersion":"3.16.5"}`))
			fmt.Fprintf(w, `{"type":"file","name":"fvm_config.json","path":"subfolder/.fvm/fvm_config.json","content":"%s","encoding":"base64"}`, content)
		case "/repos/org/pubspec_app/contents/":
			fmt.Fprintf(w, `[{"type":"file","name":"pubspec.yaml","path":"pubspec.yaml"}]`)
		case "/repos/org/pubspec_app/contents/pubspec.yaml":
			content := base64.StdEncoding.EncodeToString([]byte("name: pubspec_app\nenvironment:\n  sdk: '>=2.12.0 <3.0.0'\n  flutter: '3.7.0'\n"))
			fmt.Fprintf(w, `{"type":"file","name":"pubspec.yaml","path":"pubspec.yaml","content":"%s","encoding":"base64"}`, content)
		case "/repos/org/pubspec_version_sub/contents/":
			fmt.Fprintf(w, `[{"type":"dir","name":"sub","path":"sub"}]`)
		case "/repos/org/pubspec_version_sub/contents/sub":
			fmt.Fprintf(w, `[{"type":"file","name":"pubspec.yaml","path":"sub/pubspec.yaml"}]`)
		case "/repos/org/pubspec_version_sub/contents/sub/.fvm/fvm_config.json":
			w.WriteHeader(http.StatusNotFound)
		case "/repos/org/pubspec_version_sub/contents/sub/.flutter-version":
			content := base64.StdEncoding.EncodeToString([]byte("3.10.1\n"))
			fmt.Fprintf(w, `{"type":"file","name":".flutter-version","path":"sub/.flutter-version","content":"%s","encoding":"base64"}`, content)
		case "/repos/org/no_ver_app/contents/":
			fmt.Fprintf(w, `[{"type":"file","name":"pubspec.yaml","path":"pubspec.yaml"}]`)
		case "/repos/org/no_ver_app/contents/pubspec.yaml":
			content := base64.StdEncoding.EncodeToString([]byte("name: no_ver\n"))
			fmt.Fprintf(w, `{"type":"file","name":"pubspec.yaml","path":"pubspec.yaml","content":"%s","encoding":"base64"}`, content)
		case "/repos/org/other_app/contents/":
			fmt.Fprintf(w, `[]`)
		case "/repos/org/flutter_version_app/contents/.fvm/fvm_config.json":
			w.WriteHeader(http.StatusNotFound)
		case "/repos/org/flutter_version_app/contents/.flutter-version":
			content := base64.StdEncoding.EncodeToString([]byte("3.13.0\n"))
			fmt.Fprintf(w, `{"type":"file","name":".flutter-version","path":".flutter-version","content":"%s","encoding":"base64"}`, content)
		case "/repos/org/flutter_version_app/contents/pubspec.yaml":
			content := base64.StdEncoding.EncodeToString([]byte("name: flutter_version_app\n"))
			fmt.Fprintf(w, `{"type":"file","name":"pubspec.yaml","path":"pubspec.yaml","content":"%s","encoding":"base64"}`, content)
		case "/repos/org/flutter_version_app/contents/":
			fmt.Fprintf(w, `[{"type":"file","name":"pubspec.yaml","path":"pubspec.yaml"}]`)
		case "/app/installations/500":
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, `{"message":"Internal error"}`)
		default:
			if r.Method == http.MethodDelete {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			http.NotFound(w, r)
		}
	}))

	t.Setenv("GITHUB_APP_ID", "123")
	t.Setenv("GITHUB_APP_PRIVATE_KEY_PATH", keyFile)

	mgr, err := githubEngine.NewGitHubClientManager()
	if err != nil {
		t.Fatalf("failed to create client manager: %v", err)
	}

	mockGhClient := github.NewClient(ts.Client())
	mockGhClient.BaseURL, _ = url.Parse(ts.URL + "/")
	mockGhClient.UploadURL, _ = url.Parse(ts.URL + "/")

	origGetInst := getInstallationClient
	origGetApp := getAppClient
	t.Cleanup(func() {
		getInstallationClient = origGetInst
		getAppClient = origGetApp
	})

	getInstallationClient = func(m *githubEngine.GitHubClientManager, id int64) (*github.Client, error) {
		if m == nil {
			return nil, fmt.Errorf("ClientManager not initialized")
		}
		if id == 9999 {
			return nil, fmt.Errorf("mock client error")
		}
		return mockGhClient, nil
	}

	getAppClient = func(m *githubEngine.GitHubClientManager) (*github.Client, error) {
		if m == nil {
			return nil, fmt.Errorf("ClientManager not initialized")
		}
		return mockGhClient, nil
	}

	svc := NewGithubService(repo, mgr)
	return svc, ts
}

func TestGithubService_RepositoryMethods(t *testing.T) {
	svc, ts := setupServiceTest(t)
	defer ts.Close()

	userID := uint(1)
	instID := int64(123)

	// SaveInstallation
	err := svc.SaveInstallation(userID, instID, "org", "Organization", 999, "http://avatar")
	if err != nil {
		t.Fatalf("SaveInstallation failed: %v", err)
	}

	// UpdateInstallation
	err = svc.UpdateInstallation(userID, instID)
	if err != nil {
		t.Fatalf("UpdateInstallation failed: %v", err)
	}

	// GetInstallationByUser
	inst, err := svc.GetInstallationByUser(userID)
	if err != nil || inst == nil {
		t.Fatalf("GetInstallationByUser failed: %v", err)
	}

	// GetGithubInstallationByUserID
	inst2, err := svc.GetGithubInstallationByUserID(userID)
	if err != nil || inst2 == nil || inst2.ID != inst.ID {
		t.Errorf("GetGithubInstallationByUserID mismatch: %+v", inst2)
	}

	// ListInstallationsByUser
	insts, err := svc.ListInstallationsByUser(userID)
	if err != nil || len(insts) == 0 {
		t.Fatalf("ListInstallationsByUser failed: %v", err)
	}

	// GetGithubInstallationByInstallationID
	found, err := svc.GetGithubInstallationByInstallationID(instID)
	if err != nil || found == nil {
		t.Fatalf("GetGithubInstallationByInstallationID failed: %v", err)
	}

	// FindInstallationForOwner
	foundOwner, err := svc.FindInstallationForOwner(context.Background(), userID, "org")
	if err != nil || foundOwner == nil {
		t.Fatalf("FindInstallationForOwner failed: %v", err)
	}

	// FindInstallationForOwner with empty owner (returns first)
	firstInst, err := svc.FindInstallationForOwner(context.Background(), userID, "")
	if err != nil || firstInst == nil {
		t.Fatalf("FindInstallationForOwner empty owner failed: %v", err)
	}

	// FindInstallationForOwner with unknown owner fallback
	firstInst2, err := svc.FindInstallationForOwner(context.Background(), userID, "unknown_owner")
	if err != nil || firstInst2 == nil {
		t.Fatalf("FindInstallationForOwner fallback failed: %v", err)
	}

	// FindInstallationForRepo with matching owner
	foundRepo, err := svc.FindInstallationForRepo(context.Background(), userID, "org", "flutter_app")
	if err != nil || foundRepo == nil {
		t.Fatalf("FindInstallationForRepo failed: %v", err)
	}

	// FindInstallationForRepo with non-matching owner but matching repo
	foundRepo2, err := svc.FindInstallationForRepo(context.Background(), userID, "other", "flutter_app")
	if err != nil || foundRepo2 == nil {
		t.Fatalf("FindInstallationForRepo second check failed: %v", err)
	}

	// DeleteInstallationByInstallationID
	err = svc.DeleteInstallationByInstallationID(instID)
	if err != nil {
		t.Fatalf("DeleteInstallationByInstallationID failed: %v", err)
	}
}

func TestGithubService_API_SuccessAndErrors(t *testing.T) {
	svc, ts := setupServiceTest(t)
	defer ts.Close()

	ctx := context.Background()

	// ListRepositories
	repos, err := svc.ListRepositories(ctx, 123)
	if err != nil || len(repos) == 0 {
		t.Fatalf("ListRepositories failed: %v, repos: %v", err, repos)
	}

	// ListRepositories client error
	_, err = svc.ListRepositories(ctx, 9999)
	if err == nil {
		t.Errorf("expected error for installation 9999")
	}

	// GetRepoTree
	tree, err := svc.GetRepoTree(ctx, 123, "org", "flutter_app")
	if err != nil {
		t.Fatalf("GetRepoTree failed: %v", err)
	}
	_ = tree

	// GetInstallationToken
	token, err := svc.GetInstallationToken(123)
	if err != nil || token != "token-123" {
		t.Fatalf("GetInstallationToken failed: %v, got %s", err, token)
	}

	// GetGithubUser
	user, err := svc.GetGithubUser(ctx, 123)
	if err != nil || user.GetLogin() != "mockuser" {
		t.Fatalf("GetGithubUser failed: %v, user: %v", err, user)
	}

	// GetGithubInstallation
	inst, err := svc.GetGithubInstallation(ctx, 123)
	if err != nil || inst == nil || inst.GetID() != 123 {
		t.Fatalf("GetGithubInstallation failed: %v, inst: %v", err, inst)
	}

	// InstallationExists
	instExists, err := svc.InstallationExists(ctx, 123)
	if err != nil || instExists == nil {
		t.Fatalf("InstallationExists failed: %v", err)
	}

	// InstallationExists 404
	instNotExists, err := svc.InstallationExists(ctx, 404)
	if err != nil || instNotExists != nil {
		t.Fatalf("InstallationExists for 404 failed: %v, got: %v", err, instNotExists)
	}

	// FindBuildPath root
	bp, err := svc.FindBuildPath(ctx, 123, "org", "flutter_app")
	if err != nil || bp != "" {
		t.Fatalf("FindBuildPath root failed: %v, bp: %s", err, bp)
	}

	// FindBuildPath subfolder
	bpSub, err := svc.FindBuildPath(ctx, 123, "org", "sub_app")
	if err != nil || bpSub != "subfolder" {
		t.Fatalf("FindBuildPath subfolder failed: %v, bpSub: %s", err, bpSub)
	}

	// FindBuildPath not found
	_, err = svc.FindBuildPath(ctx, 123, "org", "other_app")
	if err == nil {
		t.Fatalf("expected error when pubspec not found")
	}

	// DetectFlutterProject with FVM
	det, err := svc.DetectFlutterProject(ctx, 123, "org", "flutter_app")
	if err != nil {
		t.Fatalf("DetectFlutterProject failed: %v", err)
	}
	if det.DetectedFlutterVersion != "3.19.0" || det.DetectionSource != "fvm" || !det.HasGoogleServices {
		t.Errorf("DetectFlutterProject unexpected result: %+v", det)
	}

	// DetectFlutterProject with .flutter-version
	det2, err := svc.DetectFlutterProject(ctx, 123, "org", "flutter_version_app")
	if err != nil {
		t.Fatalf("DetectFlutterProject 2 failed: %v", err)
	}
	if det2.DetectedFlutterVersion != "3.13.0" || det2.DetectionSource != "flutter-version" {
		t.Errorf("DetectFlutterProject 2 unexpected result: %+v", det2)
	}

	// DetectFlutterProject with subfolder fvm
	det3, err := svc.DetectFlutterProject(ctx, 123, "org", "sub_app")
	if err != nil {
		t.Fatalf("DetectFlutterProject 3 failed: %v", err)
	}
	if det3.DetectedFlutterVersion != "3.16.5" || det3.DetectionSource != "fvm" || det3.ProjectPath != "subfolder" {
		t.Errorf("DetectFlutterProject 3 unexpected result: %+v", det3)
	}

	// DetectFlutterProject with subfolder .flutter-version
	det4, err := svc.DetectFlutterProject(ctx, 123, "org", "pubspec_version_sub")
	if err != nil {
		t.Fatalf("DetectFlutterProject 4 failed: %v", err)
	}
	if det4.DetectedFlutterVersion != "3.10.1" || det4.DetectionSource != "flutter-version" {
		t.Errorf("DetectFlutterProject 4 unexpected result: %+v", det4)
	}

	// DetectFlutterProject with pubspec regex
	det5, err := svc.DetectFlutterProject(ctx, 123, "org", "pubspec_app")
	if err != nil {
		t.Fatalf("DetectFlutterProject 5 failed: %v", err)
	}
	if det5.DetectedFlutterVersion != "3.7.0" || det5.DetectionSource != "pubspec" {
		t.Errorf("DetectFlutterProject 5 unexpected result: %+v", det5)
	}

	// DetectFlutterProject without version
	det6, err := svc.DetectFlutterProject(ctx, 123, "org", "no_ver_app")
	if err != nil {
		t.Fatalf("DetectFlutterProject 6 failed: %v", err)
	}
	if det6.DetectedFlutterVersion != "" {
		t.Errorf("DetectFlutterProject 6 unexpected result: %+v", det6)
	}

	// Empty user FindInstallationForOwner and FindInstallationForRepo
	instNone, err := svc.FindInstallationForOwner(ctx, 999, "org")
	if err != nil || instNone != nil {
		t.Errorf("expected nil for user with no installations")
	}
	instRepoNone, err := svc.FindInstallationForRepo(ctx, 999, "org", "repo")
	if err != nil || instRepoNone != nil {
		t.Errorf("expected nil repo installation for user with no installations")
	}

	// FindInstallationForOwner with missing AccountLogin in db
	_ = svc.SaveInstallation(50, 123, "", "", 0, "")
	instFilled, err := svc.FindInstallationForOwner(ctx, 50, "org")
	if err != nil || instFilled == nil || instFilled.AccountLogin != "org" {
		t.Errorf("expected FindInstallationForOwner to fill AccountLogin, got: %v", instFilled)
	}
}

func TestGithubService_DeleteOperations(t *testing.T) {
	svc, ts := setupServiceTest(t)
	defer ts.Close()

	userID := uint(1)
	instID := int64(123)

	_ = svc.SaveInstallation(userID, instID, "org", "Organization", 999, "http://avatar")

	// DeleteUserInstallation (when no other user shares it)
	err := svc.DeleteUserInstallation(context.Background(), userID, instID)
	if err != nil {
		t.Fatalf("DeleteUserInstallation failed: %v", err)
	}

	// DeleteUserInstallation when another user shares it (otherCount > 0)
	_ = svc.SaveInstallation(userID, instID, "org", "Organization", 999, "http://avatar")
	_ = svc.SaveInstallation(2, instID, "org", "Organization", 999, "http://avatar")
	err = svc.DeleteUserInstallation(context.Background(), userID, instID)
	if err != nil {
		t.Fatalf("DeleteUserInstallation with otherCount failed: %v", err)
	}

	// DeleteUserInstallationByID
	_ = svc.SaveInstallation(userID, instID, "org", "Organization", 999, "http://avatar")
	err = svc.DeleteUserInstallationByID(context.Background(), userID, instID)
	if err != nil {
		t.Fatalf("DeleteUserInstallationByID failed: %v", err)
	}

	// DeleteUserInstallationByID with otherCount > 0
	_ = svc.SaveInstallation(userID, instID, "org", "Organization", 999, "http://avatar")
	_ = svc.SaveInstallation(2, instID, "org", "Organization", 999, "http://avatar")
	err = svc.DeleteUserInstallationByID(context.Background(), userID, instID)
	if err != nil {
		t.Fatalf("DeleteUserInstallationByID with otherCount failed: %v", err)
	}

	// DeleteInstallation 404 (handled gracefully)
	err = svc.DeleteInstallation(context.Background(), 404)
	if err != nil {
		t.Fatalf("DeleteInstallation 404 failed: %v", err)
	}

	// DeleteInstallation 500 (returns error)
	err = svc.DeleteInstallation(context.Background(), 500)
	if err == nil {
		t.Fatalf("expected error from DeleteInstallation on status 500")
	}

	// DeleteInstallation success
	_ = svc.SaveInstallation(userID, instID, "org", "Organization", 999, "http://avatar")
	err = svc.DeleteInstallation(context.Background(), instID)
	if err != nil {
		t.Fatalf("DeleteInstallation failed: %v", err)
	}
}

func TestGithubService_NilClientManagerBranches(t *testing.T) {
	db, _ := gorm.Open(sqlite.Open("file:gh_nil_memdb?mode=memory&cache=shared"), &gorm.Config{})
	_ = db.AutoMigrate(&dbEngine.GithubInstallation{})
	repo := repositories.NewGithubRepository(db)

	svcNil := NewGithubService(repo, nil)

	if _, err := svcNil.GetGithubInstallation(context.Background(), 123); err == nil {
		t.Errorf("expected error for nil ClientManager in GetGithubInstallation")
	}
	if _, err := svcNil.ListRepositories(context.Background(), 123); err == nil {
		t.Errorf("expected error for nil ClientManager in ListRepositories")
	}
	if _, err := svcNil.GetRepoTree(context.Background(), 123, "o", "r"); err == nil {
		t.Errorf("expected error for nil ClientManager in GetRepoTree")
	}
	if _, err := svcNil.GetInstallationToken(123); err == nil {
		t.Errorf("expected error for nil ClientManager in GetInstallationToken")
	}
	if _, err := svcNil.GetGithubUser(context.Background(), 123); err == nil {
		t.Errorf("expected error for nil ClientManager in GetGithubUser")
	}
	if _, err := svcNil.InstallationExists(context.Background(), 123); err == nil {
		t.Errorf("expected error for nil ClientManager in InstallationExists")
	}
	if err := svcNil.DeleteInstallation(context.Background(), 123); err == nil {
		t.Errorf("expected error for nil ClientManager in DeleteInstallation")
	}
	if _, err := svcNil.FindBuildPath(context.Background(), 123, "o", "r"); err == nil {
		t.Errorf("expected error for nil ClientManager in FindBuildPath")
	}
	if _, err := svcNil.DetectFlutterProject(context.Background(), 123, "o", "r"); err == nil {
		t.Errorf("expected error for nil ClientManager in DetectFlutterProject")
	}
}
