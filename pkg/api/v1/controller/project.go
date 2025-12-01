package controller

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/flotio-dev/api/pkg/db"
	"github.com/flotio-dev/api/pkg/kubernetes"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"gorm.io/gorm"

	middleware "github.com/flotio-dev/api/pkg/api/v1/middleware"
	utils "github.com/flotio-dev/api/pkg/utils"
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
func convertDBProject(p db.Project) Project {
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

func convertDBBuild(b db.Build) Build {
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

func convertDBProjects(projects []db.Project) []Project {
	result := make([]Project, len(projects))
	for i, p := range projects {
		result[i] = convertDBProject(p)
	}
	return result
}

func convertDBBuilds(builds []db.Build) []Build {
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
//	@Router			/projects [get]
func ProjectsGetHandler(w http.ResponseWriter, r *http.Request) {
	userInfo := middleware.GetUserFromContext(r.Context())
	if userInfo == nil {
		utils.WriteErrorJSON(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var user db.User
	if err := db.DB.Where("keycloak_id = ?", *userInfo.Keycloak.Sub).First(&user).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			utils.WriteErrorJSON(w, "User not found", http.StatusNotFound)
			return
		}
		utils.WriteErrorJSON(w, "Failed to fetch user", http.StatusInternalServerError)
		return
	}

	var projects []db.Project
	if err := db.DB.Where("user_id = ?", user.ID).Find(&projects).Error; err != nil {
		utils.WriteErrorJSON(w, "Failed to fetch projects", http.StatusInternalServerError)
		return
	}

	utils.WriteJSON(w, ProjectsResponse{Projects: convertDBProjects(projects)})
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
//	@Router			/projects [post]
func ProjectCreateHandler(w http.ResponseWriter, r *http.Request) {
	userInfo := middleware.GetUserFromContext(r.Context())
	if userInfo == nil {
		utils.WriteErrorJSON(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var user db.User
	if err := db.DB.Where("keycloak_id = ?", *userInfo.Keycloak.Sub).First(&user).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			utils.WriteErrorJSON(w, "User not found", http.StatusNotFound)
			return
		}
		utils.WriteErrorJSON(w, "Failed to fetch user", http.StatusInternalServerError)
		return
	}

	var req ProjectCreateRequest
	if err := utils.ReadJSON(r, &req); err != nil {
		utils.WriteErrorJSON(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	project := db.Project{
		Name:           req.Name,
		GitRepo:        req.GitRepo,
		BuildFolder:    req.BuildFolder,
		FlutterVersion: req.FlutterVersion,
		GitUsername:    req.GitUsername,
		GitToken:       req.GitToken,
		UserID:         user.ID,
	}

	if err := db.DB.Create(&project).Error; err != nil {
		utils.WriteErrorJSON(w, "Failed to create project", http.StatusInternalServerError)
		return
	}

	utils.WriteJSON(w, ProjectResponse{Project: convertDBProject(project)})
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
//	@Router			/projects/{id} [get]
func ProjectGetHandler(w http.ResponseWriter, r *http.Request) {
	userInfo := middleware.GetUserFromContext(r.Context())
	if userInfo == nil {
		utils.WriteErrorJSON(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(r)
	projectID, err := strconv.Atoi(vars["id"])
	if err != nil {
		utils.WriteErrorJSON(w, "Invalid project ID", http.StatusBadRequest)
		return
	}

	var project db.Project
	if err := db.DB.Preload("Builds").Preload("Envs").Where("id = ? AND user_id = (SELECT id FROM users WHERE keycloak_id = ?)", projectID, *userInfo.Keycloak.Sub).First(&project).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			utils.WriteErrorJSON(w, "Project not found", http.StatusNotFound)
			return
		}
		utils.WriteErrorJSON(w, "Failed to fetch project", http.StatusInternalServerError)
		return
	}

	utils.WriteJSON(w, ProjectResponse{Project: convertDBProject(project)})
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
//	@Router			/projects/{id} [put]
func ProjectPutHandler(w http.ResponseWriter, r *http.Request) {
	userInfo := middleware.GetUserFromContext(r.Context())
	if userInfo == nil {
		utils.WriteErrorJSON(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(r)
	projectID, err := strconv.Atoi(vars["id"])
	if err != nil {
		utils.WriteErrorJSON(w, "Invalid project ID", http.StatusBadRequest)
		return
	}

	var req ProjectUpdateRequest
	if err := utils.ReadJSON(r, &req); err != nil {
		utils.WriteErrorJSON(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	var project db.Project
	if err := db.DB.Where("id = ? AND user_id = (SELECT id FROM users WHERE keycloak_id = ?)", projectID, *userInfo.Keycloak.Sub).First(&project).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			utils.WriteErrorJSON(w, "Project not found", http.StatusNotFound)
			return
		}
		utils.WriteErrorJSON(w, "Failed to fetch project", http.StatusInternalServerError)
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

	if err := db.DB.Save(&project).Error; err != nil {
		utils.WriteErrorJSON(w, "Failed to update project", http.StatusInternalServerError)
		return
	}

	utils.WriteJSON(w, ProjectResponse{Project: convertDBProject(project)})
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
//	@Router			/projects/{id} [delete]
func ProjectDeleteHandler(w http.ResponseWriter, r *http.Request) {
	userInfo := middleware.GetUserFromContext(r.Context())
	if userInfo == nil {
		utils.WriteErrorJSON(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(r)
	projectID, err := strconv.Atoi(vars["id"])
	if err != nil {
		utils.WriteErrorJSON(w, "Invalid project ID", http.StatusBadRequest)
		return
	}

	if err := db.DB.Where("id = ? AND user_id = (SELECT id FROM users WHERE keycloak_id = ?)", projectID, *userInfo.Keycloak.Sub).Delete(&db.Project{}).Error; err != nil {
		utils.WriteErrorJSON(w, "Failed to delete project", http.StatusInternalServerError)
		return
	}

	utils.WriteJSON(w, DeleteResponse{Status: "deleted"})
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
//	@Router			/projects/{id}/build [post]
func ProjectBuildHandler(w http.ResponseWriter, r *http.Request) {
	userInfo := middleware.GetUserFromContext(r.Context())
	if userInfo == nil {
		utils.WriteErrorJSON(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(r)
	projectID, err := strconv.Atoi(vars["id"])
	if err != nil {
		utils.WriteErrorJSON(w, "Invalid project ID", http.StatusBadRequest)
		return
	}

	var req BuildRequest
	if err := utils.ReadJSON(r, &req); err != nil {
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

	var project db.Project
	if err := db.DB.Where("id = ? AND user_id = (SELECT id FROM users WHERE keycloak_id = ?)", projectID, *userInfo.Keycloak.Sub).First(&project).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			utils.WriteErrorJSON(w, "Project not found", http.StatusNotFound)
			return
		}
		utils.WriteErrorJSON(w, "Failed to fetch project", http.StatusInternalServerError)
		return
	}

	build := db.Build{
		ProjectID: project.ID,
		Status:    "pending",
		Platform:  req.Platform,
	}

	if err := db.DB.Create(&build).Error; err != nil {
		utils.WriteErrorJSON(w, "Failed to create build", http.StatusInternalServerError)
		return
	}

	// Start the build process by creating a Kubernetes pod
	buildConfig := kubernetes.BuildConfig{
		BuildID:        build.ID,
		Project:        project,
		Platform:       req.Platform,
		BuildMode:      req.BuildMode,
		BuildTarget:    req.BuildTarget,
		FlutterChannel: req.FlutterChannel,
		GitBranch:      req.GitBranch,
		GitUsername:    req.GitUsername,
		GitToken:       req.GitToken,
	}

	if err := kubernetes.CreateBuildPod(buildConfig); err != nil {
		// If pod creation fails, update build status to failed
		build.Status = "failed"
		db.DB.Save(&build)
		utils.WriteErrorJSON(w, "Failed to start build process", http.StatusInternalServerError)
		return
	}

	// Update build status to running
	build.Status = "running"
	db.DB.Save(&build)

	utils.WriteJSON(w, BuildResponse{Build: convertDBBuild(build)})
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
//	@Router			/projects/{id}/builds/{buildId}/cancel [post]
func BuildCancelHandler(w http.ResponseWriter, r *http.Request) {
	userInfo := middleware.GetUserFromContext(r.Context())
	if userInfo == nil {
		utils.WriteErrorJSON(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(r)
	projectID, err := strconv.Atoi(vars["id"])
	if err != nil {
		utils.WriteErrorJSON(w, "Invalid project ID", http.StatusBadRequest)
		return
	}
	buildID, err := strconv.Atoi(vars["buildId"])
	if err != nil {
		utils.WriteErrorJSON(w, "Invalid build ID", http.StatusBadRequest)
		return
	}

	var build db.Build
	if err := db.DB.Joins("JOIN projects ON builds.project_id = projects.id").Where("builds.id = ? AND projects.id = ? AND projects.user_id = (SELECT id FROM users WHERE keycloak_id = ?)", buildID, projectID, *userInfo.Keycloak.Sub).First(&build).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			utils.WriteErrorJSON(w, "Build not found", http.StatusNotFound)
			return
		}
		utils.WriteErrorJSON(w, "Failed to fetch build", http.StatusInternalServerError)
		return
	}

	build.Status = "cancelled"
	if err := db.DB.Save(&build).Error; err != nil {
		utils.WriteErrorJSON(w, "Failed to cancel build", http.StatusInternalServerError)
		return
	}

	utils.WriteJSON(w, BuildResponse{Build: convertDBBuild(build)})
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
//	@Router			/projects/{id}/builds [get]
func BuildsListHandler(w http.ResponseWriter, r *http.Request) {
	userInfo := middleware.GetUserFromContext(r.Context())
	if userInfo == nil {
		utils.WriteErrorJSON(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(r)
	projectID, err := strconv.Atoi(vars["id"])
	if err != nil {
		utils.WriteErrorJSON(w, "Invalid project ID", http.StatusBadRequest)
		return
	}

	var builds []db.Build
	if err := db.DB.Joins("JOIN projects ON builds.project_id = projects.id").Where("projects.id = ? AND projects.user_id = (SELECT id FROM users WHERE keycloak_id = ?)", projectID, *userInfo.Keycloak.Sub).Find(&builds).Error; err != nil {
		utils.WriteErrorJSON(w, "Failed to fetch builds", http.StatusInternalServerError)
		return
	}

	utils.WriteJSON(w, BuildsResponse{Builds: convertDBBuilds(builds)})
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
//	@Router			/projects/{id}/builds/{buildId}/logs [get]
func BuildLogsHandler(w http.ResponseWriter, r *http.Request) {
	userInfo := middleware.GetUserFromContext(r.Context())
	if userInfo == nil {
		utils.WriteErrorJSON(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(r)
	projectID, err := strconv.Atoi(vars["id"])
	if err != nil {
		utils.WriteErrorJSON(w, "Invalid project ID", http.StatusBadRequest)
		return
	}
	buildID, err := strconv.Atoi(vars["buildId"])
	if err != nil {
		utils.WriteErrorJSON(w, "Invalid build ID", http.StatusBadRequest)
		return
	}

	// Verify the build belongs to the user's project
	var build db.Build
	if err := db.DB.Joins("JOIN projects ON builds.project_id = projects.id").Where("builds.id = ? AND projects.id = ? AND projects.user_id = (SELECT id FROM users WHERE keycloak_id = ?)", buildID, projectID, *userInfo.Keycloak.Sub).First(&build).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			utils.WriteErrorJSON(w, "Build not found", http.StatusNotFound)
			return
		}
		utils.WriteErrorJSON(w, "Failed to fetch build", http.StatusInternalServerError)
		return
	}

	// Get logs from the Kubernetes pod
	logs, err := kubernetes.GetPodLogs(uint(buildID))
	if err != nil {
		utils.WriteErrorJSON(w, "Failed to fetch logs", http.StatusInternalServerError)
		return
	}

	utils.WriteJSON(w, LogsResponse{Logs: logs})
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
//	@Router			/projects/{id}/builds/{buildId}/logs/ws [get]
func BuildLogsWSHandler(w http.ResponseWriter, r *http.Request) {
	// Auth via query param
	token := r.URL.Query().Get("token")
	if token == "" {
		utils.WriteErrorJSON(w, "Missing token", http.StatusUnauthorized)
		return
	}

	// Validate token (simplified, in real app use proper validation)
	client := utils.GetKeycloakClient()
	ctx := context.Background()
	realm := os.Getenv("KEYCLOAK_REALM")
	_, err := client.GetUserInfo(ctx, token, realm)
	if err != nil {
		utils.WriteErrorJSON(w, "Invalid token", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(r)
	buildID, err := strconv.Atoi(vars["buildId"])
	if err != nil {
		utils.WriteErrorJSON(w, "Invalid build ID", http.StatusBadRequest)
		return
	}

	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true }, // Allow all origins for demo
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		utils.WriteErrorJSON(w, "Failed to upgrade", http.StatusInternalServerError)
		return
	}
	defer conn.Close()

	// Stream logs from the Kubernetes pod
	logChan := make(chan string, 100)
	go func() {
		err := kubernetes.StreamPodLogs(uint(buildID), logChan)
		if err != nil {
			fmt.Printf("Error streaming pod logs: %v\n", err)
		}
	}()

	// Get the current max line number for this build
	var maxLine db.Log
	if err := db.DB.Where("build_id = ?", buildID).Order("line_number DESC").First(&maxLine).Error; err != nil {
		// If no logs exist, start from 1
		maxLine.LineNumber = 0
	}
	lineNumber := maxLine.LineNumber + 1

	for logLine := range logChan {
		// Save log to database
		logEntry := db.Log{
			BuildID:    uint(buildID),
			LineNumber: lineNumber,
			Content:    logLine,
			Timestamp:  time.Now().Unix(),
		}
		if err := db.DB.Create(&logEntry).Error; err != nil {
			fmt.Printf("Failed to save log to database: %v\n", err)
		}

		err := conn.WriteMessage(websocket.TextMessage, []byte(logLine))
		if err != nil {
			break
		}
		lineNumber++
	}
}

// BuildDownloadHandler godoc
//
//	@Summary		Download build artifact
//	@Description	Download the artifact for a specific build
//	@Tags			builds
//	@Accept			json
//	@Produce		application/octet-stream
//	@Param			id		path	int	true	"Project ID"
//	@Param			buildId	path	int	true	"Build ID"
//	@Success		200
//	@Failure		401	{object}	map[string]string
//	@Router			/projects/{id}/builds/{buildId}/download [get]
func BuildDownloadHandler(w http.ResponseWriter, r *http.Request) {
	if middleware.GetUserFromContext(r.Context()) == nil {
		utils.WriteErrorJSON(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	vars := mux.Vars(r)
	// Simulate file download
	filename := "app-" + vars["buildId"] + ".apk"
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", "attachment; filename="+filename)
	w.Write([]byte("fake apk content"))
}
