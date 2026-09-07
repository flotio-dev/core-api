package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/gorilla/mux"
	"gorm.io/gorm"

	"github.com/flotio-dev/core-api/internal/common/crypto"
	dbEngine "github.com/flotio-dev/core-api/internal/common/database"
	googleplay "github.com/flotio-dev/core-api/internal/infra/googleplay"
	models "github.com/flotio-dev/core-api/internal/modules/release/model"
	userRepo "github.com/flotio-dev/core-api/internal/modules/user/repository"
	userServices "github.com/flotio-dev/core-api/internal/modules/user/service"
)

func setupReleaseTestEnv(t *testing.T) (*dbEngine.User, *ReleaseController, string) {
	t.Helper()
	t.Setenv("S3_ENDPOINT", "http://localhost:9000")
	t.Setenv("S3_ACCESS_KEY", "minioadmin")
	t.Setenv("S3_SECRET_KEY", "minioadmin")
	t.Setenv("S3_BUCKET", "flotio-artifacts")
	t.Setenv("S3_USE_SSL", "false")
	t.Setenv("SECRETS_ENCRYPTION_KEY", "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=") // 32 bytes base64

	dbName := fmt.Sprintf("file:release_memdb_%d?mode=memory&cache=shared", time.Now().UnixNano())
	testDB, err := gorm.Open(sqlite.Open(dbName), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}

	_ = testDB.AutoMigrate(
		&dbEngine.User{},
		&dbEngine.Project{},
		&dbEngine.ProjectConfig{},
		&dbEngine.Build{},
		&dbEngine.GooglePlayCredentials{},
		&dbEngine.Release{},
		&dbEngine.ReleaseAudit{},
	)
	dbEngine.DB = testDB

	user := &dbEngine.User{
		Email:    fmt.Sprintf("releaser_%d@example.com", time.Now().UnixNano()),
		Username: "releaser",
	}
	_ = testDB.Create(user)

	uRepo := userRepo.NewUserRepository(testDB)
	uSvc := userServices.NewUserService(uRepo)
	ctrl := NewReleaseController(uSvc)

	// Valid mock service account JSON encrypted
	saJSON := `{"type":"service_account","project_id":"p1","private_key_id":"k1","private_key":"-----BEGIN PRIVATE KEY-----\nMIIEvgIBADANBgkqhkiG9w0BAQEFAASCBKgwggSkAgEAAoIBAQC7\n-----END PRIVATE KEY-----\n","client_email":"sa@p1.iam.gserviceaccount.com","client_id":"123","auth_uri":"https://accounts.google.com/o/oauth2/auth","token_uri":"https://oauth2.googleapis.com/token"}`
	encCreds, err := crypto.Encrypt(saJSON)
	if err != nil {
		t.Fatalf("failed to encrypt SA json: %v", err)
	}

	return user, ctrl, encCreds
}

func makeAuthReleaseReq(method, url string, body interface{}, userID uint) *http.Request {
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

func TestReleaseController_Helpers(t *testing.T) {
	// firstNonEmpty
	if firstNonEmpty("", "a", "b") != "a" {
		t.Errorf("expected 'a', got '%s'", firstNonEmpty("", "a", "b"))
	}
	if firstNonEmpty("", "") != "" {
		t.Errorf("expected empty string")
	}

	// mapPublishStatus
	if mapPublishStatus("inProgress") != statusInProgress {
		t.Errorf("expected %s, got %s", statusInProgress, mapPublishStatus("inProgress"))
	}
	if mapPublishStatus("draft") != statusDraft {
		t.Errorf("expected %s, got %s", statusDraft, mapPublishStatus("draft"))
	}
	if mapPublishStatus("completed") != statusPublished {
		t.Errorf("expected %s, got %s", statusPublished, mapPublishStatus("completed"))
	}

	// parsePublishRequest with nil/empty body
	reqNil := httptest.NewRequest("POST", "/", nil)
	parsedNil := parsePublishRequest(reqNil)
	if parsedNil.Track != "" {
		t.Errorf("expected empty track for nil body")
	}

	reqBad := httptest.NewRequest("POST", "/", bytes.NewReader([]byte("{invalid-json")))
	parsedBad := parsePublishRequest(reqBad)
	if parsedBad.Track != "" {
		t.Errorf("expected empty track for invalid json")
	}

	// convertDBRelease and convertDBReleases
	now := time.Now()
	r := dbEngine.Release{
		ProjectID:       10,
		BuildID:         20,
		VersionName:     "1.0.0",
		VersionCode:     100,
		Track:           "internal",
		RolloutFraction: 0.5,
		Status:          statusPending,
		ReleaseNotes:    "First release",
	}
	r.ID = 42
	r.CreatedAt = now
	r.UpdatedAt = now

	dto := convertDBRelease(r)
	if dto.ID != 42 || dto.VersionCode != 100 || dto.RolloutFraction != 0.5 {
		t.Errorf("convertDBRelease mismatch: %+v", dto)
	}
	dtos := convertDBReleases([]dbEngine.Release{r})
	if len(dtos) != 1 || dtos[0].ID != 42 {
		t.Errorf("convertDBReleases mismatch: %+v", dtos)
	}

	// convertDBAudit and convertDBAudits
	audit := dbEngine.ReleaseAudit{
		UserID:      1,
		ProjectID:   10,
		ReleaseID:   42,
		PackageName: "com.example.app",
		VersionCode: 100,
		Track:       "internal",
		Action:      "triggered",
		Detail:      "notes",
	}
	audit.ID = 1
	audit.CreatedAt = now

	adto := convertDBAudit(audit)
	if adto.ID != 1 || adto.PackageName != "com.example.app" {
		t.Errorf("convertDBAudit mismatch: %+v", adto)
	}
	adtos := convertDBAudits([]dbEngine.ReleaseAudit{audit})
	if len(adtos) != 1 || adtos[0].ID != 1 {
		t.Errorf("convertDBAudits mismatch: %+v", adtos)
	}
}

func TestReleaseController_AuditAndStatus(t *testing.T) {
	_, _, _ = setupReleaseTestEnv(t)

	// test setReleaseStatus
	rel := dbEngine.Release{
		ProjectID: 1,
		BuildID:   1,
		Status:    statusPending,
	}
	dbEngine.DB.Create(&rel)

	setReleaseStatus(rel.ID, statusPublished, 102)
	var updatedRel dbEngine.Release
	dbEngine.DB.First(&updatedRel, rel.ID)
	if updatedRel.Status != statusPublished || updatedRel.VersionCode != 102 {
		t.Errorf("setReleaseStatus failed: %+v", updatedRel)
	}

	// test writeAudit
	writeAudit(1, 1, rel.ID, "com.example", 102, "internal", "triggered", "details")
	var audits []dbEngine.ReleaseAudit
	dbEngine.DB.Where("release_id = ?", rel.ID).Find(&audits)
	if len(audits) != 1 || audits[0].Detail != "details" {
		t.Errorf("writeAudit failed to save record")
	}
}

func TestReleaseController_Handlers(t *testing.T) {
	user, ctrl, encCreds := setupReleaseTestEnv(t)

	// Create test Project
	proj := dbEngine.Project{
		Name:   "Release Proj",
		UserID: user.ID,
	}
	dbEngine.DB.Create(&proj)

	// 1. AuditListHandler - unauthorized
	req := makeAuthReleaseReq("GET", fmt.Sprintf("/project/%d/audit", proj.ID), nil, 0)
	w := httptest.NewRecorder()
	ctrl.AuditListHandler(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}

	// 2. AuditListHandler - invalid project ID
	req = makeAuthReleaseReq("GET", "/project/invalid/audit", nil, user.ID)
	req = mux.SetURLVars(req, map[string]string{"id": "invalid"})
	w = httptest.NewRecorder()
	ctrl.AuditListHandler(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}

	// 3. AuditListHandler - project not found (different user)
	req = makeAuthReleaseReq("GET", "/project/999/audit", nil, user.ID)
	req = mux.SetURLVars(req, map[string]string{"id": "999"})
	w = httptest.NewRecorder()
	ctrl.AuditListHandler(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}

	// 4. AuditListHandler - success
	writeAudit(user.ID, proj.ID, 1, "com.example", 1, "internal", "triggered", "ok")
	req = makeAuthReleaseReq("GET", fmt.Sprintf("/project/%d/audit", proj.ID), nil, user.ID)
	req = mux.SetURLVars(req, map[string]string{"id": fmt.Sprintf("%d", proj.ID)})
	w = httptest.NewRecorder()
	ctrl.AuditListHandler(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	// 5. ReleasesListHandler - unauthorized & invalid id & not found
	req = makeAuthReleaseReq("GET", fmt.Sprintf("/project/%d/releases", proj.ID), nil, 0)
	w = httptest.NewRecorder()
	ctrl.ReleasesListHandler(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}

	req = makeAuthReleaseReq("GET", "/project/invalid/releases", nil, user.ID)
	req = mux.SetURLVars(req, map[string]string{"id": "invalid"})
	w = httptest.NewRecorder()
	ctrl.ReleasesListHandler(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}

	req = makeAuthReleaseReq("GET", "/project/999/releases", nil, user.ID)
	req = mux.SetURLVars(req, map[string]string{"id": "999"})
	w = httptest.NewRecorder()
	ctrl.ReleasesListHandler(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}

	// 6. ReleasesListHandler - success
	rel := dbEngine.Release{
		ProjectID:   proj.ID,
		BuildID:     1,
		VersionName: "1.0.0",
		Status:      statusPublished,
	}
	dbEngine.DB.Create(&rel)

	req = makeAuthReleaseReq("GET", fmt.Sprintf("/project/%d/releases", proj.ID), nil, user.ID)
	req = mux.SetURLVars(req, map[string]string{"id": fmt.Sprintf("%d", proj.ID)})
	w = httptest.NewRecorder()
	ctrl.ReleasesListHandler(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	// 7. ReleaseGetHandler - unauthorized & invalid id
	req = makeAuthReleaseReq("GET", fmt.Sprintf("/project/%d/release/%d", proj.ID, rel.ID), nil, 0)
	w = httptest.NewRecorder()
	ctrl.ReleaseGetHandler(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}

	req = makeAuthReleaseReq("GET", "/project/invalid/release/1", nil, user.ID)
	req = mux.SetURLVars(req, map[string]string{"id": "invalid", "releaseId": "1"})
	w = httptest.NewRecorder()
	ctrl.ReleaseGetHandler(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}

	req = makeAuthReleaseReq("GET", fmt.Sprintf("/project/%d/release/invalid", proj.ID), nil, user.ID)
	req = mux.SetURLVars(req, map[string]string{"id": fmt.Sprintf("%d", proj.ID), "releaseId": "invalid"})
	w = httptest.NewRecorder()
	ctrl.ReleaseGetHandler(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}

	// ReleaseGetHandler - not found
	req = makeAuthReleaseReq("GET", fmt.Sprintf("/project/%d/release/999", proj.ID), nil, user.ID)
	req = mux.SetURLVars(req, map[string]string{"id": fmt.Sprintf("%d", proj.ID), "releaseId": "999"})
	w = httptest.NewRecorder()
	ctrl.ReleaseGetHandler(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}

	// ReleaseGetHandler - success
	req = makeAuthReleaseReq("GET", fmt.Sprintf("/project/%d/release/%d", proj.ID, rel.ID), nil, user.ID)
	req = mux.SetURLVars(req, map[string]string{"id": fmt.Sprintf("%d", proj.ID), "releaseId": fmt.Sprintf("%d", rel.ID)})
	w = httptest.NewRecorder()
	ctrl.ReleaseGetHandler(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	// 8. AccessCheckHandler - unauthorized & invalid id
	req = makeAuthReleaseReq("GET", fmt.Sprintf("/project/%d/google-play/access", proj.ID), nil, 0)
	w = httptest.NewRecorder()
	ctrl.AccessCheckHandler(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}

	req = makeAuthReleaseReq("GET", "/project/invalid/google-play/access", nil, user.ID)
	req = mux.SetURLVars(req, map[string]string{"id": "invalid"})
	w = httptest.NewRecorder()
	ctrl.AccessCheckHandler(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}

	// AccessCheckHandler - not configured (no project config)
	req = makeAuthReleaseReq("GET", fmt.Sprintf("/project/%d/google-play/access", proj.ID), nil, user.ID)
	req = mux.SetURLVars(req, map[string]string{"id": fmt.Sprintf("%d", proj.ID)})
	w = httptest.NewRecorder()
	ctrl.AccessCheckHandler(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 with accessible: false, got %d", w.Code)
	}

	// Add Google Play credentials
	gpCred := dbEngine.GooglePlayCredentials{
		UserID:      user.ID,
		Name:        "Test SA",
		Credentials: encCreds,
	}
	dbEngine.DB.Create(&gpCred)

	cfg := dbEngine.ProjectConfig{
		ProjectID:               proj.ID,
		PackageName:             "com.example.app",
		GooglePlayCredentialsID: &gpCred.ID,
		GooglePlayTrack:         "internal",
	}
	dbEngine.DB.Create(&cfg)

	// AccessCheckHandler - client_error (invalid credentials)
	badGPCred := dbEngine.GooglePlayCredentials{
		UserID:      user.ID,
		Name:        "Bad SA",
		Credentials: "invalid-cipher-text",
	}
	dbEngine.DB.Create(&badGPCred)
	cfg.GooglePlayCredentialsID = &badGPCred.ID
	dbEngine.DB.Save(&cfg)

	req = makeAuthReleaseReq("GET", fmt.Sprintf("/project/%d/google-play/access", proj.ID), nil, user.ID)
	req = mux.SetURLVars(req, map[string]string{"id": fmt.Sprintf("%d", proj.ID)})
	w = httptest.NewRecorder()
	ctrl.AccessCheckHandler(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 with accessible: false, got %d", w.Code)
	}

	// Reset to valid SA creds
	cfg.GooglePlayCredentialsID = &gpCred.ID
	dbEngine.DB.Save(&cfg)

	// 9. PublishHandler tests
	// Unauthorized
	req = makeAuthReleaseReq("POST", fmt.Sprintf("/project/%d/build/1/publish", proj.ID), nil, 0)
	w = httptest.NewRecorder()
	ctrl.PublishHandler(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}

	// Invalid IDs
	req = makeAuthReleaseReq("POST", "/project/invalid/build/1/publish", nil, user.ID)
	req = mux.SetURLVars(req, map[string]string{"id": "invalid", "buildId": "1"})
	w = httptest.NewRecorder()
	ctrl.PublishHandler(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}

	req = makeAuthReleaseReq("POST", fmt.Sprintf("/project/%d/build/invalid/publish", proj.ID), nil, user.ID)
	req = mux.SetURLVars(req, map[string]string{"id": fmt.Sprintf("%d", proj.ID), "buildId": "invalid"})
	w = httptest.NewRecorder()
	ctrl.PublishHandler(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}

	// Build not found
	req = makeAuthReleaseReq("POST", fmt.Sprintf("/project/%d/build/999/publish", proj.ID), nil, user.ID)
	req = mux.SetURLVars(req, map[string]string{"id": fmt.Sprintf("%d", proj.ID), "buildId": "999"})
	w = httptest.NewRecorder()
	ctrl.PublishHandler(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}

	// Build not successful
	bPending := dbEngine.Build{
		ProjectID: proj.ID,
		Status:    "pending",
	}
	dbEngine.DB.Create(&bPending)

	req = makeAuthReleaseReq("POST", fmt.Sprintf("/project/%d/build/%d/publish", proj.ID, bPending.ID), nil, user.ID)
	req = mux.SetURLVars(req, map[string]string{"id": fmt.Sprintf("%d", proj.ID), "buildId": fmt.Sprintf("%d", bPending.ID)})
	w = httptest.NewRecorder()
	ctrl.PublishHandler(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for not successful build, got %d", w.Code)
	}

	// Successful build with broken Google Play credentials
	bSuccess := dbEngine.Build{
		ProjectID:   proj.ID,
		Status:      "success",
		VersionName: "1.0.0",
		VersionCode: 1,
	}
	dbEngine.DB.Create(&bSuccess)

	// Set invalid credentials to test accessError failure
	cfg.GooglePlayCredentialsID = &badGPCred.ID
	dbEngine.DB.Save(&cfg)

	req = makeAuthReleaseReq("POST", fmt.Sprintf("/project/%d/build/%d/publish", proj.ID, bSuccess.ID), nil, user.ID)
	req = mux.SetURLVars(req, map[string]string{"id": fmt.Sprintf("%d", proj.ID), "buildId": fmt.Sprintf("%d", bSuccess.ID)})
	w = httptest.NewRecorder()
	ctrl.PublishHandler(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 when accessError fails to build client, got %d", w.Code)
	}

	// Test loadGooglePlayContext error branches directly
	cfg.PackageName = ""
	dbEngine.DB.Save(&cfg)
	if _, _, err := loadGooglePlayContext(proj.ID, user.ID); err == nil {
		t.Errorf("expected error for empty package_name")
	}

	cfg.PackageName = "com.example"
	cfg.GooglePlayCredentialsID = nil
	dbEngine.DB.Save(&cfg)
	if _, _, err := loadGooglePlayContext(proj.ID, user.ID); err == nil {
		t.Errorf("expected error for nil GooglePlayCredentialsID")
	}

	invalidCredID := uint(999999)
	cfg.GooglePlayCredentialsID = &invalidCredID
	dbEngine.DB.Save(&cfg)
	if _, _, err := loadGooglePlayContext(proj.ID, user.ID); err == nil {
		t.Errorf("expected error for non-existent GooglePlayCredentials")
	}

	if _, _, err := loadGooglePlayContext(999999, user.ID); err == nil {
		t.Errorf("expected error for non-existent project config")
	}
}

func TestReleaseController_PublishAndAccessError(t *testing.T) {
	user, _, encCreds := setupReleaseTestEnv(t)

	// Test accessError branches
	// 1. invalid crypto
	status, _ := accessError(context.Background(), "invalid-key", "com.example")
	if status != http.StatusInternalServerError {
		t.Errorf("expected 500 on invalid credentials, got %d", status)
	}

	// 2. runPublish direct test (failing at AAB download since no S3 mock)
	input := googleplay.PublishInput{
		PackageName: "com.example",
		Track:       "internal",
	}
	rel := dbEngine.Release{
		ProjectID: 1,
		BuildID:   1,
		Status:    statusPending,
	}
	dbEngine.DB.Create(&rel)

	runPublish(user.ID, 1, rel.ID, 1, encCreds, input)

	var failedRel dbEngine.Release
	dbEngine.DB.First(&failedRel, rel.ID)
	if failedRel.Status != statusFailed {
		t.Errorf("expected release status failed, got %s", failedRel.Status)
	}
}

type mockPublisher struct {
	checkAccessErr error
	publishRes     *googleplay.PublishResult
	publishErr     error
}

func (m *mockPublisher) CheckAccess(ctx context.Context, packageName string) error {
	return m.checkAccessErr
}

func (m *mockPublisher) Publish(ctx context.Context, input googleplay.PublishInput) (*googleplay.PublishResult, error) {
	return m.publishRes, m.publishErr
}

type nopCloser struct {
	*bytes.Reader
}

func (n *nopCloser) Close() error { return nil }

func TestReleaseController_MockedPublishAndAccess(t *testing.T) {
	user, ctrl, encCreds := setupReleaseTestEnv(t)

	proj := dbEngine.Project{
		Name:   "Mocked Publish Proj",
		UserID: user.ID,
	}
	dbEngine.DB.Create(&proj)

	gpCred := dbEngine.GooglePlayCredentials{
		UserID:      user.ID,
		Name:        "Test SA",
		Credentials: encCreds,
	}
	dbEngine.DB.Create(&gpCred)

	cfg := dbEngine.ProjectConfig{
		ProjectID:               proj.ID,
		PackageName:             "com.example.mock",
		GooglePlayCredentialsID: &gpCred.ID,
		GooglePlayTrack:         "internal",
	}
	dbEngine.DB.Create(&cfg)

	build := dbEngine.Build{
		ProjectID:   proj.ID,
		Status:      "success",
		VersionName: "2.0.0",
		VersionCode: 200,
	}
	dbEngine.DB.Create(&build)

	origClient := newGooglePlayClient
	origS3 := getS3ReleaseArtifactReader
	defer func() {
		newGooglePlayClient = origClient
		getS3ReleaseArtifactReader = origS3
	}()

	mockPub := &mockPublisher{
		checkAccessErr: nil,
		publishRes: &googleplay.PublishResult{
			Track:       "internal",
			VersionCode: 200,
			Status:      "completed",
		},
	}
	newGooglePlayClient = func(ctx context.Context, encryptedCredentials string) (googlePlayPublisher, error) {
		return mockPub, nil
	}
	getS3ReleaseArtifactReader = func(buildID uint) (io.ReadCloser, error) {
		return &nopCloser{Reader: bytes.NewReader([]byte("fake aab"))}, nil
	}

	// 1. Successful AccessCheckHandler
	req := makeAuthReleaseReq("GET", fmt.Sprintf("/project/%d/google-play/access", proj.ID), nil, user.ID)
	req = mux.SetURLVars(req, map[string]string{"id": fmt.Sprintf("%d", proj.ID)})
	w := httptest.NewRecorder()
	ctrl.AccessCheckHandler(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 on check access, got %d", w.Code)
	}

	// 2. AccessCheckHandler with permission error
	mockPub.checkAccessErr = &googleplay.PublishError{Reason: googleplay.ReasonPermission, Msg: "permission denied"}
	w = httptest.NewRecorder()
	ctrl.AccessCheckHandler(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 with accessible:false, got %d", w.Code)
	}

	// 3. PublishHandler with permission error from accessError
	reqPub := makeAuthReleaseReq("POST", fmt.Sprintf("/project/%d/build/%d/publish", proj.ID, build.ID), nil, user.ID)
	reqPub = mux.SetURLVars(reqPub, map[string]string{"id": fmt.Sprintf("%d", proj.ID), "buildId": fmt.Sprintf("%d", build.ID)})
	wPub := httptest.NewRecorder()
	ctrl.PublishHandler(wPub, reqPub)
	if wPub.Code != http.StatusForbidden {
		t.Errorf("expected 403 on permission denied, got %d", wPub.Code)
	}

	// 4. PublishHandler with other error from accessError
	mockPub.checkAccessErr = &googleplay.PublishError{Reason: googleplay.ReasonUnknown, Msg: "generic err"}
	wPub = httptest.NewRecorder()
	ctrl.PublishHandler(wPub, reqPub)
	if wPub.Code != http.StatusBadGateway {
		t.Errorf("expected 502 on access check error, got %d", wPub.Code)
	}

	// 5. Successful PublishHandler with custom request options
	mockPub.checkAccessErr = nil
	rollout := 0.25
	draft := true
	pubBody := models.PublishRequest{
		Track:            "alpha",
		RolloutFraction:  &rollout,
		Draft:            &draft,
		ReleaseNotes:     "Release 2.0.0",
		ReleaseNotesLang: "en-US",
	}
	reqSuccess := makeAuthReleaseReq("POST", fmt.Sprintf("/project/%d/build/%d/publish", proj.ID, build.ID), pubBody, user.ID)
	reqSuccess = mux.SetURLVars(reqSuccess, map[string]string{"id": fmt.Sprintf("%d", proj.ID), "buildId": fmt.Sprintf("%d", build.ID)})
	wSuccess := httptest.NewRecorder()
	ctrl.PublishHandler(wSuccess, reqSuccess)
	if wSuccess.Code != http.StatusAccepted {
		t.Errorf("expected 202 on publish handler, got %d: %s", wSuccess.Code, wSuccess.Body.String())
	}

	// Allow goroutine to finish runPublish
	time.Sleep(50 * time.Millisecond)

	// Verify release got updated to published
	var publishedRel dbEngine.Release
	dbEngine.DB.Where("project_id = ? AND build_id = ?", proj.ID, build.ID).Order("id DESC").First(&publishedRel)
	if publishedRel.Status != statusPublished || publishedRel.Track != "alpha" || publishedRel.RolloutFraction != 0.25 {
		t.Errorf("expected published release with alpha and 0.25 rollout, got %+v", publishedRel)
	}

	// 6. Test runPublish with client publish error
	mockPub.publishErr = fmt.Errorf("play console publish failed")
	relFail := dbEngine.Release{
		ProjectID: proj.ID,
		BuildID:   build.ID,
		Status:    statusPending,
	}
	dbEngine.DB.Create(&relFail)
	runPublish(user.ID, proj.ID, relFail.ID, build.ID, encCreds, googleplay.PublishInput{PackageName: "com.example.mock", Track: "alpha"})
	dbEngine.DB.First(&relFail, relFail.ID)
	if relFail.Status != statusFailed {
		t.Errorf("expected failed status, got %s", relFail.Status)
	}

	// 7. Test runPublish with client build error
	newGooglePlayClient = func(ctx context.Context, encryptedCredentials string) (googlePlayPublisher, error) {
		return nil, fmt.Errorf("cannot build client")
	}
	relFailClient := dbEngine.Release{
		ProjectID: proj.ID,
		BuildID:   build.ID,
		Status:    statusPending,
	}
	dbEngine.DB.Create(&relFailClient)
	runPublish(user.ID, proj.ID, relFailClient.ID, build.ID, encCreds, googleplay.PublishInput{PackageName: "com.example.mock", Track: "alpha"})
	dbEngine.DB.First(&relFailClient, relFailClient.ID)
	if relFailClient.Status != statusFailed {
		t.Errorf("expected failed status, got %s", relFailClient.Status)
	}
}
