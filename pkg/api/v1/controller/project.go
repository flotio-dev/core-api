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

// Projects
func ProjectsGetHandler(w http.ResponseWriter, r *http.Request) {
	userInfo := middleware.GetUserFromContext(r.Context())
	if userInfo == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var user db.User
	if err := db.DB.Where("keycloak_id = ?", *userInfo.Sub).First(&user).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			http.Error(w, "User not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Failed to fetch user", http.StatusInternalServerError)
		return
	}

	var projects []db.Project
	if err := db.DB.Where("user_id = ?", user.ID).Find(&projects).Error; err != nil {
		http.Error(w, "Failed to fetch projects", http.StatusInternalServerError)
		return
	}

	utils.WriteJSON(w, map[string]interface{}{"projects": projects})
}
func ProjectCreateHandler(w http.ResponseWriter, r *http.Request) {
	userInfo := middleware.GetUserFromContext(r.Context())
	if userInfo == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var user db.User
	if err := db.DB.Where("keycloak_id = ?", *userInfo.Sub).First(&user).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			http.Error(w, "User not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Failed to fetch user", http.StatusInternalServerError)
		return
	}

	var req struct {
		Name           string `json:"name"`
		GitRepo        string `json:"git_repo"`
		BuildFolder    string `json:"build_folder,omitempty"`
		FlutterVersion string `json:"flutter_version,omitempty"`
	}
	if err := utils.ReadJSON(r, &req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	project := db.Project{
		Name:           req.Name,
		GitRepo:        req.GitRepo,
		BuildFolder:    req.BuildFolder,
		FlutterVersion: req.FlutterVersion,
		UserID:         user.ID,
	}

	if err := db.DB.Create(&project).Error; err != nil {
		http.Error(w, "Failed to create project", http.StatusInternalServerError)
		return
	}

	utils.WriteJSON(w, map[string]interface{}{"project": project})
}
func ProjectGetHandler(w http.ResponseWriter, r *http.Request) {
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

	var project db.Project
	if err := db.DB.Preload("Builds").Preload("Envs").Where("id = ? AND user_id = (SELECT id FROM users WHERE keycloak_id = ?)", projectID, *userInfo.Sub).First(&project).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			http.Error(w, "Project not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Failed to fetch project", http.StatusInternalServerError)
		return
	}

	utils.WriteJSON(w, map[string]interface{}{"project": project})
}
func ProjectPutHandler(w http.ResponseWriter, r *http.Request) {
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
		Name           string `json:"name,omitempty"`
		GitRepo        string `json:"git_repo,omitempty"`
		BuildFolder    string `json:"build_folder,omitempty"`
		FlutterVersion string `json:"flutter_version,omitempty"`
	}
	if err := utils.ReadJSON(r, &req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	var project db.Project
	if err := db.DB.Where("id = ? AND user_id = (SELECT id FROM users WHERE keycloak_id = ?)", projectID, *userInfo.Sub).First(&project).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			http.Error(w, "Project not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Failed to fetch project", http.StatusInternalServerError)
		return
	}

	if req.Name != "" {
		project.Name = req.Name
	}
	if req.GitRepo != "" {
		project.GitRepo = req.GitRepo
	}
	if req.BuildFolder != "" {
		project.BuildFolder = req.BuildFolder
	}
	if req.FlutterVersion != "" {
		project.FlutterVersion = req.FlutterVersion
	}

	if err := db.DB.Save(&project).Error; err != nil {
		http.Error(w, "Failed to update project", http.StatusInternalServerError)
		return
	}

	utils.WriteJSON(w, map[string]interface{}{"project": project})
}
func ProjectDeleteHandler(w http.ResponseWriter, r *http.Request) {
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

	if err := db.DB.Where("id = ? AND user_id = (SELECT id FROM users WHERE keycloak_id = ?)", projectID, *userInfo.Sub).Delete(&db.Project{}).Error; err != nil {
		http.Error(w, "Failed to delete project", http.StatusInternalServerError)
		return
	}

	utils.WriteJSON(w, map[string]string{"status": "deleted"})
}
func ProjectBuildHandler(w http.ResponseWriter, r *http.Request) {
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
		Platform string `json:"platform,omitempty"` // e.g., android, ios
	}
	if err := utils.ReadJSON(r, &req); err != nil {
		// If no body, use default
		req.Platform = "android"
	}

	var project db.Project
	if err := db.DB.Where("id = ? AND user_id = (SELECT id FROM users WHERE keycloak_id = ?)", projectID, *userInfo.Sub).First(&project).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			http.Error(w, "Project not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Failed to fetch project", http.StatusInternalServerError)
		return
	}

	build := db.Build{
		ProjectID: project.ID,
		Status:    "pending",
		Platform:  req.Platform,
	}

	if err := db.DB.Create(&build).Error; err != nil {
		http.Error(w, "Failed to create build", http.StatusInternalServerError)
		return
	}

	// Start the build process by creating a Kubernetes pod
	if err := kubernetes.CreateBuildPod(build.ID, project, req.Platform); err != nil {
		// If pod creation fails, update build status to failed
		build.Status = "failed"
		db.DB.Save(&build)
		http.Error(w, "Failed to start build process", http.StatusInternalServerError)
		return
	}

	// Update build status to running
	build.Status = "running"
	db.DB.Save(&build)

	utils.WriteJSON(w, map[string]interface{}{"build": build})
}

func BuildCancelHandler(w http.ResponseWriter, r *http.Request) {
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
	buildID, err := strconv.Atoi(vars["buildId"])
	if err != nil {
		http.Error(w, "Invalid build ID", http.StatusBadRequest)
		return
	}

	var build db.Build
	if err := db.DB.Joins("JOIN projects ON builds.project_id = projects.id").Where("builds.id = ? AND projects.id = ? AND projects.user_id = (SELECT id FROM users WHERE keycloak_id = ?)", buildID, projectID, *userInfo.Sub).First(&build).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			http.Error(w, "Build not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Failed to fetch build", http.StatusInternalServerError)
		return
	}

	build.Status = "cancelled"
	if err := db.DB.Save(&build).Error; err != nil {
		http.Error(w, "Failed to cancel build", http.StatusInternalServerError)
		return
	}

	utils.WriteJSON(w, map[string]interface{}{"build": build})
}

func BuildsListHandler(w http.ResponseWriter, r *http.Request) {
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

	var builds []db.Build
	if err := db.DB.Joins("JOIN projects ON builds.project_id = projects.id").Where("projects.id = ? AND projects.user_id = (SELECT id FROM users WHERE keycloak_id = ?)", projectID, *userInfo.Sub).Find(&builds).Error; err != nil {
		http.Error(w, "Failed to fetch builds", http.StatusInternalServerError)
		return
	}

	utils.WriteJSON(w, map[string]interface{}{"builds": builds})
}

func BuildLogsHandler(w http.ResponseWriter, r *http.Request) {
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
	buildID, err := strconv.Atoi(vars["buildId"])
	if err != nil {
		http.Error(w, "Invalid build ID", http.StatusBadRequest)
		return
	}

	// Verify the build belongs to the user's project
	var build db.Build
	if err := db.DB.Joins("JOIN projects ON builds.project_id = projects.id").Where("builds.id = ? AND projects.id = ? AND projects.user_id = (SELECT id FROM users WHERE keycloak_id = ?)", buildID, projectID, *userInfo.Sub).First(&build).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			http.Error(w, "Build not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Failed to fetch build", http.StatusInternalServerError)
		return
	}

	// Get logs from the Kubernetes pod
	logs, err := kubernetes.GetPodLogs(uint(buildID))
	if err != nil {
		http.Error(w, "Failed to fetch logs", http.StatusInternalServerError)
		return
	}

	utils.WriteJSON(w, map[string]interface{}{"logs": logs})
}

func BuildLogsWSHandler(w http.ResponseWriter, r *http.Request) {
	// Auth via query param
	token := r.URL.Query().Get("token")
	if token == "" {
		http.Error(w, "Missing token", http.StatusUnauthorized)
		return
	}

	// Validate token (simplified, in real app use proper validation)
	client := utils.GetKeycloakClient()
	ctx := context.Background()
	realm := os.Getenv("KEYCLOAK_REALM")
	_, err := client.GetUserInfo(ctx, token, realm)
	if err != nil {
		http.Error(w, "Invalid token", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(r)
	buildID, err := strconv.Atoi(vars["buildId"])
	if err != nil {
		http.Error(w, "Invalid build ID", http.StatusBadRequest)
		return
	}

	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true }, // Allow all origins for demo
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		http.Error(w, "Failed to upgrade", http.StatusInternalServerError)
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

func BuildDownloadHandler(w http.ResponseWriter, r *http.Request) {
	if middleware.GetUserFromContext(r.Context()) == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	vars := mux.Vars(r)
	// Simulate file download
	filename := "app-" + vars["buildId"] + ".apk"
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", "attachment; filename="+filename)
	w.Write([]byte("fake apk content"))
}
