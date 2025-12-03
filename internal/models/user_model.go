package models

import (
	"github.com/Nerzal/gocloak/v13"
	dbEngine "github.com/flotio-dev/api/internal/engines/db"
)

type UserContext struct {
	Keycloak *gocloak.UserInfo
	DB       *dbEngine.User
}
