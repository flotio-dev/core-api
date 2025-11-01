package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/flotio-dev/api/pkg/db"
	"github.com/flotio-dev/api/pkg/kubernetes"
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
	testProject := db.Project{
		Name:        "Test Application (Test)",
		GitRepo:     "https://github.com/flotio-dev/test_apk.git",
		BuildFolder: ".",
	}

	// Configure the build
	buildConfig := kubernetes.BuildConfig{
		BuildID:        testBuildID,
		Project:        testProject,
		Platform:       "android",
		BuildMode:      "release",
		BuildTarget:    "apk",
		FlutterChannel: "stable",
		GitBranch:      "main",
		GitUsername:    "",
		GitPassword:    "",
	}

	log.Println()
	log.Println("Build Configuration:")
	log.Printf("  Build ID: %d\n", buildConfig.BuildID)
	log.Printf("  Project: %s\n", buildConfig.Project.Name)
	log.Printf("  Git Repo: %s\n", buildConfig.Project.GitRepo)
	log.Printf("  Platform: %s\n", buildConfig.Platform)
	log.Printf("  Build Mode: %s\n", buildConfig.BuildMode)
	log.Printf("  Build Target: %s\n", buildConfig.BuildTarget)
	log.Printf("  Flutter Channel: %s\n", buildConfig.FlutterChannel)
	log.Println()

	// Create the Kubernetes pod
	log.Println("Creating Kubernetes build pod...")
	if err := kubernetes.CreateBuildPod(buildConfig); err != nil {
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
			status, err := kubernetes.GetPodStatus(buildID)
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

	logs, err := kubernetes.GetPodLogs(buildID)
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

	artifacts, err := kubernetes.GetBuildArtifacts(buildID)
	if err != nil {
		log.Printf("Warning: Failed to get artifacts: %v\n", err)
		return
	}

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
