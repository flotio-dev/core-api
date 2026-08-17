package model

import (
	"time"

	"github.com/Nerzal/gocloak/v13"
	dbEngine "github.com/flotio-dev/core-api/internal/common/database"
)

type UserContext struct {
	Keycloak *gocloak.UserInfo
	DB       *dbEngine.User
}

type UserUpdateRequest struct {
	GithubID       *string `json:"github_id"`
	GithubUsername *string `json:"github_username"`
	Email          *string `json:"email"`
	Username       *string `json:"username"`
}

// UserResponse is the payload of GET /auth/@me (contract §5.3).
type UserResponse struct {
	ID       uint      `json:"id" example:"1"`
	Email    string    `json:"email" example:"user@example.com"`
	Username string    `json:"username" example:"johndoe"`
	Created  time.Time `json:"created"`
}
