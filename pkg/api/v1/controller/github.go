package controller

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/go-github/v76/github"
	"golang.org/x/oauth2"
	githubOAuth "golang.org/x/oauth2/github"
	"gorm.io/gorm/clause"

	middleware "github.com/flotio-dev/api/pkg/api/v1/middleware"
	db "github.com/flotio-dev/api/pkg/db"
)

type GithubController struct {
	webhookSecretKey []byte
	oauthConfig      *oauth2.Config
}

func NewGithubController(secret []byte) *GithubController {
	return &GithubController{
		webhookSecretKey: secret,
		oauthConfig: &oauth2.Config{
			ClientID:     os.Getenv("GITHUB_CLIENT_ID"),
			ClientSecret: os.Getenv("GITHUB_CLIENT_SECRET"),
			RedirectURL:  os.Getenv("GITHUB_REDIRECT_URL"),
			Scopes:       []string{"user", "repo"},
			Endpoint:     githubOAuth.Endpoint,
		},
	}
}

func (c *GithubController) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	// userInfo := middleware.GetUserFromContext(r.Context())
	// if userInfo == nil {
	// 	http.Error(w, "Unauthorized", http.StatusUnauthorized)
	// 	return
	// }

	payload, err := github.ValidatePayload(r, c.webhookSecretKey)
	if err != nil {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		fmt.Println("invalid payload")
		return
	}

	event, err := github.ParseWebHook(github.WebHookType(r), payload)
	if err != nil {
		http.Error(w, "cannot parse webhook", http.StatusBadRequest)
		fmt.Println("cannot parse webhook")
		return
	}

	fmt.Printf("Webhook type: %s\n", github.WebHookType(r))
	fmt.Printf("Event type (Go): %T\n", event)

	switch e := event.(type) {
	case *github.InstallationEvent:
		handleInstallation(
			e.GetAction(),
			e.GetInstallation().GetID(),
			e.GetInstallation().GetTargetID(),
			e.GetInstallation().GetAccount().GetLogin(),
			e.GetInstallation().GetAccount().GetType(),
		)
	case *github.InstallationRepositoriesEvent:
		handleInstallation(
			e.GetAction(),
			e.GetInstallation().GetID(),
			e.GetInstallation().GetTargetID(),
			e.GetInstallation().GetAccount().GetLogin(),
			e.GetInstallation().GetAccount().GetType(),
		)
	default:
		fmt.Println("Unhandled event")
	}
}

func handleInstallation(action string, installationID, targetID int64, accountLogin, accountType string) {
	fmt.Printf("Installation: ID=%d, Account=%s, Type=%s, TargetID=%d, Action=%s\n",
		installationID, accountLogin, accountType, targetID, action)

	switch action {
	case "created", "added", "removed":

		installation := db.GithubInstallation{
			InstallationID: installationID,
			AccountLogin:   accountLogin,
			AccountType:    accountType,
			TargetID:       targetID,
		}

		if err := db.DB.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "installation_id"}},
			UpdateAll: true,
		}).Create(&installation).Error; err != nil {
			fmt.Printf("DB insertion error GithubInstallation: %v\n", err)
		}

	default:
		fmt.Println("Unhandled event action")
	}
}

// Payload attendu depuis le front après le callback GitHub
type PostInstallationPayload struct {
	InstallationID int64 `json:"installation_id"`
}

// Handler pour lier l'installation GitHub à l'utilisateur interne
func (c *GithubController) HandleGithubPostInstallation(w http.ResponseWriter, r *http.Request) {
	userInfo := middleware.GetUserFromContext(r.Context())
	if userInfo == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var payload PostInstallationPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}

	if payload.InstallationID == 0 {
		http.Error(w, "Missing required fields", http.StatusBadRequest)
		return
	}

	// Stocke l'installation dans la DB
	installation := db.GithubInstallation{
		InstallationID: payload.InstallationID,
		UserID:         &userInfo.DB.ID,
	}

	if err := db.DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "installation_id"}},
		UpdateAll: true,
	}).Create(&installation).Error; err != nil {
		http.Error(w, fmt.Sprintf("DB error: %v", err), http.StatusInternalServerError)
		return
	}

	// Réponse
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":          "ok",
		"installation_id": strconv.FormatInt(payload.InstallationID, 10),
	})
}

// GenerateGithubAppJWT génère un JWT signé par ta GitHub App
func GenerateGithubAppJWT() (string, error) {
	appID := os.Getenv("GITHUB_APP_ID")
	privateKeyPath := os.Getenv("GITHUB_APP_PRIVATE_KEY_PATH")

	keyData, err := os.ReadFile(privateKeyPath)
	if err != nil {
		return "", fmt.Errorf("cannot read private key: %w", err)
	}

	key, err := jwt.ParseRSAPrivateKeyFromPEM(keyData)
	if err != nil {
		return "", fmt.Errorf("invalid private key: %w", err)
	}

	now := time.Now().UTC()
	claims := jwt.MapClaims{
		"iat": now.Unix(),
		"exp": now.Add(10 * time.Minute).Unix(), // valide 10 min
		"iss": appID,                            // ID de ton app GitHub
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	signedToken, err := token.SignedString(key)
	if err != nil {
		return "", fmt.Errorf("failed to sign JWT: %w", err)
	}

	return signedToken, nil
}

// GenerateInstallationAccessToken génère un access token pour une installation donnée
func GenerateInstallationAccessToken(installationID int64) (string, error) {
	appToken, err := GenerateGithubAppJWT()
	if err != nil {
		return "", err
	}

	url := fmt.Sprintf("https://api.github.com/app/installations/%d/access_tokens", installationID)
	req, err := http.NewRequest("POST", url, nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", appToken))
	req.Header.Set("Accept", "application/vnd.github+json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("failed to create access token: %s", string(body))
	}

	var result struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	return result.Token, nil
}

func (c *GithubController) HandleGithubGetRepositories(w http.ResponseWriter, r *http.Request) {
	userInfo := middleware.GetUserFromContext(r.Context())
	if userInfo == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// 🔹 Pagination optionnelle
	page := r.URL.Query().Get("page")
	perPage := r.URL.Query().Get("per_page")

	if page == "" {
		page = "1"
	}
	if perPage == "" {
		perPage = "50"
	}

	// 🔹 Récupérer l'installation_id depuis la DB avec GORM
	var installation struct {
		InstallationID int64 `gorm:"column:installation_id"`
	}
	err := db.DB.Table("github_installations").
		Select("installation_id").
		Where("user_id = ?", userInfo.DB.ID).
		Limit(1).
		Scan(&installation).Error

	if err != nil || installation.InstallationID == 0 {
		http.Error(w, "Installation GitHub introuvable", http.StatusNotFound)
		return
	}

	// 🔹 Générer le token d'installation GitHub App
	token, err := GenerateInstallationAccessToken(installation.InstallationID)
	if err != nil {
		http.Error(w, "Erreur génération token GitHub", http.StatusInternalServerError)
		return
	}

	// 🔹 Construire la requête GitHub API
	url := fmt.Sprintf("https://api.github.com/installation/repositories?page=%s&per_page=%s", page, perPage)

	client := &http.Client{}
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := client.Do(req)
	if err != nil {
		http.Error(w, "Erreur lors de la requête GitHub", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		http.Error(w, fmt.Sprintf("Erreur GitHub API: %s", resp.Status), resp.StatusCode)
		return
	}

	// 🔹 Décoder la réponse GitHub
	var githubResp struct {
		TotalCount   int                      `json:"total_count"`
		Repositories []map[string]interface{} `json:"repositories"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&githubResp); err != nil {
		http.Error(w, "Erreur décodage réponse GitHub", http.StatusInternalServerError)
		return
	}

	// 🔹 Réponse JSON finale
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"page":         page,
		"per_page":     perPage,
		"total_count":  githubResp.TotalCount,
		"repositories": githubResp.Repositories,
	})
}
