package handler

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	dbEngine "github.com/flotio-dev/core-api/internal/common/database"
	helpers "github.com/flotio-dev/core-api/internal/common/server"
	githubEngine "github.com/flotio-dev/core-api/internal/infra/github"
	models "github.com/flotio-dev/core-api/internal/models"
	githubRepo "github.com/flotio-dev/core-api/internal/modules/github/repository"
	githubService "github.com/flotio-dev/core-api/internal/modules/github/service"
	userRepo "github.com/flotio-dev/core-api/internal/modules/user/repository"
	userServices "github.com/flotio-dev/core-api/internal/modules/user/service"
	"github.com/glebarez/sqlite"
	"github.com/google/go-github/v79/github"
	"gorm.io/gorm"
)

var dbSeq int64

func setupHandlerTestEnv(t *testing.T) (*GithubController, *dbEngine.User, *httptest.Server, *gorm.DB) {
	t.Helper()

	seq := atomic.AddInt64(&dbSeq, 1)
	dbName := fmt.Sprintf("file:gh_handler_memdb_%d?mode=memory&cache=shared", seq)
	db, err := gorm.Open(sqlite.Open(dbName), &gorm.Config{})
	if err != nil {
		t.Fatalf("sqlite open error: %v", err)
	}

	_ = db.AutoMigrate(&dbEngine.User{}, &dbEngine.GithubInstallation{})

	user := &dbEngine.User{
		Username: fmt.Sprintf("gh_user_%d", seq),
		Email:    fmt.Sprintf("gh_user_%d@example.com", seq),
	}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	uRepo := userRepo.NewUserRepository(db)
	uSvc := userServices.NewUserService(uRepo)

	gRepo := githubRepo.NewGithubRepository(db)

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
			fmt.Fprintf(w, `{"token":"tok123"}`)
		case "/installation/repositories":
			fmt.Fprintf(w, `{"total_count":3,"repositories":[
				{"id":101,"name":"flutter_dart","full_name":"org/flutter_dart","language":"Dart","default_branch":"main","private":false,"owner":{"login":"org"}},
				{"id":102,"name":"flutter_topic","full_name":"org/flutter_topic","language":"Other","topics":["flutter"],"default_branch":"main","private":false,"owner":{"login":"org"}},
				{"id":103,"name":"other_app","full_name":"org/other_app","language":"Python","default_branch":"main","private":true,"owner":{"login":"org"}}
			]}`)
		case "/app/installations/123":
			if r.Method == http.MethodDelete {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			fmt.Fprintf(w, `{"id":123,"account":{"login":"org","type":"Organization","id":999,"avatar_url":"http://avatar.com/123"}}`)
		case "/app/installations/404":
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprintf(w, `{"message":"Not Found"}`)
		case "/app/installations/502":
			w.WriteHeader(http.StatusBadGateway)
			fmt.Fprintf(w, `{"message":"Bad Gateway"}`)
		case "/repos/org/flutter_dart/contents/":
			fmt.Fprintf(w, `[{"type":"file","name":"pubspec.yaml","path":"pubspec.yaml"}]`)
		case "/repos/org/flutter_dart/contents/pubspec.yaml":
			content := base64.StdEncoding.EncodeToString([]byte("name: flutter_dart\nflutter:\n  sdk: flutter\n  version: 3.19.0\n"))
			fmt.Fprintf(w, `{"type":"file","name":"pubspec.yaml","path":"pubspec.yaml","content":"%s","encoding":"base64"}`, content)
		case "/repos/org/error_repo/contents/":
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, `{"message":"repo error"}`)
		case "/repos/org/flutter_dart":
			fmt.Fprintf(w, `{"id":101,"name":"flutter_dart","full_name":"org/flutter_dart"}`)
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

	cleanup := githubService.SetClientGettersForTest(
		func(m *githubEngine.GitHubClientManager) (*github.Client, error) {
			if m == nil {
				return nil, fmt.Errorf("ClientManager not initialized")
			}
			return mockGhClient, nil
		},
		func(m *githubEngine.GitHubClientManager, id int64) (*github.Client, error) {
			if m == nil {
				return nil, fmt.Errorf("ClientManager not initialized")
			}
			return mockGhClient, nil
		},
	)
	t.Cleanup(cleanup)

	gSvc := githubService.NewGithubService(gRepo, mgr)

	ctrl := NewGithubController(gSvc, uSvc)
	return ctrl, user, ts, db
}

func makeAuthRequest(method, urlStr string, body []byte, userID uint) *http.Request {
	req := httptest.NewRequest(method, urlStr, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if userID > 0 {
		ctx := context.WithValue(req.Context(), "user_id", userID)
		req = req.WithContext(ctx)
	}
	return req
}

func assertErrorStatus(t *testing.T, rec *httptest.ResponseRecorder, expectedStatus helpers.StatusType) {
	t.Helper()
	var resp models.APIErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode error response: %v, body: %s", err, rec.Body.String())
	}
	if resp.Status != string(expectedStatus) {
		t.Errorf("expected status %s, got %s", expectedStatus, resp.Status)
	}
}

func TestGithubController_HandleGithubPostInstallation(t *testing.T) {
	ctrl, user, ts, db := setupHandlerTestEnv(t)
	defer ts.Close()

	// 1. Unauthenticated -> StatusUnauthorized
	{
		req := makeAuthRequest(http.MethodPost, "/github/post-installation", []byte(`{"installation_id":123}`), 0)
		rec := httptest.NewRecorder()
		ctrl.HandleGithubPostInstallation(rec, req)
		assertErrorStatus(t, rec, helpers.StatusUnauthorized)
	}

	// 2. Method not allowed (GET) -> 405
	{
		req := makeAuthRequest(http.MethodGet, "/github/post-installation", nil, user.ID)
		rec := httptest.NewRecorder()
		ctrl.HandleGithubPostInstallation(rec, req)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("expected 405, got %d", rec.Code)
		}
		assertErrorStatus(t, rec, helpers.StatusMethodNotAllowed)
	}

	// 3. Invalid payload (bad json or installation_id == 0) -> 400
	{
		req := makeAuthRequest(http.MethodPost, "/github/post-installation", []byte(`invalid json`), user.ID)
		rec := httptest.NewRecorder()
		ctrl.HandleGithubPostInstallation(rec, req)
		assertErrorStatus(t, rec, helpers.StatusInvalidArgs)

		req0 := makeAuthRequest(http.MethodPost, "/github/post-installation", []byte(`{"installation_id":0}`), user.ID)
		rec0 := httptest.NewRecorder()
		ctrl.HandleGithubPostInstallation(rec0, req0)
		assertErrorStatus(t, rec0, helpers.StatusInvalidArgs)
	}

	// 4. Success -> 200
	{
		req := makeAuthRequest(http.MethodPost, "/github/post-installation", []byte(`{"installation_id":123}`), user.ID)
		rec := httptest.NewRecorder()
		ctrl.HandleGithubPostInstallation(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}

		var inst dbEngine.GithubInstallation
		if err := db.Where("user_id = ? AND installation_id = ?", user.ID, 123).First(&inst).Error; err != nil {
			t.Errorf("installation was not saved in DB: %v", err)
		}
	}
}

func TestGithubController_HandleGithubGetRepositories(t *testing.T) {
	ctrl, user, ts, _ := setupHandlerTestEnv(t)
	defer ts.Close()

	// 1. Unauthenticated -> 401
	{
		req := makeAuthRequest(http.MethodGet, "/github/repos", nil, 0)
		rec := httptest.NewRecorder()
		ctrl.HandleGithubGetRepositories(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", rec.Code)
		}
	}

	// 2. User has no installations -> 404
	{
		req := makeAuthRequest(http.MethodGet, "/github/repos", nil, user.ID)
		rec := httptest.NewRecorder()
		ctrl.HandleGithubGetRepositories(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Errorf("expected 404, got %d", rec.Code)
		}
	}

	// Save an installation
	_ = ctrl.Service.SaveInstallation(user.ID, 123, "org", "Organization", 999, "http://avatar")

	// 3. Success all repos
	{
		req := makeAuthRequest(http.MethodGet, "/github/repos", nil, user.ID)
		rec := httptest.NewRecorder()
		ctrl.HandleGithubGetRepositories(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
	}

	// 4. Filter flutter_only=true
	{
		req := makeAuthRequest(http.MethodGet, "/github/repos?flutter_only=true", nil, user.ID)
		rec := httptest.NewRecorder()
		ctrl.HandleGithubGetRepositories(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rec.Code)
		}
	}

	// 5. Filter owner & installation_id
	{
		req := makeAuthRequest(http.MethodGet, "/github/repos?owner=org&installation_id=123", nil, user.ID)
		rec := httptest.NewRecorder()
		ctrl.HandleGithubGetRepositories(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rec.Code)
		}

		// Non-matching owner
		reqMismatch := makeAuthRequest(http.MethodGet, "/github/repos?owner=nomatch", nil, user.ID)
		recMismatch := httptest.NewRecorder()
		ctrl.HandleGithubGetRepositories(recMismatch, reqMismatch)
		if recMismatch.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", recMismatch.Code)
		}

		// Non-matching installation_id
		reqMismatchInst := makeAuthRequest(http.MethodGet, "/github/repos?installation_id=9999", nil, user.ID)
		recMismatchInst := httptest.NewRecorder()
		ctrl.HandleGithubGetRepositories(recMismatchInst, reqMismatchInst)
		if recMismatchInst.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", recMismatchInst.Code)
		}
	}
}

func TestGithubController_HandleGithubRepoTree(t *testing.T) {
	ctrl, user, ts, _ := setupHandlerTestEnv(t)
	defer ts.Close()

	// 1. Unauthenticated -> Unauthorized
	{
		req := makeAuthRequest(http.MethodGet, "/github/repo?owner=org&repo=flutter_dart", nil, 0)
		rec := httptest.NewRecorder()
		ctrl.HandleGithubRepoTree(rec, req)
		assertErrorStatus(t, rec, helpers.StatusUnauthorized)
	}

	// 2. Missing owner or repo -> 400
	{
		req := makeAuthRequest(http.MethodGet, "/github/repo", nil, user.ID)
		rec := httptest.NewRecorder()
		ctrl.HandleGithubRepoTree(rec, req)
		assertErrorStatus(t, rec, helpers.StatusInvalidArgs)
	}

	// 3. Installation not found -> 404
	{
		req := makeAuthRequest(http.MethodGet, "/github/repo?owner=org&repo=flutter_dart", nil, user.ID)
		rec := httptest.NewRecorder()
		ctrl.HandleGithubRepoTree(rec, req)
		assertErrorStatus(t, rec, helpers.StatusNotFound)
	}

	// Save installation
	_ = ctrl.Service.SaveInstallation(user.ID, 123, "org", "Organization", 999, "http://avatar")

	// 4. Repo error -> 400
	{
		req := makeAuthRequest(http.MethodGet, "/github/repo?owner=org&repo=error_repo", nil, user.ID)
		rec := httptest.NewRecorder()
		ctrl.HandleGithubRepoTree(rec, req)
		assertErrorStatus(t, rec, helpers.StatusBadRequest)
	}

	// 5. Success -> 200
	{
		req := makeAuthRequest(http.MethodGet, "/github/repo?owner=org&repo=flutter_dart", nil, user.ID)
		rec := httptest.NewRecorder()
		ctrl.HandleGithubRepoTree(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
	}
}

func TestGithubController_HandleGithubCheckInstallation(t *testing.T) {
	ctrl, user, ts, _ := setupHandlerTestEnv(t)
	defer ts.Close()

	// 1. Unauthenticated -> StatusUnauthorized
	{
		req := makeAuthRequest(http.MethodGet, "/github/installations", nil, 0)
		rec := httptest.NewRecorder()
		ctrl.HandleGithubCheckInstallation(rec, req)
		assertErrorStatus(t, rec, helpers.StatusUnauthorized)
	}

	// 2. No installations -> StatusNotFound
	{
		req := makeAuthRequest(http.MethodGet, "/github/installations", nil, user.ID)
		rec := httptest.NewRecorder()
		ctrl.HandleGithubCheckInstallation(rec, req)
		assertErrorStatus(t, rec, helpers.StatusNotFound)
	}

	_ = ctrl.Service.SaveInstallation(user.ID, 123, "org", "Organization", 999, "http://avatar")

	// 3. all=true -> 200
	{
		req := makeAuthRequest(http.MethodGet, "/github/installations?all=true", nil, user.ID)
		rec := httptest.NewRecorder()
		ctrl.HandleGithubCheckInstallation(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rec.Code)
		}
	}

	// 4. all=false, installation exists -> 200
	{
		req := makeAuthRequest(http.MethodGet, "/github/installations", nil, user.ID)
		rec := httptest.NewRecorder()
		ctrl.HandleGithubCheckInstallation(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
	}

	// 5. all=false, installation does not exist on github (404) -> StatusNotFound
	{
		_ = ctrl.Service.DeleteInstallationByInstallationID(123)
		_ = ctrl.Service.SaveInstallation(user.ID, 404, "org404", "Organization", 999, "http://avatar")
		req := makeAuthRequest(http.MethodGet, "/github/installations", nil, user.ID)
		rec := httptest.NewRecorder()
		ctrl.HandleGithubCheckInstallation(rec, req)
		assertErrorStatus(t, rec, helpers.StatusNotFound)
	}

	// 6. all=false, github api error (502) -> StatusBadGateway
	{
		_ = ctrl.Service.DeleteInstallationByInstallationID(404)
		_ = ctrl.Service.SaveInstallation(user.ID, 502, "org502", "Organization", 999, "http://avatar")
		req := makeAuthRequest(http.MethodGet, "/github/installations", nil, user.ID)
		rec := httptest.NewRecorder()
		ctrl.HandleGithubCheckInstallation(rec, req)
		assertErrorStatus(t, rec, helpers.StatusBadGateway)
	}
}

func TestGithubController_HandleGetGithubInstallation(t *testing.T) {
	ctrl, user, ts, _ := setupHandlerTestEnv(t)
	defer ts.Close()

	// 1. Unauthenticated -> StatusUnauthorized
	{
		req := makeAuthRequest(http.MethodGet, "/github/installation?installation_id=123", nil, 0)
		rec := httptest.NewRecorder()
		ctrl.HandleGetGithubInstallation(rec, req)
		assertErrorStatus(t, rec, helpers.StatusUnauthorized)
	}

	// 2. Missing installation_id -> StatusInvalidArgs
	{
		req := makeAuthRequest(http.MethodGet, "/github/installation", nil, user.ID)
		rec := httptest.NewRecorder()
		ctrl.HandleGetGithubInstallation(rec, req)
		assertErrorStatus(t, rec, helpers.StatusInvalidArgs)
	}

	// 3. Invalid installation_id -> StatusInvalidArgs
	{
		req := makeAuthRequest(http.MethodGet, "/github/installation?installation_id=abc", nil, user.ID)
		rec := httptest.NewRecorder()
		ctrl.HandleGetGithubInstallation(rec, req)
		assertErrorStatus(t, rec, helpers.StatusInvalidArgs)
	}

	// 4. Installation not found in db -> StatusNotFound
	{
		req := makeAuthRequest(http.MethodGet, "/github/installation?installation_id=999", nil, user.ID)
		rec := httptest.NewRecorder()
		ctrl.HandleGetGithubInstallation(rec, req)
		assertErrorStatus(t, rec, helpers.StatusNotFound)
	}

	_ = ctrl.Service.SaveInstallation(user.ID, 123, "org123", "Organization", 999, "http://avatar")
	_ = ctrl.Service.SaveInstallation(user.ID, 404, "org404", "Organization", 999, "http://avatar")
	_ = ctrl.Service.SaveInstallation(user.ID, 502, "org502", "Organization", 999, "http://avatar")

	// 5. Installation exists -> 200
	{
		req := makeAuthRequest(http.MethodGet, "/github/installation?installation_id=123", nil, user.ID)
		rec := httptest.NewRecorder()
		ctrl.HandleGetGithubInstallation(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
	}

	// 6. Installation missing on github (404) -> StatusNotFound
	{
		req := makeAuthRequest(http.MethodGet, "/github/installation?installation_id=404", nil, user.ID)
		rec := httptest.NewRecorder()
		ctrl.HandleGetGithubInstallation(rec, req)
		assertErrorStatus(t, rec, helpers.StatusNotFound)
	}

	// 7. InstallationExists error -> StatusBadGateway
	{
		req := makeAuthRequest(http.MethodGet, "/github/installation?installation_id=502", nil, user.ID)
		rec := httptest.NewRecorder()
		ctrl.HandleGetGithubInstallation(rec, req)
		assertErrorStatus(t, rec, helpers.StatusBadGateway)
	}
}

func TestGithubController_HandleDisconnectGithub(t *testing.T) {
	ctrl, user, ts, _ := setupHandlerTestEnv(t)
	defer ts.Close()

	// 1. Unauthenticated -> StatusUnauthorized
	{
		req := makeAuthRequest(http.MethodDelete, "/github/disconnect", nil, 0)
		rec := httptest.NewRecorder()
		ctrl.HandleDisconnectGithub(rec, req)
		assertErrorStatus(t, rec, helpers.StatusUnauthorized)
	}

	// 2. Specific installation_id invalid -> StatusInvalidArgs
	{
		req := makeAuthRequest(http.MethodDelete, "/github/disconnect?installation_id=invalid", nil, user.ID)
		rec := httptest.NewRecorder()
		ctrl.HandleDisconnectGithub(rec, req)
		assertErrorStatus(t, rec, helpers.StatusInvalidArgs)
	}

	// 3. Without installation_id, no installations -> StatusNotFound
	{
		req := makeAuthRequest(http.MethodDelete, "/github/disconnect", nil, user.ID)
		rec := httptest.NewRecorder()
		ctrl.HandleDisconnectGithub(rec, req)
		assertErrorStatus(t, rec, helpers.StatusNotFound)
	}

	// 4. Disconnect specific installation -> 200
	{
		_ = ctrl.Service.SaveInstallation(user.ID, 123, "org", "Organization", 999, "http://avatar")
		req := makeAuthRequest(http.MethodDelete, "/github/disconnect?installation_id=123", nil, user.ID)
		rec := httptest.NewRecorder()
		ctrl.HandleDisconnectGithub(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
	}

	// 5. Disconnect all installations -> 200
	{
		_ = ctrl.Service.SaveInstallation(user.ID, 123, "org", "Organization", 999, "http://avatar")
		_ = ctrl.Service.SaveInstallation(user.ID, 456, "org", "Organization", 999, "http://avatar")
		req := makeAuthRequest(http.MethodDelete, "/github/disconnect", nil, user.ID)
		rec := httptest.NewRecorder()
		ctrl.HandleDisconnectGithub(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
	}
}
