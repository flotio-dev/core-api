package service

import (
	"context"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/alicebob/miniredis/v2"
	db "github.com/flotio-dev/core-api/internal/common/database"
	userModel "github.com/flotio-dev/core-api/internal/modules/user/model"
	repositories "github.com/flotio-dev/core-api/internal/modules/user/repository"
	"github.com/glebarez/sqlite"
	"github.com/golang-jwt/jwt/v5"
	"gorm.io/gorm"
)

func setupTestRedis(t *testing.T) *miniredis.Miniredis {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to run miniredis: %v", err)
	}
	os.Setenv("REDIS_ADDR", mr.Addr())
	db.InitRedis()
	return mr
}

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	testDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open in-memory sqlite: %v", err)
	}
	if err := testDB.AutoMigrate(&db.User{}); err != nil {
		t.Fatalf("auto-migration failed: %v", err)
	}
	db.DB = testDB
	return testDB
}

func TestAuthService_Tokens(t *testing.T) {
	AccessSecret = []byte("test-access-secret-32-bytes-long!")
	RefreshSecret = []byte("test-refresh-secret-32-bytes-long!")

	// 1. GenerateAccessToken
	tokenStr, err := GenerateAccessToken(42)
	if err != nil {
		t.Fatalf("GenerateAccessToken failed: %v", err)
	}
	token, err := jwt.ParseWithClaims(tokenStr, &userModel.AccessClaims{}, func(tok *jwt.Token) (interface{}, error) {
		return AccessSecret, nil
	})
	if err != nil || !token.Valid {
		t.Fatalf("invalid access token: %v", err)
	}
	claims := token.Claims.(*userModel.AccessClaims)
	if claims.UserID != 42 {
		t.Errorf("expected userID 42, got %d", claims.UserID)
	}

	// 2. GenerateRefreshToken
	refreshToken, tokenID, err := GenerateRefreshToken(42)
	if err != nil {
		t.Fatalf("GenerateRefreshToken failed: %v", err)
	}
	if tokenID == "" {
		t.Error("expected non-empty tokenID")
	}
	refToken, err := jwt.ParseWithClaims(refreshToken, &userModel.RefreshClaims{}, func(tok *jwt.Token) (interface{}, error) {
		return RefreshSecret, nil
	})
	if err != nil || !refToken.Valid {
		t.Fatalf("invalid refresh token: %v", err)
	}
	refClaims := refToken.Claims.(*userModel.RefreshClaims)
	if refClaims.UserID != 42 || refClaims.TokenID != tokenID {
		t.Errorf("refresh claims mismatch: %+v", refClaims)
	}
}

func TestAuthService_StoreAndRevokeRefreshToken(t *testing.T) {
	mr := setupTestRedis(t)
	defer mr.Close()

	ctx := context.Background()
	tokenID := "test-tid-123"
	userID := uint(42)

	// Store
	if err := StoreRefreshToken(ctx, tokenID, userID); err != nil {
		t.Fatalf("StoreRefreshToken failed: %v", err)
	}

	val, err := db.Redis.Get(ctx, "refresh:"+tokenID).Result()
	if err != nil || val != "42" {
		t.Errorf("expected stored token value 42, got %s (err: %v)", val, err)
	}

	// Revoke
	if err := RevokeRefreshToken(ctx, tokenID); err != nil {
		t.Fatalf("RevokeRefreshToken failed: %v", err)
	}

	_, err = db.Redis.Get(ctx, "refresh:"+tokenID).Result()
	if err == nil {
		t.Error("expected key to be deleted after revoke")
	}
}

func TestAuthService_GetUserIDFromContext(t *testing.T) {
	ctxWithout := context.Background()
	if _, ok := GetUserIDFromContext(ctxWithout); ok {
		t.Error("expected false for context without user_id")
	}

	ctxWith := context.WithValue(ctxWithout, "user_id", uint(99))
	id, ok := GetUserIDFromContext(ctxWith)
	if !ok || id != 99 {
		t.Errorf("expected 99, true; got %d, %v", id, ok)
	}
}

func TestAuthService_Cookies(t *testing.T) {
	// Dev environment (not production)
	os.Setenv("APP_ENV", "development")
	w := httptest.NewRecorder()
	SetRefreshTokenCookie(w, "dev-token", 3600)
	resp := w.Result()
	cookies := resp.Cookies()
	if len(cookies) == 0 {
		t.Fatal("expected cookie set")
	}
	if cookies[0].Value != "dev-token" || cookies[0].Secure {
		t.Errorf("unexpected dev cookie: %+v", cookies[0])
	}

	// Clear cookie in production
	os.Setenv("APP_ENV", "production")
	wProd := httptest.NewRecorder()
	SetRefreshTokenCookie(wProd, "prod-token", 3600)
	prodCookie := wProd.Result().Cookies()[0]
	if !prodCookie.Secure {
		t.Error("expected secure cookie in production")
	}

	wClear := httptest.NewRecorder()
	ClearRefreshTokenCookie(wClear)
	clearCookie := wClear.Result().Cookies()[0]
	if clearCookie.MaxAge != -1 || clearCookie.Value != "" {
		t.Errorf("expected cleared cookie, got %+v", clearCookie)
	}
}

func TestUserService(t *testing.T) {
	testDB := setupTestDB(t)
	repo := repositories.NewUserRepository(testDB)
	svc := NewUserService(repo)

	// Create user
	u := db.User{
		Email:    "test@example.com",
		Username: "tester",
	}
	if err := testDB.Create(&u).Error; err != nil {
		t.Fatal(err)
	}

	// GetUserFromContext without ID
	if _, err := svc.GetUserFromContext(context.Background()); err == nil {
		t.Error("expected error when user id not in context")
	}

	// GetUserFromContext with invalid ID
	ctxNotFound := context.WithValue(context.Background(), "user_id", uint(9999))
	if _, err := svc.GetUserFromContext(ctxNotFound); err == nil {
		t.Error("expected error when user not in database")
	}

	// GetUserFromContext valid
	ctxValid := context.WithValue(context.Background(), "user_id", u.ID)
	found, err := svc.GetUserFromContext(ctxValid)
	if err != nil || found.Email != "test@example.com" {
		t.Fatalf("failed to get user from context: %v", err)
	}

	// UpdateUser nil user
	if err := svc.UpdateUser(nil, &userModel.UserUpdateRequest{}); err == nil {
		t.Error("expected error for nil user")
	}

	// UpdateUser valid
	newEmail := "updated@example.com"
	newUsername := "updateduser"
	err = svc.UpdateUser(found, &userModel.UserUpdateRequest{
		Email:    &newEmail,
		Username: &newUsername,
	})
	if err != nil {
		t.Fatalf("UpdateUser failed: %v", err)
	}

	// Verify update in DB
	var reloaded db.User
	testDB.First(&reloaded, u.ID)
	if reloaded.Email != newEmail || reloaded.Username != newUsername {
		t.Errorf("mismatch after update: %+v", reloaded)
	}
}
