package api

import (
	"net/http"

	_ "github.com/flotio-dev/api/docs/api"
	"github.com/gorilla/mux"
	httpSwagger "github.com/swaggo/http-swagger"

	handlers "github.com/flotio-dev/api/internal/api/handlers"
)

func Router() http.Handler {
	r := mux.NewRouter()

	r.PathPrefix("/docs/").Handler(httpSwagger.WrapHandler)

	// Public auth routes
	r.HandleFunc("/auth/register", handlers.RegisterHandler).Methods("POST")
	r.HandleFunc("/auth/login", handlers.LoginHandler).Methods("POST")
	r.HandleFunc("/auth/refresh", handlers.RefreshTokenHandler).Methods("POST")
	r.HandleFunc("/auth/github/callback", handlers.GithubCallbackHandler).Methods("GET")

	// Health check
	r.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("ok"))
	}).Methods("GET")

	// Protected routes
	protected := r.PathPrefix("/").Subrouter()
	protected.Use(AuthMiddleware)

	// Protected auth routes
	protected.HandleFunc("/auth/@me", handlers.MeGetHandler).Methods("GET")
	protected.HandleFunc("/auth/@me", handlers.MePutHandler).Methods("PUT")

	// Github route (protected)
	protected.HandleFunc("/github", handlers.GithubHandler).Methods("GET")

	// Env routes (by project)
	protected.HandleFunc("/project/{id}/env", handlers.EnvGetHandler).Methods("GET")
	protected.HandleFunc("/project/{id}/env", handlers.EnvPostHandler).Methods("POST")
	protected.HandleFunc("/project/{id}/envs", handlers.EnvGetHandler).Methods("GET")
	protected.HandleFunc("/project/{id}/env/{envId}", handlers.EnvGetByIdHandler).Methods("GET")
	protected.HandleFunc("/project/{id}/env/{envId}", handlers.EnvPutByIdHandler).Methods("PUT")
	protected.HandleFunc("/project/{id}/env/{envId}", handlers.EnvDeleteByIdHandler).Methods("DELETE")

	// Project routes
	protected.HandleFunc("/project", handlers.ProjectsGetHandler).Methods("GET")
	protected.HandleFunc("/project", handlers.ProjectCreateHandler).Methods("POST")
	protected.HandleFunc("/project/{id}", handlers.ProjectGetHandler).Methods("GET")
	protected.HandleFunc("/project/{id}", handlers.ProjectPutHandler).Methods("PUT")
	protected.HandleFunc("/project/{id}", handlers.ProjectDeleteHandler).Methods("DELETE")
	protected.HandleFunc("/project/{id}/build", handlers.ProjectBuildHandler).Methods("POST")

	// Build routes
	protected.HandleFunc("/project/{id}/build/{buildId}/cancel", handlers.BuildCancelHandler).Methods("PUT")
	protected.HandleFunc("/project/{id}/builds", handlers.BuildsListHandler).Methods("GET")
	protected.HandleFunc("/project/{id}/build/{buildId}/logs", handlers.BuildLogsHandler).Methods("GET")
	protected.HandleFunc("/project/{id}/build/{buildId}/logs/ws", handlers.BuildLogsWSHandler).Methods("GET")
	protected.HandleFunc("/project/{id}/build/{buildId}/download", handlers.BuildDownloadHandler).Methods("GET")

	// Github routes
	githubController := handlers.NewGithubController()
	protected.HandleFunc("/github/webhooks", githubController.HandleWebhook)
	protected.HandleFunc("/github/post-installation", githubController.HandleGithubPostInstallation)
	protected.HandleFunc("/github/repos", githubController.HandleGithubGetRepositories).Methods("GET")
	protected.HandleFunc("/github/repo", githubController.HandleGithubRepoTree).Methods("GET")
	protected.HandleFunc("/github/installations", githubController.HandleGithubCheckInstallation).Methods("GET")

	return r
}
