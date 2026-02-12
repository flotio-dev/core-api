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

func (s *UserService) GetUserFromContext(ctx context.Context) (*dbEngine.User, error) {
	id, ok := ctx.Value("user_id").(uint)
	if !ok {
		return nil, errors.New("user id not found in context")
	}

	user, err := s.Repository.GetUserByID(id)
	if err != nil {
		return nil, err
	}

	return user, nil
}

func (s *UserService) UpdateUser(user *dbEngine.User, updateRequest *userModel.UserUpdateRequest) error {
	if user == nil {
		return errors.New("user context is invalid")
	}

	return s.Repository.UpdateUser(user, updateRequest)
}
