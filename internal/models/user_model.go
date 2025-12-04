package models

import (
	"github.com/Nerzal/gocloak/v13"
	dbEngine "github.com/flotio-dev/api/internal/engines/db"
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
