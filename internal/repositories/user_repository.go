package repositories

import (
	"errors"

	dbEngine "github.com/flotio-dev/api/internal/engines/db"
	"github.com/flotio-dev/api/internal/models"
	"gorm.io/gorm"
)

type UserRepository struct {
	DB *gorm.DB
}

func NewUserRepository(db *gorm.DB) *UserRepository {
	return &UserRepository{
		DB: db,
	}
}

func (r *UserRepository) UpdateUser(user *dbEngine.User, updateRequest *models.UserUpdateRequest) error {
	updates := map[string]interface{}{}

	if updateRequest.Email != nil {
		updates["email"] = *updateRequest.Email
	}
	if updateRequest.Username != nil {
		updates["username"] = *updateRequest.Username
	}
	if updateRequest.GithubID != nil {
		updates["github_id"] = *updateRequest.GithubID
	}
	if updateRequest.GithubUsername != nil {
		updates["github_username"] = *updateRequest.GithubUsername
	}

	if len(updates) == 0 {
		return errors.New("no fields to update")
	}

	return r.DB.Model(&dbEngine.User{}).Where("id = ?", user.ID).Updates(updates).Error
}
