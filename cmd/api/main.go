package main

import (
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/joho/godotenv"
	"github.com/rs/cors"

	api "github.com/flotio-dev/core-api/internal/api"
	"github.com/flotio-dev/core-api/internal/common/crypto"
	db "github.com/flotio-dev/core-api/internal/common/database"
)

// APIVersion is the semantic version of the Flotio Core API. It MUST stay in
// sync with the swag @version annotation above (contract M3/M4).
const APIVersion = "1.0.0"

// Flotio Core API
//
//	@title			Flotio Core API
//	@version		1.0.0
//	@description	Flotio Core API — authentication, environment/keystore/Google Play credentials assets, projects, build pipeline, releases and GitHub integration for the Flotio mobile CI platform.
//	@contact.name	Flotio Team
//	@contact.email	support@flotio.dev
//	@license.name	Proprietary
//
//	@securityDefinitions.apikey	BearerAuth
//	@in							header
//	@name						Authorization
//	@description				Type "Bearer {token}"
func main() {
	godotenv.Load()

	log.Printf("Flotio Core API version %s", APIVersion)

	// Fail closed: refuse to start without a valid secrets encryption key.
	if err := crypto.Init(); err != nil {
		log.Fatalf("Secrets encryption not configured: %v", err)
	}

	db.InitDB()
	db.InitRedis()

	log.Println("Starting Flotio API server")
	r := api.Router()
	log.Println("Router configured")

	originsEnv := os.Getenv("ALLOWED_ORIGINS")
	allowedOrigins := strings.Split(originsEnv, ",")

	c := cors.New(cors.Options{
		AllowedOrigins:   allowedOrigins,
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
