package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/Nerzal/gocloak/v13"
	middleware "github.com/flotio-dev/api/pkg/api/v1/middleware"
	"github.com/flotio-dev/api/pkg/db"
	utils "github.com/flotio-dev/api/pkg/utils"
)

func getAdminToken(ctx context.Context, client *gocloak.GoCloak) (*gocloak.JWT, error) {
	return client.LoginAdmin(ctx, "admin", "admin", "master")
}

// seededRand is a package-level RNG seeded once
var seededRand = rand.New(rand.NewSource(time.Now().UnixNano()))

// genRandomName returns a realistic-looking first and last name.
// It picks randomly from small predefined lists; this keeps tests stable
// and avoids adding external resources.
func genRandomName() (first, last string) {
	firstNames := []string{
		"Alice", "Bob", "Caroline", "David", "Emma", "Frank", "Grace", "Hugo", "Iris", "Julien",
		"Lucas", "Maya", "Noah", "Olivia", "Paul", "Quentin", "Romain", "Sophie", "Thomas", "Victor",
	}
	lastNames := []string{
		"Martin", "Bernard", "Dubois", "Leroy", "Moreau", "Faure", "Rousseau", "Garnier", "Laurent", "Petit",
		"Lambert", "Dupont", "Simon", "Michel", "Garcia", "David", "Bertrand", "Morel", "Robin", "Leclerc",
	}

	first = firstNames[seededRand.Intn(len(firstNames))]
	last = lastNames[seededRand.Intn(len(lastNames))]
	return
}

// Auth handlers
func RegisterHandler(w http.ResponseWriter, r *http.Request) {
	var userData struct {
		Username string `json:"username"`
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&userData); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	client := utils.GetKeycloakClient()
	ctx := context.Background()
	token, err := getAdminToken(ctx, client)
	if err != nil {
		http.Error(w, "Failed to authenticate with Keycloak", http.StatusInternalServerError)
		return
	}

	realm := os.Getenv("KEYCLOAK_REALM")

	// Create user
	// Ensure required actions are empty so the account is considered fully set up
	// (avoids Keycloak returning "Account is not fully set up" on direct grant)
	requiredActions := []string{}
	firstName, lastName := genRandomName()
	user := &gocloak.User{
		Username:        &userData.Username,
		Email:           &userData.Email,
		FirstName:       gocloak.StringP(firstName),
		LastName:        gocloak.StringP(lastName),
		Enabled:         gocloak.BoolP(true),
		EmailVerified:   gocloak.BoolP(true),
		RequiredActions: &requiredActions,
	}
	userID, err := client.CreateUser(ctx, token.AccessToken, realm, *user)
	if err != nil {
		log.Printf("CreateUser failed for %s: %v", userData.Username, err)
		http.Error(w, "Failed to create user", http.StatusInternalServerError)
		return
	}
	log.Printf("Created Keycloak user: %s (username=%s)", userID, userData.Username)

	// Set password
	err = client.SetPassword(ctx, token.AccessToken, userID, realm, userData.Password, false)
	if err != nil {
		http.Error(w, "Failed to set password", http.StatusInternalServerError)
		return
	}

	// Create user in DB
	dbUser := db.User{
		KeycloakID: userID,
		Email:      userData.Email,
		Username:   userData.Username,
	}
	if err := db.DB.Create(&dbUser).Error; err != nil {
		http.Error(w, "Failed to create user in database", http.StatusInternalServerError)
		return
	}

	// After successful registration, perform a direct login to return the same response as LoginHandler
	clientID := os.Getenv("KEYCLOAK_CLIENT_ID")
	clientSecret := os.Getenv("KEYCLOAK_CLIENT_SECRET")

	tokenResp, err := client.Login(ctx, clientID, clientSecret, realm, userData.Username, userData.Password)
	if err != nil {
		// If login fails for any reason, fall back to returning a registration success message
		log.Printf("Auto-login failed for %s: %v", userData.Username, err)
		utils.WriteJSON(w, map[string]string{"status": "registered", "message": "User registered successfully. Please login."})
		return
	}

	utils.WriteJSON(w, map[string]string{
		"access_token":  tokenResp.AccessToken,
		"refresh_token": tokenResp.RefreshToken,
		"expires_in":    fmt.Sprintf("%d", tokenResp.ExpiresIn),
	})
}

func LoginHandler(w http.ResponseWriter, r *http.Request) {
	var creds struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&creds); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	client := utils.GetKeycloakClient()
	ctx := context.Background()
	realm := os.Getenv("KEYCLOAK_REALM")
	clientID := os.Getenv("KEYCLOAK_CLIENT_ID")
	clientSecret := os.Getenv("KEYCLOAK_CLIENT_SECRET")

	log.Printf("Login attempt - Realm: %s, ClientID: %s, Username: %s", realm, clientID, creds.Username)

	token, err := client.Login(ctx, clientID, clientSecret, realm, creds.Username, creds.Password)
	if err != nil {
		log.Printf("Login failed for user %s: %v", creds.Username, err)
		http.Error(w, "Invalid credentials", http.StatusUnauthorized)
		return
	}

	utils.WriteJSON(w, map[string]string{
		"access_token":  token.AccessToken,
		"refresh_token": token.RefreshToken,
		"expires_in":    fmt.Sprintf("%d", token.ExpiresIn),
	})
}

func RefreshTokenHandler(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	client := utils.GetKeycloakClient()
	ctx := context.Background()
	realm := os.Getenv("KEYCLOAK_REALM")
	clientID := os.Getenv("KEYCLOAK_CLIENT_ID")
	clientSecret := os.Getenv("KEYCLOAK_CLIENT_SECRET")

	token, err := client.RefreshToken(ctx, body.RefreshToken, clientID, clientSecret, realm)
	if err != nil {
		log.Printf("Refresh token failed: %v", err)
		http.Error(w, "Invalid refresh token", http.StatusUnauthorized)
		return
	}

	utils.WriteJSON(w, map[string]string{
		"access_token":  token.AccessToken,
		"refresh_token": token.RefreshToken,
		"expires_in":    fmt.Sprintf("%d", token.ExpiresIn),
	})
}

func MeGetHandler(w http.ResponseWriter, r *http.Request) {
	userInfo := middleware.GetUserFromContext(r.Context())
	if userInfo == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	utils.WriteJSON(w, userInfo)
}

func MePutHandler(w http.ResponseWriter, r *http.Request) {
	userInfo := middleware.GetUserFromContext(r.Context())
	if userInfo == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var updateData struct {
		Email    *string `json:"email,omitempty"`
		Username *string `json:"username,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&updateData); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	client := utils.GetKeycloakClient()
	ctx := context.Background()
	realm := os.Getenv("KEYCLOAK_REALM")

	adminToken, err := getAdminToken(ctx, client)
	if err != nil {
		http.Error(w, "Failed to authenticate with Keycloak", http.StatusInternalServerError)
		return
	}

	// Update user
	userUpdate := &gocloak.User{
		ID:       userInfo.Sub,
		Email:    updateData.Email,
		Username: updateData.Username,
	}
	err = client.UpdateUser(ctx, adminToken.AccessToken, realm, *userUpdate)
	if err != nil {
		http.Error(w, "Failed to update user", http.StatusInternalServerError)
		return
	}

	// Persist changes to local DB as well (e.g., email)
	var dbUser db.User
	if err := db.DB.Where("keycloak_id = ?", *userInfo.Sub).First(&dbUser).Error; err != nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	if updateData.Email != nil {
		dbUser.Email = *updateData.Email
	}
	// Note: first/last name are stored in Keycloak; update local username only if desired.
	if updateData.Username != nil {
		// Optionally update username from first name if the app uses it; keep current username by default.
		dbUser.Username = *updateData.Username
	}

	if err := db.DB.Save(&dbUser).Error; err != nil {
		http.Error(w, "Failed to update user in database", http.StatusInternalServerError)
		return
	}

	utils.WriteJSON(w, map[string]string{"status": "updated"})
}

func GithubCallbackHandler(w http.ResponseWriter, r *http.Request) {
	// This is a public endpoint for GitHub OAuth callback
	// It should redirect to the frontend with the code
	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "Missing code parameter", http.StatusBadRequest)
		return
	}

	// Redirect to frontend with the code
	frontendURL := "http://localhost:3000/auth/github/callback?code=" + code
	http.Redirect(w, r, frontendURL, http.StatusFound)
}

func GithubHandler(w http.ResponseWriter, r *http.Request) {
	userInfo := middleware.GetUserFromContext(r.Context())
	if userInfo == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	action := r.URL.Query().Get("action")
	switch action {
	case "login":
		// Generate GitHub OAuth URL
		clientID := os.Getenv("GITHUB_CLIENT_ID")
		if clientID == "" {
			http.Error(w, "GitHub client ID not configured", http.StatusInternalServerError)
			return
		}
		redirectURI := "http://localhost:8080/auth/github/callback" // API callback URL
		scope := "repo,user"
		url := fmt.Sprintf("https://github.com/login/oauth/authorize?client_id=%s&redirect_uri=%s&scope=%s", clientID, url.QueryEscape(redirectURI), scope)
		utils.WriteJSON(w, map[string]string{"login_url": url})

	case "callback":
		// Handle GitHub OAuth callback
		code := r.URL.Query().Get("code")
		if code == "" {
			http.Error(w, "Missing code parameter", http.StatusBadRequest)
			return
		}

		// Exchange code for tokens
		clientID := os.Getenv("GITHUB_CLIENT_ID")
		clientSecret := os.Getenv("GITHUB_CLIENT_SECRET")
		if clientID == "" || clientSecret == "" {
			http.Error(w, "GitHub client not configured", http.StatusInternalServerError)
			return
		}

		// Make request to GitHub to exchange code for tokens
		tokenURL := "https://github.com/login/oauth/access_token"
		data := url.Values{}
		data.Set("client_id", clientID)
		data.Set("client_secret", clientSecret)
		data.Set("code", code)

		resp, err := http.PostForm(tokenURL, data)
		if err != nil {
			http.Error(w, "Failed to exchange code", http.StatusInternalServerError)
			return
		}
		defer resp.Body.Close()

		var tokenResp struct {
			AccessToken  string `json:"access_token"`
			TokenType    string `json:"token_type"`
			Scope        string `json:"scope"`
			RefreshToken string `json:"refresh_token,omitempty"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
			http.Error(w, "Failed to parse token response", http.StatusInternalServerError)
			return
		}

		// Store tokens in DB
		var user db.User
		if err := db.DB.Where("keycloak_id = ?", *userInfo.Sub).First(&user).Error; err != nil {
			http.Error(w, "User not found", http.StatusNotFound)
			return
		}

		user.GithubAccessToken = tokenResp.AccessToken
		user.GithubRefreshToken = tokenResp.RefreshToken
		if err := db.DB.Save(&user).Error; err != nil {
			http.Error(w, "Failed to save tokens", http.StatusInternalServerError)
			return
		}

		utils.WriteJSON(w, map[string]string{"status": "connected"})

	case "list-repo":
		// Get user's GitHub repos using stored token
		var user db.User
		if err := db.DB.Where("keycloak_id = ?", *userInfo.Sub).First(&user).Error; err != nil {
			http.Error(w, "User not found", http.StatusNotFound)
			return
		}

		if user.GithubAccessToken == "" {
			http.Error(w, "GitHub not connected", http.StatusUnauthorized)
			return
		}

		// Make request to GitHub API
		req, err := http.NewRequest("GET", "https://api.github.com/user/repos", nil)
		if err != nil {
			http.Error(w, "Failed to create request", http.StatusInternalServerError)
			return
		}
		req.Header.Set("Authorization", "token "+user.GithubAccessToken)
		req.Header.Set("Accept", "application/vnd.github.v3+json")

		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			http.Error(w, "Failed to fetch repos", http.StatusInternalServerError)
			return
		}
		defer resp.Body.Close()

		var repos []map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&repos); err != nil {
			http.Error(w, "Failed to parse repos", http.StatusInternalServerError)
			return
		}

		utils.WriteJSON(w, map[string]interface{}{"repos": repos})

	case "detail-repo":
		id := r.URL.Query().Get("id")
		if id == "" {
			http.Error(w, "Missing id parameter", http.StatusBadRequest)
			return
		}

		// Get user's GitHub token
		var user db.User
		if err := db.DB.Where("keycloak_id = ?", *userInfo.Sub).First(&user).Error; err != nil {
			http.Error(w, "User not found", http.StatusNotFound)
			return
		}

		if user.GithubAccessToken == "" {
			http.Error(w, "GitHub not connected", http.StatusUnauthorized)
			return
		}

		// Make request to GitHub API for repo contents
		apiURL := fmt.Sprintf("https://api.github.com/repositories/%s/contents", id)
		req, err := http.NewRequest("GET", apiURL, nil)
		if err != nil {
			http.Error(w, "Failed to create request", http.StatusInternalServerError)
			return
		}
		req.Header.Set("Authorization", "token "+user.GithubAccessToken)
		req.Header.Set("Accept", "application/vnd.github.v3+json")

		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			http.Error(w, "Failed to fetch repo contents", http.StatusInternalServerError)
			return
		}
		defer resp.Body.Close()

		var contents []map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&contents); err != nil {
			http.Error(w, "Failed to parse contents", http.StatusInternalServerError)
			return
		}

		// Extract folder names
		var folders []string
		for _, item := range contents {
			if item["type"] == "dir" {
				if name, ok := item["name"].(string); ok {
					folders = append(folders, name)
				}
			}
		}

		utils.WriteJSON(w, map[string]interface{}{"repo_id": id, "folders": folders})

	default:
		http.Error(w, "Invalid action", http.StatusBadRequest)
	}
}
