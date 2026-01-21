package model

import (
	"time"
)

type BuildRequest struct {
	Platform       string `json:"platform"`
	BuildMode      string `json:"build_mode"`
	BuildTarget    string `json:"build_target"`
	FlutterChannel string `json:"flutter_channel"`
	GitBranch      string `json:"git_branch"`
}

type BuildDTO struct {
	ID          uint      `json:"id"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	ProjectID   uint      `json:"project_id"`
	Status      string    `json:"status"`
	Platform    string    `json:"platform"`
	ContainerID string    `json:"container_id"`
	Duration    int64     `json:"duration"`
	APKURL      string    `json:"apk_url"`
}

type BuildResponse struct {
	Build BuildDTO `json:"build"`
}

type BuildsResponse struct {
	Builds []BuildDTO `json:"builds"`
}

type LogsResponse struct {
	Logs []string `json:"logs"`
}

type DeleteResponse struct {
	Status string `json:"status"`
}
