package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/flotio-dev/api/pkg/api/v1/service"
	"github.com/flotio-dev/api/pkg/db"
	"github.com/google/go-github/v76/github"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) *gorm.DB {
	gormDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)

	assert.NoError(t, gormDB.AutoMigrate(&db.GithubInstallation{}))

	return gormDB
}

func mockClientFactory(_ int64) (*github.Client, error) {
	return &github.Client{}, nil
}

func TestSaveAndGetInstallation(t *testing.T) {
	gormDB := setupTestDB(t)

	svc := service.NewGithubService(gormDB, mockClientFactory)

	err := svc.SaveInstallation(1, 12345, "user_login", "User", 56789)
	assert.NoError(t, err)

	inst, err := svc.GetInstallationByUser(1)
	assert.NoError(t, err)
	assert.NotNil(t, inst)
	assert.Equal(t, int64(12345), inst.InstallationID)
	assert.Equal(t, "user_login", inst.AccountLogin)
	assert.Equal(t, "User", inst.AccountType)
	assert.Equal(t, int64(56789), inst.TargetID)
}

func TestGetRepoTree_Error(t *testing.T) {
	gormDB := setupTestDB(t)

	clientFactory := func(_ int64) (*github.Client, error) {
		return nil, errors.New("client error")
	}

	svc := service.NewGithubService(gormDB, clientFactory)

	tree, err := svc.GetRepoTree(context.Background(), 12345, "owner", "repo")
	assert.Error(t, err)
	assert.Nil(t, tree)
}

func TestListRepositories_Error(t *testing.T) {
	gormDB := setupTestDB(t)

	clientFactory := func(_ int64) (*github.Client, error) {
		return nil, errors.New("client error")
	}

	svc := service.NewGithubService(gormDB, clientFactory)

	repos, err := svc.ListRepositories(context.Background(), 12345)
	assert.Error(t, err)
	assert.Nil(t, repos)
}
