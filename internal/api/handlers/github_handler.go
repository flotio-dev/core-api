package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"

	"github.com/google/go-github/v79/github"
	"golang.org/x/oauth2"
	githubOAuth "golang.org/x/oauth2/github"
	"gorm.io/gorm/clause"

	models "github.com/flotio-dev/api/internal/models"
	services "github.com/flotio-dev/api/internal/services"

	dbEngine "github.com/flotio-dev/api/internal/engines/db"
	helpers "github.com/flotio-dev/api/internal/helpers"

	"github.com/bradleyfalzon/ghinstallation/v2"
)

type GithubController struct {
	webhookSecretKey []byte
	oauthConfig      *oauth2.Config
	appID            int64
	privateKeyPath   string
	appTransport     *ghinstallation.AppsTransport
	Service          *services.GithubService
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

	c := &GithubController{
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

	// Initialise le service après avoir créé le controller
	c.Service = services.NewGithubService(dbEngine.DB, c.clientForInstallation)

	return c
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
	// userInfo := services.GetUserFromContext(r.Context())
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

		installation := dbEngine.GithubInstallation{
			InstallationID: installationID,
			AccountLogin:   accountLogin,
			AccountType:    accountType,
			TargetID:       targetID,
		}

		if err := dbEngine.DB.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "installation_id"}},
			UpdateAll: true,
		}).Create(&installation).Error; err != nil {
			fmt.Printf("DB insertion error GithubInstallation: %v\n", err)
		}

	default:
		fmt.Println("Unhandled event action")
	}
}

// HandleGithubPostInstallation godoc
// @Summary      Enregistre ou met à jour une installation GitHub pour l'utilisateur authentifié
// @Description  Reçoit l'ID d'installation GitHub et l'associe à l'utilisateur actuel
// @Tags         github
// @Accept       json
// @Produce      json
// @Param        payload body models.PostInstallationPayload true "Installation payload"
// @Success      200  {object} models.PostInstallationResponse
// @Failure      400  {object} models.APIErrorResponse
// @Router       /github/post-installation [post]
// @Security     BearerAuth
func (c *GithubController) HandleGithubPostInstallation(w http.ResponseWriter, r *http.Request) {
	userInfo := services.GetUserFromContext(r.Context())
	if userInfo == nil {
		helpers.RespondWithError(w, &helpers.ResponseOptions{
			Status:  helpers.StatusUnauthorized,
			Message: "Unauthorized",
		})
		return
	}

	if r.Method != http.MethodPost {
		helpers.RespondWithError(w, &helpers.ResponseOptions{
			Status:  helpers.StatusMethodNotAllowed,
			Message: "Method not allowed",
		})
		return
	}

	var payload models.PostInstallationRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil || payload.InstallationID == 0 {
		helpers.RespondWithError(w, &helpers.ResponseOptions{
			Status:  helpers.StatusInvalidArgs,
			Message: "Invalid or missing payload",
		})
		return
	}

	err := c.Service.SaveInstallation(userInfo.DB.ID, payload.InstallationID, "", "", 0)
	if err != nil {
		helpers.RespondWithError(w, &helpers.ResponseOptions{
			Status:  helpers.StatusInternalError,
			Message: fmt.Sprintf("DB error: %v", err),
		})
		return
	}

	helpers.RespondWithSuccess(w, &models.PostInstallationResponse{
		InstallationID: payload.InstallationID,
	}, nil)
}

// @Summary      Get GitHub Repositories
// @Description  Liste les repos accessibles pour l'installation GitHub de l'utilisateur
// @Tags         github
// @Produce      json
// @Success      200  {object} models.GithubRepositoriesResponse
// @Failure      400  {object} models.APIErrorResponse
// @Router       /github/repos [get]
// @Security     BearerAuth
func (c *GithubController) HandleGithubGetRepositories(w http.ResponseWriter, r *http.Request) {
	user := services.GetUserFromContext(r.Context())
	if user == nil {
		helpers.RespondWithError(w, &helpers.ResponseOptions{
			HTTPCode: http.StatusUnauthorized,
		})
		return
	}

	inst, err := c.Service.GetInstallationByUser(user.DB.ID)
	if err != nil || inst == nil {
		helpers.RespondWithError(w, &helpers.ResponseOptions{
			HTTPCode: http.StatusNotFound,
			Message:  "Installation GitHub introuvable",
		})
		return
	}

	repos, err := c.Service.ListRepositories(r.Context(), inst.InstallationID)
	if err != nil {
		helpers.RespondWithError(w, &helpers.ResponseOptions{
			HTTPCode: http.StatusBadGateway,
			Message:  "Erreur GitHub API",
		})
		return
	}

	var out []models.GithubRepository
	for _, repo := range repos {
		out = append(out, models.GithubRepository{
			ID:       repo.GetID(),
			Owner:    repo.GetOwner().GetLogin(),
			Name:     repo.GetName(),
			FullName: repo.GetFullName(),
			Private:  repo.GetPrivate(),
		})
	}

	helpers.RespondWithSuccess(w, &models.GithubRepositoriesResponse{
		Repositories: out,
	}, nil)
}

// GetRepoTree godoc
// @Summary      Récupère l'arborescence d'un repo GitHub
// @Description  Retourne l'arborescence complète des dossiers d'un dépôt GitHub
// @Tags         github
// @Produce      json
// @Param owner query string true "Owner du repo"
// @Param repo query string true "Nom du repo"
// @Success      200  {object} models.GithubTreeResponse
// @Failure      400  {object} models.APIErrorResponse
// @Router       /github/repo [get]
// @Security     BearerAuth
func (c *GithubController) HandleGithubRepoTree(w http.ResponseWriter, r *http.Request) {
	user := services.GetUserFromContext(r.Context())
	if user == nil {
		helpers.RespondWithError(w, &helpers.ResponseOptions{
			Status:  helpers.StatusUnauthorized,
			Message: "Unauthorized",
		})
		return
	}

	query := models.GithubRepoTreeQuery{
		Owner: r.URL.Query().Get("owner"),
		Repo:  r.URL.Query().Get("repo"),
	}
	if query.Owner == "" || query.Repo == "" {
		helpers.RespondWithError(w, &helpers.ResponseOptions{
			Status:  helpers.StatusInvalidArgs,
			Message: "owner et repo sont requis",
		})
		return
	}

	inst, err := c.Service.GetInstallationByUser(user.DB.ID)
	if err != nil || inst == nil {
		helpers.RespondWithError(w, &helpers.ResponseOptions{
			Status:  helpers.StatusNotFound,
			Message: "Installation GitHub introuvable",
		})
		return
	}

	tree, err := c.Service.GetRepoTree(r.Context(), inst.InstallationID, query.Owner, query.Repo)
	if err != nil {
		helpers.RespondWithError(w, &helpers.ResponseOptions{
			Status:  helpers.StatusBadRequest,
			Message: "Repo vide ou introuvable",
		})
		return
	}

	var out []models.GithubRepoTreeItem
	for _, item := range tree {
		out = append(out, models.GithubRepoTreeItem{
			Name: item.GetName(),
			Path: item.GetPath(),
			Type: item.GetType(),
			URL:  item.GetHTMLURL(),
		})
	}

	helpers.RespondWithSuccess(w, &models.GithubTreeResponse{
		Owner: query.Owner,
		Repo:  query.Repo,
		Tree:  out,
	}, nil)
}

// CheckInstallation godoc
// @Summary      Vérifie si l'utilisateur a une installation GitHub
// @Description  Retourne l'installation GitHub liée à l'utilisateur authentifié
// @Tags         github
// @Produce      json
// @Success      200  {object} models.GithubInstallationResponse
// @Failure      400  {object} models.APIErrorResponse "Bad request"
// @Router       /github/installations [get]
// @Security     BearerAuth
func (c *GithubController) HandleGithubCheckInstallation(w http.ResponseWriter, r *http.Request) {
	user := services.GetUserFromContext(r.Context())
	if user == nil {
		helpers.RespondWithError(w, &helpers.ResponseOptions{
			Status:  helpers.StatusUnauthorized,
			Message: "Unauthorized",
		})
		return
	}

	inst, err := c.Service.GetInstallationByUser(user.DB.ID)
	if err != nil {
		helpers.RespondWithError(w, &helpers.ResponseOptions{
			Status:  helpers.StatusInternalError,
			Message: fmt.Sprintf("DB error: %v", err),
		})
		return
	}
	if inst == nil {
		helpers.RespondWithError(w, &helpers.ResponseOptions{
			Status:  helpers.StatusNotFound,
			Message: "Installation GitHub introuvable",
		})
		return
	}

	helpers.RespondWithSuccess(w, &models.GithubInstallationResponse{
		ID: inst.ID,
		UserID: func() uint {
			if inst.UserID != nil {
				return *inst.UserID
			}
			return 0
		}(),
		InstallationID: inst.InstallationID,
		AccountLogin:   inst.AccountLogin,
		AccountType:    inst.AccountType,
	}, nil)
}
