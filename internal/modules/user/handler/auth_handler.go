package handler

import (
	"encoding/json"
	"net/http"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	dbEngine "github.com/flotio-dev/core-api/internal/common/database"
	helpers "github.com/flotio-dev/core-api/internal/common/server"

	authModel "github.com/flotio-dev/core-api/internal/modules/user/model"
	authServices "github.com/flotio-dev/core-api/internal/modules/user/service"
)

func RegisterHandler(w http.ResponseWriter, r *http.Request) {
	var body authModel.RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		helpers.WriteErrorJSON(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	hash, _ := bcrypt.GenerateFromPassword([]byte(body.Password), 12)

	user := dbEngine.User{
		Email:        body.Email,
		Username:     body.Username,
		PasswordHash: string(hash),
	}

	if err := dbEngine.DB.Create(&user).Error; err != nil {
		helpers.WriteErrorJSON(w, "User already exists", http.StatusBadRequest)
		return
	}

	access, _ := authServices.GenerateAccessToken(user.ID)
	refresh, tid, _ := authServices.GenerateRefreshToken(user.ID)
	authServices.StoreRefreshToken(r.Context(), tid, user.ID)

	helpers.WriteJSON(w, authModel.AuthResponse{
		AccessToken:  access,
		RefreshToken: refresh,
		ExpiresIn:    "900",
	})
}

func LoginHandler(w http.ResponseWriter, r *http.Request) {
	var body authModel.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		helpers.WriteErrorJSON(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	var user dbEngine.User
	if err := dbEngine.DB.Where("username = ?", body.Username).First(&user).Error; err != nil {
		helpers.WriteErrorJSON(w, "Invalid credentials", http.StatusUnauthorized)
		return
	}

	if bcrypt.CompareHashAndPassword(
		[]byte(user.PasswordHash),
		[]byte(body.Password),
	) != nil {
		helpers.WriteErrorJSON(w, "Invalid credentials", http.StatusUnauthorized)
		return
	}

	access, _ := authServices.GenerateAccessToken(user.ID)
	refresh, tid, _ := authServices.GenerateRefreshToken(user.ID)
	authServices.StoreRefreshToken(r.Context(), tid, user.ID)

	helpers.WriteJSON(w, authModel.AuthResponse{
		AccessToken:  access,
		RefreshToken: refresh,
		ExpiresIn:    "900",
	})
}

func RefreshTokenHandler(w http.ResponseWriter, r *http.Request) {
	var body authModel.RefreshTokenRequest
	json.NewDecoder(r.Body).Decode(&body)

	token, err := jwt.ParseWithClaims(
		body.RefreshToken,
		&authModel.RefreshClaims{},
		func(t *jwt.Token) (interface{}, error) {
			return authServices.RefreshSecret, nil
		},
	)
	if err != nil || !token.Valid {
		helpers.WriteErrorJSON(w, "Invalid refresh token", http.StatusUnauthorized)
		return
	}

	claims := token.Claims.(*authModel.RefreshClaims)

	key := "refresh:" + claims.TokenID
	if _, err := dbEngine.Redis.Get(r.Context(), key).Result(); err != nil {
		helpers.WriteErrorJSON(w, "Refresh token revoked", http.StatusUnauthorized)
		return
	}

	// rotation
	authServices.RevokeRefreshToken(r.Context(), claims.TokenID)

	access, _ := authServices.GenerateAccessToken(claims.UserID)
	refresh, tid, _ := authServices.GenerateRefreshToken(claims.UserID)
	authServices.StoreRefreshToken(r.Context(), tid, claims.UserID)
	helpers.WriteJSON(w, authModel.AuthResponse{
		AccessToken:  access,
		RefreshToken: refresh,
		ExpiresIn:    "900",
	})
}

func LogoutHandler(w http.ResponseWriter, r *http.Request) {
	var body authModel.RefreshTokenRequest
	json.NewDecoder(r.Body).Decode(&body)

	token, _ := jwt.ParseWithClaims(
		body.RefreshToken,
		&authModel.RefreshClaims{},
		func(t *jwt.Token) (interface{}, error) {
			return authServices.RefreshSecret, nil
		},
	)

	if claims, ok := token.Claims.(*authModel.RefreshClaims); ok {
		authServices.RevokeRefreshToken(r.Context(), claims.TokenID)
	}

	helpers.WriteJSON(w, authModel.StatusResponse{Status: "logged_out"})
}

func MeGetHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := authServices.GetUserIDFromContext(r.Context())
	if !ok {
		helpers.WriteErrorJSON(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var user dbEngine.User
	if err := dbEngine.DB.First(&user, userID).Error; err != nil {
		helpers.WriteErrorJSON(w, "User not found", http.StatusNotFound)
		return
	}

	// ⚠️ Ne JAMAIS retourner le password hash
	response := map[string]interface{}{
		"id":       user.ID,
		"email":    user.Email,
		"username": user.Username,
		"created":  user.CreatedAt,
	}

	helpers.WriteJSON(w, response)
}

func MePutHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := authServices.GetUserIDFromContext(r.Context())
	if !ok {
		helpers.WriteErrorJSON(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var body authModel.UpdateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		helpers.WriteErrorJSON(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	var user dbEngine.User
	if err := dbEngine.DB.First(&user, userID).Error; err != nil {
		helpers.WriteErrorJSON(w, "User not found", http.StatusNotFound)
		return
	}

	// Mise à jour contrôlée
	if body.Email != nil {
		user.Email = *body.Email
	}
	if body.Username != nil {
		user.Username = *body.Username
	}

	if err := dbEngine.DB.Save(&user).Error; err != nil {
		helpers.WriteErrorJSON(w, "Failed to update user", http.StatusInternalServerError)
		return
	}

	helpers.WriteJSON(w, authModel.StatusResponse{Status: "updated"})
}
