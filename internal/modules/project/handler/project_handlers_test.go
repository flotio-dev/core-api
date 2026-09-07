package handler

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/flotio-dev/core-api/internal/common/crypto"
	dbEngine "github.com/flotio-dev/core-api/internal/common/database"
	userRepo "github.com/flotio-dev/core-api/internal/modules/user/repository"
	userServices "github.com/flotio-dev/core-api/internal/modules/user/service"
	"github.com/glebarez/sqlite"
	"github.com/gorilla/mux"
	"gorm.io/gorm"
)

func setupTestEnv(t *testing.T) (*dbEngine.User, *userServices.UserService) {
	t.Helper()
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 1)
	}
	t.Setenv("SECRETS_ENCRYPTION_KEY", base64.StdEncoding.EncodeToString(key))
	_ = crypto.Init()

	testDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	_ = testDB.AutoMigrate(
		&dbEngine.User{},
		&dbEngine.Project{},
		&dbEngine.ProjectConfig{},
		&dbEngine.Build{},
		&dbEngine.Env{},
		&dbEngine.Keystore{},
		&dbEngine.GooglePlayCredentials{},
	)
	dbEngine.DB = testDB

	user := &dbEngine.User{
		Email:    "test@example.com",
		Username: "testuser",
	}
	user.ID = 1
	if err := testDB.Create(user).Error; err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	repo := userRepo.NewUserRepository(testDB)
	svc := userServices.NewUserService(repo)
	return user, svc
}

func makeAuthRequest(method, url string, body interface{}, userID uint) *http.Request {
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

func TestProjectController_Comprehensive(t *testing.T) {
	user, svc := setupTestEnv(t)
	ctrl := NewProjectController(svc)

	// 1. ProjectsGetHandler - empty
	req := makeAuthRequest("GET", "/projects", nil, user.ID)
	w := httptest.NewRecorder()
	ctrl.ProjectsGetHandler(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	// 2. ProjectCreateHandler - valid
	createReq := ProjectCreateRequest{
		Name: "New Project",
		Config: &dbEngine.ProjectConfig{
			Platforms: []string{"android", "ios"},
		},
	}
	req = makeAuthRequest("POST", "/projects", createReq, user.ID)
	w = httptest.NewRecorder()
	ctrl.ProjectCreateHandler(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var createdResp ProjectResponse
	_ = json.Unmarshal(w.Body.Bytes(), &createdResp)
	projID := createdResp.Project.ID

	// 3. ProjectCreateHandler - invalid body
	req = httptest.NewRequest("POST", "/projects", bytes.NewReader([]byte("invalid json")))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), "user_id", user.ID))
	w = httptest.NewRecorder()
	ctrl.ProjectCreateHandler(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}

	// 4. ProjectsGetHandler - populated
	req = makeAuthRequest("GET", "/projects", nil, user.ID)
	w = httptest.NewRecorder()
	ctrl.ProjectsGetHandler(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	// 5. ProjectGetHandler - found
	req = makeAuthRequest("GET", fmt.Sprintf("/projects/%d", projID), nil, user.ID)
	req = mux.SetURLVars(req, map[string]string{"id": fmt.Sprintf("%d", projID)})
	w = httptest.NewRecorder()
	ctrl.ProjectGetHandler(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	// 6. ProjectGetHandler - not found
	req = makeAuthRequest("GET", "/projects/99999", nil, user.ID)
	req = mux.SetURLVars(req, map[string]string{"id": "99999"})
	w = httptest.NewRecorder()
	ctrl.ProjectGetHandler(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}

	// 7. ProjectGetHandler - invalid ID
	req = makeAuthRequest("GET", "/projects/abc", nil, user.ID)
	req = mux.SetURLVars(req, map[string]string{"id": "abc"})
	w = httptest.NewRecorder()
	ctrl.ProjectGetHandler(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}

	// 8. ProjectPutHandler - update name & config
	updateReq := ProjectUpdateRequest{
		Name: "Updated Project",
		Config: &dbEngine.ProjectConfig{
			Platforms: []string{"web"},
		},
	}
	req = makeAuthRequest("PUT", fmt.Sprintf("/projects/%d", projID), updateReq, user.ID)
	req = mux.SetURLVars(req, map[string]string{"id": fmt.Sprintf("%d", projID)})
	w = httptest.NewRecorder()
	ctrl.ProjectPutHandler(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// 9. ProjectPutHandler - invalid id / not found / bad json
	req = makeAuthRequest("PUT", "/projects/abc", updateReq, user.ID)
	req = mux.SetURLVars(req, map[string]string{"id": "abc"})
	w = httptest.NewRecorder()
	ctrl.ProjectPutHandler(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}

	req = makeAuthRequest("PUT", "/projects/99999", updateReq, user.ID)
	req = mux.SetURLVars(req, map[string]string{"id": "99999"})
	w = httptest.NewRecorder()
	ctrl.ProjectPutHandler(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}

	req = httptest.NewRequest("PUT", fmt.Sprintf("/projects/%d", projID), bytes.NewReader([]byte("{invalid")))
	req = req.WithContext(context.WithValue(req.Context(), "user_id", user.ID))
	req = mux.SetURLVars(req, map[string]string{"id": fmt.Sprintf("%d", projID)})
	w = httptest.NewRecorder()
	ctrl.ProjectPutHandler(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}

	// 10. ProjectDeleteHandler - invalid ID, not found, success
	req = makeAuthRequest("DELETE", "/projects/abc", nil, user.ID)
	req = mux.SetURLVars(req, map[string]string{"id": "abc"})
	w = httptest.NewRecorder()
	ctrl.ProjectDeleteHandler(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}

	req = makeAuthRequest("DELETE", "/projects/99999", nil, user.ID)
	req = mux.SetURLVars(req, map[string]string{"id": "99999"})
	w = httptest.NewRecorder()
	ctrl.ProjectDeleteHandler(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	req = makeAuthRequest("DELETE", fmt.Sprintf("/projects/%d", projID), nil, user.ID)
	req = mux.SetURLVars(req, map[string]string{"id": fmt.Sprintf("%d", projID)})
	w = httptest.NewRecorder()
	ctrl.ProjectDeleteHandler(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	// Test helper models conversion
	b := dbEngine.Build{ContainerID: "cid", Duration: 10, APKURL: "url"}
	_ = convertDBBuild(b)
	_ = convertDBBuilds([]dbEngine.Build{b})
	p := dbEngine.Project{Name: "p"}
	_ = convertDBProject(p)
	_ = convertDBProjects([]dbEngine.Project{p})
}

func TestConfigController_Comprehensive(t *testing.T) {
	user, svc := setupTestEnv(t)
	ctrl := NewConfigController(svc)

	proj := dbEngine.Project{Name: "Proj 1", UserID: user.ID}
	dbEngine.DB.Create(&proj)

	// 1. ConfigGetHandler - default fallback when no config exists
	req := makeAuthRequest("GET", fmt.Sprintf("/project/%d/config", proj.ID), nil, user.ID)
	req = mux.SetURLVars(req, map[string]string{"id": fmt.Sprintf("%d", proj.ID)})
	w := httptest.NewRecorder()
	ctrl.ConfigGetHandler(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	// 2. ConfigPostHandler - create config
	cfg := dbEngine.ProjectConfig{
		Platforms:         []string{"android"},
		BuildTrigger:      "manual",
		DependencyCaching: true,
	}
	req = makeAuthRequest("POST", fmt.Sprintf("/project/%d/config", proj.ID), cfg, user.ID)
	req = mux.SetURLVars(req, map[string]string{"id": fmt.Sprintf("%d", proj.ID)})
	w = httptest.NewRecorder()
	ctrl.ConfigPostHandler(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// 3. ConfigGetHandler - now found
	req = makeAuthRequest("GET", fmt.Sprintf("/project/%d/config", proj.ID), nil, user.ID)
	req = mux.SetURLVars(req, map[string]string{"id": fmt.Sprintf("%d", proj.ID)})
	w = httptest.NewRecorder()
	ctrl.ConfigGetHandler(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	// 4. ConfigPostHandler - partial update (save)
	cfgUpdate := map[string]interface{}{
		"build_trigger": "tag",
	}
	req = makeAuthRequest("POST", fmt.Sprintf("/project/%d/config", proj.ID), cfgUpdate, user.ID)
	req = mux.SetURLVars(req, map[string]string{"id": fmt.Sprintf("%d", proj.ID)})
	w = httptest.NewRecorder()
	ctrl.ConfigPostHandler(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	// 5. ConfigDeleteHandler
	req = makeAuthRequest("DELETE", fmt.Sprintf("/project/%d/config", proj.ID), nil, user.ID)
	req = mux.SetURLVars(req, map[string]string{"id": fmt.Sprintf("%d", proj.ID)})
	w = httptest.NewRecorder()
	ctrl.ConfigDeleteHandler(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	// Errors
	req = makeAuthRequest("GET", "/project/abc/config", nil, user.ID)
	req = mux.SetURLVars(req, map[string]string{"id": "abc"})
	w = httptest.NewRecorder()
	ctrl.ConfigGetHandler(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}

	req = makeAuthRequest("POST", "/project/abc/config", nil, user.ID)
	req = mux.SetURLVars(req, map[string]string{"id": "abc"})
	w = httptest.NewRecorder()
	ctrl.ConfigPostHandler(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}

	req = makeAuthRequest("DELETE", "/project/abc/config", nil, user.ID)
	req = mux.SetURLVars(req, map[string]string{"id": "abc"})
	w = httptest.NewRecorder()
	ctrl.ConfigDeleteHandler(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestEnvController_Comprehensive(t *testing.T) {
	user, svc := setupTestEnv(t)
	ctrl := NewEnvController(svc)

	proj := dbEngine.Project{Name: "Env Proj", UserID: user.ID}
	dbEngine.DB.Create(&proj)

	// 1. EnvPostHandler - create env
	createReq := EnvCreateRequest{
		ProjectID: &proj.ID,
		Key:       "MY_KEY",
		Value:     "secret_val",
		Type:      "env",
	}
	req := makeAuthRequest("POST", "/env", createReq, user.ID)
	w := httptest.NewRecorder()
	ctrl.EnvPostHandler(w, req)
	if w.Code != http.StatusCreated && w.Code != http.StatusOK {
		t.Fatalf("expected 201/200, got %d: %s", w.Code, w.Body.String())
	}
	var envResp EnvResponse
	_ = json.Unmarshal(w.Body.Bytes(), &envResp)
	envID := envResp.Env.ID

	// 2. EnvGetHandler / EnvsGetHandler
	req = makeAuthRequest("GET", "/env", nil, user.ID)
	w = httptest.NewRecorder()
	ctrl.EnvGetHandler(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	req = makeAuthRequest("GET", fmt.Sprintf("/envs?project_id=%d", proj.ID), nil, user.ID)
	w = httptest.NewRecorder()
	ctrl.EnvsGetHandler(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	// 3. EnvGetByIdHandler
	req = makeAuthRequest("GET", fmt.Sprintf("/env/%d", envID), nil, user.ID)
	req = mux.SetURLVars(req, map[string]string{"envId": fmt.Sprintf("%d", envID)})
	w = httptest.NewRecorder()
	ctrl.EnvGetByIdHandler(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	// 4. EnvPutByIdHandler
	updateReq := EnvUpdateRequest{
		Key:   "UPDATED_KEY",
		Value: "new_val",
	}
	req = makeAuthRequest("PUT", fmt.Sprintf("/env/%d", envID), updateReq, user.ID)
	req = mux.SetURLVars(req, map[string]string{"envId": fmt.Sprintf("%d", envID)})
	w = httptest.NewRecorder()
	ctrl.EnvPutByIdHandler(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	// 5. EnvDeleteByIdHandler
	req = makeAuthRequest("DELETE", fmt.Sprintf("/env/%d", envID), nil, user.ID)
	req = mux.SetURLVars(req, map[string]string{"envId": fmt.Sprintf("%d", envID)})
	w = httptest.NewRecorder()
	ctrl.EnvDeleteByIdHandler(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	// Error handling
	req = makeAuthRequest("GET", "/env/abc", nil, user.ID)
	req = mux.SetURLVars(req, map[string]string{"envId": "abc"})
	w = httptest.NewRecorder()
	ctrl.EnvGetByIdHandler(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}

	req = makeAuthRequest("GET", "/env/99999", nil, user.ID)
	req = mux.SetURLVars(req, map[string]string{"envId": "99999"})
	w = httptest.NewRecorder()
	ctrl.EnvGetByIdHandler(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}

	_ = convertDBEnvs([]dbEngine.Env{{Key: "k"}})
}

func TestKeystoreController_Comprehensive(t *testing.T) {
	user, svc := setupTestEnv(t)
	ctrl := NewKeystoreController(svc)

	// 1. KeystorePostHandler - valid
	validB64 := base64.StdEncoding.EncodeToString([]byte("keystore file raw bytes"))
	createReq := KeystoreCreateRequest{
		Name:          "Android Keystore",
		KeystoreFile:  validB64,
		StorePassword: "storepassword",
		KeyPassword:   "keypassword",
		KeyAlias:      "myalias",
	}
	req := makeAuthRequest("POST", "/keystore", createReq, user.ID)
	w := httptest.NewRecorder()
	ctrl.KeystorePostHandler(w, req)
	if w.Code != http.StatusCreated && w.Code != http.StatusOK {
		t.Fatalf("expected 201/200, got %d: %s", w.Code, w.Body.String())
	}
	var ksResp KeystoreResponse
	_ = json.Unmarshal(w.Body.Bytes(), &ksResp)
	ksID := ksResp.Keystore.ID

	// 2. KeystorePostHandler - invalid body
	req = httptest.NewRequest("POST", "/keystore", bytes.NewReader([]byte("{invalid-json")))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), "user_id", user.ID))
	w = httptest.NewRecorder()
	ctrl.KeystorePostHandler(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}

	// 3. KeystoreGetHandler / KeystoresGetHandler
	req = makeAuthRequest("GET", "/keystore", nil, user.ID)
	w = httptest.NewRecorder()
	ctrl.KeystoreGetHandler(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	req = makeAuthRequest("GET", "/keystores", nil, user.ID)
	w = httptest.NewRecorder()
	ctrl.KeystoresGetHandler(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	// 4. KeystoreDeleteHandler
	req = makeAuthRequest("DELETE", fmt.Sprintf("/keystore/%d", ksID), nil, user.ID)
	req = mux.SetURLVars(req, map[string]string{"keystoreId": fmt.Sprintf("%d", ksID)})
	w = httptest.NewRecorder()
	ctrl.KeystoreDeleteHandler(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	// Delete not found / bad id
	req = makeAuthRequest("DELETE", "/keystore/abc", nil, user.ID)
	req = mux.SetURLVars(req, map[string]string{"keystoreId": "abc"})
	w = httptest.NewRecorder()
	ctrl.KeystoreDeleteHandler(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}

	req = makeAuthRequest("DELETE", "/keystore/99999", nil, user.ID)
	req = mux.SetURLVars(req, map[string]string{"keystoreId": "99999"})
	w = httptest.NewRecorder()
	ctrl.KeystoreDeleteHandler(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}

	_ = convertDBKeystores([]dbEngine.Keystore{{Name: "ks"}})
}

func TestGooglePlayCredentialsController_Comprehensive(t *testing.T) {
	user, svc := setupTestEnv(t)
	ctrl := NewGooglePlayCredentialsController(svc)

	// Valid Google Play Service Account JSON
	validSA := `{
		"type": "service_account",
		"project_id": "test-p",
		"private_key_id": "kid",
		"private_key": "-----BEGIN PRIVATE KEY-----\nMIIEvgIBADANBgkqhkiG9w0BAQEFAASCBKgwggSkAgEAAoIBAQC...\n-----END PRIVATE KEY-----\n",
		"client_email": "test@test-p.iam.gserviceaccount.com",
		"client_id": "123",
		"auth_uri": "https://accounts.google.com/o/oauth2/auth",
		"token_uri": "https://oauth2.googleapis.com/token"
	}`

	// 1. GooglePlayCredentialsPostHandler - valid
	createReq := GooglePlayCredentialsCreateRequest{
		Name:        "My GP Credentials",
		Credentials: validSA,
	}
	req := makeAuthRequest("POST", "/google-play-credentials", createReq, user.ID)
	w := httptest.NewRecorder()
	ctrl.GooglePlayCredentialsPostHandler(w, req)
	if w.Code != http.StatusCreated && w.Code != http.StatusOK {
		t.Fatalf("expected 201/200, got %d: %s", w.Code, w.Body.String())
	}
	var gpResp GooglePlayCredentialsResponse
	_ = json.Unmarshal(w.Body.Bytes(), &gpResp)
	gpID := gpResp.GooglePlayCredentials.ID

	// 2. GooglePlayCredentialsPostHandler - invalid JSON / SA
	badSAReq := GooglePlayCredentialsCreateRequest{
		Name:        "Bad GP",
		Credentials: "not-a-valid-sa-json",
	}
	req = makeAuthRequest("POST", "/google-play-credentials", badSAReq, user.ID)
	w = httptest.NewRecorder()
	ctrl.GooglePlayCredentialsPostHandler(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}

	// 3. GooglePlayCredentialsGetHandler
	req = makeAuthRequest("GET", "/google-play-credentials", nil, user.ID)
	w = httptest.NewRecorder()
	ctrl.GooglePlayCredentialsGetHandler(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	// 4. GooglePlayCredentialsDeleteHandler
	req = makeAuthRequest("DELETE", fmt.Sprintf("/google-play-credentials/%d", gpID), nil, user.ID)
	req = mux.SetURLVars(req, map[string]string{"credentialsId": fmt.Sprintf("%d", gpID)})
	w = httptest.NewRecorder()
	ctrl.GooglePlayCredentialsDeleteHandler(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	// Delete not found / bad ID
	req = makeAuthRequest("DELETE", "/google-play-credentials/abc", nil, user.ID)
	req = mux.SetURLVars(req, map[string]string{"credentialsId": "abc"})
	w = httptest.NewRecorder()
	ctrl.GooglePlayCredentialsDeleteHandler(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}

	req = makeAuthRequest("DELETE", "/google-play-credentials/99999", nil, user.ID)
	req = mux.SetURLVars(req, map[string]string{"credentialsId": "99999"})
	w = httptest.NewRecorder()
	ctrl.GooglePlayCredentialsDeleteHandler(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}

	_ = convertDBGooglePlayCredentialsList([]dbEngine.GooglePlayCredentials{{Name: "gp"}})
}

func TestFlutterController_Comprehensive(t *testing.T) {
	ctrl := NewFlutterController()

	// 1. First fetch - will attempt Google storage or fall back to defaults
	req := httptest.NewRequest("GET", "/flutter/versions", nil)
	w := httptest.NewRecorder()
	ctrl.VersionsGetHandler(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	// 2. Second fetch - should hit cache
	w2 := httptest.NewRecorder()
	ctrl.VersionsGetHandler(w2, req)
	if w2.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w2.Code)
	}
}

func TestAllControllers_UnauthorizedAndEdgeCases(t *testing.T) {
	user, svc := setupTestEnv(t)
	projCtrl := NewProjectController(svc)
	cfgCtrl := NewConfigController(svc)
	envCtrl := NewEnvController(svc)
	ksCtrl := NewKeystoreController(svc)
	gpCtrl := NewGooglePlayCredentialsController(svc)

	unauthReq := httptest.NewRequest("GET", "/test", nil)

	// Unauthorized tests
	w := httptest.NewRecorder()
	projCtrl.ProjectsGetHandler(w, unauthReq)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}

	w = httptest.NewRecorder()
	projCtrl.ProjectPutHandler(w, unauthReq)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}

	w = httptest.NewRecorder()
	projCtrl.ProjectDeleteHandler(w, unauthReq)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}

	w = httptest.NewRecorder()
	cfgCtrl.ConfigGetHandler(w, unauthReq)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}

	w = httptest.NewRecorder()
	cfgCtrl.ConfigPostHandler(w, unauthReq)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}

	w = httptest.NewRecorder()
	cfgCtrl.ConfigDeleteHandler(w, unauthReq)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}

	w = httptest.NewRecorder()
	envCtrl.EnvGetHandler(w, unauthReq)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}

	w = httptest.NewRecorder()
	envCtrl.EnvPostHandler(w, unauthReq)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}

	w = httptest.NewRecorder()
	envCtrl.EnvGetByIdHandler(w, unauthReq)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}

	w = httptest.NewRecorder()
	envCtrl.EnvPutByIdHandler(w, unauthReq)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}

	w = httptest.NewRecorder()
	envCtrl.EnvDeleteByIdHandler(w, unauthReq)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}

	w = httptest.NewRecorder()
	ksCtrl.KeystoreGetHandler(w, unauthReq)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}

	w = httptest.NewRecorder()
	ksCtrl.KeystorePostHandler(w, unauthReq)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}

	w = httptest.NewRecorder()
	ksCtrl.KeystoreDeleteHandler(w, unauthReq)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}

	w = httptest.NewRecorder()
	gpCtrl.GooglePlayCredentialsGetHandler(w, unauthReq)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}

	w = httptest.NewRecorder()
	gpCtrl.GooglePlayCredentialsPostHandler(w, unauthReq)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}

	w = httptest.NewRecorder()
	gpCtrl.GooglePlayCredentialsDeleteHandler(w, unauthReq)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}

	// EnvPostHandler with non-existent ProjectID
	nonExistentPID := uint(99999)
	req := makeAuthRequest("POST", "/env", EnvCreateRequest{
		ProjectID: &nonExistentPID,
		Key:       "K",
		Value:     "V",
	}, user.ID)
	w = httptest.NewRecorder()
	envCtrl.EnvPostHandler(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}

	// EnvPutByIdHandler with non-existent ProjectID
	env := dbEngine.Env{UserID: user.ID, Key: "KEY"}
	dbEngine.DB.Create(&env)
	req = makeAuthRequest("PUT", fmt.Sprintf("/env/%d", env.ID), EnvUpdateRequest{
		ProjectID: &nonExistentPID,
		Key:       "K",
	}, user.ID)
	req = mux.SetURLVars(req, map[string]string{"envId": fmt.Sprintf("%d", env.ID)})
	w = httptest.NewRecorder()
	envCtrl.EnvPutByIdHandler(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}

	// EnvPutByIdHandler invalid body and invalid id
	req = httptest.NewRequest("PUT", "/env/1", bytes.NewReader([]byte("{invalid")))
	req = req.WithContext(context.WithValue(req.Context(), "user_id", user.ID))
	req = mux.SetURLVars(req, map[string]string{"envId": "1"})
	w = httptest.NewRecorder()
	envCtrl.EnvPutByIdHandler(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}

	req = makeAuthRequest("PUT", "/env/abc", EnvUpdateRequest{}, user.ID)
	req = mux.SetURLVars(req, map[string]string{"envId": "abc"})
	w = httptest.NewRecorder()
	envCtrl.EnvPutByIdHandler(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}

	// EnvDeleteByIdHandler invalid id and not found
	req = makeAuthRequest("DELETE", "/env/abc", nil, user.ID)
	req = mux.SetURLVars(req, map[string]string{"envId": "abc"})
	w = httptest.NewRecorder()
	envCtrl.EnvDeleteByIdHandler(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}

	req = makeAuthRequest("DELETE", "/env/99999", nil, user.ID)
	req = mux.SetURLVars(req, map[string]string{"envId": "99999"})
	w = httptest.NewRecorder()
	envCtrl.EnvDeleteByIdHandler(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

