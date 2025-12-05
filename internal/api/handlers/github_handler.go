package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	models "github.com/flotio-dev/api/internal/models"
	services "github.com/flotio-dev/api/internal/services"

	helpers "github.com/flotio-dev/api/internal/helpers"

	"gorm.io/gorm"
)

type GithubController struct {
	Service     *services.GithubService
	UserService *services.UserService
}

func NewGithubController(service *services.GithubService, userService *services.UserService) *GithubController {
	return &GithubController{
		Service:     service,
		UserService: userService,
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
	user := services.GetUserFromContext(r.Context())
	if user == nil {
		helpers.RespondWithError(w, &helpers.ResponseOptions{
			Status: helpers.StatusUnauthorized,
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
			Message: "Invalid payload",
		})
		return
	}
	// Vérifier si l'installation est déjà liée à un utilisateur
	respMessage := ""
	existingInst, gerr := c.Service.GetGithubInstallationByInstallationID(payload.InstallationID)
	if gerr == nil && existingInst != nil {
		if existingInst.UserID != nil && *existingInst.UserID != user.DB.ID {
			helpers.RespondWithError(w, &helpers.ResponseOptions{
				Status:  helpers.StatusInvalidArgs,
				Message: "Cette installation GitHub est déjà liée à un autre compte",
			})
			return
		}
		// si liée au même utilisateur, on indique que c'est une mise à jour
		respMessage = "Installation GitHub mise à jour"
	} else if gerr != nil {
		if !errors.Is(gerr, gorm.ErrRecordNotFound) {
			helpers.RespondWithError(w, &helpers.ResponseOptions{
				Status:  helpers.StatusInternalError,
				Message: fmt.Sprintf("DB error: %v", gerr),
			})
			return
		}
		// si record not found, on continue normalement
	}

	if err := c.Service.SaveInstallation(user.DB.ID, payload.InstallationID, "", "", 0); err != nil {
		helpers.RespondWithError(w, &helpers.ResponseOptions{
			Status:  helpers.StatusInternalError,
			Message: fmt.Sprintf("DB error: %v", err),
		})
		return
	}

	opts := (*helpers.ResponseOptions)(nil)
	if respMessage != "" {
		opts = &helpers.ResponseOptions{Message: respMessage}
	}

	helpers.RespondWithSuccess(w, &models.PostInstallationResponse{
		InstallationID: payload.InstallationID,
	}, opts)
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

	out := make([]models.GithubRepository, 0, len(repos))
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
			Status: helpers.StatusUnauthorized,
		})
		return
	}

	owner := r.URL.Query().Get("owner")
	repo := r.URL.Query().Get("repo")

	if owner == "" || repo == "" {
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

	tree, err := c.Service.GetRepoTree(r.Context(), inst.InstallationID, owner, repo)
	if err != nil {
		helpers.RespondWithError(w, &helpers.ResponseOptions{
			Status:  helpers.StatusBadRequest,
			Message: "Repo vide ou introuvable",
		})
		return
	}

	out := make([]models.GithubRepoTreeItem, 0, len(tree))
	for _, item := range tree {
		out = append(out, models.GithubRepoTreeItem{
			Name: item.GetName(),
			Path: item.GetPath(),
			Type: item.GetType(),
			URL:  item.GetHTMLURL(),
		})
	}

	helpers.RespondWithSuccess(w, &models.GithubTreeResponse{
		Owner: owner,
		Repo:  repo,
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
			Status: helpers.StatusUnauthorized,
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

	// Vérifier l'existence côté GitHub via l'API
	ghInst, err := c.Service.InstallationExists(r.Context(), inst.InstallationID)
	if err != nil {
		helpers.RespondWithError(w, &helpers.ResponseOptions{
			Status:  helpers.StatusBadGateway,
			Message: fmt.Sprintf("Erreur GitHub API: %v", err),
		})
		return
	}
	if ghInst == nil {
		helpers.RespondWithError(w, &helpers.ResponseOptions{
			Status:  helpers.StatusNotFound,
			Message: "Installation GitHub introuvable (GitHub API)",
		})
		return
	}

	helpers.RespondWithSuccess(w, &models.GithubInstallationResponse{
		ID:             inst.ID,
		UserID:         *inst.UserID,
		InstallationID: inst.InstallationID,
		AccountLogin:   inst.AccountLogin,
		AccountType:    inst.AccountType,
	}, nil)
}

// HandleGetGithubInstallation godoc
// @Summary      Vérifie l'existence d'une installation GitHub par installation_id
// @Description  Retourne l'enregistrement GithubInstallation correspondant à installation_id si présent
// @Tags         github
// @Produce      json
// @Param        installation_id query int true "Installation ID"
// @Success      200  {object} models.GithubInstallationResponse
// @Failure      400  {object} models.APIErrorResponse
// @Failure      404  {object} models.APIErrorResponse
// @Router       /github/installation [get]
// @Security     BearerAuth
func (c *GithubController) HandleGetGithubInstallation(w http.ResponseWriter, r *http.Request) {
	user := services.GetUserFromContext(r.Context())
	if user == nil {
		helpers.RespondWithError(w, &helpers.ResponseOptions{
			Status: helpers.StatusUnauthorized,
		})
		return
	}

	q := r.URL.Query().Get("installation_id")
	if q == "" {
		helpers.RespondWithError(w, &helpers.ResponseOptions{
			Status:  helpers.StatusInvalidArgs,
			Message: "installation_id is required",
		})
		return
	}

	installationID, err := strconv.ParseInt(q, 10, 64)
	if err != nil {
		helpers.RespondWithError(w, &helpers.ResponseOptions{
			Status:  helpers.StatusInvalidArgs,
			Message: "installation_id must be a valid integer",
		})
		return
	}

	inst, err := c.Service.GetGithubInstallationByInstallationID(installationID)
	if err != nil || inst == nil {
		helpers.RespondWithError(w, &helpers.ResponseOptions{
			Status:  helpers.StatusNotFound,
			Message: "Installation GitHub introuvable",
		})
		return
	}

	// Vérifier l'existence côté GitHub via l'API
	ghInst, err := c.Service.InstallationExists(r.Context(), installationID)
	if err != nil {
		helpers.RespondWithError(w, &helpers.ResponseOptions{
			Status:  helpers.StatusBadGateway,
			Message: fmt.Sprintf("Erreur GitHub API: %v", err),
		})
		return
	}
	if ghInst == nil {
		helpers.RespondWithError(w, &helpers.ResponseOptions{
			Status:  helpers.StatusNotFound,
			Message: "Installation GitHub introuvable (GitHub API)",
		})
		return
	}

	helpers.RespondWithSuccess(w, &models.GithubInstallationResponse{
		ID:             inst.ID,
		UserID:         *inst.UserID,
		InstallationID: inst.InstallationID,
		AccountLogin:   inst.AccountLogin,
		AccountType:    inst.AccountType,
	}, nil)
}

// HandleDisconnectGithub godoc
// @Summary      Déconnecte l'utilisateur courant de GitHub
// @Description  Supprime l'enregistrement `GithubInstallation` de l'utilisateur courant et tente de supprimer l'installation via l'API GitHub
// @Tags         github
// @Produce      json
// @Success      200  {object} map[string]string
// @Failure      400  {object} models.APIErrorResponse
// @Failure      404  {object} models.APIErrorResponse
// @Router       /github/disconnect [delete]
// @Security     BearerAuth
func (c *GithubController) HandleDisconnectGithub(w http.ResponseWriter, r *http.Request) {
	user := services.GetUserFromContext(r.Context())
	if user == nil {
		helpers.RespondWithError(w, &helpers.ResponseOptions{
			Status: helpers.StatusUnauthorized,
		})
		return
	}

	// Récupérer l'installation liée à l'utilisateur
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

	// Tenter la suppression (GitHub + DB)
	if err := c.Service.DeleteInstallation(r.Context(), inst.InstallationID); err != nil {
		helpers.RespondWithError(w, &helpers.ResponseOptions{
			Status:  helpers.StatusBadGateway,
			Message: fmt.Sprintf("Erreur lors de la suppression de l'installation: %v", err),
		})
		return
	}

	type deletedResp struct {
		Status string `json:"status"`
	}
	resp := &deletedResp{Status: "deleted"}
	helpers.RespondWithSuccess(w, resp, nil)
}

// GetBuildPath godoc
// @Summary      Trouve le chemin de pubspec.yaml dans un repo
// @Description  Parcourt le repo pour renvoyer le path du pubspec.yaml (utilisé pour déterminer build folder)
// @Tags         github
// @Produce      json
// @Param        owner query string true "Owner du repo"
// @Param        repo  query string true "Nom du repo"
// @Success      200 {object} map[string]string
// @Failure      400 {object} models.APIErrorResponse
// @Failure      404 {object} models.APIErrorResponse
// @Router       /github/pubspec-path [get]
// @Security     BearerAuth
func (c *GithubController) HandleGetBuildPath(w http.ResponseWriter, r *http.Request) {
	user := services.GetUserFromContext(r.Context())
	if user == nil {
		helpers.RespondWithError(w, &helpers.ResponseOptions{
			Status: helpers.StatusUnauthorized,
		})
		return
	}

	owner := r.URL.Query().Get("owner")
	repo := r.URL.Query().Get("repo")
	if owner == "" || repo == "" {
		helpers.RespondWithError(w, &helpers.ResponseOptions{
			Status:  helpers.StatusInvalidArgs,
			Message: "owner and repo are required",
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

	path, err := c.Service.FindBuildPath(r.Context(), inst.InstallationID, owner, repo)
	if err != nil {
		helpers.RespondWithError(w, &helpers.ResponseOptions{
			Status:  helpers.StatusNotFound,
			Message: fmt.Sprintf("pubspec not found: %v", err),
		})
		return
	}

	helpers.RespondWithSuccess(w, &models.BuildPathResponse{Path: path}, nil)
}
