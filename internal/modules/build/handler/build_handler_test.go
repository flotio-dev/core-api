package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	dbEngine "github.com/flotio-dev/core-api/internal/common/database"
	buildModels "github.com/flotio-dev/core-api/internal/modules/build/model"
	githubRepo "github.com/flotio-dev/core-api/internal/modules/github/repository"
	githubServices "github.com/flotio-dev/core-api/internal/modules/github/service"
	userRepo "github.com/flotio-dev/core-api/internal/modules/user/repository"
	userServices "github.com/flotio-dev/core-api/internal/modules/user/service"
	"github.com/glebarez/sqlite"
	"github.com/gorilla/mux"
	"gorm.io/gorm"
)

func setupBuildTestEnv(t *testing.T) (*dbEngine.User, *BuildController, *httptest.Server) {
	t.Helper()

	// Mock k8s server
	mockK8s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/api/v1/namespaces/default/pods/build-") {
			if strings.HasSuffix(r.URL.Path, "-99") {
				http.NotFound(w, r)
				return
			}
			phase := "Running"
			if strings.HasSuffix(r.URL.Path, "-20") {
				phase = "Succeeded"
			} else if strings.HasSuffix(r.URL.Path, "-21") {
				phase = "Failed"
			} else if strings.HasSuffix(r.URL.Path, "-22") {
				phase = "Pending"
			}
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"metadata": map[string]string{"name": "build-pod"},
				"status":   map[string]string{"phase": phase},
			})
			return
		}
		if r.Method == "GET" && strings.Contains(r.URL.Path, "/api/v1/nodes") {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"items": []map[string]interface{}{
					{
						"status": map[string]interface{}{
							"allocatable": map[string]string{"memory": "16Gi"},
							"conditions":  []map[string]string{{"type": "Ready", "status": "True"}},
						},
					},
				},
			})
			return
		}
		if r.Method == "POST" {
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"metadata": map[string]string{"name": "res"}})
			return
		}
		if r.Method == "DELETE" {
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"status": "Success"})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"items": []interface{}{}})
	}))

	t.Setenv("KUBECTL_API", mockK8s.URL)
	t.Setenv("KUBECTL_TOKEN", "mock-token")
	t.Setenv("K8S_NAMESPACE", "default")

	testDB, err := gorm.Open(sqlite.Open("file:build_memdb?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	_ = testDB.AutoMigrate(
		&dbEngine.User{},
		&dbEngine.Project{},
		&dbEngine.ProjectConfig{},
		&dbEngine.Build{},
		&dbEngine.Log{},
		&dbEngine.Env{},
		&dbEngine.Keystore{},
		&dbEngine.GooglePlayCredentials{},
		&dbEngine.GithubInstallation{},
	)
	dbEngine.DB = testDB

	user := &dbEngine.User{
		Email:    "builder@example.com",
		Username: "builder",
	}
	user.ID = 1
	_ = testDB.Create(user)

	uRepo := userRepo.NewUserRepository(testDB)
	uSvc := userServices.NewUserService(uRepo)

	ctrl := NewBuildController(nil, uSvc)
	return user, ctrl, mockK8s
}

func makeAuthBuildReq(method, url string, body interface{}, userID uint) *http.Request {
	var bodyReader *bytes.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		bodyReader = bytes.NewReader(data)
	} else {
		bodyReader = bytes.NewReader([]byte{})
	}
	req := httptest.NewRequest(method, url, bodyReader)
	req.Header.Set("Content-Type", "application/json")
	if userID > 0 {
		ctx := context.WithValue(req.Context(), "user_id", userID)
		req = req.WithContext(ctx)
	}
	return req
}

func TestBuildController_Helpers(t *testing.T) {
	// stringPtrValue
	s := "  hello  "
	if stringPtrValue(&s) != "hello" {
		t.Errorf("expected 'hello', got %s", stringPtrValue(&s))
	}
	if stringPtrValue(nil) != "" {
		t.Errorf("expected empty string for nil ptr")
	}

	// hasProjectGitCredentials
	cfgNoCreds := dbEngine.ProjectConfig{}
	_, _, ok := hasProjectGitCredentials(cfgNoCreds)
	if ok {
		t.Errorf("expected false for empty creds")
	}
	cfgCreds := dbEngine.ProjectConfig{GitUsername: "u", GitToken: "t"}
	u, tok, ok := hasProjectGitCredentials(cfgCreds)
	if !ok || u != "u" || tok != "t" {
		t.Errorf("expected true with u/t creds")
	}

	// isGitHubHTTPSRepo
	if !isGitHubHTTPSRepo("https://github.com/org/repo") {
		t.Errorf("expected true for https github repo")
	}
	if isGitHubHTTPSRepo("git@github.com:org/repo.git") {
		t.Errorf("expected false for ssh repo")
	}

	// parseGitHubOwnerAndRepo
	owner, repo := parseGitHubOwnerAndRepo("https://github.com/flotio/core-api.git")
	if owner != "flotio" || repo != "core-api" {
		t.Errorf("parseGitHubOwnerAndRepo https failed: %s/%s", owner, repo)
	}
	owner, repo = parseGitHubOwnerAndRepo("git@github.com:flotio/core-api.git")
	if owner != "flotio" || repo != "core-api" {
		t.Errorf("parseGitHubOwnerAndRepo ssh failed: %s/%s", owner, repo)
	}
	owner, repo = parseGitHubOwnerAndRepo("invalid-repo")
	if owner != "" || repo != "" {
		t.Errorf("parseGitHubOwnerAndRepo invalid failed")
	}

	// sanitizeCacheFingerprint
	if sanitizeCacheFingerprint("bad/fingerprint#1") != "bad-fingerprint-1" {
		t.Errorf("sanitizeCacheFingerprint failed: %s", sanitizeCacheFingerprint("bad/fingerprint#1"))
	}

	// buildCacheNamespace
	ns := buildCacheNamespace(10, "feature/test")
	if ns != "project-10/feature-test" {
		t.Errorf("buildCacheNamespace failed: %s", ns)
	}
	ns = buildCacheNamespace(10, "")
	if ns != "project-10/default" {
		t.Errorf("buildCacheNamespace empty branch failed: %s", ns)
	}

	// parseCacheScopeFromQuery
	ns, fp, err := parseCacheScopeFromQuery(1, "main", "fp1")
	if err != nil || ns != "project-1/main" || fp != "fp1" {
		t.Errorf("parseCacheScopeFromQuery failed: %v, %s, %s", err, ns, fp)
	}
	if _, _, err := parseCacheScopeFromQuery(1, "", "fp1"); err == nil {
		t.Errorf("expected error for fingerprint without branch")
	}
	if parseBranchCacheNamespace(1, "") != "project-1" {
		t.Errorf("expected project-1 for empty branch")
	}

	// isPodBackedBuildStatus & buildHasMore
	if !isPodBackedBuildStatus("running") || !isPodBackedBuildStatus("pending") {
		t.Errorf("expected true for running/pending pod backed status")
	}
	if isPodBackedBuildStatus("success") || isPodBackedBuildStatus("cancelled") {
		t.Errorf("expected false for terminal status")
	}
	if !buildHasMore("running") || buildHasMore("success") || buildHasMore("failed") || buildHasMore("cancelled") {
		t.Errorf("buildHasMore check failed")
	}

	// normalizeBuildDefaults & normalizeBuildRequestDefaults
	b := &dbEngine.Build{}
	normalizeBuildDefaults(b)
	if b.Platform != "android" || b.BuildMode != "release" || b.BuildTarget != "apk" || b.FlutterChannel != "stable" || b.GitBranch != "main" {
		t.Errorf("normalizeBuildDefaults failed: %+v", b)
	}

	breq := &buildModels.BuildRequest{}
	normalizeBuildRequestDefaults(breq)
	if breq.Platform != "android" {
		t.Errorf("normalizeBuildRequestDefaults failed: %+v", breq)
	}

	// DTO conversion
	dto := convertDBBuild(*b)
	if dto.Platform != "android" {
		t.Errorf("convertDBBuild failed")
	}
	dtos := convertDBBuilds([]dbEngine.Build{*b})
	if len(dtos) != 1 {
		t.Errorf("convertDBBuilds failed")
	}
}

func TestBuildCancelAndDeleteHandlers(t *testing.T) {
	user, ctrl, ts := setupBuildTestEnv(t)
	defer ts.Close()

	proj := dbEngine.Project{Name: "Build Proj", UserID: user.ID}
	dbEngine.DB.Create(&proj)

	b1 := dbEngine.Build{ProjectID: proj.ID, Status: "running", Duration: 0}
	dbEngine.DB.Create(&b1)

	// 1. BuildCancelHandler - success
	req := makeAuthBuildReq("POST", fmt.Sprintf("/project/%d/build/%d/cancel", proj.ID, b1.ID), nil, user.ID)
	req = mux.SetURLVars(req, map[string]string{"id": fmt.Sprintf("%d", proj.ID), "buildId": fmt.Sprintf("%d", b1.ID)})
	w := httptest.NewRecorder()
	ctrl.BuildCancelHandler(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// 2. BuildCancelHandler - unauth, bad id, not found
	unauthReq := httptest.NewRequest("POST", "/test", nil)
	w = httptest.NewRecorder()
	ctrl.BuildCancelHandler(w, unauthReq)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}

	req = makeAuthBuildReq("POST", "/test", nil, user.ID)
	req = mux.SetURLVars(req, map[string]string{"id": "abc", "buildId": "1"})
	w = httptest.NewRecorder()
	ctrl.BuildCancelHandler(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}

	req = makeAuthBuildReq("POST", "/test", nil, user.ID)
	req = mux.SetURLVars(req, map[string]string{"id": "1", "buildId": "abc"})
	w = httptest.NewRecorder()
	ctrl.BuildCancelHandler(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}

	req = makeAuthBuildReq("POST", "/test", nil, user.ID)
	req = mux.SetURLVars(req, map[string]string{"id": "1", "buildId": "99999"})
	w = httptest.NewRecorder()
	ctrl.BuildCancelHandler(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}

	// 3. BuildDeleteHandler - success (with APKURL, running status, and logs)
	b2 := dbEngine.Build{ProjectID: proj.ID, Status: "running", APKURL: "flotio/builds/1/app.apk"}
	dbEngine.DB.Create(&b2)
	dbEngine.DB.Create(&dbEngine.Log{BuildID: b2.ID, Content: "line 1"})

	req = makeAuthBuildReq("DELETE", fmt.Sprintf("/project/%d/build/%d", proj.ID, b2.ID), nil, user.ID)
	req = mux.SetURLVars(req, map[string]string{"id": fmt.Sprintf("%d", proj.ID), "buildId": fmt.Sprintf("%d", b2.ID)})
	w = httptest.NewRecorder()
	ctrl.BuildDeleteHandler(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// 4. BuildDeleteHandler - unauth, bad id, not found
	w = httptest.NewRecorder()
	ctrl.BuildDeleteHandler(w, unauthReq)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}

	req = makeAuthBuildReq("DELETE", "/test", nil, user.ID)
	req = mux.SetURLVars(req, map[string]string{"id": "abc", "buildId": "1"})
	w = httptest.NewRecorder()
	ctrl.BuildDeleteHandler(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}

	req = makeAuthBuildReq("DELETE", "/test", nil, user.ID)
	req = mux.SetURLVars(req, map[string]string{"id": "1", "buildId": "abc"})
	w = httptest.NewRecorder()
	ctrl.BuildDeleteHandler(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}

	req = makeAuthBuildReq("DELETE", "/test", nil, user.ID)
	req = mux.SetURLVars(req, map[string]string{"id": "1", "buildId": "99999"})
	w = httptest.NewRecorder()
	ctrl.BuildDeleteHandler(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestBuildsListAndLogsHandlers(t *testing.T) {
	user, ctrl, ts := setupBuildTestEnv(t)
	defer ts.Close()

	proj := dbEngine.Project{Name: "List Proj", UserID: user.ID}
	dbEngine.DB.Create(&proj)

	b1 := dbEngine.Build{ProjectID: proj.ID, Status: "running"}
	b2 := dbEngine.Build{ProjectID: proj.ID, Status: "success"}
	dbEngine.DB.Create(&b1)
	dbEngine.DB.Create(&b2)

	// Add logs for b1
	dbEngine.DB.Create(&dbEngine.Log{BuildID: b1.ID, LineNumber: 1, Content: "log 1"})
	dbEngine.DB.Create(&dbEngine.Log{BuildID: b1.ID, LineNumber: 2, Content: "log 2"})

	// 1. BuildsListHandler - success
	req := makeAuthBuildReq("GET", fmt.Sprintf("/project/%d/builds", proj.ID), nil, user.ID)
	req = mux.SetURLVars(req, map[string]string{"id": fmt.Sprintf("%d", proj.ID)})
	w := httptest.NewRecorder()
	ctrl.BuildsListHandler(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	// 2. BuildsListHandler - unauth, invalid id
	unauthReq := httptest.NewRequest("GET", "/test", nil)
	w = httptest.NewRecorder()
	ctrl.BuildsListHandler(w, unauthReq)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}

	req = makeAuthBuildReq("GET", "/test", nil, user.ID)
	req = mux.SetURLVars(req, map[string]string{"id": "abc"})
	w = httptest.NewRecorder()
	ctrl.BuildsListHandler(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}

	// 3. BuildLogsHandler - success
	req = makeAuthBuildReq("GET", fmt.Sprintf("/project/%d/build/%d/logs", proj.ID, b1.ID), nil, user.ID)
	req = mux.SetURLVars(req, map[string]string{"id": fmt.Sprintf("%d", proj.ID), "buildId": fmt.Sprintf("%d", b1.ID)})
	w = httptest.NewRecorder()
	ctrl.BuildLogsHandler(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	// 4. BuildLogsHandler - unauth, bad id, not found
	w = httptest.NewRecorder()
	ctrl.BuildLogsHandler(w, unauthReq)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}

	req = makeAuthBuildReq("GET", "/test", nil, user.ID)
	req = mux.SetURLVars(req, map[string]string{"id": "abc", "buildId": "1"})
	w = httptest.NewRecorder()
	ctrl.BuildLogsHandler(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}

	req = makeAuthBuildReq("GET", "/test", nil, user.ID)
	req = mux.SetURLVars(req, map[string]string{"id": "1", "buildId": "abc"})
	w = httptest.NewRecorder()
	ctrl.BuildLogsHandler(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}

	req = makeAuthBuildReq("GET", "/test", nil, user.ID)
	req = mux.SetURLVars(req, map[string]string{"id": "1", "buildId": "99999"})
	w = httptest.NewRecorder()
	ctrl.BuildLogsHandler(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}

	// 5. BuildLogsSyncHandler - unauth, missing connectionId, completed build
	w = httptest.NewRecorder()
	ctrl.BuildLogsSyncHandler(w, unauthReq)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}

	req = makeAuthBuildReq("GET", fmt.Sprintf("/project/%d/build/%d/logs/sync", proj.ID, b2.ID), nil, user.ID)
	req = mux.SetURLVars(req, map[string]string{"id": fmt.Sprintf("%d", proj.ID), "buildId": fmt.Sprintf("%d", b2.ID)})
	w = httptest.NewRecorder()
	ctrl.BuildLogsSyncHandler(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing connectionId, got %d", w.Code)
	}

	// Completed build (b2 status is "success") with connectionId returns immediately
	req = makeAuthBuildReq("GET", fmt.Sprintf("/project/%d/build/%d/logs/sync?connectionId=c1&lastLine=0", proj.ID, b2.ID), nil, user.ID)
	req = mux.SetURLVars(req, map[string]string{"id": fmt.Sprintf("%d", proj.ID), "buildId": fmt.Sprintf("%d", b2.ID)})
	w = httptest.NewRecorder()
	ctrl.BuildLogsSyncHandler(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	// Running build with immediate new logs
	req = makeAuthBuildReq("GET", fmt.Sprintf("/project/%d/build/%d/logs/sync?connectionId=c2&lastLine=0", proj.ID, b1.ID), nil, user.ID)
	req = mux.SetURLVars(req, map[string]string{"id": fmt.Sprintf("%d", proj.ID), "buildId": fmt.Sprintf("%d", b1.ID)})
	w = httptest.NewRecorder()
	ctrl.BuildLogsSyncHandler(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestCacheAndDownloadAndTriggerHandlers(t *testing.T) {
	user, ctrl, ts := setupBuildTestEnv(t)
	defer ts.Close()

	proj := dbEngine.Project{Name: "Trigger Proj", UserID: user.ID}
	dbEngine.DB.Create(&proj)

	cfg := dbEngine.ProjectConfig{
		ProjectID:   proj.ID,
		GitRepo:     "https://gitlab.com/other/repo.git",
		GitUsername: "gituser",
		GitToken:    "gittoken",
	}
	dbEngine.DB.Create(&cfg)

	// 1. ProjectBuildHandler - unauth, bad id, not found
	unauthReq := httptest.NewRequest("POST", "/test", nil)
	w := httptest.NewRecorder()
	ctrl.ProjectBuildHandler(w, unauthReq)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}

	req := makeAuthBuildReq("POST", "/test", nil, user.ID)
	req = mux.SetURLVars(req, map[string]string{"id": "abc"})
	w = httptest.NewRecorder()
	ctrl.ProjectBuildHandler(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}

	req = makeAuthBuildReq("POST", "/test", nil, user.ID)
	req = mux.SetURLVars(req, map[string]string{"id": "99999"})
	w = httptest.NewRecorder()
	ctrl.ProjectBuildHandler(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}

	// 2. ProjectBuildHandler - success trigger
	buildReq := buildModels.BuildRequest{
		Platform:       "android",
		BuildMode:      "release",
		BuildTarget:    "apk",
		FlutterChannel: "stable",
		GitBranch:      "main",
	}
	req = makeAuthBuildReq("POST", fmt.Sprintf("/project/%d/build", proj.ID), buildReq, user.ID)
	req = mux.SetURLVars(req, map[string]string{"id": fmt.Sprintf("%d", proj.ID)})
	w = httptest.NewRecorder()
	ctrl.ProjectBuildHandler(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// 3. BuildDownloadHandler - unauth, not found, no artifact
	w = httptest.NewRecorder()
	ctrl.BuildDownloadHandler(w, unauthReq)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}

	bNoArtifact := dbEngine.Build{ProjectID: proj.ID, Status: "success", APKURL: ""}
	dbEngine.DB.Create(&bNoArtifact)

	req = makeAuthBuildReq("GET", fmt.Sprintf("/project/%d/build/%d/download", proj.ID, bNoArtifact.ID), nil, user.ID)
	req = mux.SetURLVars(req, map[string]string{"id": fmt.Sprintf("%d", proj.ID), "buildId": fmt.Sprintf("%d", bNoArtifact.ID)})
	w = httptest.NewRecorder()
	ctrl.BuildDownloadHandler(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for missing artifact, got %d", w.Code)
	}

	// 4. CachePurgeHandler, CacheMetricsHandler, CacheEntriesHandler - unauth & not found
	w = httptest.NewRecorder()
	ctrl.CachePurgeHandler(w, unauthReq)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}

	w = httptest.NewRecorder()
	ctrl.CacheMetricsHandler(w, unauthReq)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}

	w = httptest.NewRecorder()
	ctrl.CacheEntriesHandler(w, unauthReq)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}

	req = makeAuthBuildReq("DELETE", "/project/99999/cache", nil, user.ID)
	req = mux.SetURLVars(req, map[string]string{"id": "99999"})
	w = httptest.NewRecorder()
	ctrl.CachePurgeHandler(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}

	req = makeAuthBuildReq("GET", "/project/99999/cache/metrics", nil, user.ID)
	req = mux.SetURLVars(req, map[string]string{"id": "99999"})
	w = httptest.NewRecorder()
	ctrl.CacheMetricsHandler(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}

	req = makeAuthBuildReq("GET", "/project/99999/cache/entries", nil, user.ID)
	req = mux.SetURLVars(req, map[string]string{"id": "99999"})
	w = httptest.NewRecorder()
	ctrl.CacheEntriesHandler(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}

	// 5. ensureProjectOwnership
	err := ctrl.ensureProjectOwnership(user.ID, proj.ID)
	if err != nil {
		t.Errorf("ensureProjectOwnership failed: %v", err)
	}
	err = ctrl.ensureProjectOwnership(user.ID, 99999)
	if err == nil {
		t.Errorf("expected error for non-existent project")
	}

	// 6. resolveGitCredentials
	u, tok := ctrl.resolveGitCredentials(context.Background(), user.ID, cfg)
	if u != "gituser" || tok != "gittoken" {
		t.Errorf("resolveGitCredentials failed: %s, %s", u, tok)
	}
}

func TestBuildController_MoreCoverage(t *testing.T) {
	user, ctrl, ts := setupBuildTestEnv(t)
	defer ts.Close()

	// 1. defaultBuildTarget
	if defaultBuildTarget("android") != "apk" {
		t.Errorf("expected apk")
	}
	if defaultBuildTarget("ios") != "ios" {
		t.Errorf("expected ios")
	}
	if defaultBuildTarget("web") != "web" {
		t.Errorf("expected web")
	}
	if defaultBuildTarget("linux") != "linux" {
		t.Errorf("expected linux")
	}

	// 2. requireBranch
	if _, err := requireBranch(""); err == nil {
		t.Errorf("expected error for empty branch")
	}
	if b, err := requireBranch("  main  "); err != nil || b != "main" {
		t.Errorf("expected trimmed main branch, got %s", b)
	}

	// 3. parseBranchCacheNamespace
	bns := parseBranchCacheNamespace(5, "develop")
	if bns != "project-5/develop" {
		t.Errorf("expected project-5/develop, got %s", bns)
	}

	// 4. BuildDownloadHandler with mock S3
	t.Setenv("AWS_ACCESS_KEY_ID", "mock-key")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "mock-secret")
	t.Setenv("AWS_S3_ENDPOINT", "localhost:9000")
	t.Setenv("AWS_S3_BUCKET", "mock-bucket")

	proj := dbEngine.Project{Name: "More Proj", UserID: user.ID}
	dbEngine.DB.Create(&proj)

	bSuccess := dbEngine.Build{ProjectID: proj.ID, Status: "success", APKURL: "builds/1/app.apk"}
	dbEngine.DB.Create(&bSuccess)

	req := makeAuthBuildReq("GET", fmt.Sprintf("/project/%d/build/%d/download", proj.ID, bSuccess.ID), nil, user.ID)
	req = mux.SetURLVars(req, map[string]string{"id": fmt.Sprintf("%d", proj.ID), "buildId": fmt.Sprintf("%d", bSuccess.ID)})
	w := httptest.NewRecorder()
	ctrl.BuildDownloadHandler(w, req)
	if w.Code != http.StatusOK && w.Code != http.StatusInternalServerError {
		t.Errorf("expected 200 or 500, got %d: %s", w.Code, w.Body.String())
	}

	// Download invalid project / build ID
	req = makeAuthBuildReq("GET", "/project/abc/build/1/download", nil, user.ID)
	req = mux.SetURLVars(req, map[string]string{"id": "abc", "buildId": "1"})
	w = httptest.NewRecorder()
	ctrl.BuildDownloadHandler(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}

	req = makeAuthBuildReq("GET", "/project/1/build/abc/download", nil, user.ID)
	req = mux.SetURLVars(req, map[string]string{"id": "1", "buildId": "abc"})
	w = httptest.NewRecorder()
	ctrl.BuildDownloadHandler(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}

	// 5. CacheEntriesHandler missing branch
	req = makeAuthBuildReq("GET", fmt.Sprintf("/project/%d/cache/entries", proj.ID), nil, user.ID)
	req = mux.SetURLVars(req, map[string]string{"id": fmt.Sprintf("%d", proj.ID)})
	w = httptest.NewRecorder()
	ctrl.CacheEntriesHandler(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing branch, got %d", w.Code)
	}

	req = makeAuthBuildReq("GET", fmt.Sprintf("/project/%d/cache/entries?branch=main", proj.ID), nil, user.ID)
	req = mux.SetURLVars(req, map[string]string{"id": fmt.Sprintf("%d", proj.ID)})
	w = httptest.NewRecorder()
	ctrl.CacheEntriesHandler(w, req)
	if w.Code != http.StatusOK && w.Code != http.StatusInternalServerError {
		t.Errorf("expected 200 or 500, got %d", w.Code)
	}

	// 6. CacheMetricsHandler missing/invalid branch & scope
	req = makeAuthBuildReq("GET", fmt.Sprintf("/project/%d/cache/metrics", proj.ID), nil, user.ID)
	req = mux.SetURLVars(req, map[string]string{"id": fmt.Sprintf("%d", proj.ID)})
	w = httptest.NewRecorder()
	ctrl.CacheMetricsHandler(w, req)
	// Even if s3 fails, it executes through to S3 call
	if w.Code != http.StatusOK && w.Code != http.StatusInternalServerError {
		t.Errorf("expected 200 or 500, got %d", w.Code)
	}

	// 7. CachePurgeHandler with query params
	req = makeAuthBuildReq("DELETE", fmt.Sprintf("/project/%d/cache?branch=main&fingerprint=fp1", proj.ID), nil, user.ID)
	req = mux.SetURLVars(req, map[string]string{"id": fmt.Sprintf("%d", proj.ID)})
	w = httptest.NewRecorder()
	ctrl.CachePurgeHandler(w, req)
	if w.Code != http.StatusOK && w.Code != http.StatusInternalServerError {
		t.Errorf("expected 200 or 500, got %d", w.Code)
	}

	// 8. startBuildPod with no project config in DB
	projNoConfig := dbEngine.Project{Name: "No Config Proj", UserID: user.ID}
	dbEngine.DB.Create(&projNoConfig)
	bTemp := dbEngine.Build{ProjectID: projNoConfig.ID}
	err := ctrl.startBuildPod(context.Background(), &bTemp, projNoConfig, user.ID)
	if err == nil {
		t.Errorf("expected error when project config missing")
	}

	// 8b. ProjectBuildHandler with default body and on projNoConfig
	req = makeAuthBuildReq("POST", fmt.Sprintf("/project/%d/build", projNoConfig.ID), nil, user.ID)
	req = mux.SetURLVars(req, map[string]string{"id": fmt.Sprintf("%d", projNoConfig.ID)})
	w = httptest.NewRecorder()
	ctrl.ProjectBuildHandler(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 for missing config, got %d", w.Code)
	}

	// 9. resolveGitCredentials with no credentials
	noUser, noTok := ctrl.resolveGitCredentials(context.Background(), user.ID, dbEngine.ProjectConfig{GitRepo: "https://github.com/foo/bar"})
	if noUser != "" || noTok != "" {
		t.Errorf("expected empty creds for nil githubService, got %s, %s", noUser, noTok)
	}
}

func TestBuildController_StatusReconciliationAndQueue(t *testing.T) {
	user, ctrl, ts := setupBuildTestEnv(t)
	defer ts.Close()

	proj := dbEngine.Project{Name: "Reconcile Proj", UserID: user.ID}
	dbEngine.DB.Create(&proj)

	cfg := dbEngine.ProjectConfig{
		ProjectID: proj.ID,
		GitRepo:   "https://github.com/testorg/testrepo",
	}
	dbEngine.DB.Create(&cfg)

	// Create builds with specific IDs matching the mock k8s server rules
	b20 := dbEngine.Build{ProjectID: proj.ID, Status: "running"}
	b20.ID = 20
	dbEngine.DB.Create(&b20)

	b21 := dbEngine.Build{ProjectID: proj.ID, Status: "running"}
	b21.ID = 21
	dbEngine.DB.Create(&b21)

	b22 := dbEngine.Build{ProjectID: proj.ID, Status: "running"}
	b22.ID = 22
	dbEngine.DB.Create(&b22)

	b99 := dbEngine.Build{ProjectID: proj.ID, Status: "running"}
	b99.ID = 99
	dbEngine.DB.Create(&b99)

	// 1. BuildsListHandler: will reconcile b20 to success, b21 to failed, b22 to pending, b99 to failed
	req := makeAuthBuildReq("GET", fmt.Sprintf("/project/%d/builds", proj.ID), nil, user.ID)
	req = mux.SetURLVars(req, map[string]string{"id": fmt.Sprintf("%d", proj.ID)})
	w := httptest.NewRecorder()
	ctrl.BuildsListHandler(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// 2. BuildLogsSyncHandler timeout path
	origTimeout := syncPollingTimeout
	origInterval := syncPollingInterval
	defer func() {
		syncPollingTimeout = origTimeout
		syncPollingInterval = origInterval
	}()
	syncPollingTimeout = 10 * time.Millisecond
	syncPollingInterval = 50 * time.Millisecond

	// b22 is pending (pod backed status), will hit case <-timeout and execute timeout logic
	req = makeAuthBuildReq("GET", fmt.Sprintf("/project/%d/build/%d/logs/sync?connectionId=conn-t1&lastLine=0", proj.ID, b22.ID), nil, user.ID)
	req = mux.SetURLVars(req, map[string]string{"id": fmt.Sprintf("%d", proj.ID), "buildId": fmt.Sprintf("%d", b22.ID)})
	w = httptest.NewRecorder()
	ctrl.BuildLogsSyncHandler(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 on sync timeout, got %d: %s", w.Code, w.Body.String())
	}

	// 3. Queue processing: enableBuildCapacityQueue = true
	origQueue := enableBuildCapacityQueue
	defer func() { enableBuildCapacityQueue = origQueue }()
	enableBuildCapacityQueue = true

	bQueue := dbEngine.Build{ProjectID: proj.ID}
	dbEngine.DB.Create(&bQueue)
	_ = ctrl.startBuildOrQueue(context.Background(), &bQueue, proj, user.ID)

	waitingBuild := dbEngine.Build{ProjectID: proj.ID, Status: "waiting"}
	dbEngine.DB.Create(&waitingBuild)

	// Call processWaitingBuildQueue and triggerWaitingBuildProcessing
	ctrl.processWaitingBuildQueue()
	ctrl.triggerWaitingBuildProcessing()

	// 4. resolveGitCredentials with githubService
	ghRepository := githubRepo.NewGithubRepository(dbEngine.DB)
	ghService := githubServices.NewGithubService(ghRepository, nil)
	ctrl.githubService = ghService

	dbEngine.DB.Create(&dbEngine.GithubInstallation{
		UserID:         &user.ID,
		InstallationID: 12345,
	})

	u, tok := ctrl.resolveGitCredentials(context.Background(), user.ID, cfg)
	// Since ClientManager is nil, tokenErr != nil, falls back to empty creds
	if u != "" || tok != "" {
		t.Errorf("expected empty token fallback, got %s, %s", u, tok)
	}

	// 5. Test BuildCancelHandler with duration already != 0
	bCancelDuration := dbEngine.Build{
		ProjectID: proj.ID,
		Status:    "running",
		Duration:  55,
	}
	dbEngine.DB.Create(&bCancelDuration)
	req = makeAuthBuildReq("POST", fmt.Sprintf("/project/%d/build/%d/cancel", proj.ID, bCancelDuration.ID), nil, user.ID)
	req = mux.SetURLVars(req, map[string]string{"id": fmt.Sprintf("%d", proj.ID), "buildId": fmt.Sprintf("%d", bCancelDuration.ID)})
	w = httptest.NewRecorder()
	ctrl.BuildCancelHandler(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 on cancel, got %d: %s", w.Code, w.Body.String())
	}

	// 6. Test BuildDeleteHandler with pending build (tests pod cleanup when pending)
	bPending := dbEngine.Build{
		ProjectID: proj.ID,
		Status:    "pending",
		APKURL:    "artifacts/app.apk",
	}
	dbEngine.DB.Create(&bPending)
	req = makeAuthBuildReq("DELETE", fmt.Sprintf("/project/%d/build/%d", proj.ID, bPending.ID), nil, user.ID)
	req = mux.SetURLVars(req, map[string]string{"id": fmt.Sprintf("%d", proj.ID), "buildId": fmt.Sprintf("%d", bPending.ID)})
	w = httptest.NewRecorder()
	ctrl.BuildDeleteHandler(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 on delete pending, got %d: %s", w.Code, w.Body.String())
	}

	// 7. Test startWaitingBuildScheduler directly (when queue disabled returns immediately, when enabled runs tick)
	origInt := waitingBuildSchedulerInterval
	waitingBuildSchedulerInterval = 1 * time.Millisecond
	defer func() { waitingBuildSchedulerInterval = origInt }()

	doneCh := make(chan struct{})
	go func() {
		ctrl.startWaitingBuildScheduler()
		close(doneCh)
	}()
	time.Sleep(5 * time.Millisecond)
	enableBuildCapacityQueue = false
	// calling again when false returns immediately
	ctrl.startWaitingBuildScheduler()
}
