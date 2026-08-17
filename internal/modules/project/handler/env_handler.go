package handler

import (
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
	"gorm.io/gorm"

	dbEngine "github.com/flotio-dev/core-api/internal/common/database"
	helpers "github.com/flotio-dev/core-api/internal/common/server"
	models "github.com/flotio-dev/core-api/internal/models"
	services "github.com/flotio-dev/core-api/internal/modules/user/service"
)

type EnvController struct {
	UserService *services.UserService
}

func NewEnvController(userService *services.UserService) *EnvController {
	return &EnvController{
		UserService: userService,
	}
}

// EnvGetHandler godoc
//
//	@Summary		Get environment assets
//	@Description	Get all environment assets/files for the authenticated user, optionally filtered by project_id
//	@Tags			environment
//	@Accept			json
//	@Produce		json
//	@Param			project_id	query	int	false	"Filter by Project ID"
//	@Success		200			{object}	EnvListResponse
//	@Failure		401			{object}	models.APIErrorResponse
//	@Failure		500			{object}	models.APIErrorResponse
//	@Security		BearerAuth
//	@ID				EnvGetHandler
//	@Router			/env [get]
func (c *EnvController) EnvGetHandler(w http.ResponseWriter, r *http.Request) {
	userInfo, err := c.UserService.GetUserFromContext(r.Context())
	if err != nil {
		helpers.WriteErrorJSON(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	projectIDStr := r.URL.Query().Get("project_id")
	query := dbEngine.DB.Where("user_id = ?", userInfo.ID)
	if projectIDStr != "" {
		if pid, err := strconv.Atoi(projectIDStr); err == nil {
			query = query.Where("project_id = ?", pid)
		}
	}

	var envs []dbEngine.Env
	if err := query.Find(&envs).Error; err != nil {
		helpers.WriteErrorJSON(w, "Failed to fetch envs", http.StatusInternalServerError)
		return
	}

	helpers.WriteJSON(w, EnvListResponse{Envs: convertDBEnvs(envs)})
}

// EnvsGetHandler godoc
//
//	@Summary		Get environment assets
//	@Description	Alias of GET /env: get all environment assets/files for the authenticated user, optionally filtered by project_id
//	@Tags			environment
//	@Accept			json
//	@Produce		json
//	@Param			project_id	query	int	false	"Filter by Project ID"
//	@Success		200			{object}	EnvListResponse
//	@Failure		401			{object}	models.APIErrorResponse
//	@Failure		500			{object}	models.APIErrorResponse
//	@Security		BearerAuth
//	@ID				EnvsGetHandler
//	@Router			/envs [get]
func (c *EnvController) EnvsGetHandler(w http.ResponseWriter, r *http.Request) {
	c.EnvGetHandler(w, r)
}

// EnvPostHandler godoc
//
//	@Summary		Create environment asset
//	@Description	Create a new environment variable or configuration file for the user
//	@Tags			environment
//	@Accept			json
//	@Produce		json
//	@Param			env	body	EnvCreateRequest	true	"Environment asset data"
//	@Success		201	{object}	EnvResponse
//	@Failure		400	{object}	models.APIErrorResponse
//	@Failure		401	{object}	models.APIErrorResponse
//	@Failure		404	{object}	models.APIErrorResponse
//	@Failure		500	{object}	models.APIErrorResponse
//	@Security		BearerAuth
//	@ID				EnvPostHandler
//	@Router			/env [post]
func (c *EnvController) EnvPostHandler(w http.ResponseWriter, r *http.Request) {
	userInfo, err := c.UserService.GetUserFromContext(r.Context())
	if err != nil {
		helpers.WriteErrorJSON(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req EnvCreateRequest
	if err := helpers.ReadJSON(r, &req); err != nil {
		helpers.WriteErrorJSON(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Verify project ownership if projectID is provided
	if req.ProjectID != nil {
		var project dbEngine.Project
		if err := dbEngine.DB.Where("id = ? AND user_id = ?", *req.ProjectID, userInfo.ID).First(&project).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				helpers.WriteErrorJSON(w, "Project not found", http.StatusNotFound)
				return
			}
			helpers.WriteErrorJSON(w, "Failed to fetch project", http.StatusInternalServerError)
			return
		}
	}

	env := dbEngine.Env{
		UserID:    userInfo.ID,
		ProjectID: req.ProjectID,
		Key:       req.Key,
		Value:     req.Value,
		Type:      req.Type,
		Path:      req.Path,
		IsBase64:  req.IsBase64,
	}

	if err := dbEngine.DB.Create(&env).Error; err != nil {
		helpers.WriteErrorJSON(w, "Failed to create env", http.StatusInternalServerError)
		return
	}

	helpers.WriteJSON(w, EnvResponse{Env: convertDBEnv(env)})
}

// EnvGetByIdHandler godoc
//
//	@Summary		Get environment asset by ID
//	@Description	Get a specific environment asset by its ID
//	@Tags			environment
//	@Accept			json
//	@Produce		json
//	@Param			envId	path	int	true	"Environment asset ID"	Format(int64)
//	@Success		200		{object}	EnvResponse
//	@Failure		400		{object}	models.APIErrorResponse
//	@Failure		401		{object}	models.APIErrorResponse
//	@Failure		404		{object}	models.APIErrorResponse
//	@Failure		500		{object}	models.APIErrorResponse
//	@Security		BearerAuth
//	@ID				EnvGetByIdHandler
//	@Router			/env/{envId} [get]
func (c *EnvController) EnvGetByIdHandler(w http.ResponseWriter, r *http.Request) {
	userInfo, err := c.UserService.GetUserFromContext(r.Context())
	if err != nil {
		helpers.WriteErrorJSON(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(r)
	envID, err := strconv.Atoi(vars["envId"])
	if err != nil {
		helpers.WriteErrorJSON(w, "Invalid env ID", http.StatusBadRequest)
		return
	}

	var env dbEngine.Env
	if err := dbEngine.DB.Where("id = ? AND user_id = ?", envID, userInfo.ID).First(&env).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			helpers.WriteErrorJSON(w, "Env not found", http.StatusNotFound)
			return
		}
		helpers.WriteErrorJSON(w, "Failed to fetch env", http.StatusInternalServerError)
		return
	}

	helpers.WriteJSON(w, EnvResponse{Env: convertDBEnv(env)})
}

// EnvPutByIdHandler godoc
//
//	@Summary		Update environment asset
//	@Description	Update an existing environment asset
//	@Tags			environment
//	@Accept			json
//	@Produce		json
//	@Param			envId	path	int				true	"Environment asset ID"	Format(int64)
//	@Param			env		body	EnvUpdateRequest	true	"Updated environment asset data"
//	@Success		200		{object}	EnvResponse
//	@Failure		400		{object}	models.APIErrorResponse
//	@Failure		401		{object}	models.APIErrorResponse
//	@Failure		404		{object}	models.APIErrorResponse
//	@Failure		500		{object}	models.APIErrorResponse
//	@Security		BearerAuth
//	@ID				EnvPutByIdHandler
//	@Router			/env/{envId} [put]
func (c *EnvController) EnvPutByIdHandler(w http.ResponseWriter, r *http.Request) {
	userInfo, err := c.UserService.GetUserFromContext(r.Context())
	if err != nil {
		helpers.WriteErrorJSON(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(r)
	envID, err := strconv.Atoi(vars["envId"])
	if err != nil {
		helpers.WriteErrorJSON(w, "Invalid env ID", http.StatusBadRequest)
		return
	}

	var req EnvUpdateRequest
	if err := helpers.ReadJSON(r, &req); err != nil {
		helpers.WriteErrorJSON(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	var env dbEngine.Env
	if err := dbEngine.DB.Where("id = ? AND user_id = ?", envID, userInfo.ID).First(&env).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			helpers.WriteErrorJSON(w, "Env not found", http.StatusNotFound)
			return
		}
		helpers.WriteErrorJSON(w, "Failed to fetch env", http.StatusInternalServerError)
		return
	}

	// Verify project ownership if projectID is provided/changed
	if req.ProjectID != nil {
		var project dbEngine.Project
		if err := dbEngine.DB.Where("id = ? AND user_id = ?", *req.ProjectID, userInfo.ID).First(&project).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				helpers.WriteErrorJSON(w, "Project not found", http.StatusNotFound)
				return
			}
			helpers.WriteErrorJSON(w, "Failed to fetch project", http.StatusInternalServerError)
			return
		}
	}

	env.ProjectID = req.ProjectID
	env.Key = req.Key
	env.Value = req.Value
	env.Type = req.Type
	env.Path = req.Path
	env.IsBase64 = req.IsBase64

	if err := dbEngine.DB.Save(&env).Error; err != nil {
		helpers.WriteErrorJSON(w, "Failed to update env", http.StatusInternalServerError)
		return
	}

	helpers.WriteJSON(w, EnvResponse{Env: convertDBEnv(env)})
}

// EnvDeleteByIdHandler godoc
//
//	@Summary		Delete environment asset
//	@Description	Delete an environment asset by its ID
//	@Tags			environment
//	@Accept			json
//	@Produce		json
//	@Param			envId	path	int	true	"Environment asset ID"	Format(int64)
//	@Success		200	{object}	DeleteResponse
//	@Failure		400	{object}	models.APIErrorResponse
//	@Failure		401	{object}	models.APIErrorResponse
//	@Failure		500	{object}	models.APIErrorResponse
//	@Security		BearerAuth
//	@ID				EnvDeleteByIdHandler
//	@Router			/env/{envId} [delete]
func (c *EnvController) EnvDeleteByIdHandler(w http.ResponseWriter, r *http.Request) {
	userInfo, err := c.UserService.GetUserFromContext(r.Context())
	if err != nil {
		helpers.WriteErrorJSON(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(r)
	envID, err := strconv.Atoi(vars["envId"])
	if err != nil {
		helpers.WriteErrorJSON(w, "Invalid env ID", http.StatusBadRequest)
		return
	}

	if err := dbEngine.DB.Where("id = ? AND user_id = ?", envID, userInfo.ID).Delete(&dbEngine.Env{}).Error; err != nil {
		helpers.WriteErrorJSON(w, "Failed to delete env", http.StatusInternalServerError)
		return
	}

	helpers.WriteJSON(w, DeleteResponse{Status: "deleted"})
}

// Keep the swag annotation import alive (used only in @Failure comments).
var _ = models.APIErrorResponse{}
