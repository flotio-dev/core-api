package payload

type GithubPostInstallationPayload struct {
	InstallationID int64 `json:"installation_id" example:"123456789"`
}

type GithubRepoTreeQuery struct {
	Owner string `json:"owner" example:"flotio-dev"`
	Repo  string `json:"repo" example:"api"`
}

type PostInstallationPayload struct {
	InstallationID int64 `json:"installation_id" example:"12345678"`
}
