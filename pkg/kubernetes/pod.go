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

func CreateBuildPod(buildID uint, project db.Project, platform string) error {
	config, err := getKubernetesConfig()
	if err != nil {
		return err
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("failed to create clientset: %v", err)
	}

	// Define the pod
	podName := fmt.Sprintf("build-%d", buildID)
	namespace := "default" // or get from env

	// Commands to run in the container
	var commands []string
	if project.BuildFolder != "" {
		commands = []string{
			"sh", "-c",
			fmt.Sprintf(`
				git clone %s /tmp/repo &&
				cd /tmp/repo/%s &&
				flutter pub get &&
				flutter build %s
			`, project.GitRepo, project.BuildFolder, getBuildTarget(platform)),
		}
	} else {
		commands = []string{
			"sh", "-c",
			fmt.Sprintf(`
				git clone %s /tmp/repo &&
				cd /tmp/repo &&
				flutter pub get &&
				flutter build %s
			`, project.GitRepo, getBuildTarget(platform)),
		}
	}

	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: podName,
			Labels: map[string]string{
				"app":        "flotio-build",
				"build-id":   strconv.Itoa(int(buildID)),
				"project-id": strconv.Itoa(int(project.ID)),
			},
		},
		Spec: v1.PodSpec{
			RestartPolicy: v1.RestartPolicyNever,
			Containers: []v1.Container{
				{
					Name:    "build",
					Image:   getFlutterImage(project.FlutterVersion),
					Command: commands,
					// Add volume mounts if needed for artifacts
				},
			},
		},
	}

	// Create the pod
	_, err = clientset.CoreV1().Pods(namespace).Create(context.TODO(), pod, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create pod: %v", err)
	}

	return nil
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
	namespace := "default"

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
	namespace := "default"

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

func getFlutterImage(version string) string {
	if version == "" {
		return "flutter:latest"
	}
	return fmt.Sprintf("flutter:%s", version)
}

func getBuildTarget(platform string) string {
	switch platform {
	case "ios":
		return "ios"
	case "android":
		return "apk"
	default:
		return "apk"
	}
}
