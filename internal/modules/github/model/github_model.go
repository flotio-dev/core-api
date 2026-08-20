package model

// REQUEST

type GithubPostInstallationRequest struct {
	InstallationID int64 `json:"installation_id" example:"123456789"`
}

type GithubRepoTreeQuery struct {
	Owner string `json:"owner" example:"flotio-dev"`
	Repo  string `json:"repo" example:"api"`
}

type PostInstallationRequest struct {
	InstallationID int64 `json:"installation_id" example:"12345678"`
}

// RESPONSE

type GithubRepository struct {
	ID             int64  `json:"id" example:"123456"`
	Owner          string `json:"owner" example:"flotio-dev"`
	Name           string `json:"name" example:"api"`
	FullName       string `json:"full_name" example:"flotio-dev/api"`
	Private        bool   `json:"private" example:"false"`
	Language       string `json:"language,omitempty" example:"Dart"`
	Description    string `json:"description,omitempty"`
	DefaultBranch  string `json:"default_branch,omitempty" example:"main"`
	IsFlutter      bool   `json:"is_flutter" example:"true"`
	InstallationID int64  `json:"installation_id,omitempty" example:"12345678"`
}

type GithubRepositoriesResponse struct {
	Repositories []GithubRepository `json:"repositories"`
}

type GithubRepoTreeItem struct {
	Name     string               `json:"name" example:"cmd"`
	Path     string               `json:"path" example:"cmd/service"`
	Type     string               `json:"type" example:"dir"`
	URL      string               `json:"url" example:"https://github.com/..."`
	Children []GithubRepoTreeItem `json:"children,omitempty"`
}

type GithubTreeResponse struct {
	Owner                  string               `json:"owner" example:"flotio-dev"`
	Repo                   string               `json:"repo" example:"api"`
	Tree                   []GithubRepoTreeItem `json:"tree"`
	ProjectPath            string               `json:"project_path,omitempty" example:"."`
	DetectedFlutterVersion string               `json:"detected_flutter_version,omitempty" example:"3.24.5"`
	DetectionSource        string               `json:"detection_source,omitempty" example:"fvm"`
	HasGoogleServices      bool                 `json:"has_google_services,omitempty" example:"false"`
}

type GithubInstallationResponse struct {
	ID             uint   `json:"id" example:"123456"`
	UserID         uint   `json:"user_id" example:"42"`
	InstallationID int64  `json:"installation_id" example:"987654"`
	AccountLogin   string `json:"account_login" example:"flotio-dev"`
	AccountType    string `json:"account_type" example:"User"`
	AvatarURL      string `json:"avatar_url,omitempty" example:"https://avatars.githubusercontent.com/u/123"`
}

type GithubInstallationsListResponse struct {
	Installations []GithubInstallationResponse `json:"installations"`
}

type PostInstallationResponse struct {
	InstallationID int64 `json:"installation_id" example:"12345678"`
}

type BuildPathResponse struct {
	Path string `json:"path"`
}

type FlutterProjectDetection struct {
	ProjectPath            string `json:"project_path" example:"."`
	DetectedFlutterVersion string `json:"detected_flutter_version,omitempty" example:"3.24.5"`
	DetectionSource        string `json:"detection_source,omitempty" example:"fvm"`
	HasGoogleServices      bool   `json:"has_google_services" example:"false"`
}

// DeleteResponse is the payload of DELETE /github/disconnect (contract §5.3).
type DeleteResponse struct {
	Status string `json:"status" example:"deleted"`
}
