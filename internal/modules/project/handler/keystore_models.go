package handler

import (
	"time"

	dbEngine "github.com/flotio-dev/core-api/internal/common/database"
)

// KeystoreDTO is the API representation of a keystore asset. Secret fields
// (keystore file, passwords) are never serialized (contract §5.3).
type KeystoreDTO struct {
	ID        uint      `json:"id" example:"1"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	UserID    uint      `json:"user_id" example:"1"`
	Name      string    `json:"name" example:"My App Keystore"`
	KeyAlias  string    `json:"key_alias" example:"my-alias"`
}

// KeystoreListResponse is the payload of GET /keystore and GET /keystores.
type KeystoreListResponse struct {
	Keystores []KeystoreDTO `json:"keystores"`
}

// KeystoreResponse is the payload of POST /keystore.
type KeystoreResponse struct {
	Keystore KeystoreDTO `json:"keystore"`
}

// KeystoreCreateRequest is the body of POST /keystore.
type KeystoreCreateRequest struct {
	Name          string `json:"name" example:"My App Keystore"`
	KeystoreFile  string `json:"keystore_file" example:"base64-encoded .jks content"`
	StorePassword string `json:"store_password" example:"store-secret"`
	KeyAlias      string `json:"key_alias" example:"my-alias"`
	KeyPassword   string `json:"key_password" example:"key-secret"`
}

func convertDBKeystore(k dbEngine.Keystore) KeystoreDTO {
	return KeystoreDTO{
		ID:        k.ID,
		CreatedAt: k.CreatedAt,
		UpdatedAt: k.UpdatedAt,
		UserID:    k.UserID,
		Name:      k.Name,
		KeyAlias:  k.KeyAlias,
	}
}

func convertDBKeystores(keystores []dbEngine.Keystore) []KeystoreDTO {
	out := make([]KeystoreDTO, len(keystores))
	for i, k := range keystores {
		out[i] = convertDBKeystore(k)
	}
	return out
}
