package handlers

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"gorm.io/gorm"

	dbEngine "github.com/flotio-dev/api/internal/engines/db"
	keycloakEngine "github.com/flotio-dev/api/internal/engines/keycloak"
	kubernetesEngine "github.com/flotio-dev/api/internal/engines/kubernetes"
	s3Engine "github.com/flotio-dev/api/internal/engines/s3"
	helpers "github.com/flotio-dev/api/internal/helpers"
	services "github.com/flotio-dev/api/internal/services"
)

type ProjectController struct {
	githubService *services.GithubService
}

func NewProjectController(githubService *services.GithubService) *ProjectController {
	return &ProjectController{
		githubService: githubService,
	}
}

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
	userInfo := services.GetUserFromContext(r.Context())
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
	userInfo := services.GetUserFromContext(r.Context())
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
	userInfo := services.GetUserFromContext(r.Context())
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
	userInfo := services.GetUserFromContext(r.Context())
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
	userInfo := services.GetUserFromContext(r.Context())
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

// ProjectBuildHandler godoc
//
//	@Summary		Build a project
//	@Description	Start a build for a specific project
//	@Tags			projects
//	@Accept			json
//	@Produce		json
//	@Param			id		path	int				true	"Project ID"
//	@Param			build	body	BuildRequest	true	"Build data"
//	@Success		200		{object}	BuildResponse
//	@Failure		400		{object}	map[string]string
//	@Failure		401		{object}	map[string]string
//	@Failure		404		{object}	map[string]string
//	@Failure		500		{object}	map[string]string
//	@Router			/project/{id}/build [post]
func (c *ProjectController) ProjectBuildHandler(w http.ResponseWriter, r *http.Request) {
	userInfo := services.GetUserFromContext(r.Context())
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

	var req BuildRequest
	if err := helpers.ReadJSON(r, &req); err != nil {
		// If no body, use defaults
		req.Platform = "android"
		req.BuildMode = "release"
		req.BuildTarget = "apk"
		req.FlutterChannel = "stable"
		req.GitBranch = "main"
	}

	// Set defaults for empty fields
	if req.Platform == "" {
		req.Platform = "android"
	}
	if req.BuildMode == "" {
		req.BuildMode = "release"
	}
	if req.BuildTarget == "" {
		if req.Platform == "android" {
			req.BuildTarget = "apk"
		} else {
			req.BuildTarget = req.Platform
		}
	}
	if req.FlutterChannel == "" {
		req.FlutterChannel = "stable"
	}
	if req.GitBranch == "" {
		req.GitBranch = "main"
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

	build := dbEngine.Build{
		ProjectID: project.ID,
		Status:    "pending",
		Platform:  req.Platform,
	}

	if err := dbEngine.DB.Create(&build).Error; err != nil {
		helpers.WriteErrorJSON(w, "Failed to create build", http.StatusInternalServerError)
		return
	}

	githubInstallationDB, err := c.githubService.GetInstallationByUser(userInfo.DB.ID)
	if err != nil {
		helpers.WriteErrorJSON(w, "Failed to get GitHub installation", http.StatusInternalServerError)
		return
	}

	githubInstallation, err := c.githubService.GetGithubInstallation(r.Context(), githubInstallationDB.InstallationID)
	if err != nil {
		helpers.WriteErrorJSON(w, "Failed to get GitHub installation details", http.StatusInternalServerError)
		return
	}
	username := githubInstallation.Account.Login

	installationToken, err := c.githubService.GetInstallationToken(githubInstallationDB.InstallationID)
	if err != nil {
		helpers.WriteErrorJSON(w, "Failed to get GitHub installation token", http.StatusInternalServerError)
		return
	}

	// Start the build process by creating a Kubernetes pod
	buildConfig := kubernetesEngine.BuildConfig{
		BuildID:        build.ID,
		Project:        project,
		Platform:       req.Platform,
		BuildMode:      req.BuildMode,
		BuildTarget:    req.BuildTarget,
		FlutterChannel: req.FlutterChannel,
		GitBranch:      req.GitBranch,
		GitUsername:    *username,
		GitToken:       installationToken,
	}

	if err := kubernetesEngine.CreateBuildPod(buildConfig); err != nil {
		// If pod creation fails, update build status to failed
		fmt.Printf("Failed to create build pod for build %d: %v\n", build.ID, err)

		build.Status = "failed"
		dbEngine.DB.Save(&build)
		helpers.WriteErrorJSON(w, "Failed to start build process", http.StatusInternalServerError)
		return
	}

	// Start listening to pod logs in a goroutine
	go kubernetesEngine.StartPodLogListener(build.ID)

	// Update build status to running
	build.Status = "running"
	dbEngine.DB.Save(&build)

	helpers.WriteJSON(w, BuildResponse{Build: convertDBBuild(build)})
}

// BuildCancelHandler godoc
//
//	@Summary		Cancel a build
//	@Description	Cancel a specific build for a project
//	@Tags			builds
//	@Accept			json
//	@Produce		json
//	@Param			id		path	int					true	"Project ID"
//	@Param			buildId	path	int					true	"Build ID"
//	@Success		200		{object}	BuildResponse
//	@Failure		400		{object}	map[string]string
//	@Failure		401		{object}	map[string]string
//	@Failure		404		{object}	map[string]string
//	@Failure		500		{object}	map[string]string
//	@Router			/project/{id}/builds/{buildId}/cancel [post]
func BuildCancelHandler(w http.ResponseWriter, r *http.Request) {
	userInfo := services.GetUserFromContext(r.Context())
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
	buildID, err := strconv.Atoi(vars["buildId"])
	if err != nil {
		helpers.WriteErrorJSON(w, "Invalid build ID", http.StatusBadRequest)
		return
	}

	var build dbEngine.Build
	if err := dbEngine.DB.Joins("JOIN projects ON builds.project_id = projects.id").Where("builds.id = ? AND projects.id = ? AND projects.user_id = (SELECT id FROM users WHERE keycloak_id = ?)", buildID, projectID, *userInfo.Keycloak.Sub).First(&build).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			helpers.WriteErrorJSON(w, "Build not found", http.StatusNotFound)
			return
		}
		helpers.WriteErrorJSON(w, "Failed to fetch build", http.StatusInternalServerError)
		return
	}

	// Delete the Kubernetes pod
	if err := kubernetesEngine.DeleteBuildPod(build.ID); err != nil {
		fmt.Printf("Failed to delete build pod for build %d: %v\n", build.ID, err)
		// Continue with cancellation even if pod deletion fails
	}

	build.Status = "cancelled"
	if err := dbEngine.DB.Save(&build).Error; err != nil {
		helpers.WriteErrorJSON(w, "Failed to cancel build", http.StatusInternalServerError)
		return
	}

	helpers.WriteJSON(w, BuildResponse{Build: convertDBBuild(build)})
}

// BuildsListHandler godoc
//
//	@Summary		List builds
//	@Description	Get all builds for a specific project
//	@Tags			builds
//	@Accept			json
//	@Produce		json
//	@Param			id	path	int					true	"Project ID"
//	@Success		200	{object}	BuildsResponse
//	@Failure		400	{object}	map[string]string
//	@Failure		401	{object}	map[string]string
//	@Failure		500	{object}	map[string]string
//	@Router			/project/{id}/builds [get]
func BuildsListHandler(w http.ResponseWriter, r *http.Request) {
	userInfo := services.GetUserFromContext(r.Context())
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

	var builds []dbEngine.Build
	if err := dbEngine.DB.Joins("JOIN projects ON builds.project_id = projects.id").Where("projects.id = ? AND projects.user_id = (SELECT id FROM users WHERE keycloak_id = ?)", projectID, *userInfo.Keycloak.Sub).Order("builds.created_at DESC").Find(&builds).Error; err != nil {
		helpers.WriteErrorJSON(w, "Failed to fetch builds", http.StatusInternalServerError)
		return
	}

	// Check and update status for running builds by querying pod status
	// Also reconcile APKURL for successful builds that don't have it yet
	for i := range builds {
		// Reconcile APKURL for successful builds without artifact URL
		if builds[i].Status == "success" && builds[i].APKURL == "" {
			if artifactKey, err := s3Engine.FindPrimaryArtifactKey(builds[i].ID, builds[i].Platform); err == nil {
				builds[i].APKURL = artifactKey
				fmt.Printf("Build %d: found artifact at S3 key: %s\n", builds[i].ID, artifactKey)
				dbEngine.DB.Save(&builds[i])
			} else {
				fmt.Printf("Build %d: failed to find artifact in S3: %v\n", builds[i].ID, err)
			}
			continue
		}

		if builds[i].Status == "running" || builds[i].Status == "pending" {
			podStatus, err := kubernetesEngine.GetPodStatus(builds[i].ID)
			if err != nil {
				// Pod might not exist anymore, mark as failed
				builds[i].Status = "failed"
				dbEngine.DB.Save(&builds[i])
				continue
			}

			// Map pod status to build status
			var newStatus string
			switch podStatus {
			case "Succeeded":
				newStatus = "success"
			case "Failed":
				newStatus = "failed"
			case "Running":
				newStatus = "running"
			case "Pending":
				newStatus = "pending"
			default:
				continue // Don't update if unknown status
			}

			// Update if status changed
			if builds[i].Status != newStatus {
				builds[i].Status = newStatus
				// Reconcile S3 artifact key when build succeeds
				if newStatus == "success" && builds[i].APKURL == "" {
					if artifactKey, err := s3Engine.FindPrimaryArtifactKey(builds[i].ID, builds[i].Platform); err == nil {
						builds[i].APKURL = artifactKey
						fmt.Printf("Build %d: found artifact at S3 key: %s\n", builds[i].ID, artifactKey)
					} else {
						fmt.Printf("Build %d: failed to find artifact in S3: %v\n", builds[i].ID, err)
					}
				}
				dbEngine.DB.Save(&builds[i])
			}
		}
	}

	helpers.WriteJSON(w, BuildsResponse{Builds: convertDBBuilds(builds)})
}

// BuildLogsHandler godoc
//
//	@Summary		Get build logs
//	@Description	Get logs for a specific build
//	@Tags			builds
//	@Accept			json
//	@Produce		json
//	@Param			id		path	int					true	"Project ID"
//	@Param			buildId	path	int					true	"Build ID"
//	@Success		200		{object}	LogsResponse
//	@Failure		400		{object}	map[string]string
//	@Failure		401		{object}	map[string]string
//	@Failure		404		{object}	map[string]string
//	@Failure		500		{object}	map[string]string
//	@Router			/project/{id}/builds/{buildId}/logs [get]
func BuildLogsHandler(w http.ResponseWriter, r *http.Request) {
	userInfo := services.GetUserFromContext(r.Context())
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
	buildID, err := strconv.Atoi(vars["buildId"])
	if err != nil {
		helpers.WriteErrorJSON(w, "Invalid build ID", http.StatusBadRequest)
		return
	}

	// Verify the build belongs to the user's project
	var build dbEngine.Build
	if err := dbEngine.DB.Joins("JOIN projects ON builds.project_id = projects.id").Where("builds.id = ? AND projects.id = ? AND projects.user_id = (SELECT id FROM users WHERE keycloak_id = ?)", buildID, projectID, *userInfo.Keycloak.Sub).First(&build).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			helpers.WriteErrorJSON(w, "Build not found", http.StatusNotFound)
			return
		}
		helpers.WriteErrorJSON(w, "Failed to fetch build", http.StatusInternalServerError)
		return
	}

	// Get logs from the database
	var dbLogs []dbEngine.Log
	if err := dbEngine.DB.Where("build_id = ?", buildID).Order("line_number ASC").Find(&dbLogs).Error; err != nil {
		helpers.WriteErrorJSON(w, "Failed to fetch logs", http.StatusInternalServerError)
		return
	}

	// Convert database logs to string array
	logs := make([]string, len(dbLogs))
	for i, log := range dbLogs {
		logs[i] = log.Content
	}

	helpers.WriteJSON(w, LogsResponse{Logs: logs})
}

// BuildLogsWSHandler godoc
//
//	@Summary		Get build logs via WebSocket
//	@Description	Stream logs for a specific build via WebSocket
//	@Tags			builds
//	@Accept			json
//	@Produce		json
//	@Param			buildId	path	int	true	"Build ID"
//	@Param			token	query	string	true	"Auth token"
//	@Success		101
//	@Failure		400	{object}	map[string]string
//	@Failure		401	{object}	map[string]string
//	@Router			/project/{id}/builds/{buildId}/logs/ws [get]
func BuildLogsWSHandler(w http.ResponseWriter, r *http.Request) {
	// Auth via query param
	token := r.URL.Query().Get("token")
	if token == "" {
		helpers.WriteErrorJSON(w, "Missing token", http.StatusUnauthorized)
		return
	}

	// Validate token (simplified, in real app use proper validation)
	client := keycloakEngine.GetKeycloakClient()
	ctx := context.Background()
	realm := os.Getenv("KEYCLOAK_REALM")
	_, err := client.GetUserInfo(ctx, token, realm)
	if err != nil {
		helpers.WriteErrorJSON(w, "Invalid token", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(r)
	buildID, err := strconv.Atoi(vars["buildId"])
	if err != nil {
		helpers.WriteErrorJSON(w, "Invalid build ID", http.StatusBadRequest)
		return
	}

	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true }, // Allow all origins for demo
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		helpers.WriteErrorJSON(w, "Failed to upgrade", http.StatusInternalServerError)
		return
	}
	defer conn.Close()

	// Stream logs from the Kubernetes pod
	logChan := make(chan string, 100)
	go func() {
		err := kubernetesEngine.StreamPodLogs(uint(buildID), logChan)
		if err != nil {
			fmt.Printf("Error streaming pod logs: %v\n", err)
		}
	}()

	// Get the current max line number for this build
	var maxLine dbEngine.Log
	if err := dbEngine.DB.Where("build_id = ?", buildID).Order("line_number DESC").First(&maxLine).Error; err != nil {
		// If no logs exist, start from 1
		maxLine.LineNumber = 0
	}
	lineNumber := maxLine.LineNumber + 1

	for logLine := range logChan {
		// Save log to database
		logEntry := dbEngine.Log{
			BuildID:    uint(buildID),
			LineNumber: lineNumber,
			Content:    logLine,
			Timestamp:  time.Now().Unix(),
		}
		if err := dbEngine.DB.Create(&logEntry).Error; err != nil {
			fmt.Printf("Failed to save log to database: %v\n", err)
		}

		err := conn.WriteMessage(websocket.TextMessage, []byte(logLine))
		if err != nil {
			break
		}
		lineNumber++
	}
}

// BuildDownloadResponse represents the response for the build download endpoint
type BuildDownloadResponse struct {
	DownloadURL string `json:"download_url"`
	ArtifactKey string `json:"artifact_key"`
	ExpiresIn   int    `json:"expires_in"`
}

// BuildDownloadHandler godoc
//
//	@Summary		Download build artifact
//	@Description	Get a presigned URL to download the artifact for a specific build
//	@Tags			builds
//	@Accept			json
//	@Produce		json
//	@Param			id		path	int	true	"Project ID"
//	@Param			buildId	path	int	true	"Build ID"
//	@Success		200	{object}	BuildDownloadResponse
//	@Failure		401	{object}	map[string]string
//	@Failure		404	{object}	map[string]string
//	@Router			/project/{id}/build/{buildId}/download [get]
func BuildDownloadHandler(w http.ResponseWriter, r *http.Request) {
	if services.GetUserFromContext(r.Context()) == nil {
		helpers.WriteErrorJSON(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(r)
	buildID, err := strconv.ParseUint(vars["buildId"], 10, 32)
	if err != nil {
		helpers.WriteErrorJSON(w, "Invalid build ID", http.StatusBadRequest)
		return
	}

	// Get build from database
	var build dbEngine.Build
	if err := dbEngine.DB.First(&build, buildID).Error; err != nil {
		helpers.WriteErrorJSON(w, "Build not found", http.StatusNotFound)
		return
	}

	// Check if artifact key exists
	if build.APKURL == "" {
		helpers.WriteErrorJSON(w, "No artifact available for this build", http.StatusNotFound)
		return
	}

	// Generate presigned URL (valid for 1 hour)
	presignedURL, err := s3Engine.GetPresignedURL(build.APKURL, 3600)
	if err != nil {
		helpers.WriteErrorJSON(w, "Failed to generate download URL", http.StatusInternalServerError)
		return
	}

	helpers.WriteJSON(w, BuildDownloadResponse{
		DownloadURL: presignedURL,
		ArtifactKey: build.APKURL,
		ExpiresIn:   3600,
	})
}
