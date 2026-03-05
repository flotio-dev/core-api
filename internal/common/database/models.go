package database

import (
	"gorm.io/gorm"
)

// User model - additional info beyond Keycloak
type User struct {
	gorm.Model
	Email        string    `gorm:"uniqueIndex" json:"email"`
	Username     string    `json:"username"`
	Projects     []Project `gorm:"foreignKey:UserID" json:"projects"`
	PasswordHash string    `json:"-"`

	GithubInstallation *GithubInstallation `gorm:"foreignKey:UserID"`
}

// Project model
type Project struct {
	gorm.Model
	Name           string  `json:"name"`
	GitRepo        *string `json:"git_repo,omitempty"`
	BuildFolder    *string `json:"build_folder,omitempty"`
	FlutterVersion *string `json:"flutter_version,omitempty"`
	GitUsername    *string `json:"git_username,omitempty"`
	GitToken       *string `json:"git_token,omitempty"`
	UserID         uint    `json:"user_id"`
	User           User    `json:"user"`
	Builds         []Build `gorm:"foreignKey:ProjectID" json:"builds"`
	Envs           []Env   `gorm:"foreignKey:ProjectID" json:"envs"`
}

// Build model
type Build struct {
	gorm.Model
	ProjectID      uint    `json:"project_id"`
	Project        Project `json:"project"`
	Status         string  `json:"status"` // waiting, pending, running, success, failed, cancelled
	Platform       string  `json:"platform"`
	BuildMode      string  `json:"build_mode"`
	BuildTarget    string  `json:"build_target"`
	FlutterChannel string  `json:"flutter_channel"`
	GitBranch      string  `json:"git_branch"`
	ContainerID    string  `json:"container_id"` // Kubernetes container ID
	Duration       int64   `json:"duration"`     // build duration in seconds
	APKURL         string  `json:"apk_url"`      // S3 key for the build artifact (e.g., "builds/123/app-release.apk")
	Logs           []Log   `gorm:"foreignKey:BuildID" json:"logs"`
}

// Log model - stores build logs line by line
type Log struct {
	gorm.Model
	BuildID    uint   `json:"build_id"`
	Build      Build  `json:"build"`
	LineNumber int    `json:"line_number"`
	Content    string `json:"content"`
	Timestamp  int64  `json:"timestamp"` // Unix timestamp
}

// Env model - supports both environment variables and files
type Env struct {
	gorm.Model
	ProjectID uint    `json:"project_id"`
	Project   Project `json:"project"`
	Key       string  `json:"key"`       // Variable name or file identifier
	Value     string  `json:"value"`     // Variable value or file content (base64 for binary)
	Type      string  `json:"type"`      // "env" for environment variable, "file" for file
	Path      string  `json:"path"`      // Target path for files (e.g., "android/app/google-services.json")
	IsBase64  bool    `json:"is_base64"` // True if Value is base64 encoded (for binary files)
}

// AndroidSigningConfig model - stores Android signing credentials.
// A project can have one config per build_type ("debug" or "release").
// The keystore binary is stored in S3; only the path is persisted here.
// Passwords are encrypted at rest using AES-256-GCM.
type AndroidSigningConfig struct {
	gorm.Model
	ProjectID                uint    `json:"project_id"`
	Project                  Project `gorm:"foreignKey:ProjectID" json:"project"`
	BuildType                string  `json:"build_type"`   // "debug" | "release"
	KeystorePath             string  `json:"keystore_path"` // S3 object key
	KeyAlias                 string  `json:"key_alias"`
	KeystorePasswordEncrypted string `json:"keystore_password_encrypted"`
	KeyPasswordEncrypted     string  `json:"key_password_encrypted"`
}

type Organization struct {
	gorm.Model
	Name        string `json:"name" gorm:"not null;uniqueIndex"`
	Description string `json:"description,omitempty"`

	GithubInstallation *GithubInstallation `gorm:"foreignKey:OrganizationID"`
}

type GithubInstallation struct {
	gorm.Model

	InstallationID int64  `json:"github_installation_id" gorm:"not null;uniqueIndex"`
	UserID         *uint  `json:"user_id,omitempty" gorm:"unique"`
	OrganizationID *uint  `json:"organization_id,omitempty" gorm:"unique"`
	AccountLogin   string `json:"account_login" gorm:"not null"`
	AccountType    string `json:"account_type" gorm:"not null"`
	TargetID       int64  `json:"target_id" gorm:"not null"`

	User         *User         `gorm:"foreignKey:UserID;constraint:OnUpdate:CASCADE,OnDelete:SET NULL;"`
	Organization *Organization `gorm:"foreignKey:OrganizationID;constraint:OnUpdate:CASCADE,OnDelete:SET NULL;"`
}
