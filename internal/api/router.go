package api

import (
	"net/http"

	_ "github.com/flotio-dev/api/docs/api"
	"github.com/gorilla/mux"
	httpSwagger "github.com/swaggo/http-swagger"

	handlers "github.com/flotio-dev/api/internal/api/handlers"
	repositories "github.com/flotio-dev/api/internal/repositories"
	services "github.com/flotio-dev/api/internal/services"

	dbEngine "github.com/flotio-dev/api/internal/engines/db"
	githubEngine "github.com/flotio-dev/api/internal/engines/github"
)

func Router() http.Handler {

	// Inject dependencies
	githubRepository := repositories.NewGithubRepository(dbEngine.DB)
	githubClientManager, err := githubEngine.NewGitHubClientManager()
	if err != nil {
		panic("Failed to create GitHub Client Manager: " + err.Error())
	}
	githubService := services.NewGithubService(
		githubRepository,
		githubClientManager,
	)

	userRepository := repositories.NewUserRepository(dbEngine.DB)
	userService := services.NewUserService(userRepository)

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
	ProjectController := handlers.NewProjectController(githubService)
	protected.HandleFunc("/project", handlers.ProjectsGetHandler).Methods("GET")
	protected.HandleFunc("/project", handlers.ProjectCreateHandler).Methods("POST")
	protected.HandleFunc("/project/{id}", handlers.ProjectGetHandler).Methods("GET")
	protected.HandleFunc("/project/{id}", handlers.ProjectPutHandler).Methods("PUT")
	protected.HandleFunc("/project/{id}", handlers.ProjectDeleteHandler).Methods("DELETE")
	protected.HandleFunc("/project/{id}/build", ProjectController.ProjectBuildHandler).Methods("POST")

	// Build routes
	protected.HandleFunc("/project/{id}/build/{buildId}/cancel", handlers.BuildCancelHandler).Methods("PUT")
	protected.HandleFunc("/project/{id}/builds", handlers.BuildsListHandler).Methods("GET")
	protected.HandleFunc("/project/{id}/build/{buildId}/logs", handlers.BuildLogsHandler).Methods("GET")

	protected.HandleFunc("/project/{id}/build/{buildId}/download", handlers.BuildDownloadHandler).Methods("GET")
	protected.HandleFunc("/project/{id}/build/{buildId}/logs/sync", handlers.BuildLogsSyncHandler).Methods("GET")
	// Github routes
	githubController := handlers.NewGithubController(githubService, userService)
	protected.HandleFunc("/github/post-installation", githubController.HandleGithubPostInstallation)
	protected.HandleFunc("/github/repos", githubController.HandleGithubGetRepositories).Methods("GET")
	protected.HandleFunc("/github/repo", githubController.HandleGithubRepoTree).Methods("GET")
	protected.HandleFunc("/github/installations", githubController.HandleGithubCheckInstallation).Methods("GET")
	protected.HandleFunc("/github/installation", githubController.HandleGetGithubInstallation).Methods("GET")

	return r
}
