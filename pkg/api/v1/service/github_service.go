package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/go-github/v79/github"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/flotio-dev/api/pkg/db"
)

type GithubService struct {
	DB            *gorm.DB
	ClientFactory func(installationID int64) (*github.Client, error)
}

func NewGithubService(db *gorm.DB, clientFactory func(int64) (*github.Client, error)) *GithubService {
	return &GithubService{
		DB:            db,
		ClientFactory: clientFactory,
	}
}

func (s *GithubService) SaveInstallation(userID uint, installationID int64, accountLogin, accountType string, targetID int64) error {
	inst := db.GithubInstallation{
		InstallationID: installationID,
		UserID:         &userID,
		AccountLogin:   accountLogin,
		AccountType:    accountType,
		TargetID:       targetID,
	}

	return s.DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "installation_id"}},
		UpdateAll: true,
	}).Create(&inst).Error
}

func (s *GithubService) GetInstallationByUser(userID uint) (*db.GithubInstallation, error) {
	var inst db.GithubInstallation
	err := s.DB.Where("user_id = ?", userID).First(&inst).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &inst, nil
}

func (s *GithubService) ListRepositories(ctx context.Context, installationID int64) ([]*github.Repository, error) {
	client, err := s.ClientFactory(installationID)
	if err != nil {
		return nil, fmt.Errorf("cannot create github client: %w", err)
	}

	reposResp, _, err := client.Apps.ListRepos(ctx, &github.ListOptions{PerPage: 100})
	if err != nil {
		return nil, fmt.Errorf("github api error: %w", err)
	}

	return reposResp.Repositories, nil
}

func (s *GithubService) GetRepoTree(ctx context.Context, installationID int64, owner, repo string) ([]*github.RepositoryContent, error) {
	client, err := s.ClientFactory(installationID)
	if err != nil {
		return nil, err
	}

	var result []*github.RepositoryContent

	var fetch func(path string) error
	fetch = func(path string) error {
		_, contents, _, err := client.Repositories.GetContents(ctx, owner, repo, path, nil)
		if err != nil {
			return err
		}

		for _, c := range contents {
			if c.GetType() == "dir" {
				result = append(result, c)
				if err := fetch(c.GetPath()); err != nil {
					return err
				}
			}
		}
		return nil
	}

	if err := fetch(""); err != nil {
		return nil, err
	}
	return result, nil
}
