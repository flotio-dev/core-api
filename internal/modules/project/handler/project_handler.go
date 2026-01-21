package handler

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/mux"
	"gorm.io/gorm"

	dbEngine "github.com/flotio-dev/core-api/internal/common/database"
	helpers "github.com/flotio-dev/core-api/internal/common/server"
	userService "github.com/flotio-dev/core-api/internal/modules/user/service"
)

// Response structs for API documentation
type ProjectsResponse struct {
	Projects []Project `json:"projects"`
}

type ProjectResponse struct {
	Project Project `json:"project"`
}

type BuildResponse struct {
	Build Build `json:"build"`
}

type BuildsResponse struct {
	Builds []Build `json:"builds"`
}

type DeleteResponse struct {
	Status string `json:"status"`
}

type LogsResponse struct {
	Logs []string `json:"logs"`
}

// Request structs for API documentation
type ProjectCreateRequest struct {
	Name           string  `json:"name" example:"My Flutter App"`
	GitRepo        *string `json:"git_repo,omitempty" example:"https://github.com/user/repo.git"`
	BuildFolder    *string `json:"build_folder,omitempty" example:"."`
	FlutterVersion *string `json:"flutter_version,omitempty" example:"3.19.0"`
	GitUsername    *string `json:"git_username,omitempty" example:"username"`
	GitToken       *string `json:"git_token,omitempty" example:"ghp_xxx"`
}

type ProjectUpdateRequest struct {
	Name           string  `json:"name,omitempty" example:"Updated App Name"`
	GitRepo        *string `json:"git_repo,omitempty" example:"https://github.com/user/repo.git"`
	BuildFolder    *string `json:"build_folder,omitempty" example:"."`
	FlutterVersion *string `json:"flutter_version,omitempty" example:"3.19.0"`
	GitUsername    *string `json:"git_username,omitempty" example:"username"`
	GitToken       *string `json:"git_token,omitempty" example:"ghp_xxx"`
}

type BuildRequest struct {
	Platform       string `json:"platform,omitempty" example:"android"`
	BuildMode      string `json:"build_mode,omitempty" example:"release"`
	BuildTarget    string `json:"build_target,omitempty" example:"apk"`
	FlutterChannel string `json:"flutter_channel,omitempty" example:"stable"`
	GitBranch      string `json:"git_branch,omitempty" example:"main"`
	GitUsername    string `json:"git_username,omitempty" example:"username"`
	GitToken       string `json:"git_token,omitempty" example:"ghp_xxx"`
}

// Simplified structs for Swagger documentation
type Project struct {
	ID             uint      `json:"id"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	Name           string    `json:"name"`
	GitRepo        *string   `json:"git_repo,omitempty"`
	BuildFolder    *string   `json:"build_folder,omitempty"`
	FlutterVersion *string   `json:"flutter_version,omitempty"`
	GitUsername    *string   `json:"git_username,omitempty"`
	UserID         uint      `json:"user_id"`
}

type Build struct {
	ID          uint      `json:"id"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	ProjectID   uint      `json:"project_id"`
	Status      string    `json:"status"`
	Platform    string    `json:"platform"`
	ContainerID string    `json:"container_id"`
	Duration    int64     `json:"duration"`
	APKURL      string    `json:"apk_url"`
}

// Conversion functions
func convertDBProject(p dbEngine.Project) Project {
	return Project{
		ID:             p.ID,
		CreatedAt:      p.CreatedAt,
		UpdatedAt:      p.UpdatedAt,
		Name:           p.Name,
		GitRepo:        p.GitRepo,
		BuildFolder:    p.BuildFolder,
		FlutterVersion: p.FlutterVersion,
		GitUsername:    p.GitUsername,
		UserID:         p.UserID,
	}
}

func convertDBBuild(b dbEngine.Build) Build {
	return Build{
		ID:          b.ID,
		CreatedAt:   b.CreatedAt,
		UpdatedAt:   b.UpdatedAt,
		ProjectID:   b.ProjectID,
		Status:      b.Status,
		Platform:    b.Platform,
		ContainerID: b.ContainerID,
		Duration:    b.Duration,
		APKURL:      b.APKURL,
	}
}

func convertDBProjects(projects []dbEngine.Project) []Project {
	result := make([]Project, len(projects))
	for i, p := range projects {
		result[i] = convertDBProject(p)
	}
	return result
}

func convertDBBuilds(builds []dbEngine.Build) []Build {
	result := make([]Build, len(builds))
	for i, b := range builds {
		result[i] = convertDBBuild(b)
	}
	return result
}

// ProjectsGetHandler godoc
//
//	@Summary		Get projects
//	@Description	Get all projects for the authenticated user
//	@Tags			projects
//	@Accept			json
//	@Produce		json
//	@Success		200	{object}	ProjectsResponse
//	@Failure		401	{object}	map[string]string
//	@Failure		404	{object}	map[string]string
//	@Failure		500	{object}	map[string]string
//	@Router			/project [get]
func ProjectsGetHandler(w http.ResponseWriter, r *http.Request) {
	userInfo := userService.GetUserFromContext(r.Context())
	if userInfo == nil {
		helpers.WriteErrorJSON(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var user dbEngine.User
	if err := dbEngine.DB.Where("keycloak_id = ?", *userInfo.Keycloak.Sub).First(&user).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			helpers.WriteErrorJSON(w, "User not found", http.StatusNotFound)
			return
		}
		helpers.WriteErrorJSON(w, "Failed to fetch user", http.StatusInternalServerError)
		return
	}

	var projects []dbEngine.Project
	if err := dbEngine.DB.Where("user_id = ?", user.ID).Find(&projects).Error; err != nil {
		helpers.WriteErrorJSON(w, "Failed to fetch projects", http.StatusInternalServerError)
		return
	}

	helpers.WriteJSON(w, ProjectsResponse{Projects: convertDBProjects(projects)})
}

// ProjectCreateHandler godoc
//
//	@Summary		Create a project
//	@Description	Create a new project for the authenticated user
//	@Tags			projects
//	@Accept			json
//	@Produce		json
//	@Param			project	body		ProjectCreateRequest	true	"Project data"
//	@Success		200		{object}	ProjectResponse
//	@Failure		400		{object}	map[string]string
//	@Failure		401		{object}	map[string]string
//	@Failure		404		{object}	map[string]string
//	@Failure		500		{object}	map[string]string
//	@Router			/project [post]
func ProjectCreateHandler(w http.ResponseWriter, r *http.Request) {
	userInfo := userService.GetUserFromContext(r.Context())
	if userInfo == nil {
		helpers.WriteErrorJSON(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var user dbEngine.User
	if err := dbEngine.DB.Where("keycloak_id = ?", *userInfo.Keycloak.Sub).First(&user).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			helpers.WriteErrorJSON(w, "User not found", http.StatusNotFound)
			return
		}
		helpers.WriteErrorJSON(w, "Failed to fetch user", http.StatusInternalServerError)
		return
	}

	var req ProjectCreateRequest
	if err := helpers.ReadJSON(r, &req); err != nil {
		helpers.WriteErrorJSON(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	project := dbEngine.Project{
		Name:           req.Name,
		GitRepo:        req.GitRepo,
		BuildFolder:    req.BuildFolder,
		FlutterVersion: req.FlutterVersion,
		GitUsername:    req.GitUsername,
		GitToken:       req.GitToken,
		UserID:         user.ID,
	}

	if err := dbEngine.DB.Create(&project).Error; err != nil {
		helpers.WriteErrorJSON(w, "Failed to create project", http.StatusInternalServerError)
		return
	}

	helpers.WriteJSON(w, ProjectResponse{Project: convertDBProject(project)})
}

// ProjectGetHandler godoc
//
//	@Summary		Get a project
//	@Description	Get a specific project by ID for the authenticated user
//	@Tags			projects
//	@Accept			json
//	@Produce		json
//	@Param			id	path		int					true	"Project ID"
//	@Success		200	{object}	ProjectResponse
//	@Failure		400	{object}	map[string]string
//	@Failure		401	{object}	map[string]string
//	@Failure		404	{object}	map[string]string
//	@Failure		500	{object}	map[string]string
//	@Router			/project/{id} [get]
func ProjectGetHandler(w http.ResponseWriter, r *http.Request) {
	userInfo := userService.GetUserFromContext(r.Context())
	if userInfo == nil {
		helpers.WriteErrorJSON(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(r)
	projectID, err := strconv.Atoi(vars["id"])
	if err != nil {
		helpers.WriteErrorJSON(w, "Invalid project ID", http.StatusBadRequest)
		return
	}

	var project dbEngine.Project
	if err := dbEngine.DB.Preload("Builds").Preload("Envs").Where("id = ? AND user_id = (SELECT id FROM users WHERE keycloak_id = ?)", projectID, *userInfo.Keycloak.Sub).First(&project).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			helpers.WriteErrorJSON(w, "Project not found", http.StatusNotFound)
			return
		}
		helpers.WriteErrorJSON(w, "Failed to fetch project", http.StatusInternalServerError)
		return
	}

	helpers.WriteJSON(w, ProjectResponse{Project: convertDBProject(project)})
}

// ProjectPutHandler godoc
//
//	@Summary		Update a project
//	@Description	Update a specific project by ID for the authenticated user
//	@Tags			projects
//	@Accept			json
//	@Produce		json
//	@Param			id		path	int						true	"Project ID"
//	@Param			project	body	ProjectUpdateRequest	true	"Project data"
//	@Success		200		{object}	ProjectResponse
//	@Failure		400		{object}	map[string]string
//	@Failure		401		{object}	map[string]string
//	@Failure		404		{object}	map[string]string
//	@Failure		500		{object}	map[string]string
//	@Router			/project/{id} [put]
func ProjectPutHandler(w http.ResponseWriter, r *http.Request) {
	userInfo := userService.GetUserFromContext(r.Context())
	if userInfo == nil {
		helpers.WriteErrorJSON(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(r)
	projectID, err := strconv.Atoi(vars["id"])
	if err != nil {
		helpers.WriteErrorJSON(w, "Invalid project ID", http.StatusBadRequest)
		return
	}

	var req ProjectUpdateRequest
	if err := helpers.ReadJSON(r, &req); err != nil {
		helpers.WriteErrorJSON(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	var project dbEngine.Project
	if err := dbEngine.DB.Where("id = ? AND user_id = (SELECT id FROM users WHERE keycloak_id = ?)", projectID, *userInfo.Keycloak.Sub).First(&project).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			helpers.WriteErrorJSON(w, "Project not found", http.StatusNotFound)
			return
		}
		helpers.WriteErrorJSON(w, "Failed to fetch project", http.StatusInternalServerError)
		return
	}

	if req.Name != "" {
		project.Name = req.Name
	}
	if req.GitRepo != nil {
		project.GitRepo = req.GitRepo
	}
	if req.BuildFolder != nil {
		project.BuildFolder = req.BuildFolder
	}
	if req.FlutterVersion != nil {
		project.FlutterVersion = req.FlutterVersion
	}
	if req.GitUsername != nil {
		project.GitUsername = req.GitUsername
	}
	if req.GitToken != nil {
		project.GitToken = req.GitToken
	}

	if err := dbEngine.DB.Save(&project).Error; err != nil {
		helpers.WriteErrorJSON(w, "Failed to update project", http.StatusInternalServerError)
		return
	}

	helpers.WriteJSON(w, ProjectResponse{Project: convertDBProject(project)})
}

// ProjectDeleteHandler godoc
//
//	@Summary		Delete a project
//	@Description	Delete a specific project by ID for the authenticated user
//	@Tags			projects
//	@Accept			json
//	@Produce		json
//	@Param			id	path	int					true	"Project ID"
//	@Success		200	{object}	DeleteResponse
//	@Failure		400	{object}	map[string]string
//	@Failure		401	{object}	map[string]string
//	@Failure		500	{object}	map[string]string
//	@Router			/project/{id} [delete]
func ProjectDeleteHandler(w http.ResponseWriter, r *http.Request) {
	userInfo := userService.GetUserFromContext(r.Context())
	if userInfo == nil {
		helpers.WriteErrorJSON(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(r)
	projectID, err := strconv.Atoi(vars["id"])
	if err != nil {
		helpers.WriteErrorJSON(w, "Invalid project ID", http.StatusBadRequest)
		return
	}

	if err := dbEngine.DB.Where("id = ? AND user_id = (SELECT id FROM users WHERE keycloak_id = ?)", projectID, *userInfo.Keycloak.Sub).Delete(&dbEngine.Project{}).Error; err != nil {
		helpers.WriteErrorJSON(w, "Failed to delete project", http.StatusInternalServerError)
		return
	}

	helpers.WriteJSON(w, DeleteResponse{Status: "deleted"})
}