package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"

	dbEngine "github.com/flotio-dev/core-api/internal/common/database"
	helpers "github.com/flotio-dev/core-api/internal/common/server"
	googleplay "github.com/flotio-dev/core-api/internal/infra/googleplay"
	s3Engine "github.com/flotio-dev/core-api/internal/infra/s3"
	models "github.com/flotio-dev/core-api/internal/modules/release/model"
	userServices "github.com/flotio-dev/core-api/internal/modules/user/service"
)

// Release statuses.
const (
	statusPending    = "pending"
	statusUploading  = "uploading"
	statusInProgress = "in_progress"
	statusPublished  = "published"
	statusDraft      = "draft"
	statusFailed     = "failed"
)

const defaultTrack = "internal"

// ReleaseController handles Google Play publication operations.
type ReleaseController struct {
	userService *userServices.UserService
}

func NewReleaseController(userService *userServices.UserService) *ReleaseController {
	return &ReleaseController{userService: userService}
}

// PublishHandler godoc
//
//	@Summary		Publish a build to Google Play
//	@Description	Publishes a successful build's AAB to a Google Play track. Runs asynchronously; poll the release for status.
//	@Tags			releases
//	@Accept			json
//	@Produce		json
//	@Param			id		path	int						true	"Project ID"
//	@Param			buildId	path	int						true	"Build ID"
//	@Param			body	body	models.PublishRequest	false	"Publication overrides"
//	@Success		202	{object}	models.ReleaseResponse
//	@Failure		400	{object}	map[string]string
//	@Failure		401	{object}	map[string]string
//	@Failure		404	{object}	map[string]string
//	@Router			/project/{id}/build/{buildId}/publish [post]
//	@Security		BearerAuth
func (c *ReleaseController) PublishHandler(w http.ResponseWriter, r *http.Request) {
	userInfo, err := c.userService.GetUserFromContext(r.Context())
	if err != nil || userInfo == nil {
		helpers.WriteErrorJSON(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(r)
	projectID, err := strconv.ParseUint(vars["id"], 10, 32)
	if err != nil {
		helpers.WriteErrorJSON(w, "Invalid project ID", http.StatusBadRequest)
		return
	}
	buildID, err := strconv.ParseUint(vars["buildId"], 10, 32)
	if err != nil {
		helpers.WriteErrorJSON(w, "Invalid build ID", http.StatusBadRequest)
		return
	}

	// Load the build and verify ownership.
	var build dbEngine.Build
	if err := dbEngine.DB.
		Joins("JOIN projects ON builds.project_id = projects.id").
		Where("builds.id = ? AND projects.id = ? AND projects.user_id = ?", buildID, projectID, userInfo.ID).
		First(&build).Error; err != nil {
		helpers.WriteErrorJSON(w, "Build not found", http.StatusNotFound)
		return
	}
	if build.Status != "success" {
		helpers.WriteErrorJSON(w, "Build is not successful, cannot publish", http.StatusBadRequest)
		return
	}

	// Load project config + linked Google Play credentials.
	config, credentials, err := loadGooglePlayContext(uint(projectID), userInfo.ID)
	if err != nil {
		helpers.WriteErrorJSON(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Verify the service account can act on the app before doing any work.
	if status, msg := accessError(r.Context(), credentials.Credentials, config.PackageName); status != 0 {
		helpers.WriteErrorJSON(w, msg, status)
		return
	}

	// Resolve publication parameters: request overrides, then config defaults.
	req := parsePublishRequest(r)
	track := firstNonEmpty(req.Track, config.GooglePlayTrack, defaultTrack)
	rollout := config.RolloutFraction
	if req.RolloutFraction != nil {
		rollout = *req.RolloutFraction
	}
	draft := config.SubmitAsDraft
	if req.Draft != nil {
		draft = *req.Draft
	}

	// Create the release record in pending state.
	release := dbEngine.Release{
		ProjectID:       uint(projectID),
		BuildID:         build.ID,
		VersionName:     build.VersionName,
		VersionCode:     build.VersionCode,
		Track:           track,
		RolloutFraction: rollout,
		Status:          statusPending,
		ReleaseNotes:    req.ReleaseNotes,
	}
	if err := dbEngine.DB.Create(&release).Error; err != nil {
		helpers.WriteErrorJSON(w, "Failed to create release", http.StatusInternalServerError)
		return
	}

	// Run the publication asynchronously; the client polls the release status.
	input := googleplay.PublishInput{
		PackageName:      config.PackageName,
		Track:            track,
		RolloutFraction:  rollout,
		Draft:            draft,
		Name:             build.VersionName,
		ReleaseNotes:     req.ReleaseNotes,
		ReleaseNotesLang: req.ReleaseNotesLang,
	}
	go runPublish(release.ID, build.ID, credentials.Credentials, input)

	w.WriteHeader(http.StatusAccepted)
	helpers.WriteJSON(w, models.ReleaseResponse{Release: convertDBRelease(release)})
}

// runPublish performs the upload+commit and updates the release status.
func runPublish(releaseID, buildID uint, encryptedCredentials string, input googleplay.PublishInput) {
	ctx := context.Background()
	setReleaseStatus(releaseID, statusUploading, 0)

	aabKey, err := s3Engine.FindReleaseArtifactKey(buildID)
	if err != nil {
		failRelease(releaseID, "resolve AAB", err)
		return
	}
	reader, err := s3Engine.GetObject(aabKey)
	if err != nil {
		failRelease(releaseID, "open AAB", err)
		return
	}
	defer reader.Close()
	input.AAB = reader

	client, err := googleplay.NewClientFromCredentials(ctx, encryptedCredentials)
	if err != nil {
		failRelease(releaseID, "build client", err)
		return
	}

	result, err := client.Publish(ctx, input)
	if err != nil {
		failRelease(releaseID, "publish", err)
		return
	}

	setReleaseStatus(releaseID, mapPublishStatus(result.Status), result.VersionCode)
}

func setReleaseStatus(releaseID uint, status string, versionCode int64) {
	updates := map[string]interface{}{"status": status}
	if versionCode > 0 {
		updates["version_code"] = versionCode
	}
	if err := dbEngine.DB.Model(&dbEngine.Release{}).Where("id = ?", releaseID).Updates(updates).Error; err != nil {
		log.Printf("[release %d] failed to update status to %s: %v", releaseID, status, err)
	}
}

func failRelease(releaseID uint, step string, err error) {
	log.Printf("[release %d] %s failed: %v", releaseID, step, err)
	setReleaseStatus(releaseID, statusFailed, 0)
}

// mapPublishStatus converts a Google Play release status to a Release status.
func mapPublishStatus(playStatus string) string {
	switch playStatus {
	case "inProgress":
		return statusInProgress
	case "draft":
		return statusDraft
	default:
		return statusPublished
	}
}

// ReleaseGetHandler godoc
//
//	@Summary		Get a release
//	@Description	Get a release and its current status
//	@Tags			releases
//	@Produce		json
//	@Param			id			path	int	true	"Project ID"
//	@Param			releaseId	path	int	true	"Release ID"
//	@Success		200	{object}	models.ReleaseResponse
//	@Failure		401	{object}	map[string]string
//	@Failure		404	{object}	map[string]string
//	@Router			/project/{id}/release/{releaseId} [get]
//	@Security		BearerAuth
func (c *ReleaseController) ReleaseGetHandler(w http.ResponseWriter, r *http.Request) {
	userInfo, err := c.userService.GetUserFromContext(r.Context())
	if err != nil || userInfo == nil {
		helpers.WriteErrorJSON(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(r)
	projectID, err := strconv.ParseUint(vars["id"], 10, 32)
	if err != nil {
		helpers.WriteErrorJSON(w, "Invalid project ID", http.StatusBadRequest)
		return
	}
	releaseID, err := strconv.ParseUint(vars["releaseId"], 10, 32)
	if err != nil {
		helpers.WriteErrorJSON(w, "Invalid release ID", http.StatusBadRequest)
		return
	}

	release, err := c.findOwnedRelease(uint(releaseID), uint(projectID), userInfo.ID)
	if err != nil {
		helpers.WriteErrorJSON(w, "Release not found", http.StatusNotFound)
		return
	}

	helpers.WriteJSON(w, models.ReleaseResponse{Release: convertDBRelease(release)})
}

// ReleasesListHandler godoc
//
//	@Summary		List project releases
//	@Description	List all releases for a project
//	@Tags			releases
//	@Produce		json
//	@Param			id	path	int	true	"Project ID"
//	@Success		200	{object}	models.ReleasesResponse
//	@Failure		401	{object}	map[string]string
//	@Router			/project/{id}/releases [get]
//	@Security		BearerAuth
func (c *ReleaseController) ReleasesListHandler(w http.ResponseWriter, r *http.Request) {
	userInfo, err := c.userService.GetUserFromContext(r.Context())
	if err != nil || userInfo == nil {
		helpers.WriteErrorJSON(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(r)
	projectID, err := strconv.ParseUint(vars["id"], 10, 32)
	if err != nil {
		helpers.WriteErrorJSON(w, "Invalid project ID", http.StatusBadRequest)
		return
	}

	// Verify project ownership before listing.
	var project dbEngine.Project
	if err := dbEngine.DB.Where("id = ? AND user_id = ?", projectID, userInfo.ID).First(&project).Error; err != nil {
		helpers.WriteErrorJSON(w, "Project not found", http.StatusNotFound)
		return
	}

	var releases []dbEngine.Release
	if err := dbEngine.DB.Where("project_id = ?", projectID).Order("created_at DESC").Find(&releases).Error; err != nil {
		helpers.WriteErrorJSON(w, "Failed to fetch releases", http.StatusInternalServerError)
		return
	}

	helpers.WriteJSON(w, models.ReleasesResponse{Releases: convertDBReleases(releases)})
}

func (c *ReleaseController) findOwnedRelease(releaseID, projectID, userID uint) (dbEngine.Release, error) {
	var release dbEngine.Release
	err := dbEngine.DB.
		Joins("JOIN projects ON releases.project_id = projects.id").
		Where("releases.id = ? AND projects.id = ? AND projects.user_id = ?", releaseID, projectID, userID).
		First(&release).Error
	return release, err
}

// AccessCheckHandler godoc
//
//	@Summary		Check Google Play access
//	@Description	Verifies that the project's service account can publish the configured app
//	@Tags			releases
//	@Produce		json
//	@Param			id	path	int	true	"Project ID"
//	@Success		200	{object}	models.AccessCheckResponse
//	@Failure		401	{object}	map[string]string
//	@Router			/project/{id}/google-play/access [get]
//	@Security		BearerAuth
func (c *ReleaseController) AccessCheckHandler(w http.ResponseWriter, r *http.Request) {
	userInfo, err := c.userService.GetUserFromContext(r.Context())
	if err != nil || userInfo == nil {
		helpers.WriteErrorJSON(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(r)
	projectID, err := strconv.ParseUint(vars["id"], 10, 32)
	if err != nil {
		helpers.WriteErrorJSON(w, "Invalid project ID", http.StatusBadRequest)
		return
	}

	config, credentials, err := loadGooglePlayContext(uint(projectID), userInfo.ID)
	if err != nil {
		helpers.WriteJSON(w, models.AccessCheckResponse{Accessible: false, Reason: "not_configured", Message: err.Error()})
		return
	}

	client, err := googleplay.NewClientFromCredentials(r.Context(), credentials.Credentials)
	if err != nil {
		helpers.WriteJSON(w, models.AccessCheckResponse{Accessible: false, Reason: "client_error", Message: "Failed to build Google Play client"})
		return
	}

	if err := client.CheckAccess(r.Context(), config.PackageName); err != nil {
		reason, msg := googleplay.ReasonUnknown, err.Error()
		var pe *googleplay.PublishError
		if errors.As(err, &pe) {
			reason, msg = pe.Reason, pe.Msg
		}
		helpers.WriteJSON(w, models.AccessCheckResponse{Accessible: false, Reason: reason, Message: msg})
		return
	}

	helpers.WriteJSON(w, models.AccessCheckResponse{Accessible: true})
}

// loadGooglePlayContext loads the project config and its linked, owned Google
// Play credentials, validating that publication is configured.
func loadGooglePlayContext(projectID, userID uint) (dbEngine.ProjectConfig, dbEngine.GooglePlayCredentials, error) {
	var config dbEngine.ProjectConfig
	if err := dbEngine.DB.Where("project_id = ?", projectID).First(&config).Error; err != nil {
		return config, dbEngine.GooglePlayCredentials{}, errors.New("project configuration not found")
	}
	if config.PackageName == "" {
		return config, dbEngine.GooglePlayCredentials{}, errors.New("package_name is not configured")
	}
	if config.GooglePlayCredentialsID == nil {
		return config, dbEngine.GooglePlayCredentials{}, errors.New("no Google Play credentials linked to the project")
	}
	var credentials dbEngine.GooglePlayCredentials
	if err := dbEngine.DB.
		Where("id = ? AND user_id = ?", *config.GooglePlayCredentialsID, userID).
		First(&credentials).Error; err != nil {
		return config, credentials, errors.New("Google Play credentials not found")
	}
	return config, credentials, nil
}

// accessError verifies SA access and returns (httpStatus, message). A status of
// 0 means access is granted.
func accessError(ctx context.Context, encryptedCredentials, packageName string) (int, string) {
	client, err := googleplay.NewClientFromCredentials(ctx, encryptedCredentials)
	if err != nil {
		return http.StatusInternalServerError, "Failed to build Google Play client"
	}
	if err := client.CheckAccess(ctx, packageName); err != nil {
		var pe *googleplay.PublishError
		if errors.As(err, &pe) {
			if pe.Reason == googleplay.ReasonPermission {
				return http.StatusForbidden, pe.Msg
			}
			return http.StatusBadGateway, pe.Msg
		}
		return http.StatusBadGateway, "Google Play access check failed"
	}
	return 0, ""
}

// parsePublishRequest decodes the optional request body, tolerating an empty body.
func parsePublishRequest(r *http.Request) models.PublishRequest {
	var req models.PublishRequest
	if r.Body == nil {
		return req
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		// Ignore malformed body: fall back to config defaults.
		return models.PublishRequest{}
	}
	return req
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func convertDBRelease(r dbEngine.Release) models.ReleaseDTO {
	return models.ReleaseDTO{
		ID:              r.ID,
		CreatedAt:       r.CreatedAt,
		UpdatedAt:       r.UpdatedAt,
		ProjectID:       r.ProjectID,
		BuildID:         r.BuildID,
		VersionName:     r.VersionName,
		VersionCode:     r.VersionCode,
		Track:           r.Track,
		RolloutFraction: r.RolloutFraction,
		Status:          r.Status,
		ReleaseNotes:    r.ReleaseNotes,
	}
}

func convertDBReleases(releases []dbEngine.Release) []models.ReleaseDTO {
	out := make([]models.ReleaseDTO, len(releases))
	for i, r := range releases {
		out[i] = convertDBRelease(r)
	}
	return out
}
