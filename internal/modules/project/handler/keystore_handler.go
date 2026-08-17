package handler

import (
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
	"gorm.io/gorm"

	"github.com/flotio-dev/core-api/internal/common/crypto"
	dbEngine "github.com/flotio-dev/core-api/internal/common/database"
	helpers "github.com/flotio-dev/core-api/internal/common/server"
	models "github.com/flotio-dev/core-api/internal/models"
	services "github.com/flotio-dev/core-api/internal/modules/user/service"
)

type KeystoreController struct {
	UserService *services.UserService
}

func NewKeystoreController(userService *services.UserService) *KeystoreController {
	return &KeystoreController{
		UserService: userService,
	}
}

// KeystoreGetHandler godoc
//
//	@Summary		Get user keystores
//	@Description	Get all keystores owned by the authenticated user
//	@Tags			keystore
//	@Accept			json
//	@Produce		json
//	@Success		200	{object}	KeystoreListResponse
//	@Failure		401	{object}	models.APIErrorResponse
//	@Failure		500	{object}	models.APIErrorResponse
//	@Security		BearerAuth
//	@ID				KeystoreGetHandler
//	@Router			/keystore [get]
func (c *KeystoreController) KeystoreGetHandler(w http.ResponseWriter, r *http.Request) {
	userInfo, err := c.UserService.GetUserFromContext(r.Context())
	if err != nil {
		helpers.WriteErrorJSON(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var keystores []dbEngine.Keystore
	if err := dbEngine.DB.Where("user_id = ?", userInfo.ID).Find(&keystores).Error; err != nil {
		helpers.WriteErrorJSON(w, "Failed to fetch keystores", http.StatusInternalServerError)
		return
	}

	helpers.WriteJSON(w, KeystoreListResponse{Keystores: convertDBKeystores(keystores)})
}

// KeystoresGetHandler godoc
//
//	@Summary		Get user keystores
//	@Description	Alias of GET /keystore: get all keystores owned by the authenticated user
//	@Tags			keystore
//	@Accept			json
//	@Produce		json
//	@Success		200	{object}	KeystoreListResponse
//	@Failure		401	{object}	models.APIErrorResponse
//	@Failure		500	{object}	models.APIErrorResponse
//	@Security		BearerAuth
//	@ID				KeystoresGetHandler
//	@Router			/keystores [get]
func (c *KeystoreController) KeystoresGetHandler(w http.ResponseWriter, r *http.Request) {
	c.KeystoreGetHandler(w, r)
}

// KeystorePostHandler godoc
//
//	@Summary		Create user keystore
//	@Description	Upload a new keystore for the authenticated user
//	@Tags			keystore
//	@Accept			json
//	@Produce		json
//	@Param			keystore	body	KeystoreCreateRequest	true	"Keystore data"
//	@Success		201			{object}	KeystoreResponse
//	@Failure		400			{object}	models.APIErrorResponse
//	@Failure		401			{object}	models.APIErrorResponse
//	@Failure		500			{object}	models.APIErrorResponse
//	@Security		BearerAuth
//	@ID				KeystorePostHandler
//	@Router			/keystore [post]
func (c *KeystoreController) KeystorePostHandler(w http.ResponseWriter, r *http.Request) {
	userInfo, err := c.UserService.GetUserFromContext(r.Context())
	if err != nil {
		helpers.WriteErrorJSON(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req KeystoreCreateRequest
	if err := helpers.ReadJSON(r, &req); err != nil {
		helpers.WriteErrorJSON(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Encrypt secrets at rest before storing.
	encFile, err := crypto.Encrypt(req.KeystoreFile)
	if err != nil {
		helpers.WriteErrorJSON(w, "Failed to encrypt keystore", http.StatusInternalServerError)
		return
	}
	encStore, err := crypto.Encrypt(req.StorePassword)
	if err != nil {
		helpers.WriteErrorJSON(w, "Failed to encrypt keystore", http.StatusInternalServerError)
		return
	}
	encKey, err := crypto.Encrypt(req.KeyPassword)
	if err != nil {
		helpers.WriteErrorJSON(w, "Failed to encrypt keystore", http.StatusInternalServerError)
		return
	}

	keystore := dbEngine.Keystore{
		UserID:        userInfo.ID,
		Name:          req.Name,
		KeystoreFile:  encFile,
		StorePassword: encStore,
		KeyAlias:      req.KeyAlias,
		KeyPassword:   encKey,
	}

	if err := dbEngine.DB.Create(&keystore).Error; err != nil {
		helpers.WriteErrorJSON(w, "Failed to create keystore", http.StatusInternalServerError)
		return
	}

	helpers.WriteJSON(w, KeystoreResponse{Keystore: convertDBKeystore(keystore)})
}

// KeystoreDeleteHandler godoc
//
//	@Summary		Delete user keystore
//	@Description	Delete a specific keystore by ID
//	@Tags			keystore
//	@Accept			json
//	@Produce		json
//	@Param			keystoreId	path	int	true	"Keystore ID"	Format(int64)
//	@Success		200			{object}	DeleteResponse
//	@Failure		400			{object}	models.APIErrorResponse
//	@Failure		401			{object}	models.APIErrorResponse
//	@Failure		404			{object}	models.APIErrorResponse
//	@Failure		500			{object}	models.APIErrorResponse
//	@Security		BearerAuth
//	@ID				KeystoreDeleteHandler
//	@Router			/keystore/{keystoreId} [delete]
func (c *KeystoreController) KeystoreDeleteHandler(w http.ResponseWriter, r *http.Request) {
	userInfo, err := c.UserService.GetUserFromContext(r.Context())
	if err != nil {
		helpers.WriteErrorJSON(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(r)
	keystoreID, err := strconv.Atoi(vars["keystoreId"])
	if err != nil {
		helpers.WriteErrorJSON(w, "Invalid keystore ID", http.StatusBadRequest)
		return
	}

	var keystore dbEngine.Keystore
	if err := dbEngine.DB.Where("id = ? AND user_id = ?", keystoreID, userInfo.ID).First(&keystore).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			helpers.WriteErrorJSON(w, "Keystore not found", http.StatusNotFound)
			return
		}
		helpers.WriteErrorJSON(w, "Failed to fetch keystore", http.StatusInternalServerError)
		return
	}

	if err := dbEngine.DB.Delete(&keystore).Error; err != nil {
		helpers.WriteErrorJSON(w, "Failed to delete keystore", http.StatusInternalServerError)
		return
	}

	helpers.WriteJSON(w, DeleteResponse{Status: "deleted"})
}

// Keep the swag annotation import alive (used only in @Failure comments).
var _ = models.APIErrorResponse{}
