package repository

import (
	"errors"

	dbEngine "github.com/flotio-dev/core-api/internal/common/database"
	"gorm.io/gorm"
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
	return r.DB.Transaction(func(tx *gorm.DB) error {
		// Supprimer toute installation existante pour cet utilisateur
		// (y compris soft-deleted pour éviter les violations de contrainte UNIQUE sur user_id)
		if err := tx.Unscoped().
			Where("user_id = ?", userID).
			Delete(&dbEngine.GithubInstallation{}).Error; err != nil {
			return err
		}

		inst := dbEngine.GithubInstallation{
			InstallationID: installationID,
			UserID:         &userID,
			AccountLogin:   accountLogin,
			AccountType:    accountType,
			TargetID:       targetID,
		}

		return tx.Create(&inst).Error
	})
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
	err := r.DB.Where("installation_id = ?", installationID).First(&inst).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &inst, nil
}

func (r *GithubRepository) DeleteInstallationByInstallationID(installationID int64) error {
	return r.DB.Unscoped().Where("installation_id = ?", installationID).Delete(&dbEngine.GithubInstallation{}).Error
}

func (r *GithubRepository) DeleteInstallationByUser(userID uint) error {
	return r.DB.Unscoped().Where("user_id = ?", userID).Delete(&dbEngine.GithubInstallation{}).Error
}

func (r *GithubRepository) CountOtherInstallations(installationID int64, excludeUserID uint) (int64, error) {
	var count int64
	err := r.DB.Model(&dbEngine.GithubInstallation{}).
		Where("installation_id = ? AND user_id != ?", installationID, excludeUserID).
		Count(&count).Error
	return count, err
}
