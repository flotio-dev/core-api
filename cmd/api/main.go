package main

import (
	"log"
	"net/http"
	"os"

	"github.com/joho/godotenv"
	"github.com/rs/cors"

	api "github.com/flotio-dev/core-api/internal/api"
	db "github.com/flotio-dev/core-api/internal/common/database"
)

// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
// @description Type "Bearer {token}"
func main() {
	godotenv.Load()

	db.InitDB()
	db.InitRedis()

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
