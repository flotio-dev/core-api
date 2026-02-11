package handler

import (
	"context"
	"net/http"
	"strings"

	helpers "github.com/flotio-dev/core-api/internal/common/server"
	authModel "github.com/flotio-dev/core-api/internal/modules/user/model"
	authServices "github.com/flotio-dev/core-api/internal/modules/user/service"
	"github.com/golang-jwt/jwt/v5"
)

func AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			helpers.WriteErrorJSON(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		tokenStr := strings.TrimPrefix(auth, "Bearer ")

		token, err := jwt.ParseWithClaims(
			tokenStr,
			&authModel.AccessClaims{},
			func(t *jwt.Token) (interface{}, error) {
				return authServices.AccessSecret, nil
			},
		)
		if err != nil || !token.Valid {
			helpers.WriteErrorJSON(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		claims := token.Claims.(*authModel.AccessClaims)
		ctx := context.WithValue(r.Context(), "user_id", claims.UserID)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
