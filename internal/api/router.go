package api

import (
	"log"
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
	releaseHandler "github.com/flotio-dev/core-api/internal/modules/release/handler"
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
		log.Printf("[github] GitHub Client Manager disabled: %v", err)
		ghClientManager = &githubEngine.GitHubClientManager{}
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
	r.HandleFunc("/healthz", HealthzHandler).Methods("GET")

	// Protected routes
	protected := r.PathPrefix("/").Subrouter()
	protected.Use(userHandler.AuthMiddleware)

	// Flutter routes
	flutterCtrl := projectHandler.NewFlutterController()
	protected.HandleFunc("/flutter/versions", flutterCtrl.VersionsGetHandler).Methods("GET")

	// Protected auth routes (User Module)
	protected.HandleFunc("/auth/@me", userHandler.MeGetHandler).Methods("GET")
	protected.HandleFunc("/auth/@me", userHandler.MePutHandler).Methods("PUT")

	// Env routes (User Module - Assets)
	EnvCtrl := projectHandler.NewEnvController(uService)
	protected.HandleFunc("/env", EnvCtrl.EnvGetHandler).Methods("GET")
	protected.HandleFunc("/env", EnvCtrl.EnvPostHandler).Methods("POST")
	protected.HandleFunc("/envs", EnvCtrl.EnvsGetHandler).Methods("GET")
	protected.HandleFunc("/env/{envId:[0-9]+}", EnvCtrl.EnvGetByIdHandler).Methods("GET")
	protected.HandleFunc("/env/{envId:[0-9]+}", EnvCtrl.EnvPutByIdHandler).Methods("PUT")
	protected.HandleFunc("/env/{envId:[0-9]+}", EnvCtrl.EnvDeleteByIdHandler).Methods("DELETE")

	// Project routes (Project Module)
	projectCtrl := projectHandler.NewProjectController(uService)
	configCtrl := projectHandler.NewConfigController(uService)
	protected.HandleFunc("/project", projectCtrl.ProjectsGetHandler).Methods("GET")
	protected.HandleFunc("/project", projectCtrl.ProjectCreateHandler).Methods("POST")
	protected.HandleFunc("/project/{id:[0-9]+}", projectCtrl.ProjectGetHandler).Methods("GET")
	protected.HandleFunc("/project/{id:[0-9]+}", projectCtrl.ProjectPutHandler).Methods("PUT")
	protected.HandleFunc("/project/{id:[0-9]+}", projectCtrl.ProjectDeleteHandler).Methods("DELETE")
	protected.HandleFunc("/project/{id:[0-9]+}/config", configCtrl.ConfigGetHandler).Methods("GET")
	protected.HandleFunc("/project/{id:[0-9]+}/config", configCtrl.ConfigPostHandler).Methods("POST")
	protected.HandleFunc("/project/{id:[0-9]+}/config", configCtrl.ConfigDeleteHandler).Methods("DELETE")

	// Keystore routes (User Module - Assets)
	keystoreCtrl := projectHandler.NewKeystoreController(uService)
	protected.HandleFunc("/keystore", keystoreCtrl.KeystoreGetHandler).Methods("GET")
	protected.HandleFunc("/keystore", keystoreCtrl.KeystorePostHandler).Methods("POST")
	protected.HandleFunc("/keystores", keystoreCtrl.KeystoresGetHandler).Methods("GET")
	protected.HandleFunc("/keystore/{keystoreId:[0-9]+}", keystoreCtrl.KeystoreDeleteHandler).Methods("DELETE")

	// Google Play Credentials routes (User Module - Assets)
	googlePlayCtrl := projectHandler.NewGooglePlayCredentialsController(uService)
	protected.HandleFunc("/google-play-credentials", googlePlayCtrl.GooglePlayCredentialsGetHandler).Methods("GET")
	protected.HandleFunc("/google-play-credentials", googlePlayCtrl.GooglePlayCredentialsPostHandler).Methods("POST")
	protected.HandleFunc("/google-play-credentials/{credentialsId:[0-9]+}", googlePlayCtrl.GooglePlayCredentialsDeleteHandler).Methods("DELETE")

	// Build routes (Build Module)
	// Inject dependencies into Build Controller
	buildCtrl := buildHandler.NewBuildController(ghService, uService)
	protected.HandleFunc("/project/{id:[0-9]+}/build", buildCtrl.ProjectBuildHandler).Methods("POST")
	protected.HandleFunc("/project/{id:[0-9]+}/build/{buildId:[0-9]+}", buildCtrl.BuildDeleteHandler).Methods("DELETE")
	protected.HandleFunc("/project/{id:[0-9]+}/build/{buildId:[0-9]+}/cancel", buildCtrl.BuildCancelHandler).Methods("PUT")
	protected.HandleFunc("/project/{id:[0-9]+}/builds", buildCtrl.BuildsListHandler).Methods("GET")
	protected.HandleFunc("/project/{id:[0-9]+}/build/{buildId:[0-9]+}/logs", buildCtrl.BuildLogsHandler).Methods("GET")
	protected.HandleFunc("/project/{id:[0-9]+}/build/{buildId:[0-9]+}/download", buildCtrl.BuildDownloadHandler).Methods("GET")
	protected.HandleFunc("/project/{id:[0-9]+}/build/{buildId:[0-9]+}/logs/sync", buildCtrl.BuildLogsSyncHandler).Methods("GET")
	protected.HandleFunc("/project/{id:[0-9]+}/cache", buildCtrl.CachePurgeHandler).Methods("DELETE")
	protected.HandleFunc("/project/{id:[0-9]+}/cache/metrics", buildCtrl.CacheMetricsHandler).Methods("GET")
	protected.HandleFunc("/project/{id:[0-9]+}/cache/entries", buildCtrl.CacheEntriesHandler).Methods("GET")

	// Release routes (Release Module - Google Play publishing)
	releaseCtrl := releaseHandler.NewReleaseController(uService)
	protected.HandleFunc("/project/{id:[0-9]+}/build/{buildId:[0-9]+}/publish", releaseCtrl.PublishHandler).Methods("POST")
	protected.HandleFunc("/project/{id:[0-9]+}/release/{releaseId:[0-9]+}", releaseCtrl.ReleaseGetHandler).Methods("GET")
	protected.HandleFunc("/project/{id:[0-9]+}/releases", releaseCtrl.ReleasesListHandler).Methods("GET")
	protected.HandleFunc("/project/{id:[0-9]+}/google-play/access", releaseCtrl.AccessCheckHandler).Methods("GET")
	protected.HandleFunc("/project/{id:[0-9]+}/audit", releaseCtrl.AuditListHandler).Methods("GET")

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
