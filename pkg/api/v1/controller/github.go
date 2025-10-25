package controller

import (
	"fmt"
	"net/http"

	"github.com/google/go-github/v76/github"
	"gorm.io/gorm/clause"

	// middleware "github.com/flotio-dev/api/pkg/api/v1/middleware"
	db "github.com/flotio-dev/api/pkg/db"
)

type GithubController struct {
	webhookSecretKey []byte
}

func NewGithubController(secret []byte) *GithubController {
	return &GithubController{webhookSecretKey: secret}
}

func (c *GithubController) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	// userInfo := middleware.GetUserFromContext(r.Context())
	// if userInfo == nil {
	// 	http.Error(w, "Unauthorized", http.StatusUnauthorized)
	// 	return
	// }

	payload, err := github.ValidatePayload(r, c.webhookSecretKey)
	if err != nil {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		fmt.Println("invalid payload")
		return
	}

	event, err := github.ParseWebHook(github.WebHookType(r), payload)
	if err != nil {
		http.Error(w, "cannot parse webhook", http.StatusBadRequest)
		fmt.Println("cannot parse webhook")
		return
	}

	fmt.Printf("Webhook type: %s\n", github.WebHookType(r))
	fmt.Printf("Event type (Go): %T\n", event)

	switch e := event.(type) {
	case *github.InstallationEvent:
		handleInstallation(
			e.GetAction(),
			e.GetInstallation().GetID(),
			e.GetInstallation().GetTargetID(),
			e.GetInstallation().GetAccount().GetLogin(),
			e.GetInstallation().GetAccount().GetType(),
		)
	case *github.InstallationRepositoriesEvent:
		handleInstallation(
			e.GetAction(),
			e.GetInstallation().GetID(),
			e.GetInstallation().GetTargetID(),
			e.GetInstallation().GetAccount().GetLogin(),
			e.GetInstallation().GetAccount().GetType(),
		)
	default:
		fmt.Println("Unhandled event")
	}
}

func handleInstallation(action string, installationID, targetID int64, accountLogin, accountType string) {
	fmt.Printf("Installation: ID=%d, Account=%s, Type=%s, TargetID=%d, Action=%s\n",
		installationID, accountLogin, accountType, targetID, action)

	switch action {
	case "created", "added", "removed":

		installation := db.GithubInstallation{
			InstallationID: installationID,
			AccountLogin:   accountLogin,
			AccountType:    accountType,
			TargetID:       targetID,
		}

		if err := db.DB.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "installation_id"}},
			UpdateAll: true,
		}).Create(&installation).Error; err != nil {
			fmt.Printf("DB insertion error GithubInstallation: %v\n", err)
		}

	default:
		fmt.Println("Unhandled event action")
	}
}
