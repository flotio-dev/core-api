package handler

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	dbEngine "github.com/flotio-dev/core-api/internal/common/database"
	helpers "github.com/flotio-dev/core-api/internal/common/server"
	models "github.com/flotio-dev/core-api/internal/models"

	authModel "github.com/flotio-dev/core-api/internal/modules/user/model"
	authServices "github.com/flotio-dev/core-api/internal/modules/user/service"
)

// RegisterHandler godoc
//
//	@Summary		Register a new user
//	@Description	Create a new user account and return access & refresh tokens
//	@Tags			auth
//	@Accept			json
//	@Produce		json
//	@Param			register	body		authModel.RegisterRequest	true	"Register payload"
//	@Success		200			{object}	authModel.AuthResponse
//	@Failure		400			{object}	models.APIErrorResponse
//	@Failure		500			{object}	models.APIErrorResponse
//	@ID				RegisterHandler
//	@Router			/auth/register [post]
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

	authServices.SetRefreshTokenCookie(w, refresh, 7*24*3600)

	helpers.WriteJSON(w, authModel.AuthResponse{
		AccessToken:  access,
		RefreshToken: refresh,
		ExpiresIn:    "900",
	})
}

// LoginHandler godoc
//
//	@Summary		Login
//	@Description	Authenticate a user and return access & refresh tokens
//	@Tags			auth
//	@Accept			json
//	@Produce		json
//	@Param			login	body		authModel.LoginRequest	true	"Login payload"
//	@Success		200		{object}	authModel.AuthResponse
//	@Failure		400		{object}	models.APIErrorResponse
//	@Failure		401		{object}	models.APIErrorResponse
//	@ID				LoginHandler
//	@Router			/auth/login [post]
func LoginHandler(w http.ResponseWriter, r *http.Request) {
	var body authModel.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		helpers.WriteErrorJSON(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	var user dbEngine.User
	if err := dbEngine.DB.Where("email = ?", body.Email).First(&user).Error; err != nil {
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

	authServices.SetRefreshTokenCookie(w, refresh, 7*24*3600)

	helpers.WriteJSON(w, authModel.AuthResponse{
		AccessToken:  access,
		RefreshToken: refresh,
		ExpiresIn:    "900",
	})
}

// RefreshTokenHandler godoc
//
//	@Summary		Refresh access token
//	@Description	Generate a new access token using a valid refresh token (rotation enabled)
//	@Tags			auth
//	@Accept			json
//	@Produce		json
//	@Param			refresh	body		authModel.RefreshTokenRequest	true	"Refresh token payload"
//	@Success		200		{object}	authModel.AuthResponse
//	@Failure		400		{object}	models.APIErrorResponse
//	@Failure		401		{object}	models.APIErrorResponse
//	@ID				RefreshTokenHandler
//	@Router			/auth/refresh [post]
func RefreshTokenHandler(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("refresh_token")
	if err != nil {
		helpers.WriteJSON(w, "Refresh token not provided")
		return
	}
	refreshToken := cookie.Value

	token, err := jwt.ParseWithClaims(
		refreshToken,
		&authModel.RefreshClaims{},
		func(t *jwt.Token) (interface{}, error) {
			return authServices.RefreshSecret, nil
		},
	)
	if err != nil || !token.Valid {
		helpers.WriteJSON(w, "Invalid refresh token")
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

	authServices.SetRefreshTokenCookie(w, refresh, 7*24*3600)

	helpers.WriteJSON(w, authModel.AuthResponse{
		AccessToken:  access,
		RefreshToken: refresh,
		ExpiresIn:    "900",
	})
}

// LogoutHandler godoc
//
//	@Summary		Logout
//	@Description	Revoke a refresh token and logout the user
//	@Tags			auth
//	@Accept			json
//	@Produce		json
//	@Param			logout	body		authModel.RefreshTokenRequest	true	"Refresh token payload"
//	@Success		200		{object}	authModel.StatusResponse
//	@Failure		400		{object}	models.APIErrorResponse
//	@ID				LogoutHandler
//	@Router			/auth/logout [post]
func LogoutHandler(w http.ResponseWriter, r *http.Request) {
	var body authModel.RefreshTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil && err != io.EOF {
		helpers.WriteErrorJSON(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	refreshToken := body.RefreshToken
	if refreshToken == "" {
		if cookie, err := r.Cookie("refresh_token"); err == nil {
			refreshToken = cookie.Value
		}
	}

	if refreshToken != "" {
		token, err := jwt.ParseWithClaims(
			refreshToken,
			&authModel.RefreshClaims{},
			func(t *jwt.Token) (interface{}, error) {
				return authServices.RefreshSecret, nil
			},
		)

		if err == nil && token != nil {
			if claims, ok := token.Claims.(*authModel.RefreshClaims); ok && claims.TokenID != "" {
				_ = authServices.RevokeRefreshToken(r.Context(), claims.TokenID)
			}
		}
	}

	authServices.ClearRefreshTokenCookie(w)

	helpers.WriteJSON(w, authModel.StatusResponse{Status: "logged_out"})
}

// MeGetHandler godoc
//
//	@Summary		Get current user
//	@Description	Return information about the authenticated user
//	@Tags			auth
//	@Produce		json
//	@Success		200	{object}	authModel.UserResponse
//	@Failure		401	{object}	models.APIErrorResponse
//	@Failure		404	{object}	models.APIErrorResponse
//	@Security		BearerAuth
//	@ID				MeGetHandler
//	@Router			/auth/@me [get]
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

	response := authModel.UserResponse{
		ID:       user.ID,
		Email:    user.Email,
		Username: user.Username,
		Created:  user.CreatedAt,
	}

	helpers.WriteJSON(w, response)
}

// MePutHandler godoc
//
//	@Summary		Update current user
//	@Description	Update email and/or username of the authenticated user
//	@Tags			auth
//	@Accept			json
//	@Produce		json
//	@Param			user	body		authModel.UpdateUserRequest	true	"User update payload"
//	@Success		200		{object}	authModel.StatusResponse
//	@Failure		400		{object}	models.APIErrorResponse
//	@Failure		401		{object}	models.APIErrorResponse
//	@Failure		404		{object}	models.APIErrorResponse
//	@Failure		500		{object}	models.APIErrorResponse
//	@Security		BearerAuth
//	@ID				MePutHandler
//	@Router			/auth/@me [put]
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

// Keep the swag annotation import alive (used only in @Failure comments).
var _ = models.APIErrorResponse{}
