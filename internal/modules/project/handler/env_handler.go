package handler

import (
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
	"gorm.io/gorm"

	dbEngine "github.com/flotio-dev/core-api/internal/common/database"
	helpers "github.com/flotio-dev/core-api/internal/common/server"
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
//	@Success		200			{object}	map[string]interface{}
//	@Failure		401			{object}	map[string]string
//	@Failure		500			{object}	map[string]string
//	@Router			/env [get]
//	@Security		BearerAuth
func (c *EnvController) EnvGetHandler(w http.ResponseWriter, r *http.Request) {
	userInfo, err := c.UserService.GetUserFromContext(r.Context())
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
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
		http.Error(w, "Failed to fetch envs", http.StatusInternalServerError)
		return
	}

	helpers.WriteJSON(w, map[string]interface{}{"envs": envs})
}

// EnvPostHandler godoc
//
//	@Summary		Create environment asset
//	@Description	Create a new environment variable or configuration file for the user
//	@Tags			environment
//	@Accept			json
//	@Produce		json
//	@Param			env	body	dbEngine.Env	true	"Environment asset data"
//	@Success		201	{object}	map[string]interface{}
//	@Failure		401	{object}	map[string]string
//	@Failure		400	{object}	map[string]string
//	@Failure		404	{object}	map[string]string
//	@Failure		500	{object}	map[string]string
//	@Router			/env [post]
//	@Security		BearerAuth
func (c *EnvController) EnvPostHandler(w http.ResponseWriter, r *http.Request) {
	userInfo, err := c.UserService.GetUserFromContext(r.Context())
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req struct {
		ProjectID *uint  `json:"project_id,omitempty"`
		Key       string `json:"key"`
		Value     string `json:"value"`
		Type      string `json:"type"` // "env" or "file"
		Path      string `json:"path"`
		IsBase64  bool   `json:"is_base64"`
	}
	if err := helpers.ReadJSON(r, &req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Verify project ownership if projectID is provided
	if req.ProjectID != nil {
		var project dbEngine.Project
		if err := dbEngine.DB.Where("id = ? AND user_id = ?", *req.ProjectID, userInfo.ID).First(&project).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				http.Error(w, "Project not found", http.StatusNotFound)
				return
			}
			http.Error(w, "Failed to fetch project", http.StatusInternalServerError)
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
		http.Error(w, "Failed to create env", http.StatusInternalServerError)
		return
	}

	helpers.WriteJSON(w, map[string]interface{}{"env": env})
}

// EnvGetByIdHandler godoc
//
//	@Summary		Get environment asset by ID
//	@Description	Get a specific environment asset by its ID
//	@Tags			environment
//	@Accept			json
//	@Produce		json
//	@Param			envId	path	int	true	"Environment asset ID"
//	@Success		200		{object}	map[string]interface{}
//	@Failure		401		{object}	map[string]string
//	@Failure		400		{object}	map[string]string
//	@Failure		404		{object}	map[string]string
//	@Failure		500		{object}	map[string]string
//	@Router			/env/{envId} [get]
//	@Security		BearerAuth
func (c *EnvController) EnvGetByIdHandler(w http.ResponseWriter, r *http.Request) {
	userInfo, err := c.UserService.GetUserFromContext(r.Context())
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(r)
	envID, err := strconv.Atoi(vars["envId"])
	if err != nil {
		http.Error(w, "Invalid env ID", http.StatusBadRequest)
		return
	}

	var env dbEngine.Env
	if err := dbEngine.DB.Where("id = ? AND user_id = ?", envID, userInfo.ID).First(&env).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			http.Error(w, "Env not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Failed to fetch env", http.StatusInternalServerError)
		return
	}

	helpers.WriteJSON(w, map[string]interface{}{"env": env})
}

// EnvPutByIdHandler godoc
//
//	@Summary		Update environment asset
//	@Description	Update an existing environment asset
//	@Tags			environment
//	@Accept			json
//	@Produce		json
//	@Param			envId	path	int				true	"Environment asset ID"
//	@Param			env		body	dbEngine.Env	true	"Updated environment asset data"
//	@Success		200		{object}	map[string]interface{}
//	@Failure		401		{object}	map[string]string
//	@Failure		400		{object}	map[string]string
//	@Failure		404		{object}	map[string]string
//	@Failure		500		{object}	map[string]string
//	@Router			/env/{envId} [put]
//	@Security		BearerAuth
func (c *EnvController) EnvPutByIdHandler(w http.ResponseWriter, r *http.Request) {
	userInfo, err := c.UserService.GetUserFromContext(r.Context())
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(r)
	envID, err := strconv.Atoi(vars["envId"])
	if err != nil {
		http.Error(w, "Invalid env ID", http.StatusBadRequest)
		return
	}

	var req struct {
		ProjectID *uint  `json:"project_id,omitempty"`
		Key       string `json:"key"`
		Value     string `json:"value"`
		Type      string `json:"type"` // "env" or "file"
		Path      string `json:"path"`
		IsBase64  bool   `json:"is_base64"`
	}
	if err := helpers.ReadJSON(r, &req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	var env dbEngine.Env
	if err := dbEngine.DB.Where("id = ? AND user_id = ?", envID, userInfo.ID).First(&env).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			http.Error(w, "Env not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Failed to fetch env", http.StatusInternalServerError)
		return
	}

	// Verify project ownership if projectID is provided/changed
	if req.ProjectID != nil {
		var project dbEngine.Project
		if err := dbEngine.DB.Where("id = ? AND user_id = ?", *req.ProjectID, userInfo.ID).First(&project).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				http.Error(w, "Project not found", http.StatusNotFound)
				return
			}
			http.Error(w, "Failed to fetch project", http.StatusInternalServerError)
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
		http.Error(w, "Failed to update env", http.StatusInternalServerError)
		return
	}

	helpers.WriteJSON(w, map[string]interface{}{"env": env})
}

// EnvDeleteByIdHandler godoc
//
//	@Summary		Delete environment asset
//	@Description	Delete an environment asset by its ID
//	@Tags			environment
//	@Accept			json
//	@Produce		json
//	@Param			envId	path	int	true	"Environment asset ID"
//	@Success		200		{object}	map[string]interface{}
//	@Failure		401		{object}	map[string]string
//	@Failure		400		{object}	map[string]string
//	@Failure		404		{object}	map[string]string
//	@Failure		500		{object}	map[string]string
//	@Router			/env/{envId} [delete]
//	@Security		BearerAuth
func (c *EnvController) EnvDeleteByIdHandler(w http.ResponseWriter, r *http.Request) {
	userInfo, err := c.UserService.GetUserFromContext(r.Context())
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(r)
	envID, err := strconv.Atoi(vars["envId"])
	if err != nil {
		http.Error(w, "Invalid env ID", http.StatusBadRequest)
		return
	}

	if err := dbEngine.DB.Where("id = ? AND user_id = ?", envID, userInfo.ID).Delete(&dbEngine.Env{}).Error; err != nil {
		http.Error(w, "Failed to delete env", http.StatusInternalServerError)
		return
	}

	helpers.WriteJSON(w, map[string]string{"status": "deleted"})
}
