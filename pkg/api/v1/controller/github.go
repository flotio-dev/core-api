package controller

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

	middleware "github.com/flotio-dev/api/pkg/api/v1/middleware"
	payload "github.com/flotio-dev/api/pkg/api/v1/model/payload"
	response "github.com/flotio-dev/api/pkg/api/v1/model/response"
	service "github.com/flotio-dev/api/pkg/api/v1/service"
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
	Service          *service.GithubService
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
	c.Service = service.NewGithubService(db.DB, c.clientForInstallation)

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
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil || payload.InstallationID == 0 {
		utils.RespondWithError(w, &utils.ResponseOptions{
			Status:  utils.StatusInvalidArgs,
			Message: "Invalid or missing payload",
		})
		return
	}

	err := c.Service.SaveInstallation(userInfo.DB.ID, payload.InstallationID, "", "", 0)
	if err != nil {
		utils.RespondWithError(w, &utils.ResponseOptions{
			Status:  utils.StatusInternalError,
			Message: fmt.Sprintf("DB error: %v", err),
		})
		return
	}

	utils.RespondWithSuccess(w, &response.PostInstallationResponse{
		InstallationID: payload.InstallationID,
	}, nil)
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

	inst, err := c.Service.GetInstallationByUser(user.DB.ID)
	if err != nil || inst == nil {
		utils.RespondWithError(w, &utils.ResponseOptions{
			HTTPCode: http.StatusNotFound,
			Message:  "Installation GitHub introuvable",
		})
		return
	}

	repos, err := c.Service.ListRepositories(r.Context(), inst.InstallationID)
	if err != nil {
		utils.RespondWithError(w, &utils.ResponseOptions{
			HTTPCode: http.StatusBadGateway,
			Message:  "Erreur GitHub API",
		})
		return
	}

	var out []response.GithubRepository
	for _, repo := range repos {
		out = append(out, response.GithubRepository{
			ID:       repo.GetID(),
			Owner:    repo.GetOwner().GetLogin(),
			Name:     repo.GetName(),
			FullName: repo.GetFullName(),
			Private:  repo.GetPrivate(),
		})
	}

	utils.RespondWithSuccess(w, &response.GithubRepositoriesResponse{
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

	inst, err := c.Service.GetInstallationByUser(user.DB.ID)
	if err != nil || inst == nil {
		utils.RespondWithError(w, &utils.ResponseOptions{
			Status:  utils.StatusNotFound,
			Message: "Installation GitHub introuvable",
		})
		return
	}

	tree, err := c.Service.GetRepoTree(r.Context(), inst.InstallationID, query.Owner, query.Repo)
	if err != nil {
		utils.RespondWithError(w, &utils.ResponseOptions{
			Status:  utils.StatusBadRequest,
			Message: "Repo vide ou introuvable",
		})
		return
	}

	var out []response.GithubRepoTreeItem
	for _, item := range tree {
		out = append(out, response.GithubRepoTreeItem{
			Name: item.GetName(),
			Path: item.GetPath(),
			Type: item.GetType(),
			URL:  item.GetHTMLURL(),
		})
	}

	utils.RespondWithSuccess(w, &response.GithubTreeResponse{
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

	inst, err := c.Service.GetInstallationByUser(user.DB.ID)
	if err != nil {
		utils.RespondWithError(w, &utils.ResponseOptions{
			Status:  utils.StatusInternalError,
			Message: fmt.Sprintf("DB error: %v", err),
		})
		return
	}
	if inst == nil {
		utils.RespondWithError(w, &utils.ResponseOptions{
			Status:  utils.StatusNotFound,
			Message: "Installation GitHub introuvable",
		})
		return
	}

	utils.RespondWithSuccess(w, &response.GithubInstallationResponse{
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
