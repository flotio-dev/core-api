package router

import (
	"fmt"
	"net/http"
	"os"

	"github.com/gorilla/mux"

	controller "github.com/flotio-dev/api/pkg/api/v1/controller"
	middleware "github.com/flotio-dev/api/pkg/api/v1/middleware"
)

func Router() http.Handler {
	r := mux.NewRouter()

	// Public auth routes
	r.HandleFunc("/auth/register", controller.RegisterHandler).Methods("POST")
	r.HandleFunc("/auth/login", controller.LoginHandler).Methods("POST")
	r.HandleFunc("/auth/refresh", controller.RefreshTokenHandler).Methods("POST")
	r.HandleFunc("/auth/github/callback", controller.GithubCallbackHandler).Methods("GET")

	// Health check
	r.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("ok"))
	}).Methods("GET")

	// Protected routes
	protected := r.PathPrefix("/").Subrouter()
	protected.Use(middleware.AuthMiddleware)

	// Protected auth routes
	protected.HandleFunc("/auth/@me", controller.MeGetHandler).Methods("GET")
	protected.HandleFunc("/auth/@me", controller.MePutHandler).Methods("PUT")

	// Github route (protected)
	protected.HandleFunc("/github", controller.GithubHandler).Methods("GET")

	// Env routes (by project)
	protected.HandleFunc("/project/{id}/env", controller.EnvGetHandler).Methods("GET")
	protected.HandleFunc("/project/{id}/env", controller.EnvPostHandler).Methods("POST")
	protected.HandleFunc("/project/{id}/envs", controller.EnvGetHandler).Methods("GET")
	protected.HandleFunc("/project/{id}/env/{envId}", controller.EnvGetByIdHandler).Methods("GET")
	protected.HandleFunc("/project/{id}/env/{envId}", controller.EnvPutByIdHandler).Methods("PUT")
	protected.HandleFunc("/project/{id}/env/{envId}", controller.EnvDeleteByIdHandler).Methods("DELETE")

	// Project routes
	protected.HandleFunc("/project", controller.ProjectsGetHandler).Methods("GET")
	protected.HandleFunc("/project", controller.ProjectCreateHandler).Methods("POST")
	protected.HandleFunc("/project/{id}", controller.ProjectGetHandler).Methods("GET")
	protected.HandleFunc("/project/{id}", controller.ProjectPutHandler).Methods("PUT")
	protected.HandleFunc("/project/{id}", controller.ProjectDeleteHandler).Methods("DELETE")
	protected.HandleFunc("/project/{id}/build", controller.ProjectBuildHandler).Methods("POST")

	// Build routes
	protected.HandleFunc("/project/{id}/build/{buildId}/cancel", controller.BuildCancelHandler).Methods("PUT")
	protected.HandleFunc("/project/{id}/builds", controller.BuildsListHandler).Methods("GET")
	protected.HandleFunc("/project/{id}/build/{buildId}/logs", controller.BuildLogsHandler).Methods("GET")
	protected.HandleFunc("/project/{id}/build/{buildId}/logs/ws", controller.BuildLogsWSHandler).Methods("GET")
	protected.HandleFunc("/project/{id}/build/{buildId}/download", controller.BuildDownloadHandler).Methods("GET")

	// Github routes
	fmt.Printf("Webhook secret: '%s'\n", os.Getenv("GITHUB_WEBHOOK_SECRET"))
	githubController := controller.NewGithubController([]byte(os.Getenv("GITHUB_WEBHOOK_SECRET")))
	protected.HandleFunc("/github/webhooks", githubController.HandleWebhook)

	return r
}
