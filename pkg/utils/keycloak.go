package utils

import (
	"os"

	"github.com/Nerzal/gocloak/v13"
)

func GetKeycloakClient() *gocloak.GoCloak {
	return gocloak.NewClient(os.Getenv("KEYCLOAK_BASE_URL"))
}
