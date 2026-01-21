package service

import (
	"context"
	"errors"

	userModel "github.com/flotio-dev/core-api/internal/modules/user/model"

	dbEngine "github.com/flotio-dev/core-api/internal/common/database"
	repositories "github.com/flotio-dev/core-api/internal/modules/user/repository"
)

type UserService struct {
	Repository *repositories.UserRepository
}

func NewUserService(repository *repositories.UserRepository) *UserService {
	return &UserService{
		Repository: repository,
	}
}

type contextKey string

const UserContextKey contextKey = "user"

func GetUserFromContext(ctx context.Context) *userModel.UserContext {
	if user, ok := ctx.Value(UserContextKey).(*userModel.UserContext); ok {
		return user
	}
	return nil
}

func (s *UserService) UpdateUser(user *dbEngine.User, updateRequest *userModel.UserUpdateRequest) error {
	if user == nil {
		return errors.New("user context is invalid")
	}

	return s.Repository.UpdateUser(user, updateRequest)
}