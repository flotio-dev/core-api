package services

import (
	"context"
	"errors"

	middlewareModel "github.com/flotio-dev/api/internal/models"

	dbEngine "github.com/flotio-dev/api/internal/engines/db"
	repositories "github.com/flotio-dev/api/internal/repositories"
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

func GetUserFromContext(ctx context.Context) *middlewareModel.UserContext {
	if user, ok := ctx.Value(UserContextKey).(*middlewareModel.UserContext); ok {
		return user
	}
	return nil
}

func (s *UserService) UpdateUser(user *dbEngine.User, updateRequest *middlewareModel.UserUpdateRequest) error {
	if user == nil {
		return errors.New("user context is invalid")
	}

	return s.Repository.UpdateUser(user, updateRequest)
}
