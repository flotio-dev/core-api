package repository

import (
	"testing"

	dbEngine "github.com/flotio-dev/core-api/internal/common/database"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:gh_repo_memdb?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	_ = db.AutoMigrate(&dbEngine.GithubInstallation{}, &dbEngine.User{})
	return db
}

func TestGithubRepository_AllMethods(t *testing.T) {
	db := setupTestDB(t)
	repo := NewGithubRepository(db)

	userID := uint(1)
	instID := int64(1001)

	// 1. SaveInstallation - new insert
	err := repo.SaveInstallation(userID, instID, "octocat", "User", 123, "http://avatar")
	if err != nil {
		t.Fatalf("SaveInstallation failed: %v", err)
	}

	// 2. GetInstallationByUser
	inst, err := repo.GetInstallationByUser(userID)
	if err != nil || inst == nil {
		t.Fatalf("GetInstallationByUser failed: %v", err)
	}
	if inst.AccountLogin != "octocat" || inst.InstallationID != instID {
		t.Errorf("unexpected installation: %+v", inst)
	}

	// 3. SaveInstallation - update existing with accountLogin
	err = repo.SaveInstallation(userID, instID, "octocat", "Organization", 1234, "http://avatar2")
	if err != nil {
		t.Fatalf("SaveInstallation update failed: %v", err)
	}
	inst, err = repo.GetInstallationByUser(userID)
	if err != nil || inst.AccountType != "Organization" || inst.AvatarURL != "http://avatar2" {
		t.Errorf("update failed: %+v", inst)
	}

	// SaveInstallation - update existing without accountLogin
	err = repo.SaveInstallation(userID, instID, "", "", 0, "")
	if err != nil {
		t.Fatalf("SaveInstallation partial update failed: %v", err)
	}

	// 4. ListInstallationsByUser
	// Add another installation
	err = repo.SaveInstallation(userID, int64(1002), "octoorg", "Organization", 456, "http://avatar3")
	if err != nil {
		t.Fatalf("SaveInstallation 2 failed: %v", err)
	}
	insts, err := repo.ListInstallationsByUser(userID)
	if err != nil || len(insts) < 2 {
		t.Errorf("ListInstallationsByUser failed: len=%d, err=%v", len(insts), err)
	}

	// 5. GetGithubInstallationByInstallationID
	found, err := repo.GetGithubInstallationByInstallationID(1002)
	if err != nil || found == nil || found.AccountLogin != "octoorg" {
		t.Errorf("GetGithubInstallationByInstallationID failed: %+v, err=%v", found, err)
	}
	notFound, err := repo.GetGithubInstallationByInstallationID(99999)
	if err != nil || notFound != nil {
		t.Errorf("expected nil for non-existent installation")
	}

	// 6. CountOtherInstallations
	count, err := repo.CountOtherInstallations(1001, 2)
	if err != nil || count != 1 {
		t.Errorf("CountOtherInstallations failed: count=%d, err=%v", count, err)
	}
	count, err = repo.CountOtherInstallations(1001, userID)
	if err != nil || count != 0 {
		t.Errorf("CountOtherInstallations for self failed: count=%d", count)
	}

	// 7. DeleteUserInstallationByID
	err = repo.DeleteUserInstallationByID(userID, 1002)
	if err != nil {
		t.Fatalf("DeleteUserInstallationByID failed: %v", err)
	}
	found, _ = repo.GetGithubInstallationByInstallationID(1002)
	if found != nil {
		t.Errorf("expected 1002 to be deleted")
	}

	// 8. DeleteInstallationByInstallationID
	err = repo.DeleteInstallationByInstallationID(1001)
	if err != nil {
		t.Fatalf("DeleteInstallationByInstallationID failed: %v", err)
	}
	inst, err = repo.GetInstallationByUser(userID)
	if err != nil || inst != nil {
		t.Errorf("expected user installations to be empty")
	}

	// 9. DeleteInstallationByUser
	_ = repo.SaveInstallation(userID, 1003, "testuser", "User", 789, "")
	err = repo.DeleteInstallationByUser(userID)
	if err != nil {
		t.Fatalf("DeleteInstallationByUser failed: %v", err)
	}
	inst, _ = repo.GetInstallationByUser(userID)
	if inst != nil {
		t.Errorf("expected deleted by user")
	}
}
