package controller

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"

	"github.com/google/go-github/v76/github"
	"golang.org/x/oauth2"
	githubOAuth "golang.org/x/oauth2/github"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	middleware "github.com/flotio-dev/api/pkg/api/v1/middleware"
	payload "github.com/flotio-dev/api/pkg/api/v1/model/payload"
	response "github.com/flotio-dev/api/pkg/api/v1/model/response"
	db "github.com/flotio-dev/api/pkg/db"
	utils "github.com/flotio-dev/api/pkg/utils"

	"github.com/bradleyfalzon/ghinstallation/v2"
)

type GithubController struct {
	webhookSecretKey []byte
	oauthConfig      *oauth2.Config
	appID            int64
	privateKeyPath   string
	appTransport     *ghinstallation.AppsTransport
}

type GithubControllerOption func(*GithubController)

func WithWebhookSecret(secret []byte) GithubControllerOption {
	return func(c *GithubController) {
		c.webhookSecretKey = secret
	}
}

func WithOAuthConfig(cfg *oauth2.Config) GithubControllerOption {
	return func(c *GithubController) {
		c.oauthConfig = cfg
	}
}

func NewGithubController() *GithubController {
	appID, _ := strconv.ParseInt(os.Getenv("GITHUB_APP_ID"), 10, 64)
	privateKeyPath := os.Getenv("GITHUB_APP_PRIVATE_KEY_PATH")

	appTransport, err := ghinstallation.NewAppsTransportKeyFromFile(
		http.DefaultTransport,
		appID,
		privateKeyPath,
	)
	if err != nil {
		panic(err)
	}

	return &GithubController{
		webhookSecretKey: []byte(os.Getenv("GITHUB_WEBHOOK_SECRET")),
		oauthConfig: &oauth2.Config{
			ClientID:     os.Getenv("GITHUB_CLIENT_ID"),
			ClientSecret: os.Getenv("GITHUB_CLIENT_SECRET"),
			RedirectURL:  os.Getenv("GITHUB_REDIRECT_URL"),
			Scopes:       []string{"user", "repo"},
			Endpoint:     githubOAuth.Endpoint,
		},
		appID:          appID,
		privateKeyPath: privateKeyPath,
		appTransport:   appTransport,
	}
}

func (c *GithubController) clientForInstallation(installationID int64) (*github.Client, error) {
	if c.appTransport == nil {
		return nil, fmt.Errorf("appTransport not initialized")
	}

	tr := ghinstallation.NewFromAppsTransport(c.appTransport, installationID)

	client := github.NewClient(&http.Client{Transport: tr})
	return client, nil
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

// HandleGithubPostInstallation godoc
// @Summary      Enregistre ou met à jour une installation GitHub pour l'utilisateur authentifié
// @Description  Reçoit l'ID d'installation GitHub et l'associe à l'utilisateur actuel
// @Tags         github
// @Accept       json
// @Produce      json
// @Param        payload body payload.PostInstallationPayload true "Installation payload"
// @Success      200  {object} response.PostInstallationResponse
// @Failure      400  {object} response.APIErrorResponse
// @Router       /github/post-installation [post]
// @Security     BearerAuth
func (c *GithubController) HandleGithubPostInstallation(w http.ResponseWriter, r *http.Request) {
	userInfo := middleware.GetUserFromContext(r.Context())
	if userInfo == nil {
		utils.RespondWithError(w, &utils.ResponseOptions{
			Status:  utils.StatusUnauthorized,
			Message: "Unauthorized",
		})
		return
	}

	if r.Method != http.MethodPost {
		utils.RespondWithError(w, &utils.ResponseOptions{
			Status:  utils.StatusMethodNotAllowed,
			Message: "Method not allowed",
		})
		return
	}

	var payload payload.PostInstallationPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		utils.RespondWithError(w, &utils.ResponseOptions{
			Status:  utils.StatusInvalidArgs,
			Message: "Invalid JSON payload",
		})
		return
	}

	if payload.InstallationID == 0 {
		utils.RespondWithError(w, &utils.ResponseOptions{
			Status:  utils.StatusInvalidArgs,
			Message: "Missing required fields",
		})
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
		utils.RespondWithError(w, &utils.ResponseOptions{
			Status:  utils.StatusInternalError,
			Message: fmt.Sprintf("DB error: %v", err),
		})
		return
	}

	responseData := response.PostInstallationResponse{
		InstallationID: payload.InstallationID,
	}
	utils.RespondWithSuccess(w, &responseData, nil)
}

// @Summary      Get GitHub Repositories
// @Description  Liste les repos accessibles pour l'installation GitHub de l'utilisateur
// @Tags         github
// @Produce      json
// @Success      200  {object} response.GithubRepositoriesResponse
// @Failure      400  {object} response.APIErrorResponse
// @Router       /github/repos [get]
// @Security     BearerAuth
func (c *GithubController) HandleGithubGetRepositories(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUserFromContext(r.Context())
	if user == nil {
		utils.RespondWithError(w, &utils.ResponseOptions{
			HTTPCode: http.StatusUnauthorized,
		})
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
		utils.RespondWithError(w, &utils.ResponseOptions{
			HTTPCode: http.StatusNotFound,
			Message:  "Installation GitHub introuvable",
		})
		return
	}

	client, err := c.clientForInstallation(installation.InstallationID)
	if err != nil {
		utils.RespondWithError(w, &utils.ResponseOptions{
			HTTPCode: http.StatusInternalServerError,
			Message:  "Erreur création client GitHub",
		})
		return
	}

	reposResp, _, err := client.Apps.ListRepos(r.Context(), &github.ListOptions{PerPage: 50})
	if err != nil {
		utils.RespondWithError(w, &utils.ResponseOptions{
			HTTPCode: http.StatusBadGateway,
			Message:  "Erreur GitHub API",
		})
		return
	}

	var repos []response.GithubRepository
	for _, repo := range reposResp.Repositories {
		repos = append(repos, response.GithubRepository{
			ID:       repo.GetID(),
			Owner:    repo.GetOwner().GetLogin(),
			Name:     repo.GetName(),
			FullName: repo.GetFullName(),
			Private:  repo.GetPrivate(),
		})
	}

	utils.RespondWithSuccess(w, &response.GithubRepositoriesResponse{
		Repositories: repos,
	}, &utils.ResponseOptions{})
}

// GetRepoTree godoc
// @Summary      Récupère l'arborescence d'un repo GitHub
// @Description  Retourne l'arborescence complète des dossiers d'un dépôt GitHub
// @Tags         github
// @Produce      json
// @Param owner query string true "Owner du repo"
// @Param repo query string true "Nom du repo"
// @Success      200  {object} response.GithubTreeResponse
// @Failure      400  {object} response.APIErrorResponse
// @Router       /github/repo [get]
// @Security     BearerAuth
func (c *GithubController) HandleGithubRepoTree(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUserFromContext(r.Context())
	if user == nil {
		utils.RespondWithError(w, &utils.ResponseOptions{
			Status:  utils.StatusUnauthorized,
			Message: "Unauthorized",
		})
		return
	}

	query := payload.GithubRepoTreeQuery{
		Owner: r.URL.Query().Get("owner"),
		Repo:  r.URL.Query().Get("repo"),
	}

	if query.Owner == "" || query.Repo == "" {
		utils.RespondWithError(w, &utils.ResponseOptions{
			Status:  utils.StatusInvalidArgs,
			Message: "owner et repo sont requis",
		})
		return
	}

	var installation struct {
		InstallationID int64 `gorm:"column:installation_id"`
	}
	if err := db.DB.Table("github_installations").
		Select("installation_id").
		Where("user_id = ?", user.DB.ID).
		First(&installation).Error; err != nil || installation.InstallationID == 0 {
		utils.RespondWithError(w, &utils.ResponseOptions{
			Status:  utils.StatusNotFound,
			Message: "Installation GitHub introuvable",
		})
		return
	}

	client, err := c.clientForInstallation(installation.InstallationID)
	if err != nil {
		utils.RespondWithError(w, &utils.ResponseOptions{
			Status:  utils.StatusInternalError,
			Message: "Erreur création client GitHub",
		})
		return
	}

	var fetchTree func(path string) ([]response.GithubRepoTreeItem, error)
	fetchTree = func(path string) ([]response.GithubRepoTreeItem, error) {
		_, directoryContents, _, err := client.Repositories.GetContents(r.Context(), query.Owner, query.Repo, path, nil)
		if err != nil {
			return nil, err
		}

		var tree []response.GithubRepoTreeItem
		for _, c := range directoryContents {
			if c.GetType() != "dir" {
				continue
			}
			item := response.GithubRepoTreeItem{
				Name: c.GetName(),
				Path: c.GetPath(),
				Type: c.GetType(),
				URL:  c.GetHTMLURL(),
			}
			subTree, err := fetchTree(c.GetPath())
			if err != nil {
				return nil, err
			}
			if len(subTree) > 0 {
				item.Children = subTree
			}
			tree = append(tree, item)
		}
		return tree, nil
	}

	tree, err := fetchTree("")
	if err != nil {
		utils.RespondWithError(w, &utils.ResponseOptions{
			Status:  utils.StatusBadRequest,
			Message: "Repo vide ou introuvable",
		})
		return
	}

	responseData := response.GithubTreeResponse{
		Owner: query.Owner,
		Repo:  query.Repo,
		Tree:  tree,
	}

	utils.RespondWithSuccess(w, &responseData, nil)
}

// CheckInstallation godoc
// @Summary      Vérifie si l'utilisateur a une installation GitHub
// @Description  Retourne l'installation GitHub liée à l'utilisateur authentifié
// @Tags         github
// @Produce      json
// @Success      200  {object} response.GithubInstallationResponse
// @Failure      400  {object} response.APIErrorResponse "Bad request"
// @Router       /github/installations [get]
// @Security     BearerAuth
func (c *GithubController) HandleGithubCheckInstallation(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUserFromContext(r.Context())
	if user == nil {
		utils.RespondWithError(w, &utils.ResponseOptions{
			Status:  utils.StatusUnauthorized,
			Message: "Unauthorized",
		})
		return
	}

	var installation db.GithubInstallation
	if err := db.DB.Where("user_id = ?", user.DB.ID).First(&installation).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			utils.RespondWithError(w, &utils.ResponseOptions{
				Status:  utils.StatusNotFound,
				Message: "Installation GitHub introuvable",
			})
			return
		}
		utils.RespondWithError(w, &utils.ResponseOptions{
			Status:  utils.StatusInternalError,
			Message: fmt.Sprintf("DB error: %v", err),
		})
		return
	}

	responseData := response.GithubInstallationResponse{
		ID: installation.ID,
		UserID: func() uint {
			if installation.UserID != nil {
				return *installation.UserID
			}
			return 0
		}(),
		InstallationID: installation.InstallationID,
		AccountLogin:   installation.AccountLogin,
		AccountType:    installation.AccountType,
	}

	utils.RespondWithSuccess(w, &responseData, nil)
}
