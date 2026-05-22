package handler

import (
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
	"gorm.io/gorm"

	dbEngine "github.com/flotio-dev/core-api/internal/common/database"
	helpers "github.com/flotio-dev/core-api/internal/common/server"
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
//	@Success		200	{object}	map[string]interface{}
//	@Failure		401	{object}	map[string]string
//	@Failure		500	{object}	map[string]string
//	@Router			/keystore [get]
//	@Security		BearerAuth
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

	helpers.WriteJSON(w, map[string]interface{}{"keystores": keystores})
}

// KeystorePostHandler godoc
//
//	@Summary		Create user keystore
//	@Description	Upload a new keystore for the authenticated user
//	@Tags			keystore
//	@Accept			json
//	@Produce		json
//	@Param			keystore	body	dbEngine.Keystore	true	"Keystore data"
//	@Success		201			{object}	map[string]interface{}
//	@Failure		401			{object}	map[string]string
//	@Failure		500			{object}	map[string]string
//	@Router			/keystore [post]
//	@Security		BearerAuth
func (c *KeystoreController) KeystorePostHandler(w http.ResponseWriter, r *http.Request) {
	userInfo, err := c.UserService.GetUserFromContext(r.Context())
	if err != nil {
		helpers.WriteErrorJSON(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req struct {
		Name          string `json:"name"`
		KeystoreFile  string `json:"keystore_file"`
		StorePassword string `json:"store_password"`
		KeyAlias      string `json:"key_alias"`
		KeyPassword   string `json:"key_password"`
	}
	if err := helpers.ReadJSON(r, &req); err != nil {
		helpers.WriteErrorJSON(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	keystore := dbEngine.Keystore{
		UserID:        userInfo.ID,
		Name:          req.Name,
		KeystoreFile:  req.KeystoreFile,
		StorePassword: req.StorePassword,
		KeyAlias:      req.KeyAlias,
		KeyPassword:   req.KeyPassword,
	}

	if err := dbEngine.DB.Create(&keystore).Error; err != nil {
		helpers.WriteErrorJSON(w, "Failed to create keystore", http.StatusInternalServerError)
		return
	}

	helpers.WriteJSON(w, map[string]interface{}{"keystore": keystore})
}

// KeystoreDeleteHandler godoc
//
//	@Summary		Delete user keystore
//	@Description	Delete a specific keystore by ID
//	@Tags			keystore
//	@Accept			json
//	@Produce		json
//	@Param			keystoreId	path	int	true	"Keystore ID"
//	@Success		200			{object}	map[string]string
//	@Failure		401			{object}	map[string]string
//	@Failure		404			{object}	map[string]string
//	@Failure		500			{object}	map[string]string
//	@Router			/keystore/{keystoreId} [delete]
//	@Security		BearerAuth
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

	helpers.WriteJSON(w, map[string]string{"status": "deleted"})
}
