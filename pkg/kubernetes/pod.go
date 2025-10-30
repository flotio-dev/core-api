package kubernetes

import (
	"context"
	"fmt"
	"os"
	"strconv"

	"github.com/flotio-dev/api/pkg/db"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// BuildConfig contains all configuration for creating a build pod
type BuildConfig struct {
	BuildID        uint
	Project        db.Project
	Platform       string
	BuildMode      string // release, debug, profile
	BuildTarget    string // apk, aab, ios, web
	FlutterChannel string // stable, beta, dev
	GitBranch      string
	GitUsername    string
	GitPassword    string
}

// CreateBuildPod creates a Kubernetes pod to build a Flutter application
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
	podName := fmt.Sprintf("build-%d", config.BuildID)

	// Create PVC for artifacts
	pvcName, err := CreatePersistentVolumeClaimForArtifacts(clientset, config.BuildID, namespace)
	if err != nil {
		return fmt.Errorf("failed to create PVC: %v", err)
	}

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
	var dbEnvs []db.Env
	if err := db.DB.Where("project_id = ? AND type = ?", config.Project.ID, "env").Find(&dbEnvs).Error; err == nil {
		for _, dbEnv := range dbEnvs {
			envVars = append(envVars, v1.EnvVar{
				Name:  dbEnv.Key,
				Value: dbEnv.Value,
			})
		}
	}

	// Build volume mounts
	volumeMounts := []v1.VolumeMount{
		{
			Name:      "artifacts",
			MountPath: "/outputs",
		},
	}

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
	volumes := []v1.Volume{
		{
			Name: "artifacts",
			VolumeSource: v1.VolumeSource{
				PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{
					ClaimName: pvcName,
				},
			},
		},
	}

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
	envVars := []v1.EnvVar{
		{Name: "GIT_REPO", Value: config.Project.GitRepo},
		{Name: "BUILD_FOLDER", Value: config.Project.BuildFolder},
		{Name: "PLATFORM", Value: config.Platform},
		{Name: "BUILD_ID", Value: strconv.Itoa(int(config.BuildID))},
		{Name: "BUILD_MODE", Value: getBuildMode(config.BuildMode)},
		{Name: "BUILD_TARGET", Value: getBuildTarget(config.Platform, config.BuildTarget)},
		{Name: "FLUTTER_CHANNEL", Value: getFlutterChannel(config.FlutterChannel)},
		{Name: "OUTPUT_DIR", Value: "/outputs"},
		{Name: "ENV_FILES_DIR", Value: "/env-files"},
	}

	// Add Git branch if specified
	if config.GitBranch != "" {
		envVars = append(envVars, v1.EnvVar{Name: "GIT_BRANCH", Value: config.GitBranch})
	}

	// Add Git credentials if specified
	if config.GitUsername != "" {
		envVars = append(envVars, v1.EnvVar{Name: "GIT_USERNAME", Value: config.GitUsername})
	}
	if config.GitPassword != "" {
		envVars = append(envVars, v1.EnvVar{Name: "GIT_PASSWORD", Value: config.GitPassword})
	}

	return envVars
}

func GetPodLogs(buildID uint) ([]string, error) {
	config, err := getKubernetesConfig()
	if err != nil {
		return nil, err
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create clientset: %v", err)
	}

	podName := fmt.Sprintf("build-%d", buildID)
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

func StreamPodLogs(buildID uint, logChan chan<- string) error {
	config, err := getKubernetesConfig()
	if err != nil {
		return err
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("failed to create clientset: %v", err)
	}

	podName := fmt.Sprintf("build-%d", buildID)
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
func GetPodStatus(buildID uint) (string, error) {
	config, err := getKubernetesConfig()
	if err != nil {
		return "", err
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return "", fmt.Errorf("failed to create clientset: %v", err)
	}

	podName := fmt.Sprintf("build-%d", buildID)
	namespace := getNamespace()

	pod, err := clientset.CoreV1().Pods(namespace).Get(context.TODO(), podName, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to get pod: %v", err)
	}

	return string(pod.Status.Phase), nil
}

// CopyArtifactFromPod copies a build artifact from the pod to a local path
// This can be used to retrieve APK/AAB/IPA files after build completion
func CopyArtifactFromPod(buildID uint, artifactPath string, destinationPath string) error {
	// Note: This is a simplified version. In production, you might want to use
	// kubectl cp equivalent or directly access the PVC
	// For now, we'll document that artifacts should be uploaded to object storage
	// from within the build script itself
	return fmt.Errorf("artifact copying should be handled by the build script uploading to object storage")
}

// GetBuildArtifacts returns information about the artifacts produced by a build
func GetBuildArtifacts(buildID uint) (map[string]string, error) {
	config, err := getKubernetesConfig()
	if err != nil {
		return nil, err
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create clientset: %v", err)
	}

	podName := fmt.Sprintf("build-%d", buildID)
	namespace := getNamespace()

	// Read build-info.json from the pod
	req := clientset.CoreV1().Pods(namespace).GetLogs(podName, &v1.PodLogOptions{
		Container: "build",
	})

	logStream, err := req.Stream(context.TODO())
	if err != nil {
		return nil, fmt.Errorf("failed to get logs: %v", err)
	}
	defer logStream.Close()

	// In a real implementation, you would:
	// 1. Mount the PVC to another pod to read the artifacts
	// 2. Or have the build script upload artifacts to S3/MinIO/GCS
	// 3. Return URLs to the artifacts

	artifacts := make(map[string]string)
	artifacts["status"] = "Build artifacts should be retrieved from object storage"

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
