package main

import (
	"fmt"
	"log"
	"os"
	"time"

	dbEngine "github.com/flotio-dev/api/internal/engines/db"
	kubernetesEngine "github.com/flotio-dev/api/internal/engines/kubernetes"
	"github.com/joho/godotenv"
)

func main() {
	log.Println("===========================================")
	log.Println("Flutter Android Build Test Script")
	log.Println("===========================================")
	log.Println()

	// Load .env file
	if err := godotenv.Load(); err != nil {
		log.Printf("Warning: No .env file found or error loading it: %v\n", err)
	} else {
		log.Println("✓ Environment variables loaded from .env file")
	}
	log.Println()

	// Use a test build ID (timestamp-based)
	testBuildID := uint(time.Now().Unix())
	log.Printf("✓ Test build ID: %d\n", testBuildID)

	// Create a mock project (no database)
	gitRepo := "flotio-dev/test_apk.git"
	buildFolder := "."
	testProject := dbEngine.Project{
		Name:        "Test Application (Test)",
		GitRepo:     &gitRepo,
		BuildFolder: &buildFolder,
	}

	// Configure the build
	buildConfig := kubernetesEngine.BuildConfig{
		BuildID:        testBuildID,
		Project:        testProject,
		Platform:       "android",
		BuildMode:      "release",
		BuildTarget:    "apk",
		FlutterChannel: "stable",
		GitBranch:      "main",
		GitUsername:    "",
		GitToken:       "",
	}

	log.Println()
	log.Println("Build Configuration:")
	log.Printf("  Build ID: %d\n", buildConfig.BuildID)
	log.Printf("  Project: %s\n", buildConfig.Project.Name)
	displayGitRepo := ""
	if buildConfig.Project.GitRepo != nil {
		displayGitRepo = *buildConfig.Project.GitRepo
	}
	log.Printf("  Git Repo: %s\n", displayGitRepo)
	displayBuildFolder := ""
	if buildConfig.Project.BuildFolder != nil {
		displayBuildFolder = *buildConfig.Project.BuildFolder
	}
	log.Printf("  Build Folder: %s\n", displayBuildFolder)
	log.Printf("  Platform: %s\n", buildConfig.Platform)
	log.Printf("  Build Mode: %s\n", buildConfig.BuildMode)
	log.Printf("  Build Target: %s\n", buildConfig.BuildTarget)
	log.Printf("  Flutter Channel: %s\n", buildConfig.FlutterChannel)
	log.Println()

	log.Println("S3 Storage Configuration:")
	log.Printf("  Bucket: %s\n", getEnvOrDefault("AWS_S3_BUCKET", "not configured"))
	log.Printf("  Prefix: %s\n", getEnvOrDefault("AWS_S3_PREFIX", "builds"))
	log.Printf("  Endpoint: %s\n", getEnvOrDefault("AWS_S3_ENDPOINT", "not configured"))
	log.Printf("  Region: %s\n", getEnvOrDefault("AWS_REGION", "garage"))
	log.Println()

	// Create the Kubernetes pod
	log.Println("Creating Kubernetes build pod...")
	if err := kubernetesEngine.CreateBuildPod(buildConfig); err != nil {
		log.Fatalf("Failed to create build pod: %v", err)
	}
	log.Printf("✓ Build pod created successfully: build-%d\n", testBuildID)

	// Monitor the pod status
	log.Println()
	log.Println("Monitoring build pod status...")
	log.Println("Press Ctrl+C to stop monitoring (build will continue in background)")
	log.Println()

	monitorPodStatus(testBuildID)
}

func monitorPodStatus(buildID uint) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	lastStatus := ""
	startTime := time.Now()

	for {
		select {
		case <-ticker.C:
			status, err := kubernetesEngine.GetPodStatus(buildID)
			if err != nil {
				log.Printf("Error getting pod status: %v\n", err)
				continue
			}

			if status != lastStatus {
				elapsed := time.Since(startTime).Round(time.Second)
				log.Printf("[%s] Pod Status: %s\n", elapsed, status)
				lastStatus = status

				// If pod completed or failed, show logs and exit
				if status == "Succeeded" {
					log.Println()
					log.Println("✓ Build completed successfully!")
					log.Println()
					showPodLogs(buildID)
					showArtifacts(buildID)
					return
				} else if status == "Failed" {
					log.Println()
					log.Println("✗ Build failed!")
					log.Println()
					showPodLogs(buildID)
					os.Exit(1)
				}
			}
		}
	}
}

func showPodLogs(buildID uint) {
	log.Println("Fetching pod logs...")
	log.Println("-------------------------------------------")

	logs, err := kubernetesEngine.GetPodLogs(buildID)
	if err != nil {
		log.Printf("Warning: Failed to get pod logs: %v\n", err)
		return
	}

	for _, logLine := range logs {
		fmt.Print(logLine)
	}

	log.Println()
	log.Println("-------------------------------------------")
}

func showArtifacts(buildID uint) {
	log.Println("Build artifacts information:")
	log.Println()

	// Show S3 configuration
	log.Println("S3 Storage Configuration:")
	log.Printf("  Bucket: %s\n", getEnvOrDefault("AWS_S3_BUCKET", "not configured"))
	log.Printf("  Prefix: %s\n", getEnvOrDefault("AWS_S3_PREFIX", "builds"))
	log.Printf("  Endpoint: %s\n", getEnvOrDefault("AWS_S3_ENDPOINT", "not configured"))
	log.Printf("  Region: %s\n", getEnvOrDefault("AWS_REGION", "garage"))
	log.Println()

	artifacts, err := kubernetesEngine.GetBuildArtifacts(buildID)
	if err != nil {
		log.Printf("Warning: Failed to get artifacts: %v\n", err)
		return
	}

	log.Println("Artifact URLs:")
	for key, value := range artifacts {
		log.Printf("  %s: %s\n", key, value)
	}
	log.Println()
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
