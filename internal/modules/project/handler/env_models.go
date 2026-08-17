package handler

import (
	"time"

	dbEngine "github.com/flotio-dev/core-api/internal/common/database"
)

// EnvDTO is the API representation of an environment asset (env var or file),
// matching the JSON keys the env handlers serialize (contract §5.3).
type EnvDTO struct {
	ID        uint      `json:"id" example:"1"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	UserID    uint      `json:"user_id" example:"1"`
	ProjectID *uint     `json:"project_id,omitempty" example:"2"`
	Key       string    `json:"key" example:"API_KEY"`
	Value     string    `json:"value" example:"super-secret"`
	Type      string    `json:"type" example:"env"` // "env" or "file"
	Path      string    `json:"path" example:"android/app/google-services.json"`
	IsBase64  bool      `json:"is_base64" example:"false"`
}

// EnvListResponse is the payload of GET /env and GET /envs.
type EnvListResponse struct {
	Envs []EnvDTO `json:"envs"`
}

// EnvResponse is the payload of GET/POST/PUT /env single-resource operations.
type EnvResponse struct {
	Env EnvDTO `json:"env"`
}

// EnvCreateRequest is the body of POST /env.
type EnvCreateRequest struct {
	ProjectID *uint  `json:"project_id,omitempty" example:"2"`
	Key       string `json:"key" example:"API_KEY"`
	Value     string `json:"value" example:"super-secret"`
	Type      string `json:"type" example:"env"` // "env" or "file"
	Path      string `json:"path" example:"android/app/google-services.json"`
	IsBase64  bool   `json:"is_base64" example:"false"`
}

// EnvUpdateRequest is the body of PUT /env/{envId}.
type EnvUpdateRequest struct {
	ProjectID *uint  `json:"project_id,omitempty" example:"2"`
	Key       string `json:"key" example:"API_KEY"`
	Value     string `json:"value" example:"super-secret"`
	Type      string `json:"type" example:"env"` // "env" or "file"
	Path      string `json:"path" example:"android/app/google-services.json"`
	IsBase64  bool   `json:"is_base64" example:"false"`
}

func convertDBEnv(e dbEngine.Env) EnvDTO {
	return EnvDTO{
		ID:        e.ID,
		CreatedAt: e.CreatedAt,
		UpdatedAt: e.UpdatedAt,
		UserID:    e.UserID,
		ProjectID: e.ProjectID,
		Key:       e.Key,
		Value:     e.Value,
		Type:      e.Type,
		Path:      e.Path,
		IsBase64:  e.IsBase64,
	}
}

func convertDBEnvs(envs []dbEngine.Env) []EnvDTO {
	out := make([]EnvDTO, len(envs))
	for i, e := range envs {
		out[i] = convertDBEnv(e)
	}
	return out
}
