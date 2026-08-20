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

	GithubInstallations []GithubInstallation `gorm:"foreignKey:UserID" json:"github_installations,omitempty"`
}

// Project model
type Project struct {
	gorm.Model
	Name   string         `json:"name"`
	UserID uint           `json:"user_id"`
	User   User           `json:"user"`
	Builds []Build        `gorm:"foreignKey:ProjectID" json:"builds"`
	Config *ProjectConfig `gorm:"foreignKey:ProjectID" json:"config"`
}

// ProjectConfig model
type ProjectConfig struct {
	gorm.Model
	ProjectID             uint                   `gorm:"uniqueIndex" json:"project_id"`
	Platforms             []string               `gorm:"serializer:json" json:"platforms"`
	BuildTrigger          string                 `json:"build_trigger"`
	WatchedBranchPatterns []WatchedBranchPattern `gorm:"serializer:json" json:"watched_branch_patterns"`
	WatchedTagPatterns    []WatchedTagPattern    `gorm:"serializer:json" json:"watched_tag_patterns"`
	DependencyCaching     bool                   `json:"dependency_caching"`
	DependencyDirs        []string               `gorm:"serializer:json" json:"dependency_dirs"`

	// Git Connection
	GitRepo     string `json:"git_repo"`
	GitUsername string `json:"git_username"`
	GitToken    string `json:"git_token"`

	// Webhooks
	WebhookURLs []string `gorm:"serializer:json" json:"webhook_urls"`

	// Scripts
	PostCloneScript  string `json:"post_clone_script"`
	PreTestScript    string `json:"pre_test_script"`
	PostTestScript   string `json:"post_test_script"`
	PreBuildScript   string `json:"pre_build_script"`
	PostBuildScript  string `json:"post_build_script"`
	PrePublishScript string `json:"pre_publish_script"`

	// Testing
	Test                 bool     `json:"test"`
	EnableFlutterAnalyze bool     `json:"enable_flutter_analyze"`
	FlutterAnalyzeArgs   string   `json:"flutter_analyze_args"`
	EnableFlutterTest    bool     `json:"enable_flutter_test"`
	FlutterTestArgs      string   `json:"flutter_test_args"`
	EnableFlutterDriver  bool     `json:"enable_flutter_driver"`
	FlutterDriverArgs    string   `json:"flutter_driver_args"`
	FlutterDriverTargets []string `gorm:"serializer:json" json:"flutter_driver_targets"`

	// Build
	FlutterVersion     string `json:"flutter_version"`
	XcodeVersion       string `json:"xcode_version"`
	CocoaPodsVersion   string `json:"cocoapods_version"`
	ProjectPath        string `json:"project_path"`
	AndroidBuildFormat string `json:"android_build_format"` // "apk" or "aab"
	BuildMode          string `json:"build_mode"`           // "debug", "release", "profile"
	AndroidBuildArgs   string `json:"android_build_args"`
	IosBuildArgs       string `json:"ios_build_args"`
	WebBuildArgs       string `json:"web_build_args"`

	// Distribution
	PackageName                string  `json:"package_name"` // Android applicationId, identifies the app on the Play Store
	EnableAndroidCodeSigning   bool    `json:"enable_android_code_signing"`
	EnableGooglePlayPublishing bool    `json:"enable_google_play_publishing"`
	GooglePlayTrack            string  `json:"google_play_track"`
	UpdatePriority             int     `json:"update_priority"`
	RolloutFraction            float64 `json:"rollout_fraction"`
	DoNotSendForReview         bool    `json:"do_not_send_for_review"`
	SubmitAsDraft              bool    `json:"submit_as_draft"`
	PublishEvenIfTestsFail     bool    `json:"publish_even_if_tests_fail"`

	// Linked User-level Assets
	KeystoreID              *uint `json:"keystore_id"`
	GooglePlayCredentialsID *uint `json:"google_play_credentials_id"`

	// Notifications
	EnableEmailNotifications bool     `json:"enable_email_notifications"`
	EmailRecipients          []string `gorm:"serializer:json" json:"email_recipients"`
}

type WatchedBranchPattern struct {
	Pattern string `json:"pattern"`
	Type    string `json:"type"`   // "include" or "exclude"
	Target  string `json:"target"` // "source" or "target"
}

type WatchedTagPattern struct {
	Pattern string `json:"pattern"`
	Type    string `json:"type"` // "include" or "exclude"
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
	VersionName    string  `json:"version_name"` // Flutter --build-name baked into the artifact (empty = use pubspec)
	VersionCode    int64   `json:"version_code"` // Flutter --build-number baked into the artifact (0 = use pubspec)
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

// Release model - represents a Google Play publication of a build's artifact.
// A Release is distinct from a Build: one successful Build can be published as a
// Release on a track, with its own version metadata and rollout state.
type Release struct {
	gorm.Model
	ProjectID       uint    `json:"project_id"`
	BuildID         uint    `json:"build_id"`
	VersionName     string  `json:"version_name"`     // Human-readable version shown to users (e.g. "1.3.0")
	VersionCode     int64   `json:"version_code"`     // Monotonic integer baked into the AAB
	Track           string  `json:"track"`            // internal, alpha, beta, production
	RolloutFraction float64 `json:"rollout_fraction"` // 0..1 for staged rollout (1 = full)
	Status          string  `json:"status"`           // pending, uploading, in_progress, published, halted, failed
	ReleaseNotes    string  `json:"release_notes"`
}

// ReleaseAudit records who published what, when and with what result. It never
// stores secrets (no credentials, keystore or tokens).
type ReleaseAudit struct {
	gorm.Model
	UserID      uint   `json:"user_id"`
	ProjectID   uint   `json:"project_id"`
	ReleaseID   uint   `json:"release_id"`
	PackageName string `json:"package_name"`
	VersionCode int64  `json:"version_code"`
	Track       string `json:"track"`
	Action      string `json:"action"` // triggered, published, in_progress, draft, failed
	Detail      string `json:"detail"` // human-readable, secret-free
}

// Env model - supports both environment variables and files, owned by User
type Env struct {
	gorm.Model
	UserID    uint     `json:"user_id"`
	User      User     `json:"user"`
	ProjectID *uint    `json:"project_id,omitempty"` // Nullable: can be associated to a project
	Project   *Project `json:"project,omitempty"`
	Key       string   `json:"key"`       // Variable name or file identifier
	Value     string   `json:"value"`     // Variable value or file content (base64 for binary)
	Type      string   `json:"type"`      // "env" for environment variable, "file" for file
	Path      string   `json:"path"`      // Target path for files (e.g., "android/app/google-services.json")
	IsBase64  bool     `json:"is_base64"` // True if Value is base64 encoded (for binary files)
}

// Keystore model - stores Android signing credentials, owned by User.
// Secret fields are encrypted at rest (AES-256-GCM) and never serialized in API
// responses (json:"-").
type Keystore struct {
	gorm.Model
	UserID        uint   `json:"user_id"`
	User          User   `json:"user"`
	Name          string `json:"name"`      // Friendly name
	KeystoreFile  string `json:"-"`         // Base64-encoded .jks, encrypted at rest
	StorePassword string `json:"-"`         // Encrypted at rest
	KeyAlias      string `json:"key_alias"` // Not secret
	KeyPassword   string `json:"-"`         // Encrypted at rest
}

// GooglePlayCredentials model - stores Google Play distribution keys, owned by User.
// Credentials are encrypted at rest (AES-256-GCM) and never serialized in API
// responses (json:"-").
type GooglePlayCredentials struct {
	gorm.Model
	UserID      uint   `json:"user_id"`
	User        User   `json:"user"`
	Name        string `json:"name"` // Friendly name
	Credentials string `json:"-"`    // Base64-encoded JSON service account key, encrypted at rest
}

type Organization struct {
	gorm.Model
	Name        string `json:"name" gorm:"not null;uniqueIndex"`
	Description string `json:"description,omitempty"`

	GithubInstallations []GithubInstallation `gorm:"foreignKey:OrganizationID" json:"github_installations,omitempty"`
}

type GithubInstallation struct {
	gorm.Model

	InstallationID int64  `json:"github_installation_id" gorm:"not null;uniqueIndex:idx_user_installation"`
	UserID         *uint  `json:"user_id,omitempty" gorm:"uniqueIndex:idx_user_installation"`
	OrganizationID *uint  `json:"organization_id,omitempty"`
	AccountLogin   string `json:"account_login" gorm:"not null"`
	AccountType    string `json:"account_type" gorm:"not null"`
	TargetID       int64  `json:"target_id" gorm:"not null"`
	AvatarURL      string `json:"avatar_url,omitempty"`

	User         *User         `gorm:"foreignKey:UserID;constraint:OnUpdate:CASCADE,OnDelete:SET NULL;"`
	Organization *Organization `gorm:"foreignKey:OrganizationID;constraint:OnUpdate:CASCADE,OnDelete:SET NULL;"`
}
