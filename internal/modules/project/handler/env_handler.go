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
//	@Summary		Get environment variables
//	@Description	Get all environment variables for a project
//	@Tags			environment
//	@Accept			json
//	@Produce		json
//	@Param			id	path	int	true	"Project ID"
//	@Success		200	{object}	map[string]interface{}
//	@Failure		401	{object}	map[string]string
//	@Failure		400	{object}	map[string]string
//	@Failure		500	{object}	map[string]string
//	@Router			/project/{id}/env [get]
//	@Security		BearerAuth
func (c *EnvController) EnvGetHandler(w http.ResponseWriter, r *http.Request) {
	userInfo, err := c.UserService.GetUserFromContext(r.Context())
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(r)
	projectID, err := strconv.Atoi(vars["id"])
	if err != nil {
		http.Error(w, "Invalid project ID", http.StatusBadRequest)
		return
	}

	var envs []dbEngine.Env
	if err := dbEngine.DB.Joins("JOIN projects ON envs.project_id = projects.id").Where("projects.id = ? AND projects.user_id = ?", projectID, userInfo.ID).Find(&envs).Error; err != nil {
		http.Error(w, "Failed to fetch envs", http.StatusInternalServerError)
		return
	}

	helpers.WriteJSON(w, map[string]interface{}{"envs": envs})
}

// EnvPostHandler godoc
//
//	@Summary		Create environment variable
//	@Description	Create a new environment variable for a project
//	@Tags			environment
//	@Accept			json
//	@Produce		json
//	@Param			id	path	int	true	"Project ID"
//	@Param			env	body	map[string]string	true	"Environment variable data"
//	@Success		201	{object}	map[string]interface{}
//	@Failure		401	{object}	map[string]string
//	@Failure		400	{object}	map[string]string
//	@Failure		404	{object}	map[string]string
//	@Failure		500	{object}	map[string]string
//	@Router			/project/{id}/env [post]
//	@Security		BearerAuth
func (c *EnvController) EnvPostHandler(w http.ResponseWriter, r *http.Request) {
	userInfo, err := c.UserService.GetUserFromContext(r.Context())
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(r)
	projectID, err := strconv.Atoi(vars["id"])
	if err != nil {
		http.Error(w, "Invalid project ID", http.StatusBadRequest)
		return
	}

	var req struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}
	if err := helpers.ReadJSON(r, &req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Verify project ownership
	var project dbEngine.Project
	if err := dbEngine.DB.Where("id = ? AND user_id = ?", projectID, userInfo.ID).First(&project).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			http.Error(w, "Project not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Failed to fetch project", http.StatusInternalServerError)
		return
	}

	env := dbEngine.Env{
		ProjectID: project.ID,
		Key:       req.Key,
		Value:     req.Value,
	}

	if err := dbEngine.DB.Create(&env).Error; err != nil {
		http.Error(w, "Failed to create env", http.StatusInternalServerError)
		return
	}

	helpers.WriteJSON(w, map[string]interface{}{"env": env})
}

// EnvGetByIdHandler godoc
//
//	@Summary		Get environment variable by ID
//	@Description	Get a specific environment variable by its ID
//	@Tags			environment
//	@Accept			json
//	@Produce		json
//	@Param			id		path	int	true	"Project ID"
//	@Param			envId	path	int	true	"Environment variable ID"
//	@Success		200		{object}	map[string]interface{}
//	@Failure		401		{object}	map[string]string
//	@Failure		400		{object}	map[string]string
//	@Failure		404		{object}	map[string]string
//	@Failure		500		{object}	map[string]string
//	@Router			/project/{id}/env/{envId} [get]
//	@Security		BearerAuth
func (c *EnvController) EnvGetByIdHandler(w http.ResponseWriter, r *http.Request) {
	userInfo, err := c.UserService.GetUserFromContext(r.Context())
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(r)
	projectID, err := strconv.Atoi(vars["id"])
	if err != nil {
		http.Error(w, "Invalid project ID", http.StatusBadRequest)
		return
	}

	envID, err := strconv.Atoi(vars["envId"])
	if err != nil {
		http.Error(w, "Invalid env ID", http.StatusBadRequest)
		return
	}

	var env dbEngine.Env
	if err := dbEngine.DB.Joins("JOIN projects ON envs.project_id = projects.id").Where("envs.id = ? AND projects.id = ? AND projects.user_id = ?", envID, projectID, userInfo.ID).First(&env).Error; err != nil {
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
//	@Summary		Update environment variable
//	@Description	Update an existing environment variable
//	@Tags			environment
//	@Accept			json
//	@Produce		json
//	@Param			id		path	int					true	"Project ID"
//	@Param			envId	path	int					true	"Environment variable ID"
//	@Param			env		body	map[string]string	true	"Updated environment variable data"
//	@Success		200		{object}	map[string]interface{}
//	@Failure		401		{object}	map[string]string
//	@Failure		400		{object}	map[string]string
//	@Failure		404		{object}	map[string]string
//	@Failure		500		{object}	map[string]string
//	@Router			/project/{id}/env/{envId} [put]
//	@Security		BearerAuth
func (c *EnvController) EnvPutByIdHandler(w http.ResponseWriter, r *http.Request) {
	userInfo, err := c.UserService.GetUserFromContext(r.Context())
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(r)
	projectID, err := strconv.Atoi(vars["id"])
	if err != nil {
		http.Error(w, "Invalid project ID", http.StatusBadRequest)
		return
	}

	envID, err := strconv.Atoi(vars["envId"])
	if err != nil {
		http.Error(w, "Invalid env ID", http.StatusBadRequest)
		return
	}

	var req struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}
	if err := helpers.ReadJSON(r, &req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	var env dbEngine.Env
	if err := dbEngine.DB.Joins("JOIN projects ON envs.project_id = projects.id").Where("envs.id = ? AND projects.id = ? AND projects.user_id = ?", envID, projectID, userInfo.ID).First(&env).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			http.Error(w, "Env not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Failed to fetch env", http.StatusInternalServerError)
		return
	}

	env.Key = req.Key
	env.Value = req.Value

	if err := dbEngine.DB.Save(&env).Error; err != nil {
		http.Error(w, "Failed to update env", http.StatusInternalServerError)
		return
	}

	helpers.WriteJSON(w, map[string]interface{}{"env": env})
}

// EnvDeleteByIdHandler godoc
//
//	@Summary		Delete environment variable
//	@Description	Delete an environment variable by its ID
//	@Tags			environment
//	@Accept			json
//	@Produce		json
//	@Param			id		path	int	true	"Project ID"
//	@Param			envId	path	int	true	"Environment variable ID"
//	@Success		200		{object}	map[string]interface{}
//	@Failure		401		{object}	map[string]string
//	@Failure		400		{object}	map[string]string
//	@Failure		404		{object}	map[string]string
//	@Failure		500		{object}	map[string]string
//	@Router			/project/{id}/env/{envId} [delete]
//	@Security		BearerAuth
func (c *EnvController) EnvDeleteByIdHandler(w http.ResponseWriter, r *http.Request) {
	userInfo, err := c.UserService.GetUserFromContext(r.Context())
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(r)
	projectID, err := strconv.Atoi(vars["id"])
	if err != nil {
		http.Error(w, "Invalid project ID", http.StatusBadRequest)
		return
	}

	envID, err := strconv.Atoi(vars["envId"])
	if err != nil {
		http.Error(w, "Invalid env ID", http.StatusBadRequest)
		return
	}

	if err := dbEngine.DB.Joins("JOIN projects ON envs.project_id = projects.id").Where("envs.id = ? AND projects.id = ? AND projects.user_id = ?", envID, projectID, userInfo.ID).Delete(&dbEngine.Env{}).Error; err != nil {
		http.Error(w, "Failed to delete env", http.StatusInternalServerError)
		return
	}

	helpers.WriteJSON(w, map[string]string{"status": "deleted"})
}
