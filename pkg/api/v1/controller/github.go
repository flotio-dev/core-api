package controller

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"

	"github.com/google/go-github/v76/github"
	"golang.org/x/oauth2"
	githubOAuth "golang.org/x/oauth2/github"
	"gorm.io/gorm/clause"

	middleware "github.com/flotio-dev/api/pkg/api/v1/middleware"
	db "github.com/flotio-dev/api/pkg/db"

	"context"

	"github.com/bradleyfalzon/ghinstallation/v2"
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

type PostInstallationPayload struct {
	InstallationID int64 `json:"installation_id"`
}

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

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":          "ok",
		"installation_id": strconv.FormatInt(payload.InstallationID, 10),
	})
}

func (c *GithubController) HandleGithubGetRepositories(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUserFromContext(r.Context())
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var installation struct {
		InstallationID int64 `gorm:"column:installation_id"`
	}
	if err := db.DB.
		Table("github_installations").
		Select("installation_id").
		Where("user_id = ?", user.DB.ID).
		First(&installation).Error; err != nil || installation.InstallationID == 0 {
		http.Error(w, "Installation GitHub introuvable", http.StatusNotFound)
		return
	}

	appIDStr := os.Getenv("GITHUB_APP_ID")
	privateKeyPath := os.Getenv("GITHUB_APP_PRIVATE_KEY_PATH")

	appID, err := strconv.ParseInt(appIDStr, 10, 64)
	if err != nil {
		http.Error(w, "GITHUB_APP_ID invalide", http.StatusInternalServerError)
		return
	}

	tr, err := ghinstallation.NewKeyFromFile(http.DefaultTransport, appID, installation.InstallationID, privateKeyPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Erreur création transport GitHub: %v", err), http.StatusInternalServerError)
		return
	}

	client := github.NewClient(&http.Client{Transport: tr})

	reposResponse, _, err := client.Apps.ListRepos(context.Background(), &github.ListOptions{PerPage: 50})
	if err != nil {
		http.Error(w, fmt.Sprintf("Erreur GitHub API: %v", err), http.StatusBadGateway)
		return
	}

	var repos []map[string]interface{}
	for _, repo := range reposResponse.Repositories {
		repos = append(repos, map[string]interface{}{
			"id":        repo.GetID(),
			"owner":     repo.GetOwner().GetLogin(),
			"name":      repo.GetName(),
			"full_name": repo.GetFullName(),
			"private":   repo.GetPrivate(),
		})
	}

	json.NewEncoder(w).Encode(repos)
}

func (c *GithubController) HandleGithubRepoTree(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUserFromContext(r.Context())
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	owner := r.URL.Query().Get("owner")
	repo := r.URL.Query().Get("repo")
	if owner == "" || repo == "" {
		http.Error(w, "owner et repo sont requis", http.StatusBadRequest)
		return
	}

	var installation struct {
		InstallationID int64 `gorm:"column:installation_id"`
	}
	if err := db.DB.Table("github_installations").
		Select("installation_id").
		Where("user_id = ?", user.DB.ID).
		First(&installation).Error; err != nil || installation.InstallationID == 0 {
		http.Error(w, "Installation GitHub introuvable", http.StatusNotFound)
		return
	}

	appIDStr := os.Getenv("GITHUB_APP_ID")
	appID, err := strconv.ParseInt(appIDStr, 10, 64)
	if err != nil {
		http.Error(w, "GITHUB_APP_ID invalide", http.StatusInternalServerError)
		return
	}

	privateKeyPath := os.Getenv("GITHUB_APP_PRIVATE_KEY_PATH")
	if privateKeyPath == "" {
		http.Error(w, "GITHUB_APP_PRIVATE_KEY_PATH manquant", http.StatusInternalServerError)
		return
	}

	itr, err := ghinstallation.NewAppsTransportKeyFromFile(http.DefaultTransport, appID, privateKeyPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Erreur création transport App: %v", err), http.StatusInternalServerError)
		return
	}

	tr := ghinstallation.NewFromAppsTransport(itr, installation.InstallationID)
	client := github.NewClient(&http.Client{Transport: tr})

	var fetchTree func(path string) ([]map[string]interface{}, error)
	fetchTree = func(path string) ([]map[string]interface{}, error) {
		_, directoryContents, _, err := client.Repositories.GetContents(context.Background(), owner, repo, path, nil)
		if err != nil {
			return nil, err
		}

		tree := []map[string]interface{}{}
		for _, c := range directoryContents {
			if c.GetType() != "dir" {
				continue
			}
			item := map[string]interface{}{
				"name": c.GetName(),
				"path": c.GetPath(),
				"type": c.GetType(),
				"url":  c.GetHTMLURL(),
			}
			subTree, err := fetchTree(c.GetPath())
			if err != nil {
				return nil, err
			}
			if len(subTree) > 0 {
				item["children"] = subTree
			}
			tree = append(tree, item)
		}
		return tree, nil
	}

	tree, err := fetchTree("")
	if err != nil {
		http.Error(w, fmt.Sprintf("Erreur récupération arborescence: %v", err), http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"owner": owner,
		"repo":  repo,
		"tree":  tree,
	})
}
