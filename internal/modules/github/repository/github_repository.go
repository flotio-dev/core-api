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

func (r *GithubRepository) SaveInstallation(userID uint, installationID int64, accountLogin, accountType string, targetID int64, avatarURL string) error {
	return r.DB.Transaction(func(tx *gorm.DB) error {
		var existing dbEngine.GithubInstallation
		err := tx.Unscoped().Where("user_id = ? AND installation_id = ?", userID, installationID).First(&existing).Error
		if err == nil {
			// Update existing record
			existing.AccountLogin = accountLogin
			existing.AccountType = accountType
			existing.TargetID = targetID
			if avatarURL != "" {
				existing.AvatarURL = avatarURL
			}
			existing.DeletedAt = gorm.DeletedAt{}
			return tx.Save(&existing).Error
		}

		inst := dbEngine.GithubInstallation{
			InstallationID: installationID,
			UserID:         &userID,
			AccountLogin:   accountLogin,
			AccountType:    accountType,
			TargetID:       targetID,
			AvatarURL:      avatarURL,
		}

		return tx.Create(&inst).Error
	})
}

func (r *GithubRepository) GetInstallationByUser(userID uint) (*dbEngine.GithubInstallation, error) {
	var inst dbEngine.GithubInstallation
	err := r.DB.Where("user_id = ?", userID).Order("id desc").First(&inst).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &inst, nil
}

func (r *GithubRepository) ListInstallationsByUser(userID uint) ([]dbEngine.GithubInstallation, error) {
	var insts []dbEngine.GithubInstallation
	err := r.DB.Where("user_id = ?", userID).Order("id asc").Find(&insts).Error
	if err != nil {
		return nil, err
	}
	return insts, nil
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

func (r *GithubRepository) DeleteUserInstallationByID(userID uint, installationID int64) error {
	return r.DB.Unscoped().Where("user_id = ? AND installation_id = ?", userID, installationID).Delete(&dbEngine.GithubInstallation{}).Error
}

func (r *GithubRepository) CountOtherInstallations(installationID int64, excludeUserID uint) (int64, error) {
	var count int64
	err := r.DB.Model(&dbEngine.GithubInstallation{}).
		Where("installation_id = ? AND user_id != ?", installationID, excludeUserID).
		Count(&count).Error
	return count, err
}
