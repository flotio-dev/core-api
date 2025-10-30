package db

import (
	"gorm.io/gorm"
)

// User model - additional info beyond Keycloak
type User struct {
	gorm.Model
	KeycloakID         string    `gorm:"uniqueIndex" json:"keycloak_id"`
	Email              string    `gorm:"uniqueIndex" json:"email"`
	Username           string    `json:"username"`
	GithubAccessToken  string    `json:"github_access_token"`
	GithubRefreshToken string    `json:"github_refresh_token"`
	Projects           []Project `gorm:"foreignKey:UserID" json:"projects"`

	GithubInstallation *GithubInstallation `gorm:"foreignKey:UserID"`
}

// Project model
type Project struct {
	gorm.Model
	Name           string  `json:"name"`
	GitRepo        string  `json:"git_repo"`
	BuildFolder    string  `json:"build_folder"`
	FlutterVersion string  `json:"flutter_version"`
	UserID         uint    `json:"user_id"`
	User           User    `json:"user"`
	Builds         []Build `gorm:"foreignKey:ProjectID" json:"builds"`
	Envs           []Env   `gorm:"foreignKey:ProjectID" json:"envs"`
}

// Build model
type Build struct {
	gorm.Model
	ProjectID   uint    `json:"project_id"`
	Project     Project `json:"project"`
	Status      string  `json:"status"`       // pending, running, success, failed
	Platform    string  `json:"platform"`     // e.g., android, ios
	ContainerID string  `json:"container_id"` // Kubernetes container ID
	Duration    int64   `json:"duration"`     // build duration in seconds
	APKURL      string  `json:"apk_url"`
	Logs        []Log   `gorm:"foreignKey:BuildID" json:"logs"`
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

// Keystore model - stores Android signing credentials
type Keystore struct {
	gorm.Model
	ProjectID     uint    `json:"project_id"`
	Project       Project `json:"project"`
	Name          string  `json:"name"`           // Friendly name
	KeystoreFile  string  `json:"keystore_file"`  // Base64 encoded keystore file
	StorePassword string  `json:"store_password"` // Encrypted
	KeyAlias      string  `json:"key_alias"`
	KeyPassword   string  `json:"key_password"` // Encrypted
	IsActive      bool    `json:"is_active"`    // Only one active keystore per project
}

type Organization struct {
	gorm.Model
	Name                   string `json:"name" gorm:"not null;uniqueIndex"`
	KeycloakOrganizationID int64  `json:"keycloak_organization_id" gorm:"not null;uniqueIndex"`
	Description            string `json:"description,omitempty"`

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
