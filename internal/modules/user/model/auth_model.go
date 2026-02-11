package model

import (
	"github.com/golang-jwt/jwt/v5"
)

type AccessClaims struct {
	UserID uint `json:"uid"`
	jwt.RegisteredClaims
}

type RefreshClaims struct {
	UserID  uint   `json:"uid"`
	TokenID string `json:"tid"`
	jwt.RegisteredClaims
}

type RegisterRequest struct {
	Email    string `json:"email"`
	Username string `json:"username"`
	Password string `json:"password"`
}

type AuthResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    string `json:"expires_in"`
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type RefreshTokenRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type StatusResponse struct {
	Status string `json:"status"`
}

type UpdateUserRequest struct {
	GithubID       *string `json:"github_id"`
	GithubUsername *string `json:"github_username"`
	Email          *string `json:"email"`
	Username       *string `json:"username"`
}
