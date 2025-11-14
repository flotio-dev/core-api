package middleware

import (
	"context"
	"net/http"
	"os"

	"github.com/Nerzal/gocloak/v13"
	db "github.com/flotio-dev/api/pkg/db"
	utils "github.com/flotio-dev/api/pkg/utils"
)

type contextKey string

type UserContext struct {
	Keycloak *gocloak.UserInfo
	DB       *db.User
}

const userContextKey contextKey = "user"

func AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" || len(authHeader) < 7 || authHeader[:7] != "Bearer " {
			next.ServeHTTP(w, r)
			return
		}
		token := authHeader[7:]

		client := utils.GetKeycloakClient()
		ctx := context.Background()
		realm := os.Getenv("KEYCLOAK_REALM")

		// Get user info from token
		userInfo, err := client.GetUserInfo(ctx, token, realm)
		if err != nil {
			next.ServeHTTP(w, r)
			return
		}

		// Cherche l'utilisateur correspondant dans la DB
		var user db.User
		if err := db.DB.Where("keycloak_id = ?", userInfo.Sub).First(&user).Error; err != nil {
			// Si pas trouvé par keycloak_id, essaie avec email
			if err := db.DB.Where("email = ?", userInfo.Email).First(&user).Error; err != nil {
				next.ServeHTTP(w, r)
				return
			}
		}

		// Combine les infos
		combined := &UserContext{
			Keycloak: userInfo,
			DB:       &user,
		}

		// Add user info to context
		ctxWithUser := context.WithValue(r.Context(), userContextKey, combined)
		r = r.WithContext(ctxWithUser)

		next.ServeHTTP(w, r)
	})
}

func GetUserFromContext(ctx context.Context) *UserContext {
	if user, ok := ctx.Value(userContextKey).(*UserContext); ok {
		return user
	}
	return nil
}
