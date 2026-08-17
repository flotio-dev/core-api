package handler

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/mux"
	"gorm.io/gorm"

	dbEngine "github.com/flotio-dev/core-api/internal/common/database"
	helpers "github.com/flotio-dev/core-api/internal/common/server"
	models "github.com/flotio-dev/core-api/internal/models"
	userServices "github.com/flotio-dev/core-api/internal/modules/user/service"
)

type ProjectController struct {
	UserService *userServices.UserService
}

func NewProjectController(userService *userServices.UserService) *ProjectController {
	return &ProjectController{
		UserService: userService,
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
	Name   string                  `json:"name" example:"My Flutter App"`
	Config *dbEngine.ProjectConfig `json:"config,omitempty"`
}

type ProjectUpdateRequest struct {
	Name   string                  `json:"name,omitempty" example:"Updated App Name"`
	Config *dbEngine.ProjectConfig `json:"config,omitempty"`
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
	ID        uint                    `json:"id"`
	CreatedAt time.Time               `json:"created_at"`
	UpdatedAt time.Time               `json:"updated_at"`
	Name      string                  `json:"name"`
	UserID    uint                    `json:"user_id"`
	Config    *dbEngine.ProjectConfig `json:"config"`
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
	var configPtr *dbEngine.ProjectConfig
	if p.Config != nil {
		configPtr = p.Config
	} else {
		var config dbEngine.ProjectConfig
		if err := dbEngine.DB.Where("project_id = ?", p.ID).First(&config).Error; err == nil {
			configPtr = &config
		}
	}
	return Project{
		ID:        p.ID,
		CreatedAt: p.CreatedAt,
		UpdatedAt: p.UpdatedAt,
		Name:      p.Name,
		UserID:    p.UserID,
		Config:    configPtr,
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
//	@Failure		401	{object}	models.APIErrorResponse
//	@Failure		404	{object}	models.APIErrorResponse
//	@Failure		500	{object}	models.APIErrorResponse
//	@Security		BearerAuth
//	@ID				ProjectsGetHandler
//	@Router			/project [get]
func (c *ProjectController) ProjectsGetHandler(w http.ResponseWriter, r *http.Request) {
	userInfo, err := c.UserService.GetUserFromContext(r.Context())
	if err != nil {
		helpers.WriteErrorJSON(w, "Failed to get user info", http.StatusUnauthorized)
		return
	}
	if userInfo == nil {
		helpers.WriteErrorJSON(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var projects []dbEngine.Project
	if err := dbEngine.DB.Preload("Config").Where("user_id = ?", userInfo.ID).Find(&projects).Error; err != nil {
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
//	@Failure		400		{object}	models.APIErrorResponse
//	@Failure		401		{object}	models.APIErrorResponse
//	@Failure		404		{object}	models.APIErrorResponse
//	@Failure		500		{object}	models.APIErrorResponse
//	@Security		BearerAuth
//	@ID				ProjectCreateHandler
//	@Router			/project [post]
func (c *ProjectController) ProjectCreateHandler(w http.ResponseWriter, r *http.Request) {
	userInfo, err := c.UserService.GetUserFromContext(r.Context())
	if err != nil {
		helpers.WriteErrorJSON(w, "Failed to get user info", http.StatusUnauthorized)
		return
	}
	if userInfo == nil {
		helpers.WriteErrorJSON(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req ProjectCreateRequest
	if err := helpers.ReadJSON(r, &req); err != nil {
		helpers.WriteErrorJSON(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	project := dbEngine.Project{
		Name:   req.Name,
		UserID: userInfo.ID,
	}

	if err := dbEngine.DB.Create(&project).Error; err != nil {
		helpers.WriteErrorJSON(w, "Failed to create project", http.StatusInternalServerError)
		return
	}

	// Create associated ProjectConfig
	var projectConfig dbEngine.ProjectConfig
	if req.Config != nil {
		projectConfig = *req.Config
	}
	projectConfig.ProjectID = project.ID

	if err := dbEngine.DB.Create(&projectConfig).Error; err != nil {
		helpers.WriteErrorJSON(w, "Failed to create project configuration", http.StatusInternalServerError)
		return
	}

	project.Config = &projectConfig

	helpers.WriteJSON(w, ProjectResponse{Project: convertDBProject(project)})
}

// ProjectGetHandler godoc
//
//	@Summary		Get a project
//	@Description	Get a specific project by ID for the authenticated user
//	@Tags			projects
//	@Accept			json
//	@Produce		json
//	@Param			id	path		int					true	"Project ID"	Format(int64)
//	@Success		200	{object}	ProjectResponse
//	@Failure		400	{object}	models.APIErrorResponse
//	@Failure		401	{object}	models.APIErrorResponse
//	@Failure		404	{object}	models.APIErrorResponse
//	@Failure		500	{object}	models.APIErrorResponse
//	@Security		BearerAuth
//	@ID				ProjectGetHandler
//	@Router			/project/{id} [get]
func (c *ProjectController) ProjectGetHandler(w http.ResponseWriter, r *http.Request) {
	userInfo, err := c.UserService.GetUserFromContext(r.Context())
	if err != nil {
		helpers.WriteErrorJSON(w, "Failed to get user info", http.StatusUnauthorized)
		return
	}
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
	if err := dbEngine.DB.Preload("Builds").Preload("Config").Where("id = ? AND user_id = ?", projectID, userInfo.ID).First(&project).Error; err != nil {
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
//	@Param			id		path	int						true	"Project ID"	Format(int64)
//	@Param			project	body	ProjectUpdateRequest	true	"Project data"
//	@Success		200		{object}	ProjectResponse
//	@Failure		400		{object}	models.APIErrorResponse
//	@Failure		401		{object}	models.APIErrorResponse
//	@Failure		404		{object}	models.APIErrorResponse
//	@Failure		500		{object}	models.APIErrorResponse
//	@Security		BearerAuth
//	@ID				ProjectPutHandler
//	@Router			/project/{id} [put]
func (c *ProjectController) ProjectPutHandler(w http.ResponseWriter, r *http.Request) {
	userInfo, err := c.UserService.GetUserFromContext(r.Context())
	if err != nil {
		helpers.WriteErrorJSON(w, "Failed to get user info", http.StatusUnauthorized)
		return
	}
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
	if err := dbEngine.DB.Preload("Config").Where("id = ? AND user_id = ?", projectID, userInfo.ID).First(&project).Error; err != nil {
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

	if err := dbEngine.DB.Save(&project).Error; err != nil {
		helpers.WriteErrorJSON(w, "Failed to update project", http.StatusInternalServerError)
		return
	}

	// Update or Create ProjectConfig
	var projectConfig dbEngine.ProjectConfig
	hasConfig := true
	if err := dbEngine.DB.Where("project_id = ?", project.ID).First(&projectConfig).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			hasConfig = false
			projectConfig.ProjectID = project.ID
		} else {
			helpers.WriteErrorJSON(w, "Failed to fetch project configuration", http.StatusInternalServerError)
			return
		}
	}

	// If the request contains a full nested config, apply its fields!
	if req.Config != nil {
		c := req.Config
		projectConfig.Platforms = c.Platforms
		projectConfig.BuildTrigger = c.BuildTrigger
		projectConfig.WatchedBranchPatterns = c.WatchedBranchPatterns
		projectConfig.WatchedTagPatterns = c.WatchedTagPatterns
		projectConfig.DependencyCaching = c.DependencyCaching
		projectConfig.DependencyDirs = c.DependencyDirs
		
		if c.GitRepo != "" { projectConfig.GitRepo = c.GitRepo }
		if c.GitUsername != "" { projectConfig.GitUsername = c.GitUsername }
		if c.GitToken != "" { projectConfig.GitToken = c.GitToken }
		
		projectConfig.WebhookURLs = c.WebhookURLs
		projectConfig.PostCloneScript = c.PostCloneScript
		projectConfig.PreTestScript = c.PreTestScript
		projectConfig.PostTestScript = c.PostTestScript
		projectConfig.PreBuildScript = c.PreBuildScript
		projectConfig.PostBuildScript = c.PostBuildScript
		projectConfig.PrePublishScript = c.PrePublishScript
		
		projectConfig.Test = c.Test
		projectConfig.EnableFlutterAnalyze = c.EnableFlutterAnalyze
		projectConfig.FlutterAnalyzeArgs = c.FlutterAnalyzeArgs
		projectConfig.EnableFlutterTest = c.EnableFlutterTest
		projectConfig.FlutterTestArgs = c.FlutterTestArgs
		projectConfig.EnableFlutterDriver = c.EnableFlutterDriver
		projectConfig.FlutterDriverArgs = c.FlutterDriverArgs
		projectConfig.FlutterDriverTargets = c.FlutterDriverTargets
		
		if c.FlutterVersion != "" { projectConfig.FlutterVersion = c.FlutterVersion }
		projectConfig.XcodeVersion = c.XcodeVersion
		projectConfig.CocoaPodsVersion = c.CocoaPodsVersion
		if c.ProjectPath != "" { projectConfig.ProjectPath = c.ProjectPath }
		projectConfig.AndroidBuildFormat = c.AndroidBuildFormat
		projectConfig.BuildMode = c.BuildMode
		projectConfig.AndroidBuildArgs = c.AndroidBuildArgs
		projectConfig.IosBuildArgs = c.IosBuildArgs
		projectConfig.WebBuildArgs = c.WebBuildArgs
		
		projectConfig.PackageName = c.PackageName
		projectConfig.EnableAndroidCodeSigning = c.EnableAndroidCodeSigning
		projectConfig.EnableGooglePlayPublishing = c.EnableGooglePlayPublishing
		projectConfig.GooglePlayTrack = c.GooglePlayTrack
		projectConfig.UpdatePriority = c.UpdatePriority
		projectConfig.RolloutFraction = c.RolloutFraction
		projectConfig.DoNotSendForReview = c.DoNotSendForReview
		projectConfig.SubmitAsDraft = c.SubmitAsDraft
		projectConfig.PublishEvenIfTestsFail = c.PublishEvenIfTestsFail
		
		projectConfig.EnableEmailNotifications = c.EnableEmailNotifications
		projectConfig.EmailRecipients = c.EmailRecipients
	}

	var configErr error
	if hasConfig {
		configErr = dbEngine.DB.Save(&projectConfig).Error
	} else {
		configErr = dbEngine.DB.Create(&projectConfig).Error
	}

	if configErr != nil {
		helpers.WriteErrorJSON(w, "Failed to update project configuration", http.StatusInternalServerError)
		return
	}

	project.Config = &projectConfig

	helpers.WriteJSON(w, ProjectResponse{Project: convertDBProject(project)})
}

// ProjectDeleteHandler godoc
//
//	@Summary		Delete a project
//	@Description	Delete a specific project by ID for the authenticated user
//	@Tags			projects
//	@Accept			json
//	@Produce		json
//	@Param			id	path	int					true	"Project ID"	Format(int64)
//	@Success		200	{object}	DeleteResponse
//	@Failure		400	{object}	models.APIErrorResponse
//	@Failure		401	{object}	models.APIErrorResponse
//	@Failure		500	{object}	models.APIErrorResponse
//	@Security		BearerAuth
//	@ID				ProjectDeleteHandler
//	@Router			/project/{id} [delete]
func (c *ProjectController) ProjectDeleteHandler(w http.ResponseWriter, r *http.Request) {
	userInfo, err := c.UserService.GetUserFromContext(r.Context())
	if err != nil {
		helpers.WriteErrorJSON(w, "Failed to get user info", http.StatusUnauthorized)
		return
	}
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

	if err := dbEngine.DB.Where("id = ? AND user_id = ?", projectID, userInfo.ID).Delete(&dbEngine.Project{}).Error; err != nil {
		helpers.WriteErrorJSON(w, "Failed to delete project", http.StatusInternalServerError)
		return
	}

	helpers.WriteJSON(w, DeleteResponse{Status: "deleted"})
}

// Keep the swag annotation import alive (used only in @Failure comments).
var _ = models.APIErrorResponse{}
