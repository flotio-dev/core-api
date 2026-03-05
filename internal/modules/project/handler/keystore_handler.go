package handler

import (
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"gorm.io/gorm"

	appCrypto "github.com/flotio-dev/core-api/internal/common/crypto"
	dbEngine "github.com/flotio-dev/core-api/internal/common/database"
	helpers "github.com/flotio-dev/core-api/internal/common/server"
	s3Client "github.com/flotio-dev/core-api/internal/infra/s3"
	userServices "github.com/flotio-dev/core-api/internal/modules/user/service"
)

const maxKeystoreSize = 5 << 20 // 5 MB

type KeystoreController struct {
	UserService *userServices.UserService
}

func NewKeystoreController(userService *userServices.UserService) *KeystoreController {
	return &KeystoreController{UserService: userService}
}

// AndroidSigningConfigResponse is the safe public representation (no encrypted fields exposed).
type AndroidSigningConfigResponse struct {
	ID           uint   `json:"id"`
	ProjectID    uint   `json:"project_id"`
	BuildType    string `json:"build_type"`
	KeystorePath string `json:"keystore_path"`
	KeyAlias     string `json:"key_alias"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
}

func toKeystoreResponse(cfg dbEngine.AndroidSigningConfig) AndroidSigningConfigResponse {
	return AndroidSigningConfigResponse{
		ID:           cfg.ID,
		ProjectID:    cfg.ProjectID,
		BuildType:    cfg.BuildType,
		KeystorePath: cfg.KeystorePath,
		KeyAlias:     cfg.KeyAlias,
		CreatedAt:    cfg.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:    cfg.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

// UploadKeystoreHandler godoc
//
//	@Summary		Upload Android keystore
//	@Description	Upload an Android keystore file (.jks or .keystore) for a project.
//	                A project can have one keystore per build_type ("debug" or "release").
//	                If one already exists for the given build_type it will be replaced.
//	@Tags			android
//	@Accept			multipart/form-data
//	@Produce		json
//	@Param			id					path		int		true	"Project ID"
//	@Param			keystore_file		formData	file	true	"Keystore file (.jks or .keystore, max 5MB)"
//	@Param			keystore_password	formData	string	true	"Keystore password"
//	@Param			key_alias			formData	string	true	"Key alias"
//	@Param			key_password		formData	string	true	"Key password"
//	@Param			build_type			formData	string	false	"Build type: debug or release (default: release)"
//	@Success		200					{object}	map[string]string
//	@Failure		400					{object}	map[string]string
//	@Failure		401					{object}	map[string]string
//	@Failure		404					{object}	map[string]string
//	@Failure		500					{object}	map[string]string
//	@Router			/project/{id}/android/keystore [post]
//	@Security		BearerAuth
func (c *KeystoreController) UploadKeystoreHandler(w http.ResponseWriter, r *http.Request) {
	userInfo, err := c.UserService.GetUserFromContext(r.Context())
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

	// Parse multipart form (limit to maxKeystoreSize + some overhead for other fields)
	if err := r.ParseMultipartForm(maxKeystoreSize + (1 << 20)); err != nil {
		helpers.WriteErrorJSON(w, "Failed to parse form: file too large or invalid", http.StatusBadRequest)
		return
	}

	// Read the keystore file
	file, header, err := r.FormFile("keystore_file")
	if err != nil {
		helpers.WriteErrorJSON(w, "keystore_file is required", http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Validate extension
	ext := strings.ToLower(filepath.Ext(header.Filename))
	if ext != ".jks" && ext != ".keystore" {
		helpers.WriteErrorJSON(w, "keystore_file must have a .jks or .keystore extension", http.StatusBadRequest)
		return
	}

	// Validate file size
	if header.Size > maxKeystoreSize {
		helpers.WriteErrorJSON(w, "keystore_file must be smaller than 5MB", http.StatusBadRequest)
		return
	}

	// Read required form fields
	keystorePassword := strings.TrimSpace(r.FormValue("keystore_password"))
	keyAlias := strings.TrimSpace(r.FormValue("key_alias"))
	keyPassword := strings.TrimSpace(r.FormValue("key_password"))

	if keystorePassword == "" {
		helpers.WriteErrorJSON(w, "keystore_password is required", http.StatusBadRequest)
		return
	}
	if keyAlias == "" {
		helpers.WriteErrorJSON(w, "key_alias is required", http.StatusBadRequest)
		return
	}
	if keyPassword == "" {
		helpers.WriteErrorJSON(w, "key_password is required", http.StatusBadRequest)
		return
	}

	buildType := strings.ToLower(strings.TrimSpace(r.FormValue("build_type")))
	if buildType == "" {
		buildType = "release"
	}
	if buildType != "debug" && buildType != "release" {
		helpers.WriteErrorJSON(w, "build_type must be 'debug' or 'release'", http.StatusBadRequest)
		return
	}

	// Read file bytes
	fileBytes := make([]byte, header.Size)
	if _, err := file.Read(fileBytes); err != nil {
		helpers.WriteErrorJSON(w, "Failed to read keystore file", http.StatusInternalServerError)
		return
	}

	// Encrypt passwords
	encryptedKeystorePassword, err := appCrypto.Encrypt(keystorePassword)
	if err != nil {
		helpers.WriteErrorJSON(w, "Failed to encrypt keystore password", http.StatusInternalServerError)
		return
	}
	encryptedKeyPassword, err := appCrypto.Encrypt(keyPassword)
	if err != nil {
		helpers.WriteErrorJSON(w, "Failed to encrypt key password", http.StatusInternalServerError)
		return
	}

	// Check for existing config with the same build_type to handle upsert
	var existing dbEngine.AndroidSigningConfig
	existingFound := false
	if err := dbEngine.DB.Where("project_id = ? AND build_type = ?", project.ID, buildType).First(&existing).Error; err == nil {
		existingFound = true
	}

	// Generate a unique ID for the keystore S3 object
	keystoreID := uuid.New().String()

	// Upload keystore to S3
	s3Key, err := s3Client.UploadKeystore(project.ID, keystoreID, fileBytes)
	if err != nil {
		helpers.WriteErrorJSON(w, "Failed to upload keystore to storage", http.StatusInternalServerError)
		return
	}

	if existingFound {
		// Delete old keystore from S3 (best-effort, don't fail on error)
		_ = s3Client.DeleteObject(existing.KeystorePath)

		// Update existing record
		existing.KeystorePath = s3Key
		existing.KeyAlias = keyAlias
		existing.KeystorePasswordEncrypted = encryptedKeystorePassword
		existing.KeyPasswordEncrypted = encryptedKeyPassword
		if err := dbEngine.DB.Save(&existing).Error; err != nil {
			helpers.WriteErrorJSON(w, "Failed to update signing config", http.StatusInternalServerError)
			return
		}
	} else {
		// Create new record
		config := dbEngine.AndroidSigningConfig{
			ProjectID:                 project.ID,
			BuildType:                 buildType,
			KeystorePath:              s3Key,
			KeyAlias:                  keyAlias,
			KeystorePasswordEncrypted: encryptedKeystorePassword,
			KeyPasswordEncrypted:      encryptedKeyPassword,
		}
		if err := dbEngine.DB.Create(&config).Error; err != nil {
			helpers.WriteErrorJSON(w, "Failed to save signing config", http.StatusInternalServerError)
			return
		}
	}

	helpers.WriteJSON(w, map[string]string{"message": "Android keystore uploaded successfully"})
}

// GetKeystoresHandler godoc
//
//	@Summary		List Android signing configs
//	@Description	List all Android signing configurations for a project
//	@Tags			android
//	@Produce		json
//	@Param			id	path		int	true	"Project ID"
//	@Success		200	{object}	map[string]interface{}
//	@Failure		401	{object}	map[string]string
//	@Failure		404	{object}	map[string]string
//	@Failure		500	{object}	map[string]string
//	@Router			/project/{id}/android/keystores [get]
//	@Security		BearerAuth
func (c *KeystoreController) GetKeystoresHandler(w http.ResponseWriter, r *http.Request) {
	userInfo, err := c.UserService.GetUserFromContext(r.Context())
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

	var configs []dbEngine.AndroidSigningConfig
	if err := dbEngine.DB.Where("project_id = ?", project.ID).Find(&configs).Error; err != nil {
		helpers.WriteErrorJSON(w, "Failed to fetch signing configs", http.StatusInternalServerError)
		return
	}

	result := make([]AndroidSigningConfigResponse, len(configs))
	for i, cfg := range configs {
		result[i] = toKeystoreResponse(cfg)
	}

	helpers.WriteJSON(w, map[string]interface{}{"keystores": result})
}

// DeleteKeystoreHandler godoc
//
//	@Summary		Delete an Android signing config
//	@Description	Delete an Android signing configuration and its associated keystore file from storage
//	@Tags			android
//	@Produce		json
//	@Param			id			path		int		true	"Project ID"
//	@Param			buildType	path		string	true	"Build type (debug or release)"
//	@Success		200			{object}	map[string]string
//	@Failure		401			{object}	map[string]string
//	@Failure		404			{object}	map[string]string
//	@Failure		500			{object}	map[string]string
//	@Router			/project/{id}/android/keystore/{buildType} [delete]
//	@Security		BearerAuth
func (c *KeystoreController) DeleteKeystoreHandler(w http.ResponseWriter, r *http.Request) {
	userInfo, err := c.UserService.GetUserFromContext(r.Context())
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

	buildType := strings.ToLower(vars["buildType"])
	if buildType != "debug" && buildType != "release" {
		helpers.WriteErrorJSON(w, "buildType must be 'debug' or 'release'", http.StatusBadRequest)
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

	var config dbEngine.AndroidSigningConfig
	if err := dbEngine.DB.Where("project_id = ? AND build_type = ?", project.ID, buildType).First(&config).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			helpers.WriteErrorJSON(w, "Signing config not found", http.StatusNotFound)
			return
		}
		helpers.WriteErrorJSON(w, "Failed to fetch signing config", http.StatusInternalServerError)
		return
	}

	// Delete from S3 (best-effort)
	_ = s3Client.DeleteObject(config.KeystorePath)

	if err := dbEngine.DB.Delete(&config).Error; err != nil {
		helpers.WriteErrorJSON(w, "Failed to delete signing config", http.StatusInternalServerError)
		return
	}

	helpers.WriteJSON(w, map[string]string{"message": "Android signing config deleted successfully"})
}
