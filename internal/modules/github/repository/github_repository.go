package repository

import (
	"errors"

	dbEngine "github.com/flotio-dev/core-api/internal/common/database"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type GithubRepository struct {
	DB *gorm.DB
}

func NewGithubRepository(db *gorm.DB) *GithubRepository {
	return &GithubRepository{
		DB: db,
	}
}

func (r *GithubRepository) SaveInstallation(userID uint, installationID int64, accountLogin, accountType string, targetID int64) error {
	inst := dbEngine.GithubInstallation{
		InstallationID: installationID,
		UserID:         &userID,
		AccountLogin:   accountLogin,
		AccountType:    accountType,
		TargetID:       targetID,
	}

	return r.DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "installation_id"}},
		UpdateAll: true,
	}).Create(&inst).Error
}

func (r *GithubRepository) GetInstallationByUser(userID uint) (*dbEngine.GithubInstallation, error) {
	var inst dbEngine.GithubInstallation
	err := r.DB.Where("user_id = ?", userID).First(&inst).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &inst, nil
}

func (r *GithubRepository) GetGithubInstallationByInstallationID(installationID int64) (*dbEngine.GithubInstallation, error) {
	var inst dbEngine.GithubInstallation
	if err := r.DB.Where("installation_id = ?", installationID).First(&inst).Error; err != nil {
		return nil, err
	}
	return &inst, nil
}

func (r *GithubRepository) DeleteInstallationByInstallationID(installationID int64) error {
	return r.DB.Where("installation_id = ?", installationID).Delete(&dbEngine.GithubInstallation{}).Error
}
