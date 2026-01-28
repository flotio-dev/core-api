package handler

import (
	"context"
	"net/http"
	"os"

	dbEngine "github.com/flotio-dev/core-api/internal/common/database"
	keycloakEngine "github.com/flotio-dev/core-api/internal/infra/keycloak"
	models "github.com/flotio-dev/core-api/internal/modules/user/model"
	services "github.com/flotio-dev/core-api/internal/modules/user/service"
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
				// Utilisateur non trouvé en base, le créer automatiquement
				username := ""
				if userInfo.PreferredUsername != nil {
					username = *userInfo.PreferredUsername
				}
				email := ""
				if userInfo.Email != nil {
					email = *userInfo.Email
				}
				keycloakID := ""
				if userInfo.Sub != nil {
					keycloakID = *userInfo.Sub
				}

				user = dbEngine.User{
					KeycloakID: keycloakID,
					Email:      email,
					Username:   username,
				}

				if err := dbEngine.DB.Create(&user).Error; err != nil {
					// Si la création échoue, on continue sans utilisateur
					next.ServeHTTP(w, r)
					return
				}
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
