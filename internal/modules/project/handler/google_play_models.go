package handler

import (
	"time"

	dbEngine "github.com/flotio-dev/core-api/internal/common/database"
)

// GooglePlayCredentialsDTO is the API representation of Google Play
// distribution credentials. The service-account key is never serialized
// (contract §5.3).
type GooglePlayCredentialsDTO struct {
	ID        uint      `json:"id" example:"1"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	UserID    uint      `json:"user_id" example:"1"`
	Name      string    `json:"name" example:"My Service Account"`
}

// GooglePlayCredentialsListResponse is the payload of GET /google-play-credentials.
type GooglePlayCredentialsListResponse struct {
	GooglePlayCredentials []GooglePlayCredentialsDTO `json:"google_play_credentials"`
}

// GooglePlayCredentialsResponse is the payload of POST /google-play-credentials.
type GooglePlayCredentialsResponse struct {
	GooglePlayCredentials GooglePlayCredentialsDTO `json:"google_play_credentials"`
}

// GooglePlayCredentialsCreateRequest is the body of POST /google-play-credentials.
type GooglePlayCredentialsCreateRequest struct {
	Name        string `json:"name" example:"My Service Account"`
	Credentials string `json:"credentials" example:"base64-encoded service account JSON"`
}

func convertDBGooglePlayCredentials(c dbEngine.GooglePlayCredentials) GooglePlayCredentialsDTO {
	return GooglePlayCredentialsDTO{
		ID:        c.ID,
		CreatedAt: c.CreatedAt,
		UpdatedAt: c.UpdatedAt,
		UserID:    c.UserID,
		Name:      c.Name,
	}
}

func convertDBGooglePlayCredentialsList(credentials []dbEngine.GooglePlayCredentials) []GooglePlayCredentialsDTO {
	out := make([]GooglePlayCredentialsDTO, len(credentials))
	for i, c := range credentials {
		out[i] = convertDBGooglePlayCredentials(c)
	}
	return out
}
