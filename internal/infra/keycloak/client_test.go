package keycloak

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Nerzal/gocloak/v13"
)

func TestGetKeycloakClient(t *testing.T) {
	t.Setenv("KEYCLOAK_BASE_URL", "http://localhost:8080")
	client := GetKeycloakClient()
	if client == nil {
		t.Fatalf("expected non-nil gocloak client")
	}
}

func setupMockKeycloakServer(t *testing.T, shouldFailLogin bool, shouldFailUsers bool) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch r.URL.Path {
		case "/realms/testrealm/protocol/openid-connect/token",
			"/realms/master/protocol/openid-connect/token",
			"/realms/custom-admin/protocol/openid-connect/token":
			if shouldFailLogin {
				w.WriteHeader(http.StatusUnauthorized)
				fmt.Fprintf(w, `{"error":"invalid_grant","error_description":"Invalid user credentials"}`)
				return
			}
			token := gocloak.JWT{
				AccessToken:      "mock_access_token",
				ExpiresIn:        300,
				RefreshExpiresIn: 1800,
				RefreshToken:     "mock_refresh_token",
				TokenType:        "Bearer",
			}
			_ = json.NewEncoder(w).Encode(token)

		case "/admin/realms/testrealm/users":
			if shouldFailUsers {
				w.WriteHeader(http.StatusInternalServerError)
				fmt.Fprintf(w, `{"error":"internal_error"}`)
				return
			}
			uid1 := "uid-111"
			uid2 := "uid-222"
			users := []*gocloak.User{
				{
					ID:       &uid1,
					Username: gocloak.StringP("user1"),
					Email:    gocloak.StringP("user1@example.com"),
				},
				{
					ID:       &uid2,
					Username: gocloak.StringP("user2"),
					Email:    gocloak.StringP("user2@example.com"),
				},
				{
					// User without ID
					Username: gocloak.StringP("no_id_user"),
				},
			}
			_ = json.NewEncoder(w).Encode(users)

		default:
			http.NotFound(w, r)
		}
	}))
}

func TestGetAdminToken_ServiceAccountSuccess(t *testing.T) {
	ts := setupMockKeycloakServer(t, false, false)
	defer ts.Close()

	t.Setenv("KEYCLOAK_BASE_URL", ts.URL)
	t.Setenv("KEYCLOAK_REALM", "testrealm")
	t.Setenv("KEYCLOAK_CLIENT_ID", "test-client")
	t.Setenv("KEYCLOAK_CLIENT_SECRET", "test-secret")

	token, err := GetAdminToken(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token.AccessToken != "mock_access_token" {
		t.Errorf("expected mock_access_token, got %s", token.AccessToken)
	}
}

func TestGetAdminToken_AdminFallbackSuccess(t *testing.T) {
	ts := setupMockKeycloakServer(t, false, false)
	defer ts.Close()

	t.Setenv("KEYCLOAK_BASE_URL", ts.URL)
	t.Setenv("KEYCLOAK_REALM", "testrealm")
	t.Setenv("KEYCLOAK_CLIENT_ID", "")
	t.Setenv("KEYCLOAK_CLIENT_SECRET", "")
	t.Setenv("KEYCLOAK_ADMIN_USERNAME", "admin_user")
	t.Setenv("KEYCLOAK_ADMIN_PASSWORD", "admin_pass")
	t.Setenv("KEYCLOAK_ADMIN_REALM", "custom-admin")

	token, err := GetAdminToken(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token.AccessToken != "mock_access_token" {
		t.Errorf("expected mock_access_token, got %s", token.AccessToken)
	}
}

func TestGetAdminToken_DefaultCredentials(t *testing.T) {
	ts := setupMockKeycloakServer(t, false, false)
	defer ts.Close()

	t.Setenv("KEYCLOAK_BASE_URL", ts.URL)
	t.Setenv("KEYCLOAK_REALM", "testrealm")
	t.Setenv("KEYCLOAK_CLIENT_ID", "")
	t.Setenv("KEYCLOAK_CLIENT_SECRET", "")
	t.Setenv("KEYCLOAK_ADMIN_USERNAME", "")
	t.Setenv("KEYCLOAK_ADMIN_PASSWORD", "")
	t.Setenv("KEYCLOAK_ADMIN_REALM", "")

	token, err := GetAdminToken(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token.AccessToken != "mock_access_token" {
		t.Errorf("expected mock_access_token, got %s", token.AccessToken)
	}
}

func TestGetAdminToken_Failure(t *testing.T) {
	ts := setupMockKeycloakServer(t, true, false)
	defer ts.Close()

	t.Setenv("KEYCLOAK_BASE_URL", ts.URL)
	t.Setenv("KEYCLOAK_REALM", "testrealm")
	t.Setenv("KEYCLOAK_CLIENT_ID", "test-client")
	t.Setenv("KEYCLOAK_CLIENT_SECRET", "test-secret")

	token, err := GetAdminToken(context.Background())
	if err == nil || token != nil {
		t.Errorf("expected error, got token: %v", token)
	}
}

func TestGetAllKeycloakUsers_Success(t *testing.T) {
	ts := setupMockKeycloakServer(t, false, false)
	defer ts.Close()

	t.Setenv("KEYCLOAK_BASE_URL", ts.URL)
	t.Setenv("KEYCLOAK_REALM", "testrealm")
	t.Setenv("KEYCLOAK_CLIENT_ID", "test-client")
	t.Setenv("KEYCLOAK_CLIENT_SECRET", "test-secret")

	users, err := GetAllKeycloakUsers(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(users) != 3 {
		t.Errorf("expected 3 users, got %d", len(users))
	}
}

func TestGetAllKeycloakUsers_AuthFailure(t *testing.T) {
	ts := setupMockKeycloakServer(t, true, false)
	defer ts.Close()

	t.Setenv("KEYCLOAK_BASE_URL", ts.URL)
	t.Setenv("KEYCLOAK_REALM", "testrealm")
	t.Setenv("KEYCLOAK_CLIENT_ID", "test-client")
	t.Setenv("KEYCLOAK_CLIENT_SECRET", "test-secret")

	_, err := GetAllKeycloakUsers(context.Background())
	if err == nil {
		t.Errorf("expected error when admin token fails")
	}
}

func TestGetAllKeycloakUsers_GetUsersFailure(t *testing.T) {
	ts := setupMockKeycloakServer(t, false, true)
	defer ts.Close()

	t.Setenv("KEYCLOAK_BASE_URL", ts.URL)
	t.Setenv("KEYCLOAK_REALM", "testrealm")
	t.Setenv("KEYCLOAK_CLIENT_ID", "test-client")
	t.Setenv("KEYCLOAK_CLIENT_SECRET", "test-secret")

	_, err := GetAllKeycloakUsers(context.Background())
	if err == nil {
		t.Errorf("expected error when GetUsers fails")
	}
}

func TestGetKeycloakUserIDs_Success(t *testing.T) {
	ts := setupMockKeycloakServer(t, false, false)
	defer ts.Close()

	t.Setenv("KEYCLOAK_BASE_URL", ts.URL)
	t.Setenv("KEYCLOAK_REALM", "testrealm")
	t.Setenv("KEYCLOAK_CLIENT_ID", "test-client")
	t.Setenv("KEYCLOAK_CLIENT_SECRET", "test-secret")

	userIDs, err := GetKeycloakUserIDs(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !userIDs["uid-111"] || !userIDs["uid-222"] {
		t.Errorf("expected uid-111 and uid-222 in userIDs map: %+v", userIDs)
	}
	if len(userIDs) != 2 {
		t.Errorf("expected 2 valid user IDs in map, got %d", len(userIDs))
	}
}

func TestGetKeycloakUserIDs_Failure(t *testing.T) {
	ts := setupMockKeycloakServer(t, true, false)
	defer ts.Close()

	t.Setenv("KEYCLOAK_BASE_URL", ts.URL)
	t.Setenv("KEYCLOAK_REALM", "testrealm")
	t.Setenv("KEYCLOAK_CLIENT_ID", "test-client")
	t.Setenv("KEYCLOAK_CLIENT_SECRET", "test-secret")

	_, err := GetKeycloakUserIDs(context.Background())
	if err == nil {
		t.Errorf("expected error when GetAllKeycloakUsers fails")
	}
}
