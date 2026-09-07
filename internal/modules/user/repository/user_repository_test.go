package repository

import (
	"testing"

	dbEngine "github.com/flotio-dev/core-api/internal/common/database"
	"github.com/flotio-dev/core-api/internal/modules/user/model"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestUserRepository(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open in-memory db: %v", err)
	}
	if err := db.AutoMigrate(&dbEngine.User{}); err != nil {
		t.Fatalf("failed to auto migrate: %v", err)
	}
	_ = db.Exec("ALTER TABLE users ADD COLUMN github_id text").Error
	_ = db.Exec("ALTER TABLE users ADD COLUMN github_username text").Error

	repo := NewUserRepository(db)

	// 1. GetUserByID Not Found
	_, err = repo.GetUserByID(999)
	if err == nil {
		t.Error("expected error for non-existent user")
	}

	// 2. Create user and GetUserByID Found
	u := dbEngine.User{
		Email:    "repo_test@example.com",
		Username: "repo_user",
	}
	if err := db.Create(&u).Error; err != nil {
		t.Fatal(err)
	}

	found, err := repo.GetUserByID(u.ID)
	if err != nil || found.Email != u.Email {
		t.Fatalf("GetUserByID failed: %v, found: %+v", err, found)
	}

	// 3. UpdateUser with no fields -> error
	emptyReq := &model.UserUpdateRequest{}
	if err := repo.UpdateUser(found, emptyReq); err == nil {
		t.Error("expected error when no fields to update")
	}

	// 4. UpdateUser with all fields
	newEmail := "new_repo@example.com"
	newUsername := "new_repo_user"
	newGithubID := "gh-12345"
	newGithubUser := "gh_user"
	req := &model.UserUpdateRequest{
		Email:          &newEmail,
		Username:       &newUsername,
		GithubID:       &newGithubID,
		GithubUsername: &newGithubUser,
	}

	if err := repo.UpdateUser(found, req); err != nil {
		t.Fatalf("UpdateUser failed: %v", err)
	}

	// Verify updates persisted
	reloaded, err := repo.GetUserByID(u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Email != newEmail || reloaded.Username != newUsername {
		t.Errorf("fields not updated properly: %+v", reloaded)
	}
}
