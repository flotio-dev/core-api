package kubernetes

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	dbEngine "github.com/flotio-dev/api/internal/engines/db"
	s3Engine "github.com/flotio-dev/api/internal/engines/s3"
)

// BuildConfig contains all configuration for creating a build pod
type BuildConfig struct {
	BuildID        uint
	Project        dbEngine.Project
	Platform       string
	BuildMode      string // release, debug, profile
	BuildTarget    string // apk, aab, ios, web
	FlutterChannel string // stable, beta, dev
	GitBranch      string
	GitUsername    string
	GitToken       string
}

// GetPodName generates a unique pod name prefixed with the server hostname
func GetPodName(buildID uint) string {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}
	hostname = strings.ToLower(hostname)
	return fmt.Sprintf("build-%s-%d", hostname, buildID)
}

// CreateBuildPod creates a Kubernetes pod to build a Flutter application
// @Summary		Create a build pod
// @Description	Creates a Kubernetes pod to build a Flutter application with AWS S3 for artifact storage
// @Tags			kubernetes
// @Accept		json
// @Produce		json
// @Param		config	body		BuildConfig	true	"Build configuration"
// @Success		200
// @Failure		500	{object}	map[string]string
// @Router		/internal/kubernetes/pod [post]
func CreateBuildPod(config BuildConfig) error {
	kubeConfig, err := getKubernetesConfig()
	if err != nil {
		return err
	}

	clientset, err := kubernetes.NewForConfig(kubeConfig)
	if err != nil {
		return fmt.Errorf("failed to create clientset: %v", err)
	}

	namespace := getNamespace()
	podName := GetPodName(config.BuildID)

	// Create ConfigMap for environment files
	configMapName, err := CreateConfigMapForEnvFiles(clientset, config.BuildID, config.Project.ID, namespace)
	if err != nil {
		return fmt.Errorf("failed to create ConfigMap: %v", err)
	}

	// Create Secret for keystore (Android only)
	var secretName string
	if config.Platform == "android" {
		secretName, err = CreateSecretForKeystore(clientset, config.BuildID, config.Project.ID, namespace)
		if err != nil {
			return fmt.Errorf("failed to create Secret: %v", err)
		}
	}

	// Build environment variables
	envVars := buildEnvironmentVariables(config)

	// Add environment variables from database
	if dbEngine.DB != nil {
		var dbEnvs []dbEngine.Env
		if err := dbEngine.DB.Where("project_id = ? AND type = ?", config.Project.ID, "env").Find(&dbEnvs).Error; err == nil {
			for _, dbEnv := range dbEnvs {
				envVars = append(envVars, v1.EnvVar{
					Name:  dbEnv.Key,
					Value: dbEnv.Value,
				})
			}
		}
	}

	// Build volume mounts
	volumeMounts := []v1.VolumeMount{}

	// Add ConfigMap volume mount if exists
	if configMapName != "" {
		volumeMounts = append(volumeMounts, v1.VolumeMount{
			Name:      "env-files",
			MountPath: "/env-files",
			ReadOnly:  true,
		})
	}

	// Add Secret volume mount for keystore if exists
	if secretName != "" {
		volumeMounts = append(volumeMounts, v1.VolumeMount{
			Name:      "keystore",
			MountPath: "/keystore",
			ReadOnly:  true,
		})

		// Add keystore environment variables
		envVars = append(envVars,
			v1.EnvVar{Name: "KEYSTORE_PATH", Value: "/keystore/keystore.jks"},
			v1.EnvVar{
				Name: "KEYSTORE_PASSWORD",
				ValueFrom: &v1.EnvVarSource{
					SecretKeyRef: &v1.SecretKeySelector{
						LocalObjectReference: v1.LocalObjectReference{Name: secretName},
						Key:                  "store-password",
					},
				},
			},
			v1.EnvVar{
				Name: "KEY_ALIAS",
				ValueFrom: &v1.EnvVarSource{
					SecretKeyRef: &v1.SecretKeySelector{
						LocalObjectReference: v1.LocalObjectReference{Name: secretName},
						Key:                  "key-alias",
					},
				},
			},
			v1.EnvVar{
				Name: "KEY_PASSWORD",
				ValueFrom: &v1.EnvVarSource{
					SecretKeyRef: &v1.SecretKeySelector{
						LocalObjectReference: v1.LocalObjectReference{Name: secretName},
						Key:                  "key-password",
					},
				},
			},
		)
	}

	// Build volumes
	volumes := []v1.Volume{}

	// Add ConfigMap volume if exists
	if configMapName != "" {
		volumes = append(volumes, v1.Volume{
			Name: "env-files",
			VolumeSource: v1.VolumeSource{
				ConfigMap: &v1.ConfigMapVolumeSource{
					LocalObjectReference: v1.LocalObjectReference{
						Name: configMapName,
					},
				},
			},
		})
	}

	// Add Secret volume if exists
	if secretName != "" {
		volumes = append(volumes, v1.Volume{
			Name: "keystore",
			VolumeSource: v1.VolumeSource{
				Secret: &v1.SecretVolumeSource{
					SecretName: secretName,
				},
			},
		})
	}

	// Define the pod
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: namespace,
			Labels: map[string]string{
				"app":        "flotio-build",
				"build-id":   strconv.Itoa(int(config.BuildID)),
				"project-id": strconv.Itoa(int(config.Project.ID)),
				"platform":   config.Platform,
			},
		},
		Spec: v1.PodSpec{
			RestartPolicy: v1.RestartPolicyNever,
			Containers: []v1.Container{
				{
					Name:         "build",
					Image:        getFlutterBuildImage(),
					Env:          envVars,
					VolumeMounts: volumeMounts,
					Resources: v1.ResourceRequirements{
						Requests: v1.ResourceList{
							v1.ResourceCPU:    parseQuantity("1000m"),
							v1.ResourceMemory: parseQuantity("2Gi"),
						},
						Limits: v1.ResourceList{
							v1.ResourceCPU:    parseQuantity("4000m"),
							v1.ResourceMemory: parseQuantity("8Gi"),
						},
					},
				},
			},
			Volumes: volumes,
		},
	}

	// Create the pod
	_, err = clientset.CoreV1().Pods(namespace).Create(context.TODO(), pod, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create pod: %v", err)
	}

	return nil
}

// buildEnvironmentVariables creates the environment variables for the build container
func buildEnvironmentVariables(config BuildConfig) []v1.EnvVar {
	gitRepo := ""
	if config.Project.GitRepo != nil {
		gitRepo = *config.Project.GitRepo
	}
	buildFolder := ""
	if config.Project.BuildFolder != nil {
		buildFolder = *config.Project.BuildFolder
	}

	envVars := []v1.EnvVar{
		{Name: "GIT_REPO", Value: fmt.Sprintf("https://github.com/%s", gitRepo)},
		{Name: "BUILD_FOLDER", Value: buildFolder},
		{Name: "PLATFORM", Value: config.Platform},
		{Name: "BUILD_ID", Value: strconv.Itoa(int(config.BuildID))},
		{Name: "BUILD_MODE", Value: getBuildMode(config.BuildMode)},
		{Name: "BUILD_TARGET", Value: getBuildTarget(config.Platform, config.BuildTarget)},
		{Name: "FLUTTER_CHANNEL", Value: getFlutterChannel(config.FlutterChannel)},
		{Name: "ENV_FILES_DIR", Value: "/env-files"},
		{Name: "GIT_USERNAME", Value: config.GitUsername},
		{Name: "GIT_PASSWORD", Value: config.GitToken},
		// AWS S3 configuration for artifact storage (supports Garage/MinIO/AWS)
		{Name: "AWS_S3_BUCKET", Value: getAWSS3Bucket()},
		{Name: "AWS_S3_PREFIX", Value: getAWSS3Prefix()},
		{Name: "AWS_S3_ENDPOINT", Value: getAWSS3Endpoint()},
		{Name: "AWS_REGION", Value: getAWSRegion()},
		{Name: "AWS_ACCESS_KEY_ID", Value: os.Getenv("AWS_ACCESS_KEY_ID")},
		{Name: "AWS_SECRET_ACCESS_KEY", Value: os.Getenv("AWS_SECRET_ACCESS_KEY")},
	}

	// Add Git branch if specified
	if config.GitBranch != "" {
		envVars = append(envVars, v1.EnvVar{Name: "GIT_BRANCH", Value: config.GitBranch})
	}

	return envVars
}

// GetPodLogs retrieves logs from a build pod
// @Summary		Get pod logs
// @Description	Retrieves logs from a specific build pod
// @Tags			kubernetes
// @Accept		json
// @Produce		json
// @Param		buildID	path		int	true	"Build ID"
// @Success		200	{array}	string
// @Failure		500	{object}	map[string]string
// @Router		/internal/kubernetes/pod/{buildID}/logs [get]
func GetPodLogs(buildID uint) ([]string, error) {
	config, err := getKubernetesConfig()
	if err != nil {
		return nil, err
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create clientset: %v", err)
	}

	podName := GetPodName(buildID)
	namespace := getNamespace()

	req := clientset.CoreV1().Pods(namespace).GetLogs(podName, &v1.PodLogOptions{})
	logStream, err := req.Stream(context.TODO())
	if err != nil {
		return nil, fmt.Errorf("failed to get log stream: %v", err)
	}
	defer logStream.Close()

	var logs []string
	buf := make([]byte, 4096)
	for {
		n, err := logStream.Read(buf)
		if n > 0 {
			logs = append(logs, string(buf[:n]))
		}
		if err != nil {
			break
		}
	}

	return logs, nil
}

// StreamPodLogs streams logs from a build pod in real-time
// @Summary		Stream pod logs
// @Description	Streams logs from a specific build pod via channel
// @Tags			kubernetes
// @Accept		json
// @Produce		json
// @Param		buildID	path		int	true	"Build ID"
// @Success		200
// @Failure		500	{object}	map[string]string
// @Router		/internal/kubernetes/pod/{buildID}/logs/stream [get]
func StreamPodLogs(buildID uint, logChan chan<- string) error {
	config, err := getKubernetesConfig()
	if err != nil {
		return err
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("failed to create clientset: %v", err)
	}

	podName := GetPodName(buildID)
	namespace := getNamespace()

	req := clientset.CoreV1().Pods(namespace).GetLogs(podName, &v1.PodLogOptions{
		Follow: true,
	})
	logStream, err := req.Stream(context.TODO())
	if err != nil {
		return fmt.Errorf("failed to get log stream: %v", err)
	}
	defer logStream.Close()

	buf := make([]byte, 4096)
	for {
		n, err := logStream.Read(buf)
		if n > 0 {
			logChan <- string(buf[:n])
		}
		if err != nil {
			close(logChan)
			break
		}
	}

	return nil
}

// GetPodStatus returns the current status of a build pod
// @Summary		Get pod status
// @Description	Returns the current status of a specific build pod
// @Tags			kubernetes
// @Accept		json
// @Produce		json
// @Param		buildID	path		int	true	"Build ID"
// @Success		200	{string}	string
// @Failure		500	{object}	map[string]string
// @Router		/internal/kubernetes/pod/{buildID}/status [get]
func GetPodStatus(buildID uint) (string, error) {
	config, err := getKubernetesConfig()
	if err != nil {
		return "", err
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return "", fmt.Errorf("failed to create clientset: %v", err)
	}

	podName := GetPodName(buildID)
	namespace := getNamespace()

	pod, err := clientset.CoreV1().Pods(namespace).Get(context.TODO(), podName, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to get pod: %v", err)
	}

	return string(pod.Status.Phase), nil
}

// DeleteBuildPod deletes a build pod from Kubernetes
// @Summary		Delete a build pod
// @Description	Deletes a specific build pod from the Kubernetes cluster
// @Tags			kubernetes
// @Accept		json
// @Produce		json
// @Param		buildID	path		int	true	"Build ID"
// @Success		200
// @Failure		500	{object}	map[string]string
// @Router		/internal/kubernetes/pod/{buildID} [delete]
func DeleteBuildPod(buildID uint) error {
	config, err := getKubernetesConfig()
	if err != nil {
		return err
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("failed to create clientset: %v", err)
	}

	podName := GetPodName(buildID)
	namespace := getNamespace()

	deletePolicy := metav1.DeletePropagationForeground
	err = clientset.CoreV1().Pods(namespace).Delete(context.TODO(), podName, metav1.DeleteOptions{
		PropagationPolicy: &deletePolicy,
	})
	if err != nil {
		return fmt.Errorf("failed to delete pod: %v", err)
	}

	return nil
}

// StartPodLogListener starts listening to pod logs and saves them to the database
// This function should be called in a goroutine after creating a build pod
// @Summary		Start pod log listener
// @Description	Starts listening to pod logs and saves them to the database
// @Tags			kubernetes
// @Accept		json
// @Produce		json
// @Param		buildID	path		int	true	"Build ID"
// @Success		200
// @Failure		500	{object}	map[string]string
// @Router		/internal/kubernetes/pod/{buildID}/listen [post]
func StartPodLogListener(buildID uint) {
	config, err := getKubernetesConfig()
	if err != nil {
		fmt.Printf("Failed to get kubernetes config for log listener (build %d): %v\n", buildID, err)
		return
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		fmt.Printf("Failed to create clientset for log listener (build %d): %v\n", buildID, err)
		return
	}

	podName := GetPodName(buildID)
	namespace := getNamespace()

	// Wait for pod to be running before starting log collection
	for i := 0; i < 60; i++ { // Wait up to 60 seconds
		pod, err := clientset.CoreV1().Pods(namespace).Get(context.TODO(), podName, metav1.GetOptions{})
		if err != nil {
			fmt.Printf("Waiting for pod %s to be created (attempt %d): %v\n", podName, i+1, err)
			time.Sleep(1 * time.Second)
			continue
		}
		if pod.Status.Phase == v1.PodRunning || pod.Status.Phase == v1.PodSucceeded || pod.Status.Phase == v1.PodFailed {
			break
		}
		if pod.Status.Phase == v1.PodPending {
			time.Sleep(1 * time.Second)
			continue
		}
		break
	}

	// Start streaming logs
	req := clientset.CoreV1().Pods(namespace).GetLogs(podName, &v1.PodLogOptions{
		Follow: true,
	})
	logStream, err := req.Stream(context.TODO())
	if err != nil {
		fmt.Printf("Failed to get log stream for build %d: %v\n", buildID, err)
		// Still try to update build status even if log streaming failed
		updateBuildStatusFromPod(clientset, namespace, podName, buildID)
		return
	}
	defer logStream.Close()

	lineNumber := 1
	scanner := bufio.NewScanner(logStream)
	for scanner.Scan() {
		logLine := scanner.Text()

		// Save log to database
		logEntry := dbEngine.Log{
			BuildID:    buildID,
			LineNumber: lineNumber,
			Content:    logLine,
			Timestamp:  time.Now().Unix(),
		}
		if err := dbEngine.DB.Create(&logEntry).Error; err != nil {
			fmt.Printf("Failed to save log to database for build %d: %v\n", buildID, err)
		}
		lineNumber++
	}

	if err := scanner.Err(); err != nil {
		fmt.Printf("Error reading log stream for build %d: %v\n", buildID, err)
	}

	// Update build status based on pod status
	updateBuildStatusFromPod(clientset, namespace, podName, buildID)
}

// updateBuildStatusFromPod checks the final pod status and updates the build accordingly
func updateBuildStatusFromPod(clientset *kubernetes.Clientset, namespace, podName string, buildID uint) {
	pod, err := clientset.CoreV1().Pods(namespace).Get(context.TODO(), podName, metav1.GetOptions{})
	if err != nil {
		fmt.Printf("Failed to get pod status for build %d: %v\n", buildID, err)
		return
	}

	var build dbEngine.Build
	if err := dbEngine.DB.First(&build, buildID).Error; err != nil {
		fmt.Printf("Failed to get build %d from database: %v\n", buildID, err)
		return
	}

	// Only update if build is still running or pending (not cancelled)
	if build.Status != "running" && build.Status != "pending" {
		return
	}

	switch pod.Status.Phase {
	case v1.PodSucceeded:
		build.Status = "success"
		// Calculate build duration
		build.Duration = int64(time.Since(build.CreatedAt).Seconds())
		// Reconcile S3 artifact key when build succeeds
		if artifactKey, err := s3Engine.FindPrimaryArtifactKey(buildID, build.Platform); err == nil {
			build.APKURL = artifactKey
			fmt.Printf("Build %d: found artifact at S3 key: %s\n", buildID, artifactKey)
		} else {
			fmt.Printf("Build %d: failed to find artifact in S3: %v\n", buildID, err)
		}
	case v1.PodFailed:
		build.Status = "failed"
		// Calculate build duration
		build.Duration = int64(time.Since(build.CreatedAt).Seconds())
	}

	if err := dbEngine.DB.Save(&build).Error; err != nil {
		fmt.Printf("Failed to update build %d status: %v\n", buildID, err)
	}
}

// GetArtifactURL returns the S3 URL for a build artifact
// @Summary		Get artifact URL
// @Description	Returns the AWS S3 URL for a specific build artifact
// @Tags			kubernetes
// @Accept		json
// @Produce		json
// @Param		buildID		path		int		true	"Build ID"
// @Param		artifactName	path		string	true	"Artifact name (e.g., app-release.apk)"
// @Success		200	{string}	string
// @Failure		500	{object}	map[string]string
// @Router		/internal/kubernetes/pod/{buildID}/artifact/{artifactName} [get]
func GetArtifactURL(buildID uint, artifactName string) string {
	bucket := getAWSS3Bucket()
	prefix := getAWSS3Prefix()
	endpoint := getAWSS3Endpoint()

	// If custom endpoint is set (Garage/MinIO), use it
	if endpoint != "" {
		return fmt.Sprintf("%s/%s/%s/%d/%s", endpoint, bucket, prefix, buildID, artifactName)
	}
	// Default to AWS S3 URL format
	region := getAWSRegion()
	return fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s/%d/%s", bucket, region, prefix, buildID, artifactName)
}

// GetBuildArtifacts returns information about the artifacts produced by a build
// @Summary		Get build artifacts
// @Description	Returns URLs to the artifacts stored in AWS S3 for a specific build
// @Tags			kubernetes
// @Accept		json
// @Produce		json
// @Param		buildID	path		int	true	"Build ID"
// @Success		200	{object}	map[string]string
// @Failure		500	{object}	map[string]string
// @Router		/internal/kubernetes/pod/{buildID}/artifacts [get]
func GetBuildArtifacts(buildID uint) (map[string]string, error) {
	bucket := getAWSS3Bucket()
	prefix := getAWSS3Prefix()
	endpoint := getAWSS3Endpoint()

	var baseURL string
	// If custom endpoint is set (Garage/MinIO), use it
	if endpoint != "" {
		baseURL = fmt.Sprintf("%s/%s/%s/%d", endpoint, bucket, prefix, buildID)
	} else {
		// Default to AWS S3 URL format
		region := getAWSRegion()
		baseURL = fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s/%d", bucket, region, prefix, buildID)
	}

	artifacts := make(map[string]string)
	artifacts["apk"] = fmt.Sprintf("%s/app-release.apk", baseURL)
	artifacts["aab"] = fmt.Sprintf("%s/app-release.aab", baseURL)
	artifacts["build_info"] = fmt.Sprintf("%s/build-info.json", baseURL)

	return artifacts, nil
}

// Helper functions
func getKubernetesConfig() (*rest.Config, error) {
	// Try in-cluster config first
	config, err := rest.InClusterConfig()
	if err != nil {
		// Fallback to external config using env vars
		apiURL := os.Getenv("KUBECTL_API")
		token := os.Getenv("KUBECTL_TOKEN")
		if apiURL == "" || token == "" {
			return nil, fmt.Errorf("failed to get in-cluster config and no external config provided: %v", err)
		}

		config = &rest.Config{
			Host:        apiURL,
			BearerToken: token,
			TLSClientConfig: rest.TLSClientConfig{
				Insecure: true, // For localhost/dev environment
			},
		}
	}
	return config, nil
}

func getNamespace() string {
	namespace := os.Getenv("K8S_NAMESPACE")
	if namespace == "" {
		namespace = "default"
	}
	return namespace
}

func getFlutterBuildImage() string {
	image := os.Getenv("FLUTTER_BUILD_IMAGE")
	if image == "" {
		image = "flotio/flutter-build:latest" // Default image name
	}
	return image
}

func getBuildMode(mode string) string {
	if mode == "" {
		return "release"
	}
	return mode
}

func getBuildTarget(platform, target string) string {
	if target != "" {
		return target
	}

	switch platform {
	case "android":
		return "apk"
	case "ios":
		return "ios"
	case "web":
		return "web"
	default:
		return "apk"
	}
}

func getFlutterChannel(channel string) string {
	if channel == "" || channel == "latest" {
		return "stable"
	}
	return channel
}

// getAWSS3Bucket returns the S3 bucket name for storing build artifacts
func getAWSS3Bucket() string {
	bucket := os.Getenv("AWS_S3_BUCKET")
	if bucket == "" {
		bucket = "flotio-builds"
	}
	return bucket
}

// getAWSRegion returns the AWS region for S3 operations
func getAWSRegion() string {
	region := os.Getenv("AWS_REGION")
	if region == "" {
		region = "garage"
	}
	return region
}

// getAWSS3Endpoint returns the custom S3 endpoint URL (for Garage/MinIO)
func getAWSS3Endpoint() string {
	return os.Getenv("AWS_S3_ENDPOINT")
}

// getAWSS3Prefix returns the S3 prefix/folder for storing build artifacts
func getAWSS3Prefix() string {
	prefix := os.Getenv("AWS_S3_PREFIX")
	if prefix == "" {
		prefix = "builds"
	}
	return prefix
}

// parseQuantity parses a Kubernetes resource quantity string
func parseQuantity(s string) resource.Quantity {
	q, _ := resource.ParseQuantity(s)
	return q
}
