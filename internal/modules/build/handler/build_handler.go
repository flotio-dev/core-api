package handler

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"gorm.io/gorm"

	dbEngine "github.com/flotio-dev/core-api/internal/common/database"
	helpers "github.com/flotio-dev/core-api/internal/common/server"
	kubernetesEngine "github.com/flotio-dev/core-api/internal/infra/kubernetes"
	s3Engine "github.com/flotio-dev/core-api/internal/infra/s3"
	buildModels "github.com/flotio-dev/core-api/internal/modules/build/model"
	githubServices "github.com/flotio-dev/core-api/internal/modules/github/service"
	userServices "github.com/flotio-dev/core-api/internal/modules/user/service"
)

// BuildController handles build-related operations
type BuildController struct {
	githubService *githubServices.GithubService
	userService   *userServices.UserService
}

const waitingBuildSchedulerInterval = 5 * time.Second

var buildSchedulingMutex sync.Mutex
var waitingBuildSchedulerOnce sync.Once

// NewBuildController creates a new BuildController
func NewBuildController(githubService *githubServices.GithubService, userService *userServices.UserService) *BuildController {
	controller := &BuildController{
		githubService: githubService,
		userService:   userService,
	}

	waitingBuildSchedulerOnce.Do(func() {
		go controller.startWaitingBuildScheduler()
	})

	return controller
}

func stringPtrValue(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func hasProjectGitCredentials(project dbEngine.Project) (string, string, bool) {
	username := stringPtrValue(project.GitUsername)
	token := stringPtrValue(project.GitToken)
	if username == "" || token == "" {
		return username, token, false
	}
	return username, token, true
}

func isGitHubHTTPSRepo(gitRepo *string) bool {
	if gitRepo == nil {
		return false
	}
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(*gitRepo)), "https://github.com")
}

func (c *BuildController) resolveGitCredentials(ctx context.Context, userID uint, project dbEngine.Project) (string, string) {
	projectUsername, projectToken, projectHasCredentials := hasProjectGitCredentials(project)

	if !isGitHubHTTPSRepo(project.GitRepo) {
		if projectHasCredentials {
			return projectUsername, projectToken
		}
		return "", ""
	}

	githubInstallationDB, err := c.githubService.GetInstallationByUser(userID)
	if err != nil {
		fmt.Printf("Build auth: failed to get GitHub installation for user %d: %v\n", userID, err)
		if projectHasCredentials {
			return projectUsername, projectToken
		}
		return "", ""
	}

	if githubInstallationDB != nil {
		installationToken, tokenErr := c.githubService.GetInstallationToken(githubInstallationDB.InstallationID)
		if tokenErr == nil && strings.TrimSpace(installationToken) != "" {
			username := "x-access-token"
			githubInstallation, installationErr := c.githubService.GetGithubInstallation(ctx, githubInstallationDB.InstallationID)
			if installationErr == nil && githubInstallation != nil && githubInstallation.Account != nil && strings.TrimSpace(githubInstallation.Account.GetLogin()) != "" {
				username = githubInstallation.Account.GetLogin()
			} else if installationErr != nil {
				fmt.Printf("Build auth: failed to get GitHub installation details %d: %v\n", githubInstallationDB.InstallationID, installationErr)
			}
			return username, installationToken
		}

		if tokenErr != nil {
			fmt.Printf("Build auth: failed to get GitHub installation token %d: %v\n", githubInstallationDB.InstallationID, tokenErr)
		}
	}

	if projectHasCredentials {
		return projectUsername, projectToken
	}

	return "", ""
}

func normalizeBuildRequestDefaults(req *buildModels.BuildRequest) {
	if req.Platform == "" {
		req.Platform = "android"
	}
	if req.BuildMode == "" {
		req.BuildMode = "release"
	}
	if req.BuildTarget == "" {
		req.BuildTarget = defaultBuildTarget(req.Platform)
	}
	if req.FlutterChannel == "" {
		req.FlutterChannel = "stable"
	}
	if req.GitBranch == "" {
		req.GitBranch = "main"
	}
}

func defaultBuildTarget(platform string) string {
	if platform == "android" {
		return "apk"
	}
	if platform == "" {
		return "apk"
	}
	return platform
}

func normalizeBuildDefaults(build *dbEngine.Build) {
	if build.Platform == "" {
		build.Platform = "android"
	}
	if build.BuildMode == "" {
		build.BuildMode = "release"
	}
	if build.BuildTarget == "" {
		build.BuildTarget = defaultBuildTarget(build.Platform)
	}
	if build.FlutterChannel == "" {
		build.FlutterChannel = "stable"
	}
	if build.GitBranch == "" {
		build.GitBranch = "main"
	}
}

func isPodBackedBuildStatus(status string) bool {
	return status == "running" || status == "pending"
}

func buildHasMore(status string) bool {
	return status == "running" || status == "pending" || status == "waiting"
}

func (c *BuildController) startBuildPod(ctx context.Context, build *dbEngine.Build, project dbEngine.Project, userID uint) error {
	normalizeBuildDefaults(build)

	gitUsername, gitToken := c.resolveGitCredentials(ctx, userID, project)
	buildConfig := kubernetesEngine.BuildConfig{
		BuildID:        build.ID,
		Project:        project,
		Platform:       build.Platform,
		BuildMode:      build.BuildMode,
		BuildTarget:    build.BuildTarget,
		FlutterChannel: build.FlutterChannel,
		GitBranch:      build.GitBranch,
		GitUsername:    gitUsername,
		GitToken:       gitToken,
	}

	if err := kubernetesEngine.CreateBuildPod(buildConfig); err != nil {
		return err
	}

	go kubernetesEngine.StartPodLogListener(build.ID)
	build.Status = "running"
	return dbEngine.DB.Save(build).Error
}

func (c *BuildController) startBuildOrQueue(ctx context.Context, build *dbEngine.Build, project dbEngine.Project, userID uint) error {
	buildSchedulingMutex.Lock()
	defer buildSchedulingMutex.Unlock()

	hasCapacity, err := kubernetesEngine.HasBuildPodCapacity()
	if err != nil {
		return fmt.Errorf("failed to check cluster build capacity: %w", err)
	}

	if !hasCapacity {
		build.Status = "waiting"
		return dbEngine.DB.Save(build).Error
	}

	return c.startBuildPod(ctx, build, project, userID)
}

func (c *BuildController) startWaitingBuildScheduler() {
	c.processWaitingBuildQueue()

	ticker := time.NewTicker(waitingBuildSchedulerInterval)
	defer ticker.Stop()

	for range ticker.C {
		c.processWaitingBuildQueue()
	}
}

func (c *BuildController) triggerWaitingBuildProcessing() {
	go c.processWaitingBuildQueue()
}

func (c *BuildController) processWaitingBuildQueue() {
	if dbEngine.DB == nil {
		return
	}

	buildSchedulingMutex.Lock()
	defer buildSchedulingMutex.Unlock()

	for {
		var waitingBuild dbEngine.Build
		if err := dbEngine.DB.Where("status = ?", "waiting").Order("created_at ASC").First(&waitingBuild).Error; err != nil {
			if err != gorm.ErrRecordNotFound {
				fmt.Printf("Build queue: failed to fetch waiting build: %v\n", err)
			}
			return
		}

		hasCapacity, err := kubernetesEngine.HasBuildPodCapacity()
		if err != nil {
			fmt.Printf("Build queue: failed to check capacity: %v\n", err)
			return
		}
		if !hasCapacity {
			return
		}

		claimResult := dbEngine.DB.Model(&dbEngine.Build{}).
			Where("id = ? AND status = ?", waitingBuild.ID, "waiting").
			Update("status", "pending")
		if claimResult.Error != nil {
			fmt.Printf("Build queue: failed to claim build %d: %v\n", waitingBuild.ID, claimResult.Error)
			return
		}
		if claimResult.RowsAffected == 0 {
			continue
		}
		waitingBuild.Status = "pending"

		var project dbEngine.Project
		if err := dbEngine.DB.First(&project, waitingBuild.ProjectID).Error; err != nil {
			fmt.Printf("Build queue: failed to fetch project %d for build %d: %v\n", waitingBuild.ProjectID, waitingBuild.ID, err)
			waitingBuild.Status = "failed"
			if waitingBuild.Duration == 0 {
				waitingBuild.Duration = int64(time.Since(waitingBuild.CreatedAt).Seconds())
			}
			dbEngine.DB.Save(&waitingBuild)
			continue
		}

		if err := c.startBuildPod(context.Background(), &waitingBuild, project, project.UserID); err != nil {
			fmt.Printf("Build queue: failed to start waiting build %d: %v\n", waitingBuild.ID, err)
			waitingBuild.Status = "failed"
			if waitingBuild.Duration == 0 {
				waitingBuild.Duration = int64(time.Since(waitingBuild.CreatedAt).Seconds())
			}
			dbEngine.DB.Save(&waitingBuild)
			continue
		}
	}
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
//	@Success		200		{object}	buildModels.BuildResponse
//	@Failure		400		{object}	map[string]string
//	@Failure		401		{object}	map[string]string
//	@Failure		404		{object}	map[string]string
//	@Failure		500		{object}	map[string]string
//	@Router			/project/{id}/build/{buildId}/cancel [put]
func (bc *BuildController) BuildCancelHandler(w http.ResponseWriter, r *http.Request) {
	userInfo, err := bc.userService.GetUserFromContext(r.Context())
	if err != nil || userInfo == nil {
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
	if err := dbEngine.DB.Joins("JOIN projects ON builds.project_id = projects.id").Where("builds.id = ? AND projects.id = ? AND projects.user_id = ?", buildID, projectID, userInfo.ID).First(&build).Error; err != nil {
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
	// Calculate duration when build is cancelled
	if build.Duration == 0 {
		build.Duration = int64(time.Since(build.CreatedAt).Seconds())
	}
	if err := dbEngine.DB.Save(&build).Error; err != nil {
		helpers.WriteErrorJSON(w, "Failed to cancel build", http.StatusInternalServerError)
		return
	}
	bc.triggerWaitingBuildProcessing()

	helpers.WriteJSON(w, buildModels.BuildResponse{Build: convertDBBuild(build)})
}

// BuildDeleteHandler godoc
//
//	@Summary		Delete a build
//	@Description	Delete a specific build for a project, including its S3 artifacts and Kubernetes pod
//	@Tags			builds
//	@Accept			json
//	@Produce		json
//	@Param			id		path	int					true	"Project ID"
//	@Param			buildId	path	int					true	"Build ID"
//	@Success		200		{object}	buildModels.DeleteResponse
//	@Failure		400		{object}	map[string]string
//	@Failure		401		{object}	map[string]string
//	@Failure		404		{object}	map[string]string
//	@Failure		500		{object}	map[string]string
//	@Router			/project/{id}/build/{buildId} [delete]
func (bc *BuildController) BuildDeleteHandler(w http.ResponseWriter, r *http.Request) {
	userInfo, err := bc.userService.GetUserFromContext(r.Context())
	if err != nil || userInfo == nil {
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
	if err := dbEngine.DB.Joins("JOIN projects ON builds.project_id = projects.id").Where("builds.id = ? AND projects.id = ? AND projects.user_id = ?", buildID, projectID, userInfo.ID).First(&build).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			helpers.WriteErrorJSON(w, "Build not found", http.StatusNotFound)
			return
		}
		helpers.WriteErrorJSON(w, "Failed to fetch build", http.StatusInternalServerError)
		return
	}
	previousStatus := build.Status

	// Delete the Kubernetes pod (if still running)
	if build.Status == "running" || build.Status == "pending" {
		if err := kubernetesEngine.DeleteBuildPod(build.ID); err != nil {
			fmt.Printf("Failed to delete build pod for build %d: %v\n", build.ID, err)
			// Continue with deletion even if pod deletion fails
		}
	}

	// Delete S3 artifacts if APKURL is set
	if build.APKURL != "" {
		if err := s3Engine.DeleteBuildArtifacts(build.ID); err != nil {
			fmt.Printf("Failed to delete S3 artifacts for build %d: %v\n", build.ID, err)
			// Continue with deletion even if S3 deletion fails
		}
	}

	// Delete associated logs
	if err := dbEngine.DB.Where("build_id = ?", build.ID).Delete(&dbEngine.Log{}).Error; err != nil {
		fmt.Printf("Failed to delete logs for build %d: %v\n", build.ID, err)
		// Continue with deletion even if log deletion fails
	}

	// Delete the build record
	if err := dbEngine.DB.Delete(&build).Error; err != nil {
		helpers.WriteErrorJSON(w, "Failed to delete build", http.StatusInternalServerError)
		return
	}
	if isPodBackedBuildStatus(previousStatus) {
		bc.triggerWaitingBuildProcessing()
	}

	helpers.WriteJSON(w, buildModels.DeleteResponse{Status: "deleted"})
}

// BuildsListHandler godoc
//
//	@Summary		List builds
//	@Description	Get all builds for a specific project
//	@Tags			builds
//	@Accept			json
//	@Produce		json
//	@Param			id	path	int					true	"Project ID"
//	@Success		200	{object}	buildModels.BuildsResponse
//	@Failure		400	{object}	map[string]string
//	@Failure		401	{object}	map[string]string
//	@Failure		500	{object}	map[string]string
//	@Router			/project/{id}/builds [get]
func (bc *BuildController) BuildsListHandler(w http.ResponseWriter, r *http.Request) {
	userInfo, err := bc.userService.GetUserFromContext(r.Context())
	if err != nil || userInfo == nil {
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
	if err := dbEngine.DB.Joins("JOIN projects ON builds.project_id = projects.id").Where("projects.id = ? AND projects.user_id = ?", projectID, userInfo.ID).Order("builds.created_at DESC").Find(&builds).Error; err != nil {
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

		if isPodBackedBuildStatus(builds[i].Status) {
			podStatus, err := kubernetesEngine.GetPodStatus(builds[i].ID)
			if err != nil {
				// Pod might not exist anymore, mark as failed
				builds[i].Status = "failed"
				// Calculate duration when build fails
				if builds[i].Duration == 0 {
					builds[i].Duration = int64(time.Since(builds[i].CreatedAt).Seconds())
				}
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
				// Calculate duration when build finishes
				if (newStatus == "success" || newStatus == "failed") && builds[i].Duration == 0 {
					builds[i].Duration = int64(time.Since(builds[i].CreatedAt).Seconds())
				}
				if newStatus == "success" || newStatus == "failed" {
					kubernetesEngine.ScheduleBuildPodCleanup(builds[i].ID)
				}
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

	helpers.WriteJSON(w, buildModels.BuildsResponse{Builds: convertDBBuilds(builds)})
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
//	@Success		200		{object}	buildModels.LogsResponse
//	@Failure		400		{object}	map[string]string
//	@Failure		401		{object}	map[string]string
//	@Failure		404		{object}	map[string]string
//	@Failure		500		{object}	map[string]string
//	@Router			/project/{id}/build/{buildId}/logs [get]
func (bc *BuildController) BuildLogsHandler(w http.ResponseWriter, r *http.Request) {
	userInfo, err := bc.userService.GetUserFromContext(r.Context())
	if err != nil || userInfo == nil {
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
	if err := dbEngine.DB.Joins("JOIN projects ON builds.project_id = projects.id").Where("builds.id = ? AND projects.id = ? AND projects.user_id = ?", buildID, projectID, userInfo.ID).First(&build).Error; err != nil {
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

	helpers.WriteJSON(w, buildModels.LogsResponse{Logs: logs})
}

// pollingConnections stores active polling connections with their last seen line number
var pollingConnections = sync.Map{}

// PollingState tracks the state of a polling connection
type PollingState struct {
	LastLineNumber int
	LastAccess     time.Time
}

// BuildLogsSyncResponse represents the response for the sync endpoint
type BuildLogsSyncResponse struct {
	Logs        []string `json:"logs"`
	LastLine    int      `json:"last_line"`
	Status      string   `json:"status"`
	PodStatus   string   `json:"pod_status,omitempty"`
	HasMore     bool     `json:"has_more"`
	ElapsedTime int64    `json:"elapsed_time"` // Time since build started in seconds
}

// BuildLogsSyncHandler godoc
//
//	@Summary		Get build logs via HTTP polling
//	@Description	Fetch new logs for a specific build using HTTP long polling (10s timeout)
//	@Tags			builds
//	@Accept			json
//	@Produce		json
//	@Param			id			path	int		true	"Project ID"
//	@Param			buildId		path	int		true	"Build ID"
//	@Param			connectionId	query	string	true	"Connection ID (generated by client)"
//	@Param			lastLine	query	int		false	"Last line number received"
//	@Success		200	{object}	BuildLogsSyncResponse
//	@Failure		400	{object}	map[string]string
//	@Failure		401	{object}	map[string]string
//	@Failure		404	{object}	map[string]string
//	@Router			/project/{id}/build/{buildId}/logs/sync [get]
func (bc *BuildController) BuildLogsSyncHandler(w http.ResponseWriter, r *http.Request) {
	userInfo, err := bc.userService.GetUserFromContext(r.Context())
	if err != nil || userInfo == nil {
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

	// Get connection ID from query params
	connectionID := r.URL.Query().Get("connectionId")
	if connectionID == "" {
		helpers.WriteErrorJSON(w, "Missing connectionId", http.StatusBadRequest)
		return
	}

	// Get last line number from query params (default to 0)
	lastLine := 0
	if lastLineStr := r.URL.Query().Get("lastLine"); lastLineStr != "" {
		lastLine, _ = strconv.Atoi(lastLineStr)
	}

	// Verify the build belongs to the user's project
	var build dbEngine.Build
	if err := dbEngine.DB.Joins("JOIN projects ON builds.project_id = projects.id").Where("builds.id = ? AND projects.id = ? AND projects.user_id = ?", buildID, projectID, userInfo.ID).First(&build).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			helpers.WriteErrorJSON(w, "Build not found", http.StatusNotFound)
			return
		}
		helpers.WriteErrorJSON(w, "Failed to fetch build", http.StatusInternalServerError)
		return
	}

	// Update polling connection state
	connKey := fmt.Sprintf("%d-%s", buildID, connectionID)
	pollingConnections.Store(connKey, PollingState{
		LastLineNumber: lastLine,
		LastAccess:     time.Now(),
	})

	// Long polling: wait up to 10 seconds for new logs
	timeout := time.After(10 * time.Second)
	ticker := time.NewTicker(500 * time.Millisecond) // Check every 500ms
	defer ticker.Stop()

	var newLogs []dbEngine.Log
	var podStatus string

	for {
		select {
		case <-timeout:
			// Timeout reached, return current status and any new logs
			// Also get pod status and update build status in DB
			if isPodBackedBuildStatus(build.Status) {
				podStatus, err = kubernetesEngine.GetPodStatus(uint(buildID))
				if err == nil {
					// Map pod status to build status
					var newBuildStatus string
					switch podStatus {
					case "Succeeded":
						newBuildStatus = "success"
					case "Failed":
						newBuildStatus = "failed"
					case "Running":
						newBuildStatus = "running"
					case "Pending":
						newBuildStatus = "pending"
					}

					// Update build status in database if changed
					if newBuildStatus != "" && newBuildStatus != build.Status {
						build.Status = newBuildStatus
						// Calculate duration when build finishes
						if newBuildStatus == "success" || newBuildStatus == "failed" {
							build.Duration = int64(time.Since(build.CreatedAt).Seconds())
							kubernetesEngine.ScheduleBuildPodCleanup(build.ID)
						}
						dbEngine.DB.Save(&build)
					}
				}
			}

			// Get any remaining new logs
			dbEngine.DB.Where("build_id = ? AND line_number > ?", buildID, lastLine).Order("line_number ASC").Find(&newLogs)

			logs := make([]string, len(newLogs))
			newLastLine := lastLine
			for i, log := range newLogs {
				logs[i] = log.Content
				if log.LineNumber > newLastLine {
					newLastLine = log.LineNumber
				}
			}

			helpers.WriteJSON(w, BuildLogsSyncResponse{
				Logs:        logs,
				LastLine:    newLastLine,
				Status:      build.Status,
				PodStatus:   podStatus,
				HasMore:     buildHasMore(build.Status),
				ElapsedTime: int64(time.Since(build.CreatedAt).Seconds()),
			})
			return

		case <-ticker.C:
			// Check for new logs
			dbEngine.DB.Where("build_id = ? AND line_number > ?", buildID, lastLine).Order("line_number ASC").Find(&newLogs)

			if len(newLogs) > 0 {
				// Return immediately if we have new logs
				logs := make([]string, len(newLogs))
				newLastLine := lastLine
				for i, log := range newLogs {
					logs[i] = log.Content
					if log.LineNumber > newLastLine {
						newLastLine = log.LineNumber
					}
				}

				// Refresh build status
				dbEngine.DB.First(&build, buildID)

				helpers.WriteJSON(w, BuildLogsSyncResponse{
					Logs:        logs,
					LastLine:    newLastLine,
					Status:      build.Status,
					HasMore:     buildHasMore(build.Status),
					ElapsedTime: int64(time.Since(build.CreatedAt).Seconds()),
				})
				return
			}

			// Check if build is still running
			dbEngine.DB.First(&build, buildID)
			if !buildHasMore(build.Status) {
				// Build finished, return final status
				helpers.WriteJSON(w, BuildLogsSyncResponse{
					Logs:        []string{},
					LastLine:    lastLine,
					Status:      build.Status,
					HasMore:     false,
					ElapsedTime: int64(time.Since(build.CreatedAt).Seconds()),
				})
				return
			}
		}
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
func (bc *BuildController) BuildDownloadHandler(w http.ResponseWriter, r *http.Request) {
	userInfo, err := bc.userService.GetUserFromContext(r.Context())
	if err != nil || userInfo == nil {
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

// ProjectBuildHandler godoc
//
//	@Summary		Build a project
//	@Description	Start a build for a specific project
//	@Tags			builds
//	@Accept			json
//	@Produce		json
//	@Param			id		path	int				true	"Project ID"
//	@Param			build	body	buildModels.BuildRequest	true	"Build data"
//	@Success		200		{object}	buildModels.BuildResponse
//	@Failure		400		{object}	map[string]string
//	@Failure		401		{object}	map[string]string
//	@Failure		404		{object}	map[string]string
//	@Failure		500		{object}	map[string]string
//	@Router			/project/{id}/build [post]
func (c *BuildController) ProjectBuildHandler(w http.ResponseWriter, r *http.Request) {
	userInfo, err := c.userService.GetUserFromContext(r.Context())
	if err != nil || userInfo == nil {
		helpers.WriteErrorJSON(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(r)
	projectID, err := strconv.Atoi(vars["id"])
	if err != nil {
		helpers.WriteErrorJSON(w, "Invalid project ID", http.StatusBadRequest)
		return
	}

	var req buildModels.BuildRequest
	if err := helpers.ReadJSON(r, &req); err != nil {
		// If no body, use defaults
		req.Platform = "android"
		req.BuildMode = "release"
		req.BuildTarget = "apk"
		req.FlutterChannel = "stable"
		req.GitBranch = "main"
	}
	normalizeBuildRequestDefaults(&req)

	var project dbEngine.Project
	if err := dbEngine.DB.Where("id = ? AND user_id = ?", projectID, userInfo.ID).First(&project).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			helpers.WriteErrorJSON(w, "Project not found", http.StatusNotFound)
			return
		}
		helpers.WriteErrorJSON(w, "Failed to fetch project", http.StatusInternalServerError)
		return
	}

	build := dbEngine.Build{
		ProjectID:      project.ID,
		Status:         "pending",
		Platform:       req.Platform,
		BuildMode:      req.BuildMode,
		BuildTarget:    req.BuildTarget,
		FlutterChannel: req.FlutterChannel,
		GitBranch:      req.GitBranch,
	}

	if err := dbEngine.DB.Create(&build).Error; err != nil {
		helpers.WriteErrorJSON(w, "Failed to create build", http.StatusInternalServerError)
		return
	}

	if err := c.startBuildOrQueue(r.Context(), &build, project, userInfo.ID); err != nil {
		// If build start fails, update status to failed
		fmt.Printf("Failed to start build %d: %v\n", build.ID, err)
		build.Status = "failed"
		if build.Duration == 0 {
			build.Duration = int64(time.Since(build.CreatedAt).Seconds())
		}
		dbEngine.DB.Save(&build)
		helpers.WriteErrorJSON(w, "Failed to start build process", http.StatusInternalServerError)
		return
	}

	helpers.WriteJSON(w, buildModels.BuildResponse{Build: convertDBBuild(build)})
}
