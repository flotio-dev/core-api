package api

import (
	"context"
	"net/http"
	"os"

	dbEngine "github.com/flotio-dev/api/internal/engines/db"
	keycloakEngine "github.com/flotio-dev/api/internal/engines/keycloak"
	models "github.com/flotio-dev/api/internal/models"
	services "github.com/flotio-dev/api/internal/services"
)

func AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" || len(authHeader) < 7 || authHeader[:7] != "Bearer " {
			next.ServeHTTP(w, r)
			return
		}
		token := authHeader[7:]

		client := keycloakEngine.GetKeycloakClient()
		ctx := context.Background()
		realm := os.Getenv("KEYCLOAK_REALM")

		// Get user info from token
		userInfo, err := client.GetUserInfo(ctx, token, realm)
		if err != nil {
			next.ServeHTTP(w, r)
			return
		}

		// Cherche l'utilisateur correspondant dans la DB
		var user dbEngine.User
		if err := dbEngine.DB.Where("keycloak_id = ?", userInfo.Sub).First(&user).Error; err != nil {
			// Si pas trouvé par keycloak_id, essaie avec email
			if err := dbEngine.DB.Where("email = ?", userInfo.Email).First(&user).Error; err != nil {
				next.ServeHTTP(w, r)
				return
			}
		}

		// Combine les infos
		combined := &models.UserContext{
			Keycloak: userInfo,
			DB:       &user,
		}

		// Add user info to context
		ctxWithUser := context.WithValue(r.Context(), services.UserContextKey, combined)
		r = r.WithContext(ctxWithUser)

		next.ServeHTTP(w, r)
	})
}
