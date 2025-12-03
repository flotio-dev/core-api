package services

import (
	"context"

	middlewareModel "github.com/flotio-dev/api/internal/models"
)

type contextKey string

const UserContextKey contextKey = "user"

func GetUserFromContext(ctx context.Context) *middlewareModel.UserContext {
	if user, ok := ctx.Value(UserContextKey).(*middlewareModel.UserContext); ok {
		return user
	}
	return nil
}
