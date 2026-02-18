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
	ID             uint      `json:"id"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	ProjectID      uint      `json:"project_id"`
	Status         string    `json:"status"`
	Platform       string    `json:"platform"`
	BuildMode      string    `json:"build_mode"`
	BuildTarget    string    `json:"build_target"`
	FlutterChannel string    `json:"flutter_channel"`
	GitBranch      string    `json:"git_branch"`
	ContainerID    string    `json:"container_id"`
	Duration       int64     `json:"duration"`
	APKURL         string    `json:"apk_url"`
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

type CachePurgeResponse struct {
	Status         string `json:"status"`
	Namespace      string `json:"namespace"`
	Fingerprint    string `json:"fingerprint,omitempty"`
	DeletedObjects int    `json:"deleted_objects"`
}

type CacheMetricsResponse struct {
	Namespace         string     `json:"namespace"`
	Fingerprint       string     `json:"fingerprint,omitempty"`
	Prefix            string     `json:"prefix"`
	ObjectCount       int64      `json:"object_count"`
	TotalSizeBytes    int64      `json:"total_size_bytes"`
	LastModifiedAt    *time.Time `json:"last_modified_at,omitempty"`
	PurgeRequests     uint64     `json:"purge_requests"`
	PurgedObjects     uint64     `json:"purged_objects"`
	MetricsRequests   uint64     `json:"metrics_requests"`
	GeneratedAt       time.Time  `json:"generated_at"`
	RetentionTTLHours int        `json:"retention_ttl_hours"`
}

type CacheEntryDTO struct {
	Fingerprint    string     `json:"fingerprint"`
	Prefix         string     `json:"prefix"`
	ObjectCount    int64      `json:"object_count"`
	TotalSizeBytes int64      `json:"total_size_bytes"`
	LastModifiedAt *time.Time `json:"last_modified_at,omitempty"`
}

type CacheEntriesResponse struct {
	Namespace   string          `json:"namespace"`
	Branch      string          `json:"branch"`
	Entries     []CacheEntryDTO `json:"entries"`
	GeneratedAt time.Time       `json:"generated_at"`
}
