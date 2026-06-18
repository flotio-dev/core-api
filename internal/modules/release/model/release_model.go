package model

import "time"

// PublishRequest carries optional overrides for a publication. Any field left
// empty/nil falls back to the project configuration.
type PublishRequest struct {
	Track            string   `json:"track"`
	RolloutFraction  *float64 `json:"rollout_fraction"`
	Draft            *bool    `json:"draft"`
	ReleaseNotes     string   `json:"release_notes"`
	ReleaseNotesLang string   `json:"release_notes_lang"`
}

// ReleaseDTO is the API representation of a Google Play publication.
type ReleaseDTO struct {
	ID              uint      `json:"id"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
	ProjectID       uint      `json:"project_id"`
	BuildID         uint      `json:"build_id"`
	VersionName     string    `json:"version_name"`
	VersionCode     int64     `json:"version_code"`
	Track           string    `json:"track"`
	RolloutFraction float64   `json:"rollout_fraction"`
	Status          string    `json:"status"`
	ReleaseNotes    string    `json:"release_notes"`
}

type ReleaseResponse struct {
	Release ReleaseDTO `json:"release"`
}

type ReleasesResponse struct {
	Releases []ReleaseDTO `json:"releases"`
}

// AccessCheckResponse reports whether the service account can publish the app.
type AccessCheckResponse struct {
	Accessible bool   `json:"accessible"`
	Reason     string `json:"reason,omitempty"`
	Message    string `json:"message,omitempty"`
}

// AuditDTO is the API representation of a publication audit entry.
type AuditDTO struct {
	ID          uint      `json:"id"`
	CreatedAt   time.Time `json:"created_at"`
	UserID      uint      `json:"user_id"`
	ProjectID   uint      `json:"project_id"`
	ReleaseID   uint      `json:"release_id"`
	PackageName string    `json:"package_name"`
	VersionCode int64     `json:"version_code"`
	Track       string    `json:"track"`
	Action      string    `json:"action"`
	Detail      string    `json:"detail"`
}

type AuditListResponse struct {
	Audit []AuditDTO `json:"audit"`
}
