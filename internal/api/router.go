package api

import (
	"net/http"

	_ "github.com/flotio-dev/core-api/docs/api"
	"github.com/gorilla/mux"
	httpSwagger "github.com/swaggo/http-swagger"

	dbEngine "github.com/flotio-dev/core-api/internal/common/database"
	githubEngine "github.com/flotio-dev/core-api/internal/infra/github"

	buildHandler "github.com/flotio-dev/core-api/internal/modules/build/handler"
	githubHandler "github.com/flotio-dev/core-api/internal/modules/github/handler"
	githubRepo "github.com/flotio-dev/core-api/internal/modules/github/repository"
	githubService "github.com/flotio-dev/core-api/internal/modules/github/service"
	projectHandler "github.com/flotio-dev/core-api/internal/modules/project/handler"
	userHandler "github.com/flotio-dev/core-api/internal/modules/user/handler"
	userRepo "github.com/flotio-dev/core-api/internal/modules/user/repository"
	userService "github.com/flotio-dev/core-api/internal/modules/user/service"
)

func Router() http.Handler {

	// Inject dependencies
	// GitHub Module
	ghRepository := githubRepo.NewGithubRepository(dbEngine.DB)
	ghClientManager, err := githubEngine.NewGitHubClientManager()
	if err != nil {
		panic("Failed to create GitHub Client Manager: " + err.Error())
	}
	ghService := githubService.NewGithubService(
		ghRepository,
		ghClientManager,
	)

	// User Module
	uRepository := userRepo.NewUserRepository(dbEngine.DB)
	uService := userService.NewUserService(uRepository)

	r := mux.NewRouter()

	r.PathPrefix("/docs/").Handler(httpSwagger.WrapHandler)

	// Public auth routes (User Module)
	r.HandleFunc("/auth/register", userHandler.RegisterHandler).Methods("POST")
	r.HandleFunc("/auth/login", userHandler.LoginHandler).Methods("POST")
	r.HandleFunc("/auth/refresh", userHandler.RefreshTokenHandler).Methods("POST")
	r.HandleFunc("/auth/logout", userHandler.LogoutHandler).Methods("POST")

	// Health check
	r.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("ok"))
	}).Methods("GET")

	// Protected routes
	protected := r.PathPrefix("/").Subrouter()
	protected.Use(userHandler.AuthMiddleware)

	// Protected auth routes (User Module)
	protected.HandleFunc("/auth/@me", userHandler.MeGetHandler).Methods("GET")
	protected.HandleFunc("/auth/@me", userHandler.MePutHandler).Methods("PUT")

	// Env routes (by project) (Project Module)
	EnvCtrl := projectHandler.NewEnvController(uService)
	protected.HandleFunc("/project/{id}/env", EnvCtrl.EnvGetHandler).Methods("GET")
	protected.HandleFunc("/project/{id}/env", EnvCtrl.EnvPostHandler).Methods("POST")
	protected.HandleFunc("/project/{id}/envs", EnvCtrl.EnvGetHandler).Methods("GET")
	protected.HandleFunc("/project/{id}/env/{envId}", EnvCtrl.EnvGetByIdHandler).Methods("GET")
	protected.HandleFunc("/project/{id}/env/{envId}", EnvCtrl.EnvPutByIdHandler).Methods("PUT")
	protected.HandleFunc("/project/{id}/env/{envId}", EnvCtrl.EnvDeleteByIdHandler).Methods("DELETE")

	// Project routes (Project Module)
	projectCtrl := projectHandler.NewProjectController(uService)
	protected.HandleFunc("/project", projectCtrl.ProjectsGetHandler).Methods("GET")
	protected.HandleFunc("/project", projectCtrl.ProjectCreateHandler).Methods("POST")
	protected.HandleFunc("/project/{id}", projectCtrl.ProjectGetHandler).Methods("GET")
	protected.HandleFunc("/project/{id}", projectCtrl.ProjectPutHandler).Methods("PUT")
	protected.HandleFunc("/project/{id}", projectCtrl.ProjectDeleteHandler).Methods("DELETE")

	// Build routes (Build Module)
	// Inject dependencies into Build Controller
	buildCtrl := buildHandler.NewBuildController(ghService, uService)
	protected.HandleFunc("/project/{id}/build", buildCtrl.ProjectBuildHandler).Methods("POST")
	protected.HandleFunc("/project/{id}/build/{buildId}", buildCtrl.BuildDeleteHandler).Methods("DELETE")
	protected.HandleFunc("/project/{id}/build/{buildId}/cancel", buildCtrl.BuildCancelHandler).Methods("PUT")
	protected.HandleFunc("/project/{id}/builds", buildCtrl.BuildsListHandler).Methods("GET")
	protected.HandleFunc("/project/{id}/build/{buildId}/logs", buildCtrl.BuildLogsHandler).Methods("GET")
	protected.HandleFunc("/project/{id}/build/{buildId}/download", buildCtrl.BuildDownloadHandler).Methods("GET")
	protected.HandleFunc("/project/{id}/build/{buildId}/logs/sync", buildCtrl.BuildLogsSyncHandler).Methods("GET")
	protected.HandleFunc("/project/{id}/cache", buildCtrl.CachePurgeHandler).Methods("DELETE")
	protected.HandleFunc("/project/{id}/cache/metrics", buildCtrl.CacheMetricsHandler).Methods("GET")
	protected.HandleFunc("/project/{id}/cache/entries", buildCtrl.CacheEntriesHandler).Methods("GET")

	// Github routes (Github Module)
	// Inject dependencies into Github Controller
	githubCtrl := githubHandler.NewGithubController(ghService, uService)
	protected.HandleFunc("/github/post-installation", githubCtrl.HandleGithubPostInstallation)
	protected.HandleFunc("/github/repos", githubCtrl.HandleGithubGetRepositories).Methods("GET")
	protected.HandleFunc("/github/repo", githubCtrl.HandleGithubRepoTree).Methods("GET")
	protected.HandleFunc("/github/installations", githubCtrl.HandleGithubCheckInstallation).Methods("GET")
	protected.HandleFunc("/github/installation", githubCtrl.HandleGetGithubInstallation).Methods("GET")
	protected.HandleFunc("/github/disconnect", githubCtrl.HandleDisconnectGithub).Methods("DELETE")

	return r
}
