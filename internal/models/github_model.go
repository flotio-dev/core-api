package models

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
	ID       int64  `json:"id" example:"123456"`
	Owner    string `json:"owner" example:"flotio-dev"`
	Name     string `json:"name" example:"api"`
	FullName string `json:"full_name" example:"flotio-dev/api"`
	Private  bool   `json:"private" example:"false"`
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
	Owner string               `json:"owner" example:"flotio-dev"`
	Repo  string               `json:"repo" example:"api"`
	Tree  []GithubRepoTreeItem `json:"tree"`
}

type GithubInstallationResponse struct {
	ID             uint   `json:"id" example:"123456"`
	UserID         uint   `json:"user_id" example:"42"`
	InstallationID int64  `json:"installation_id" example:"987654"`
	AccountLogin   string `json:"account_login" example:"flotio-dev"`
	AccountType    string `json:"account_type" example:"User"`
}

type PostInstallationResponse struct {
	InstallationID int64 `json:"installation_id" example:"12345678"`
}
