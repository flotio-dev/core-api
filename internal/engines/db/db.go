package db

import (
	"context"
	"fmt"
	"log"
	"os"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var DB *gorm.DB

func InitDB() {
	dsn := os.Getenv("DATABASE_URL")
	fmt.Printf("DB URL : %s", dsn)
	if dsn == "" {
		log.Fatal("DATABASE_URL is not set")
	}

	var err error
	DB, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	// Auto migrate
	err = DB.AutoMigrate(&User{}, &Project{}, &Build{}, &Log{}, &Env{}, &Organization{}, &GithubInstallation{})
	if err != nil {
		log.Fatalf("Failed to migrate database: %v", err)
	}

	log.Println("Database connected and migrated")
}

// SyncUsersWithKeycloak synchronizes local users with Keycloak
// It removes users from the local database that no longer exist in Keycloak
func SyncUsersWithKeycloak(ctx context.Context, keycloakUserIDs map[string]bool) error {
	var localUsers []User
	if err := DB.Find(&localUsers).Error; err != nil {
		return fmt.Errorf("failed to fetch local users: %w", err)
	}

	var usersToDelete []uint
	for _, user := range localUsers {
		if user.KeycloakID == "" {
			continue // Skip users without KeycloakID
		}
		if !keycloakUserIDs[user.KeycloakID] {
			log.Printf("User %s (KeycloakID: %s) not found in Keycloak, marking for deletion", user.Username, user.KeycloakID)
			usersToDelete = append(usersToDelete, user.ID)
		}
	}

	if len(usersToDelete) > 0 {
		// Delete associated data first (projects, etc.) due to foreign key constraints
		for _, userID := range usersToDelete {
			// Delete user's projects and related data
			if err := DB.Where("user_id = ?", userID).Delete(&Project{}).Error; err != nil {
				log.Printf("Warning: failed to delete projects for user %d: %v", userID, err)
			}
		}

		// Now delete the users
		if err := DB.Where("id IN ?", usersToDelete).Delete(&User{}).Error; err != nil {
			return fmt.Errorf("failed to delete users: %w", err)
		}
		log.Printf("Removed %d users that no longer exist in Keycloak", len(usersToDelete))
	} else {
		log.Println("All local users are synced with Keycloak")
	}

	return nil
}
