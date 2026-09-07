package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	dbEngine "github.com/flotio-dev/core-api/internal/common/database"
	authModel "github.com/flotio-dev/core-api/internal/modules/user/model"
	authServices "github.com/flotio-dev/core-api/internal/modules/user/service"
	"github.com/glebarez/sqlite"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

func setupTestEnvironment(t *testing.T) (*gorm.DB, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis.Run failed: %v", err)
	}
	os.Setenv("REDIS_ADDR", mr.Addr())
	dbEngine.InitRedis()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("sqlite.Open failed: %v", err)
	}
	if err := db.AutoMigrate(&dbEngine.User{}); err != nil {
		t.Fatalf("migration failed: %v", err)
	}
	dbEngine.DB = db

	authServices.AccessSecret = []byte("super-secret-access-token-key-1234")
	authServices.RefreshSecret = []byte("super-secret-refresh-token-key-1234")

	return db, mr
}

func TestAuthMiddleware(t *testing.T) {
	_, mr := setupTestEnvironment(t)
	defer mr.Close()

	var calledWithUserID uint
	dummyHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, _ := authServices.GetUserIDFromContext(r.Context())
		calledWithUserID = id
		w.WriteHeader(http.StatusOK)
	})

	middleware := AuthMiddleware(dummyHandler)

	// 1. Missing Authorization header
	req1 := httptest.NewRequest("GET", "/protected", nil)
	w1 := httptest.NewRecorder()
	middleware.ServeHTTP(w1, req1)
	if w1.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for missing auth, got %d", w1.Code)
	}

	// 2. Invalid prefix (not "Bearer ")
	req2 := httptest.NewRequest("GET", "/protected", nil)
	req2.Header.Set("Authorization", "Basic abc")
	w2 := httptest.NewRecorder()
	middleware.ServeHTTP(w2, req2)
	if w2.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for non-Bearer auth, got %d", w2.Code)
	}

	// 3. Invalid token string
	req3 := httptest.NewRequest("GET", "/protected", nil)
	req3.Header.Set("Authorization", "Bearer invalid.token.value")
	w3 := httptest.NewRecorder()
	middleware.ServeHTTP(w3, req3)
	if w3.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for invalid token, got %d", w3.Code)
	}

	// 4. Valid token
	validToken, err := authServices.GenerateAccessToken(77)
	if err != nil {
		t.Fatal(err)
	}
	req4 := httptest.NewRequest("GET", "/protected", nil)
	req4.Header.Set("Authorization", "Bearer "+validToken)
	w4 := httptest.NewRecorder()
	middleware.ServeHTTP(w4, req4)
	if w4.Code != http.StatusOK {
		t.Errorf("expected 200 for valid token, got %d", w4.Code)
	}
	if calledWithUserID != 77 {
		t.Errorf("expected user_id 77 in context, got %d", calledWithUserID)
	}
}

func TestRegisterHandler(t *testing.T) {
	_, mr := setupTestEnvironment(t)
	defer mr.Close()

	// 1. Invalid JSON
	req1 := httptest.NewRequest("POST", "/auth/register", strings.NewReader("bad-json"))
	w1 := httptest.NewRecorder()
	RegisterHandler(w1, req1)
	if w1.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for bad json, got %d", w1.Code)
	}

	// 2. Successful Registration
	regReq := authModel.RegisterRequest{
		Email:    "newuser@example.com",
		Username: "newuser",
		Password: "strongPassword123!",
	}
	data, _ := json.Marshal(regReq)
	req2 := httptest.NewRequest("POST", "/auth/register", bytes.NewReader(data))
	w2 := httptest.NewRecorder()
	RegisterHandler(w2, req2)
	if w2.Code != http.StatusOK {
		t.Errorf("expected 200 for registration, got %d (body: %s)", w2.Code, w2.Body.String())
	}
	var authResp authModel.AuthResponse
	if err := json.Unmarshal(w2.Body.Bytes(), &authResp); err != nil {
		t.Fatal(err)
	}
	if authResp.AccessToken == "" || authResp.RefreshToken == "" {
		t.Errorf("expected tokens in response: %+v", authResp)
	}

	// 3. User Already Exists
	req3 := httptest.NewRequest("POST", "/auth/register", bytes.NewReader(data))
	w3 := httptest.NewRecorder()
	RegisterHandler(w3, req3)
	if w3.Code != http.StatusBadRequest {
		t.Errorf("expected 400 when user already exists, got %d", w3.Code)
	}
}

func TestLoginHandler(t *testing.T) {
	db, mr := setupTestEnvironment(t)
	defer mr.Close()

	// Seed user
	hashed, _ := bcrypt.GenerateFromPassword([]byte("secret123"), 12)
	user := dbEngine.User{
		Email:        "existing@example.com",
		Username:     "existing",
		PasswordHash: string(hashed),
	}
	db.Create(&user)

	// 1. Invalid JSON
	req1 := httptest.NewRequest("POST", "/auth/login", strings.NewReader("invalid"))
	w1 := httptest.NewRecorder()
	LoginHandler(w1, req1)
	if w1.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w1.Code)
	}

	// 2. User Not Found
	notFoundBody, _ := json.Marshal(authModel.LoginRequest{
		Email:    "unknown@example.com",
		Password: "secret123",
	})
	req2 := httptest.NewRequest("POST", "/auth/login", bytes.NewReader(notFoundBody))
	w2 := httptest.NewRecorder()
	LoginHandler(w2, req2)
	if w2.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for unknown user, got %d", w2.Code)
	}

	// 3. Wrong Password
	wrongPassBody, _ := json.Marshal(authModel.LoginRequest{
		Email:    "existing@example.com",
		Password: "wrongPassword",
	})
	req3 := httptest.NewRequest("POST", "/auth/login", bytes.NewReader(wrongPassBody))
	w3 := httptest.NewRecorder()
	LoginHandler(w3, req3)
	if w3.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for wrong password, got %d", w3.Code)
	}

	// 4. Valid Login
	validBody, _ := json.Marshal(authModel.LoginRequest{
		Email:    "existing@example.com",
		Password: "secret123",
	})
	req4 := httptest.NewRequest("POST", "/auth/login", bytes.NewReader(validBody))
	w4 := httptest.NewRecorder()
	LoginHandler(w4, req4)
	if w4.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w4.Code)
	}
	var resp authModel.AuthResponse
	if err := json.Unmarshal(w4.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.AccessToken == "" || resp.RefreshToken == "" {
		t.Errorf("expected tokens: %+v", resp)
	}
}

func TestRefreshTokenHandler(t *testing.T) {
	_, mr := setupTestEnvironment(t)
	defer mr.Close()

	// 1. No Token provided -> 400
	req1 := httptest.NewRequest("POST", "/auth/refresh", nil)
	w1 := httptest.NewRecorder()
	RefreshTokenHandler(w1, req1)
	if w1.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing token, got %d", w1.Code)
	}
	if !strings.Contains(w1.Body.String(), "Refresh token not provided") {
		t.Errorf("expected 'not provided' message: %s", w1.Body.String())
	}

	// 2. Invalid Token in Cookie -> 401
	req2 := httptest.NewRequest("POST", "/auth/refresh", nil)
	req2.AddCookie(&http.Cookie{Name: "refresh_token", Value: "invalid-token"})
	w2 := httptest.NewRecorder()
	RefreshTokenHandler(w2, req2)
	if w2.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for invalid token, got %d", w2.Code)
	}
	if !strings.Contains(w2.Body.String(), "Invalid refresh token") {
		t.Errorf("expected 'Invalid refresh token' message: %s", w2.Body.String())
	}

	// 3. Valid Token but Revoked in Redis -> 401
	refToken, tid, _ := authServices.GenerateRefreshToken(10)
	req3 := httptest.NewRequest("POST", "/auth/refresh", nil)
	req3.AddCookie(&http.Cookie{Name: "refresh_token", Value: refToken})
	w3 := httptest.NewRecorder()
	RefreshTokenHandler(w3, req3)
	if w3.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for revoked token, got %d", w3.Code)
	}

	// 4. Valid Token stored in Redis via Cookie -> success with rotation
	authServices.StoreRefreshToken(context.Background(), tid, 10)
	req4 := httptest.NewRequest("POST", "/auth/refresh", nil)
	req4.AddCookie(&http.Cookie{Name: "refresh_token", Value: refToken})
	w4 := httptest.NewRecorder()
	RefreshTokenHandler(w4, req4)
	if w4.Code != http.StatusOK {
		t.Errorf("expected 200 on refresh, got %d", w4.Code)
	}
	var authResp authModel.AuthResponse
	if err := json.Unmarshal(w4.Body.Bytes(), &authResp); err != nil {
		t.Fatal(err)
	}
	if authResp.AccessToken == "" || authResp.RefreshToken == "" {
		t.Errorf("expected renewed tokens: %+v", authResp)
	}

	// 5. Valid Token stored in Redis via JSON Body -> success with rotation
	refTokenBody, tidBody, _ := authServices.GenerateRefreshToken(11)
	authServices.StoreRefreshToken(context.Background(), tidBody, 11)
	bodyBytes, _ := json.Marshal(authModel.RefreshTokenRequest{RefreshToken: refTokenBody})
	req5 := httptest.NewRequest("POST", "/auth/refresh", bytes.NewReader(bodyBytes))
	req5.Header.Set("Content-Type", "application/json")
	w5 := httptest.NewRecorder()
	RefreshTokenHandler(w5, req5)
	if w5.Code != http.StatusOK {
		t.Errorf("expected 200 on JSON body refresh, got %d", w5.Code)
	}

	// 6. Valid Token stored in Redis via Authorization Bearer header -> success with rotation
	refTokenHdr, tidHdr, _ := authServices.GenerateRefreshToken(12)
	authServices.StoreRefreshToken(context.Background(), tidHdr, 12)
	req6 := httptest.NewRequest("POST", "/auth/refresh", nil)
	req6.Header.Set("Authorization", "Bearer "+refTokenHdr)
	w6 := httptest.NewRecorder()
	RefreshTokenHandler(w6, req6)
	if w6.Code != http.StatusOK {
		t.Errorf("expected 200 on Bearer header refresh, got %d", w6.Code)
	}
}

func TestLogoutHandler(t *testing.T) {
	_, mr := setupTestEnvironment(t)
	defer mr.Close()

	// 1. Invalid JSON
	req1 := httptest.NewRequest("POST", "/auth/logout", strings.NewReader("bad-json-syntax{"))
	w1 := httptest.NewRecorder()
	LogoutHandler(w1, req1)
	if w1.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for bad json, got %d", w1.Code)
	}

	// 2. Valid Token in Body
	tokenStr, tid, _ := authServices.GenerateRefreshToken(12)
	authServices.StoreRefreshToken(context.Background(), tid, 12)
	bodyData, _ := json.Marshal(authModel.RefreshTokenRequest{RefreshToken: tokenStr})
	req2 := httptest.NewRequest("POST", "/auth/logout", bytes.NewReader(bodyData))
	w2 := httptest.NewRecorder()
	LogoutHandler(w2, req2)
	if w2.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w2.Code)
	}

	// 3. Valid Token in Cookie
	tokenStr2, tid2, _ := authServices.GenerateRefreshToken(12)
	authServices.StoreRefreshToken(context.Background(), tid2, 12)
	req3 := httptest.NewRequest("POST", "/auth/logout", bytes.NewReader([]byte("{}")))
	req3.AddCookie(&http.Cookie{Name: "refresh_token", Value: tokenStr2})
	w3 := httptest.NewRecorder()
	LogoutHandler(w3, req3)
	if w3.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w3.Code)
	}
}

func TestMeHandlers(t *testing.T) {
	db, mr := setupTestEnvironment(t)
	defer mr.Close()

	user := dbEngine.User{
		Email:    "me@example.com",
		Username: "meuser",
	}
	db.Create(&user)

	// 1. MeGetHandler without context
	req1 := httptest.NewRequest("GET", "/auth/@me", nil)
	w1 := httptest.NewRecorder()
	MeGetHandler(w1, req1)
	if w1.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w1.Code)
	}

	// 2. MeGetHandler with unknown user_id
	req2 := httptest.NewRequest("GET", "/auth/@me", nil)
	req2 = req2.WithContext(context.WithValue(req2.Context(), "user_id", uint(999)))
	w2 := httptest.NewRecorder()
	MeGetHandler(w2, req2)
	if w2.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w2.Code)
	}

	// 3. MeGetHandler with valid user_id
	req3 := httptest.NewRequest("GET", "/auth/@me", nil)
	req3 = req3.WithContext(context.WithValue(req3.Context(), "user_id", user.ID))
	w3 := httptest.NewRecorder()
	MeGetHandler(w3, req3)
	if w3.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w3.Code)
	}

	// 4. MePutHandler without context
	req4 := httptest.NewRequest("PUT", "/auth/@me", nil)
	w4 := httptest.NewRecorder()
	MePutHandler(w4, req4)
	if w4.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w4.Code)
	}

	// 5. MePutHandler with invalid JSON
	req5 := httptest.NewRequest("PUT", "/auth/@me", strings.NewReader("bad"))
	req5 = req5.WithContext(context.WithValue(req5.Context(), "user_id", user.ID))
	w5 := httptest.NewRecorder()
	MePutHandler(w5, req5)
	if w5.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w5.Code)
	}

	// 6. MePutHandler with valid update
	newEmail := "updated_me@example.com"
	newUsername := "updated_me_user"
	putBody, _ := json.Marshal(authModel.UpdateUserRequest{
		Email:    &newEmail,
		Username: &newUsername,
	})
	req6 := httptest.NewRequest("PUT", "/auth/@me", bytes.NewReader(putBody))
	req6 = req6.WithContext(context.WithValue(req6.Context(), "user_id", user.ID))
	w6 := httptest.NewRecorder()
	MePutHandler(w6, req6)
	if w6.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w6.Code)
	}

	var reloaded dbEngine.User
	db.First(&reloaded, user.ID)
	if reloaded.Email != newEmail || reloaded.Username != newUsername {
		t.Errorf("user not updated: %+v", reloaded)
	}
}
