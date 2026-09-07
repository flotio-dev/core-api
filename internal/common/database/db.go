package database

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/redis/go-redis/v9"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/flotio-dev/core-api/internal/common/crypto"
)

var DB *gorm.DB
var Redis *redis.Client

var openDB = func(dsn string) (*gorm.DB, error) {
	return gorm.Open(postgres.Open(dsn), &gorm.Config{})
}

func InitDB() {
	dsn := os.Getenv("DATABASE_URL")
	fmt.Printf("DB URL : %s", dsn)
	if dsn == "" {
		log.Fatal("DATABASE_URL is not set")
	}

	var err error
	DB, err = openDB(dsn)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	// Auto migrate
	err = DB.AutoMigrate(&User{}, &Project{}, &Build{}, &Log{}, &Env{}, &Organization{}, &GithubInstallation{}, &ProjectConfig{}, &Keystore{}, &GooglePlayCredentials{}, &Release{}, &ReleaseAudit{})
	if err != nil {
		log.Fatalf("Failed to migrate database: %v", err)
	}

	// Ensure installation_id and user_id are non-unique independently to allow multiple installations per user and org sharing
	_ = DB.Exec("ALTER TABLE github_installations DROP CONSTRAINT IF EXISTS uni_github_installations_installation_id").Error
	_ = DB.Exec("ALTER TABLE github_installations DROP CONSTRAINT IF EXISTS idx_github_installations_user_id").Error
	_ = DB.Exec("ALTER TABLE github_installations DROP CONSTRAINT IF EXISTS uni_github_installations_user_id").Error
	_ = DB.Exec("DROP INDEX IF EXISTS idx_github_installations_installation_id").Error
	_ = DB.Exec("DROP INDEX IF EXISTS uni_github_installations_installation_id").Error
	_ = DB.Exec("DROP INDEX IF EXISTS idx_github_installations_user_id").Error
	_ = DB.Exec("DROP INDEX IF EXISTS uni_github_installations_user_id").Error

	// Encrypt any secret still stored as plaintext (one-off, idempotent).
	if err := encryptLegacySecrets(); err != nil {
		log.Fatalf("Failed to encrypt legacy secrets: %v", err)
	}

	log.Println("Database connected and migrated")
}

// encryptLegacySecrets encrypts secrets that predate encryption-at-rest.
// It is idempotent: values already encrypted (carrying the "enc:v1:" prefix)
// are skipped, so it is safe to run on every startup.
func encryptLegacySecrets() error {
	var keystores []Keystore
	if err := DB.Find(&keystores).Error; err != nil {
		return err
	}
	for _, k := range keystores {
		updates := map[string]interface{}{}
		if k.KeystoreFile != "" && !crypto.IsEncrypted(k.KeystoreFile) {
			enc, err := crypto.Encrypt(k.KeystoreFile)
			if err != nil {
				return err
			}
			updates["keystore_file"] = enc
		}
		if k.StorePassword != "" && !crypto.IsEncrypted(k.StorePassword) {
			enc, err := crypto.Encrypt(k.StorePassword)
			if err != nil {
				return err
			}
			updates["store_password"] = enc
		}
		if k.KeyPassword != "" && !crypto.IsEncrypted(k.KeyPassword) {
			enc, err := crypto.Encrypt(k.KeyPassword)
			if err != nil {
				return err
			}
			updates["key_password"] = enc
		}
		if len(updates) > 0 {
			if err := DB.Model(&Keystore{}).Where("id = ?", k.ID).Updates(updates).Error; err != nil {
				return err
			}
		}
	}

	var creds []GooglePlayCredentials
	if err := DB.Find(&creds).Error; err != nil {
		return err
	}
	for _, c := range creds {
		if c.Credentials != "" && !crypto.IsEncrypted(c.Credentials) {
			enc, err := crypto.Encrypt(c.Credentials)
			if err != nil {
				return err
			}
			if err := DB.Model(&GooglePlayCredentials{}).Where("id = ?", c.ID).Update("credentials", enc).Error; err != nil {
				return err
			}
		}
	}

	return nil
}

func InitRedis() {
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		log.Fatal("REDIS_ADDR is not set")
	}

	password := os.Getenv("REDIS_PASSWORD")
	dbStr := os.Getenv("REDIS_DB")

	db := 0
	if dbStr != "" {
		if parsed, err := strconv.Atoi(dbStr); err == nil {
			db = parsed
		}
	}

	Redis = redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})

	ctx := context.Background()
	if err := Redis.Ping(ctx).Err(); err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}

	log.Println("Redis connected")
}
