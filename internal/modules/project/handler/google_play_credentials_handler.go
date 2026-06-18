package handler

import (
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
	"gorm.io/gorm"

	"github.com/flotio-dev/core-api/internal/common/crypto"
	dbEngine "github.com/flotio-dev/core-api/internal/common/database"
	helpers "github.com/flotio-dev/core-api/internal/common/server"
	googleplay "github.com/flotio-dev/core-api/internal/infra/googleplay"
	services "github.com/flotio-dev/core-api/internal/modules/user/service"
)

type GooglePlayCredentialsController struct {
	UserService *services.UserService
}

func NewGooglePlayCredentialsController(userService *services.UserService) *GooglePlayCredentialsController {
	return &GooglePlayCredentialsController{
		UserService: userService,
	}
}

// GooglePlayCredentialsGetHandler godoc
//
//	@Summary		Get user Google Play credentials
//	@Description	Get all Google Play distribution credentials owned by the authenticated user
//	@Tags			google-play-credentials
//	@Accept			json
//	@Produce		json
//	@Success		200	{object}	map[string]interface{}
//	@Failure		401	{object}	map[string]string
//	@Failure		500	{object}	map[string]string
//	@Router			/google-play-credentials [get]
//	@Security		BearerAuth
func (c *GooglePlayCredentialsController) GooglePlayCredentialsGetHandler(w http.ResponseWriter, r *http.Request) {
	userInfo, err := c.UserService.GetUserFromContext(r.Context())
	if err != nil {
		helpers.WriteErrorJSON(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var credentials []dbEngine.GooglePlayCredentials
	if err := dbEngine.DB.Where("user_id = ?", userInfo.ID).Find(&credentials).Error; err != nil {
		helpers.WriteErrorJSON(w, "Failed to fetch credentials", http.StatusInternalServerError)
		return
	}

	helpers.WriteJSON(w, map[string]interface{}{"google_play_credentials": credentials})
}

// GooglePlayCredentialsPostHandler godoc
//
//	@Summary		Create Google Play credentials
//	@Description	Upload new Google Play distribution credentials for the authenticated user
//	@Tags			google-play-credentials
//	@Accept			json
//	@Produce		json
//	@Param			credentials	body	dbEngine.GooglePlayCredentials	true	"Google Play credentials data"
//	@Success		201			{object}	map[string]interface{}
//	@Failure		401			{object}	map[string]string
//	@Failure		500			{object}	map[string]string
//	@Router			/google-play-credentials [post]
//	@Security		BearerAuth
func (c *GooglePlayCredentialsController) GooglePlayCredentialsPostHandler(w http.ResponseWriter, r *http.Request) {
	userInfo, err := c.UserService.GetUserFromContext(r.Context())
	if err != nil {
		helpers.WriteErrorJSON(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req struct {
		Name        string `json:"name"`
		Credentials string `json:"credentials"` // Base64 encoded JSON service account key
	}
	if err := helpers.ReadJSON(r, &req); err != nil {
		helpers.WriteErrorJSON(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate the uploaded service account JSON before storing it.
	if err := googleplay.ValidateServiceAccountJSON(googleplay.DecodeServiceAccount(req.Credentials)); err != nil {
		helpers.WriteErrorJSON(w, "Invalid service account JSON", http.StatusBadRequest)
		return
	}

	// Encrypt the service account JSON at rest before storing.
	encCredentials, err := crypto.Encrypt(req.Credentials)
	if err != nil {
		helpers.WriteErrorJSON(w, "Failed to encrypt credentials", http.StatusInternalServerError)
		return
	}

	cred := dbEngine.GooglePlayCredentials{
		UserID:      userInfo.ID,
		Name:        req.Name,
		Credentials: encCredentials,
	}

	if err := dbEngine.DB.Create(&cred).Error; err != nil {
		helpers.WriteErrorJSON(w, "Failed to create credentials", http.StatusInternalServerError)
		return
	}

	helpers.WriteJSON(w, map[string]interface{}{"google_play_credentials": cred})
}

// GooglePlayCredentialsDeleteHandler godoc
//
//	@Summary		Delete Google Play credentials
//	@Description	Delete a specific Google Play credentials entry by ID
//	@Tags			google-play-credentials
//	@Accept			json
//	@Produce		json
//	@Param			credentialsId	path	int	true	"Credentials ID"
//	@Success		200				{object}	map[string]string
//	@Failure		401				{object}	map[string]string
//	@Failure		404				{object}	map[string]string
//	@Failure		500				{object}	map[string]string
//	@Router			/google-play-credentials/{credentialsId} [delete]
//	@Security		BearerAuth
func (c *GooglePlayCredentialsController) GooglePlayCredentialsDeleteHandler(w http.ResponseWriter, r *http.Request) {
	userInfo, err := c.UserService.GetUserFromContext(r.Context())
	if err != nil {
		helpers.WriteErrorJSON(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(r)
	credentialsID, err := strconv.Atoi(vars["credentialsId"])
	if err != nil {
		helpers.WriteErrorJSON(w, "Invalid credentials ID", http.StatusBadRequest)
		return
	}

	var cred dbEngine.GooglePlayCredentials
	if err := dbEngine.DB.Where("id = ? AND user_id = ?", credentialsID, userInfo.ID).First(&cred).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			helpers.WriteErrorJSON(w, "Credentials not found", http.StatusNotFound)
			return
		}
		helpers.WriteErrorJSON(w, "Failed to fetch credentials", http.StatusInternalServerError)
		return
	}

	if err := dbEngine.DB.Delete(&cred).Error; err != nil {
		helpers.WriteErrorJSON(w, "Failed to delete credentials", http.StatusInternalServerError)
		return
	}

	helpers.WriteJSON(w, map[string]string{"status": "deleted"})
}
