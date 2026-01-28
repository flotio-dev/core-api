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
)

var DB *gorm.DB
var Redis *redis.Client

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
