package keycloakEngine

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/Nerzal/gocloak/v13"
)

func GetKeycloakClient() *gocloak.GoCloak {
	return gocloak.NewClient(os.Getenv("KEYCLOAK_BASE_URL"))
}

// GetAdminToken retrieves an admin token for Keycloak API calls
func GetAdminToken(ctx context.Context) (*gocloak.JWT, error) {
	client := GetKeycloakClient()
	realm := os.Getenv("KEYCLOAK_REALM")

	// Try service account first (client credentials)
	clientID := os.Getenv("KEYCLOAK_CLIENT_ID")
	clientSecret := os.Getenv("KEYCLOAK_CLIENT_SECRET")

	if clientSecret != "" {
		token, err := client.LoginClient(ctx, clientID, clientSecret, realm)
		if err == nil {
			return token, nil
		}
		log.Printf("Service account login failed, trying admin credentials: %v", err)
	}

	// Fallback to admin username/password authentication
	adminUsername := os.Getenv("KEYCLOAK_ADMIN_USERNAME")
	adminPassword := os.Getenv("KEYCLOAK_ADMIN_PASSWORD")

	if adminUsername == "" {
		adminUsername = "admin"
	}
	if adminPassword == "" {
		adminPassword = "admin"
	}

	// Use master realm for admin login, or the configured realm
	adminRealm := os.Getenv("KEYCLOAK_ADMIN_REALM")
	if adminRealm == "" {
		adminRealm = "master"
	}

	token, err := client.LoginAdmin(ctx, adminUsername, adminPassword, adminRealm)
	if err != nil {
		return nil, fmt.Errorf("admin login failed: %w", err)
	}
	return token, nil
}

// GetAllKeycloakUsers retrieves all users from Keycloak
func GetAllKeycloakUsers(ctx context.Context) ([]*gocloak.User, error) {
	client := GetKeycloakClient()
	realm := os.Getenv("KEYCLOAK_REALM")

	token, err := GetAdminToken(ctx)
	if err != nil {
		return nil, err
	}

	// Get all users (with pagination support)
	params := gocloak.GetUsersParams{
		Max: gocloak.IntP(1000), // Adjust as needed
	}

	users, err := client.GetUsers(ctx, token.AccessToken, realm, params)
	if err != nil {
		return nil, err
	}

	log.Printf("Retrieved %d users from Keycloak", len(users))
	return users, nil
}

// GetKeycloakUserIDs returns a map of Keycloak user IDs for quick lookup
func GetKeycloakUserIDs(ctx context.Context) (map[string]bool, error) {
	users, err := GetAllKeycloakUsers(ctx)
	if err != nil {
		return nil, err
	}

	userIDs := make(map[string]bool)
	for _, user := range users {
		if user.ID != nil {
			userIDs[*user.ID] = true
		}
	}
	return userIDs, nil
}
