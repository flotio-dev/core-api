package handler

import (
	dbEngine "github.com/flotio-dev/core-api/internal/common/database"
	models "github.com/flotio-dev/core-api/internal/modules/build/model"
)

func convertDBBuild(build dbEngine.Build) models.BuildDTO {
	return models.BuildDTO{
		ID:             build.ID,
		CreatedAt:      build.CreatedAt,
		UpdatedAt:      build.UpdatedAt,
		ProjectID:      build.ProjectID,
		Status:         build.Status,
		Platform:       build.Platform,
		BuildMode:      build.BuildMode,
		BuildTarget:    build.BuildTarget,
		FlutterChannel: build.FlutterChannel,
		GitBranch:      build.GitBranch,
		ContainerID:    build.ContainerID,
		Duration:       build.Duration,
		APKURL:         build.APKURL,
	}
}

func convertDBBuilds(builds []dbEngine.Build) []models.BuildDTO {
	out := make([]models.BuildDTO, len(builds))
	for i, b := range builds {
		out[i] = convertDBBuild(b)
	}
	return out
}
