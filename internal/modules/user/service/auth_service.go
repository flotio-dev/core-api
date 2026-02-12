package service

import (
	"context"
	"net/http"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	authModel "github.com/flotio-dev/core-api/internal/modules/user/model"

	db "github.com/flotio-dev/core-api/internal/common/database"
)

var (
	AccessSecret  = []byte(os.Getenv("ACCESS_TOKEN_SECRET"))
	RefreshSecret = []byte(os.Getenv("REFRESH_TOKEN_SECRET"))
)

func GenerateAccessToken(userID uint) (string, error) {
	claims := authModel.AccessClaims{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(15 * time.Minute)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).
		SignedString(AccessSecret)
}

func GenerateRefreshToken(userID uint) (string, string, error) {
	tokenID := uuid.NewString()

	claims := authModel.RefreshClaims{
		UserID:  userID,
		TokenID: tokenID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(7 * 24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).
		SignedString(RefreshSecret)

	return token, tokenID, err
}

func StoreRefreshToken(ctx context.Context, tokenID string, userID uint) error {
	return db.Redis.Set(
		ctx,
		"refresh:"+tokenID,
		userID,
		7*24*time.Hour,
	).Err()
}

func RevokeRefreshToken(ctx context.Context, tokenID string) error {
	return db.Redis.Del(ctx, "refresh:"+tokenID).Err()
}

func GetUserIDFromContext(ctx context.Context) (uint, bool) {
	id, ok := ctx.Value("user_id").(uint)
	return id, ok
}

func SetRefreshTokenCookie(w http.ResponseWriter, token string, maxAge int) {
	secure := os.Getenv("APP_ENV") == "production"
	sameSite := http.SameSiteLaxMode

	if secure {
		sameSite = http.SameSiteNoneMode
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "refresh_token",
		Value:    token,
		HttpOnly: true,
		Secure:   secure,
		Path:     "/",
		MaxAge:   maxAge,
		SameSite: sameSite,
	})
}

func ClearRefreshTokenCookie(w http.ResponseWriter) {
	secure := os.Getenv("APP_ENV") == "production"
	sameSite := http.SameSiteLaxMode

	if secure {
		sameSite = http.SameSiteNoneMode
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "refresh_token",
		Value:    "",
		HttpOnly: true,
		Secure:   secure,
		Path:     "/",
		MaxAge:   -1,
		SameSite: sameSite,
	})
}
