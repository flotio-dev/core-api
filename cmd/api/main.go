package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"github.com/joho/godotenv"
	"github.com/rs/cors"

	api "github.com/flotio-dev/core-api/internal/api"
	db "github.com/flotio-dev/core-api/internal/common/database"
	keycloakEngine "github.com/flotio-dev/core-api/internal/infra/keycloak"
)

// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
// @description Type "Bearer {token}"
func main() {
	godotenv.Load()

	db.InitDB()

	// Sync users with Keycloak on startup
	syncUsersWithKeycloak()

	log.Println("Starting Flotio API server")
	r := api.Router()
	log.Println("Router configured")

	c := cors.New(cors.Options{
		AllowedOrigins:   []string{"http://localhost:3000"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Content-Type", "Authorization"},
		AllowCredentials: true,
	})

	handler := c.Handler(r)

	port := os.Getenv("API_PORT")
	if port == "" {
		port = "8080"
	}
	addr := ":" + port

	srv := &http.Server{
		Addr:    addr,
		Handler: handler,
	}

	log.Printf("Listening on %s", srv.Addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server failed: %v", err)
	}
}

// syncUsersWithKeycloak fetches users from Keycloak and removes local users
// that no longer exist in Keycloak
func syncUsersWithKeycloak() {
	ctx := context.Background()

	log.Println("Starting user synchronization with Keycloak...")

	keycloakUserIDs, err := keycloakEngine.GetKeycloakUserIDs(ctx)
	if err != nil {
		log.Printf("Warning: Failed to fetch Keycloak users for sync: %v", err)
		log.Println("Continuing without user sync - Keycloak may be unavailable")
		return
	}

	if err := db.SyncUsersWithKeycloak(ctx, keycloakUserIDs); err != nil {
		log.Printf("Warning: Failed to sync users with Keycloak: %v", err)
		return
	}

	log.Println("User synchronization with Keycloak completed")
}
