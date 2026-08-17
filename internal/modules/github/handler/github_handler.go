package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	models "github.com/flotio-dev/core-api/internal/models"
	githubModels "github.com/flotio-dev/core-api/internal/modules/github/model"
	services "github.com/flotio-dev/core-api/internal/modules/github/service"
	userServices "github.com/flotio-dev/core-api/internal/modules/user/service"

	helpers "github.com/flotio-dev/core-api/internal/common/server"

	"gorm.io/gorm"
)

type GithubController struct {
	Service     *services.GithubService
	UserService *userServices.UserService
}

func NewGithubController(service *services.GithubService, userService *userServices.UserService) *GithubController {
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
// @Param        payload body githubModels.PostInstallationRequest true "Installation payload"
// @Success      200  {object} models.APIResponseDoc
// @Success      200  {object} models.APIResponse[githubModels.PostInstallationResponse]
// @Failure      400  {object} models.APIErrorResponse
// @Failure      401  {object} models.APIErrorResponse
// @Failure      404  {object} models.APIErrorResponse
// @Failure      405  {object} models.APIErrorResponse
// @Failure      500  {object} models.APIErrorResponse
// @ID           HandleGithubPostInstallation
// @Router       /github/post-installation [post]
// @Security     BearerAuth
func (c *GithubController) HandleGithubPostInstallation(w http.ResponseWriter, r *http.Request) {
	user, err := c.UserService.GetUserFromContext(r.Context())
	if err != nil {
		helpers.RespondWithError(w, &helpers.ResponseOptions{
			Status: helpers.StatusUnauthorized,
		})
		return
	}

	if r.Method != http.MethodPost {
		helpers.RespondWithError(w, &helpers.ResponseOptions{
			HTTPCode: http.StatusMethodNotAllowed,
			Status:   helpers.StatusMethodNotAllowed,
			Message:  "Method not allowed",
		})
		return
	}

	var payload githubModels.PostInstallationRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil || payload.InstallationID == 0 {
		helpers.RespondWithError(w, &helpers.ResponseOptions{
			Status:  helpers.StatusInvalidArgs,
			Message: "Invalid payload",
		})
		return
	}

	existingInst, gerr := c.Service.GetGithubInstallationByUserID(user.ID)

	if gerr != nil && !errors.Is(gerr, gorm.ErrRecordNotFound) {
		helpers.RespondWithError(w, &helpers.ResponseOptions{
			Status:  helpers.StatusInternalError,
			Message: fmt.Sprintf("DB error: %v", gerr),
		})
		return
	}

	if errors.Is(gerr, gorm.ErrRecordNotFound) {
		if err := c.Service.SaveInstallation(user.ID, payload.InstallationID, "", "", 0); err != nil {
			helpers.RespondWithError(w, &helpers.ResponseOptions{
				Status:  helpers.StatusInternalError,
				Message: fmt.Sprintf("DB error: %v", err),
			})
			return
		}

		helpers.RespondWithSuccess(w, &githubModels.PostInstallationResponse{
			InstallationID: payload.InstallationID,
		}, &helpers.ResponseOptions{
			Message: "Installation GitHub liée avec succès",
		})
		return
	}

	if existingInst.InstallationID != payload.InstallationID {
		helpers.RespondWithError(w, &helpers.ResponseOptions{
			Status:  helpers.StatusInvalidArgs,
			Message: "Un utilisateur ne peut lier qu'une seule installation GitHub",
		})
		return
	}

	if err := c.Service.UpdateInstallation(user.ID, payload.InstallationID); err != nil {
		helpers.RespondWithError(w, &helpers.ResponseOptions{
			Status:  helpers.StatusInternalError,
			Message: fmt.Sprintf("DB error: %v", err),
		})
		return
	}

	helpers.RespondWithSuccess(w, &githubModels.PostInstallationResponse{
		InstallationID: payload.InstallationID,
	}, &helpers.ResponseOptions{
		Message: "Installation GitHub mise à jour",
	})
}

// @Summary      Get GitHub Repositories
// @Description  Liste les repos accessibles pour l'installation GitHub de l'utilisateur
// @Tags         github
// @Produce      json
// @Success      200  {object} models.APIResponse[githubModels.GithubRepositoriesResponse]
// @Failure      400  {object} models.APIErrorResponse
// @Failure      401  {object} models.APIErrorResponse
// @Failure      404  {object} models.APIErrorResponse
// @Failure      502  {object} models.APIErrorResponse
// @ID           HandleGithubGetRepositories
// @Router       /github/repos [get]
// @Security     BearerAuth
func (c *GithubController) HandleGithubGetRepositories(w http.ResponseWriter, r *http.Request) {
	user, err := c.UserService.GetUserFromContext(r.Context())
	if err != nil {
		helpers.RespondWithError(w, &helpers.ResponseOptions{
			HTTPCode: http.StatusUnauthorized,
		})
		return
	}

	inst, err := c.Service.GetInstallationByUser(user.ID)
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

	out := make([]githubModels.GithubRepository, 0, len(repos))
	for _, repo := range repos {
		out = append(out, githubModels.GithubRepository{
			ID:       repo.GetID(),
			Owner:    repo.GetOwner().GetLogin(),
			Name:     repo.GetName(),
			FullName: repo.GetFullName(),
			Private:  repo.GetPrivate(),
		})
	}

	helpers.RespondWithSuccess(w, &githubModels.GithubRepositoriesResponse{
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
// @Success      200  {object} models.APIResponse[githubModels.GithubTreeResponse]
// @Failure      400  {object} models.APIErrorResponse
// @Failure      401  {object} models.APIErrorResponse
// @Failure      404  {object} models.APIErrorResponse
// @ID           HandleGithubRepoTree
// @Router       /github/repo [get]
// @Security     BearerAuth
func (c *GithubController) HandleGithubRepoTree(w http.ResponseWriter, r *http.Request) {
	user, err := c.UserService.GetUserFromContext(r.Context())
	if err != nil {
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

	inst, err := c.Service.GetInstallationByUser(user.ID)
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

	out := make([]githubModels.GithubRepoTreeItem, 0, len(tree))
	for _, item := range tree {
		out = append(out, githubModels.GithubRepoTreeItem{
			Name: item.GetName(),
			Path: item.GetPath(),
			Type: item.GetType(),
			URL:  item.GetHTMLURL(),
		})
	}

	helpers.RespondWithSuccess(w, &githubModels.GithubTreeResponse{
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
// @Success      200  {object} models.APIResponse[githubModels.GithubInstallationResponse]
// @Failure      400  {object} models.APIErrorResponse "Bad request"
// @Failure      401  {object} models.APIErrorResponse
// @Failure      404  {object} models.APIErrorResponse
// @Failure      500  {object} models.APIErrorResponse
// @Failure      502  {object} models.APIErrorResponse
// @ID           HandleGithubCheckInstallation
// @Router       /github/installations [get]
// @Security     BearerAuth
func (c *GithubController) HandleGithubCheckInstallation(w http.ResponseWriter, r *http.Request) {
	user, err := c.UserService.GetUserFromContext(r.Context())
	if err != nil {
		helpers.RespondWithError(w, &helpers.ResponseOptions{
			Status: helpers.StatusUnauthorized,
		})
		return
	}

	inst, err := c.Service.GetInstallationByUser(user.ID)
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

	helpers.RespondWithSuccess(w, &githubModels.GithubInstallationResponse{
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
// @Success      200  {object} models.APIResponse[githubModels.GithubInstallationResponse]
// @Failure      400  {object} models.APIErrorResponse
// @Failure      401  {object} models.APIErrorResponse
// @Failure      404  {object} models.APIErrorResponse
// @Failure      502  {object} models.APIErrorResponse
// @ID           HandleGetGithubInstallation
// @Router       /github/installation [get]
// @Security     BearerAuth
func (c *GithubController) HandleGetGithubInstallation(w http.ResponseWriter, r *http.Request) {
	_, err := c.UserService.GetUserFromContext(r.Context())
	if err != nil {
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

	helpers.RespondWithSuccess(w, &githubModels.GithubInstallationResponse{
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
// @Success      200  {object} models.APIResponse[githubModels.DeleteResponse]
// @Failure      400  {object} models.APIErrorResponse
// @Failure      401  {object} models.APIErrorResponse
// @Failure      404  {object} models.APIErrorResponse
// @Failure      500  {object} models.APIErrorResponse
// @Failure      502  {object} models.APIErrorResponse
// @ID           HandleDisconnectGithub
// @Router       /github/disconnect [delete]
// @Security     BearerAuth
func (c *GithubController) HandleDisconnectGithub(w http.ResponseWriter, r *http.Request) {
	user, err := c.UserService.GetUserFromContext(r.Context())
	if err != nil {
		helpers.RespondWithError(w, &helpers.ResponseOptions{
			Status: helpers.StatusUnauthorized,
		})
		return
	}

	// Récupérer l'installation liée à l'utilisateur
	inst, err := c.Service.GetInstallationByUser(user.ID)
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
	if err := c.Service.DeleteInstallationByInstallationID(inst.InstallationID); err != nil {
		helpers.RespondWithError(w, &helpers.ResponseOptions{
			Status:  helpers.StatusBadGateway,
			Message: fmt.Sprintf("Erreur lors de la suppression de l'installation: %v", err),
		})
		return
	}

	helpers.RespondWithSuccess(w, &githubModels.DeleteResponse{Status: "deleted"}, nil)
}

var _ = models.APIErrorResponse{}
