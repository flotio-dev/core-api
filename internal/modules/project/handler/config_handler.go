package handler

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
	"gorm.io/gorm"

	dbEngine "github.com/flotio-dev/core-api/internal/common/database"
	helpers "github.com/flotio-dev/core-api/internal/common/server"
	services "github.com/flotio-dev/core-api/internal/modules/user/service"
)

type ConfigController struct {
	UserService *services.UserService
}

func NewConfigController(userService *services.UserService) *ConfigController {
	return &ConfigController{
		UserService: userService,
	}
}

type ProjectConfigResponse struct {
	Config dbEngine.ProjectConfig `json:"config"`
}

// ConfigGetHandler godoc
//
//	@Summary		Get project configuration
//	@Description	Get the configuration for a specific project
//	@Tags			config
//	@Accept			json
//	@Produce		json
//	@Param			id	path	int	true	"Project ID"
//	@Success		200	{object}	ProjectConfigResponse
//	@Failure		401	{object}	map[string]string
//	@Failure		404	{object}	map[string]string
//	@Failure		500	{object}	map[string]string
//	@Router			/project/{id}/config [get]
//	@Security		BearerAuth
func (c *ConfigController) ConfigGetHandler(w http.ResponseWriter, r *http.Request) {
	userInfo, err := c.UserService.GetUserFromContext(r.Context())
	if err != nil {
		helpers.WriteErrorJSON(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(r)
	projectID, err := strconv.Atoi(vars["id"])
	if err != nil {
		helpers.WriteErrorJSON(w, "Invalid project ID", http.StatusBadRequest)
		return
	}

	var config dbEngine.ProjectConfig
	if err := dbEngine.DB.Joins("JOIN projects ON project_configs.project_id = projects.id").
		Where("projects.id = ? AND projects.user_id = ?", projectID, userInfo.ID).
		First(&config).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			// Return empty config if not found
			helpers.WriteJSON(w, ProjectConfigResponse{Config: dbEngine.ProjectConfig{ProjectID: uint(projectID)}})
			return
		}
		helpers.WriteErrorJSON(w, "Failed to fetch config", http.StatusInternalServerError)
		return
	}

	helpers.WriteJSON(w, ProjectConfigResponse{Config: config})
}

// ConfigPostHandler godoc
//
//	@Summary		Update project configuration
//	@Description	Create or update the configuration for a specific project (Supports partial updates)
//	@Tags			config
//	@Accept			json
//	@Produce		json
//	@Param			id		path	int						true	"Project ID"
//	@Param			config	body	dbEngine.ProjectConfig	true	"Configuration data"
//	@Success		200		{object}	ProjectConfigResponse
//	@Failure		401		{object}	map[string]string
//	@Failure		404		{object}	map[string]string
//	@Failure		500		{object}	map[string]string
//	@Router			/project/{id}/config [post]
//	@Security		BearerAuth
func (c *ConfigController) ConfigPostHandler(w http.ResponseWriter, r *http.Request) {
	userInfo, err := c.UserService.GetUserFromContext(r.Context())
	if err != nil {
		helpers.WriteErrorJSON(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(r)
	projectID, err := strconv.Atoi(vars["id"])
	if err != nil {
		helpers.WriteErrorJSON(w, "Invalid project ID", http.StatusBadRequest)
		return
	}

	// Read raw body to detect present fields
	body, err := io.ReadAll(r.Body)
	if err != nil {
		helpers.WriteErrorJSON(w, "Failed to read request body", http.StatusBadRequest)
		return
	}

	var rawMap map[string]interface{}
	if err := json.Unmarshal(body, &rawMap); err != nil {
		helpers.WriteErrorJSON(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	var req dbEngine.ProjectConfig
	if err := json.Unmarshal(body, &req); err != nil {
		helpers.WriteErrorJSON(w, "Failed to parse configuration", http.StatusBadRequest)
		return
	}

	// Verify project ownership
	var project dbEngine.Project
	if err := dbEngine.DB.Where("id = ? AND user_id = ?", projectID, userInfo.ID).First(&project).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			helpers.WriteErrorJSON(w, "Project not found", http.StatusNotFound)
			return
		}
		helpers.WriteErrorJSON(w, "Failed to fetch project", http.StatusInternalServerError)
		return
	}

	var config dbEngine.ProjectConfig
	result := dbEngine.DB.Where("project_id = ?", projectID).First(&config)
	
	if result.Error != nil && result.Error != gorm.ErrRecordNotFound {
		helpers.WriteErrorJSON(w, "Failed to fetch existing config", http.StatusInternalServerError)
		return
	}

	// Set ProjectID
	config.ProjectID = uint(projectID)

	// Helper to check if a key was present in the JSON
	has := func(key string) bool {
		_, ok := rawMap[key]
		return ok
	}

	// Apply partial updates
	if has("platforms") { config.Platforms = req.Platforms }
	if has("build_trigger") { config.BuildTrigger = req.BuildTrigger }
	if has("watched_branch_patterns") { config.WatchedBranchPatterns = req.WatchedBranchPatterns }
	if has("watched_tag_patterns") { config.WatchedTagPatterns = req.WatchedTagPatterns }
	if has("env_variables") { config.EnvVariables = req.EnvVariables }
	if has("dependency_caching") { config.DependencyCaching = req.DependencyCaching }
	if has("dependency_dirs") { config.DependencyDirs = req.DependencyDirs }
	
	// Git Connection
	if has("git_repo") { config.GitRepo = req.GitRepo }
	if has("git_username") { config.GitUsername = req.GitUsername }
	if has("git_token") { config.GitToken = req.GitToken }

	// Webhooks
	if has("webhook_urls") { config.WebhookURLs = req.WebhookURLs }

	// Scripts
	if has("post_clone_script") { config.PostCloneScript = req.PostCloneScript }
	if has("pre_test_script") { config.PreTestScript = req.PreTestScript }
	if has("post_test_script") { config.PostTestScript = req.PostTestScript }
	if has("pre_build_script") { config.PreBuildScript = req.PreBuildScript }
	if has("post_build_script") { config.PostBuildScript = req.PostBuildScript }
	if has("pre_publish_script") { config.PrePublishScript = req.PrePublishScript }

	// Testing
	if has("test") { config.Test = req.Test }
	if has("enable_flutter_analyze") { config.EnableFlutterAnalyze = req.EnableFlutterAnalyze }
	if has("flutter_analyze_args") { config.FlutterAnalyzeArgs = req.FlutterAnalyzeArgs }
	if has("enable_flutter_test") { config.EnableFlutterTest = req.EnableFlutterTest }
	if has("flutter_test_args") { config.FlutterTestArgs = req.FlutterTestArgs }
	if has("enable_flutter_driver") { config.EnableFlutterDriver = req.EnableFlutterDriver }
	if has("flutter_driver_args") { config.FlutterDriverArgs = req.FlutterDriverArgs }
	if has("flutter_driver_targets") { config.FlutterDriverTargets = req.FlutterDriverTargets }

	// Build Settings
	if has("flutter_version") { config.FlutterVersion = req.FlutterVersion }
	if has("xcode_version") { config.XcodeVersion = req.XcodeVersion }
	if has("cocoapods_version") { config.CocoaPodsVersion = req.CocoaPodsVersion }
	if has("project_path") { config.ProjectPath = req.ProjectPath }
	if has("android_build_format") { config.AndroidBuildFormat = req.AndroidBuildFormat }
	if has("build_mode") { config.BuildMode = req.BuildMode }
	if has("android_build_args") { config.AndroidBuildArgs = req.AndroidBuildArgs }
	if has("ios_build_args") { config.IosBuildArgs = req.IosBuildArgs }
	if has("web_build_args") { config.WebBuildArgs = req.WebBuildArgs }

	// Distribution
	if has("enable_android_code_signing") { config.EnableAndroidCodeSigning = req.EnableAndroidCodeSigning }
	if has("enable_google_play_publishing") { config.EnableGooglePlayPublishing = req.EnableGooglePlayPublishing }
	if has("google_play_track") { config.GooglePlayTrack = req.GooglePlayTrack }
	if has("update_priority") { config.UpdatePriority = req.UpdatePriority }
	if has("rollout_fraction") { config.RolloutFraction = req.RolloutFraction }
	if has("do_not_send_for_review") { config.DoNotSendForReview = req.DoNotSendForReview }
	if has("submit_as_draft") { config.SubmitAsDraft = req.SubmitAsDraft }
	if has("publish_even_if_tests_fail") { config.PublishEvenIfTestsFail = req.PublishEvenIfTestsFail }

	// Linked User-level Assets
	if has("keystore_id") { config.KeystoreID = req.KeystoreID }
	if has("google_play_credentials_id") { config.GooglePlayCredentialsID = req.GooglePlayCredentialsID }

	// Notifications
	if has("enable_email_notifications") { config.EnableEmailNotifications = req.EnableEmailNotifications }
	if has("email_recipients") { config.EmailRecipients = req.EmailRecipients }

	if result.Error == gorm.ErrRecordNotFound {
		if err := dbEngine.DB.Create(&config).Error; err != nil {
			helpers.WriteErrorJSON(w, "Failed to create config", http.StatusInternalServerError)
			return
		}
	} else {
		if err := dbEngine.DB.Save(&config).Error; err != nil {
			helpers.WriteErrorJSON(w, "Failed to update config", http.StatusInternalServerError)
			return
		}
	}

	helpers.WriteJSON(w, ProjectConfigResponse{Config: config})
}

// ConfigDeleteHandler godoc
//
//	@Summary		Delete project configuration
//	@Description	Delete or reset the configuration for a specific project
//	@Tags			config
//	@Accept			json
//	@Produce		json
//	@Param			id	path	int	true	"Project ID"
//	@Success		200	{object}	map[string]string
//	@Failure		401	{object}	map[string]string
//	@Failure		404	{object}	map[string]string
//	@Failure		500	{object}	map[string]string
//	@Router			/project/{id}/config [delete]
//	@Security		BearerAuth
func (c *ConfigController) ConfigDeleteHandler(w http.ResponseWriter, r *http.Request) {
	userInfo, err := c.UserService.GetUserFromContext(r.Context())
	if err != nil {
		helpers.WriteErrorJSON(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(r)
	projectID, err := strconv.Atoi(vars["id"])
	if err != nil {
		helpers.WriteErrorJSON(w, "Invalid project ID", http.StatusBadRequest)
		return
	}

	// Verify project ownership and delete config
	if err := dbEngine.DB.Joins("JOIN projects ON project_configs.project_id = projects.id").
		Where("projects.id = ? AND projects.user_id = ?", projectID, userInfo.ID).
		Delete(&dbEngine.ProjectConfig{}).Error; err != nil {
		helpers.WriteErrorJSON(w, "Failed to delete config", http.StatusInternalServerError)
		return
	}

	helpers.WriteJSON(w, map[string]string{"status": "deleted"})
}
