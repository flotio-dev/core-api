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
	protected.HandleFunc("/project/{id}/env", projectHandler.EnvGetHandler).Methods("GET")
	protected.HandleFunc("/project/{id}/env", projectHandler.EnvPostHandler).Methods("POST")
	protected.HandleFunc("/project/{id}/envs", projectHandler.EnvGetHandler).Methods("GET")
	protected.HandleFunc("/project/{id}/env/{envId}", projectHandler.EnvGetByIdHandler).Methods("GET")
	protected.HandleFunc("/project/{id}/env/{envId}", projectHandler.EnvPutByIdHandler).Methods("PUT")
	protected.HandleFunc("/project/{id}/env/{envId}", projectHandler.EnvDeleteByIdHandler).Methods("DELETE")

	// Project routes (Project Module)
	protected.HandleFunc("/project", projectHandler.ProjectsGetHandler).Methods("GET")
	protected.HandleFunc("/project", projectHandler.ProjectCreateHandler).Methods("POST")
	protected.HandleFunc("/project/{id}", projectHandler.ProjectGetHandler).Methods("GET")
	protected.HandleFunc("/project/{id}", projectHandler.ProjectPutHandler).Methods("PUT")
	protected.HandleFunc("/project/{id}", projectHandler.ProjectDeleteHandler).Methods("DELETE")

	// Build routes (Build Module)
	// Inject dependencies into Build Controller
	buildCtrl := buildHandler.NewBuildController(ghService)
	protected.HandleFunc("/project/{id}/build", buildCtrl.ProjectBuildHandler).Methods("POST")
	protected.HandleFunc("/project/{id}/build/{buildId}", buildHandler.BuildDeleteHandler).Methods("DELETE")
	protected.HandleFunc("/project/{id}/build/{buildId}/cancel", buildHandler.BuildCancelHandler).Methods("PUT")
	protected.HandleFunc("/project/{id}/builds", buildHandler.BuildsListHandler).Methods("GET")
	protected.HandleFunc("/project/{id}/build/{buildId}/logs", buildHandler.BuildLogsHandler).Methods("GET")
	protected.HandleFunc("/project/{id}/build/{buildId}/download", buildHandler.BuildDownloadHandler).Methods("GET")
	protected.HandleFunc("/project/{id}/build/{buildId}/logs/sync", buildHandler.BuildLogsSyncHandler).Methods("GET")

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
