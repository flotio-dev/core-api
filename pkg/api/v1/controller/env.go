package controller

import (
	"net/http"
	"strconv"

	"github.com/flotio-dev/api/pkg/db"
	"github.com/gorilla/mux"
	"gorm.io/gorm"

	middleware "github.com/flotio-dev/api/pkg/api/v1/middleware"
	utils "github.com/flotio-dev/api/pkg/utils"
)

// Env handlers
func EnvGetHandler(w http.ResponseWriter, r *http.Request) {
	userInfo := middleware.GetUserFromContext(r.Context())
	if userInfo == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(r)
	projectID, err := strconv.Atoi(vars["id"])
	if err != nil {
		http.Error(w, "Invalid project ID", http.StatusBadRequest)
		return
	}

	var envs []db.Env
	if err := db.DB.Joins("JOIN projects ON envs.project_id = projects.id").Where("projects.id = ? AND projects.user_id = (SELECT id FROM users WHERE keycloak_id = ?)", projectID, *userInfo.Sub).Find(&envs).Error; err != nil {
		http.Error(w, "Failed to fetch envs", http.StatusInternalServerError)
		return
	}

	utils.WriteJSON(w, map[string]interface{}{"envs": envs})
}
func EnvPostHandler(w http.ResponseWriter, r *http.Request) {
	userInfo := middleware.GetUserFromContext(r.Context())
	if userInfo == nil {
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
	if err := utils.ReadJSON(r, &req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Verify project ownership
	var project db.Project
	if err := db.DB.Where("id = ? AND user_id = (SELECT id FROM users WHERE keycloak_id = ?)", projectID, *userInfo.Sub).First(&project).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			http.Error(w, "Project not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Failed to fetch project", http.StatusInternalServerError)
		return
	}

	env := db.Env{
		ProjectID: project.ID,
		Key:       req.Key,
		Value:     req.Value,
	}

	if err := db.DB.Create(&env).Error; err != nil {
		http.Error(w, "Failed to create env", http.StatusInternalServerError)
		return
	}

	utils.WriteJSON(w, map[string]interface{}{"env": env})
}

func EnvGetByIdHandler(w http.ResponseWriter, r *http.Request) {
	userInfo := middleware.GetUserFromContext(r.Context())
	if userInfo == nil {
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

	var env db.Env
	if err := db.DB.Joins("JOIN projects ON envs.project_id = projects.id").Where("envs.id = ? AND projects.id = ? AND projects.user_id = (SELECT id FROM users WHERE keycloak_id = ?)", envID, projectID, *userInfo.Sub).First(&env).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			http.Error(w, "Env not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Failed to fetch env", http.StatusInternalServerError)
		return
	}

	utils.WriteJSON(w, map[string]interface{}{"env": env})
}

func EnvPutByIdHandler(w http.ResponseWriter, r *http.Request) {
	userInfo := middleware.GetUserFromContext(r.Context())
	if userInfo == nil {
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
	if err := utils.ReadJSON(r, &req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	var env db.Env
	if err := db.DB.Joins("JOIN projects ON envs.project_id = projects.id").Where("envs.id = ? AND projects.id = ? AND projects.user_id = (SELECT id FROM users WHERE keycloak_id = ?)", envID, projectID, *userInfo.Sub).First(&env).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			http.Error(w, "Env not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Failed to fetch env", http.StatusInternalServerError)
		return
	}

	env.Key = req.Key
	env.Value = req.Value

	if err := db.DB.Save(&env).Error; err != nil {
		http.Error(w, "Failed to update env", http.StatusInternalServerError)
		return
	}

	utils.WriteJSON(w, map[string]interface{}{"env": env})
}

func EnvDeleteByIdHandler(w http.ResponseWriter, r *http.Request) {
	userInfo := middleware.GetUserFromContext(r.Context())
	if userInfo == nil {
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

	if err := db.DB.Joins("JOIN projects ON envs.project_id = projects.id").Where("envs.id = ? AND projects.id = ? AND projects.user_id = (SELECT id FROM users WHERE keycloak_id = ?)", envID, projectID, *userInfo.Sub).Delete(&db.Env{}).Error; err != nil {
		http.Error(w, "Failed to delete env", http.StatusInternalServerError)
		return
	}

	utils.WriteJSON(w, map[string]string{"status": "deleted"})
}
