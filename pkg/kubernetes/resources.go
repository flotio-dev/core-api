package kubernetes

import (
	"context"
	"encoding/base64"
	"fmt"

	"github.com/flotio-dev/api/pkg/db"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// CreateConfigMapForEnvFiles creates a ConfigMap containing environment files for a build
func CreateConfigMapForEnvFiles(clientset *kubernetes.Clientset, buildID uint, projectID uint, namespace string) (string, error) {
	// Check if database is initialized
	if db.DB == nil {
		// No database connection, skip environment files
		return "", nil
	}

	// Fetch environment files from database
	var envs []db.Env
	if err := db.DB.Where("project_id = ? AND type = ?", projectID, "file").Find(&envs).Error; err != nil {
		return "", fmt.Errorf("failed to fetch environment files: %v", err)
	}

	if len(envs) == 0 {
		return "", nil // No files to mount
	}

	configMapName := fmt.Sprintf("build-%d-env-files", buildID)
	data := make(map[string]string)

	for _, env := range envs {
		var content string
		if env.IsBase64 {
			// Decode base64 content
			decoded, err := base64.StdEncoding.DecodeString(env.Value)
			if err != nil {
				return "", fmt.Errorf("failed to decode base64 content for %s: %v", env.Key, err)
			}
			content = string(decoded)
		} else {
			content = env.Value
		}

		// Use path as key with special encoding to preserve directory structure
		// Format: path::actual_path where __ represents /
		// Example: google-services.json::android__app__google-services.json
		fileName := env.Key
		if env.Path != "" {
			// Encode path: replace / with __
			encodedPath := env.Path
			for old, new := range map[string]string{"/": "__"} {
				encodedPath = replaceAll(encodedPath, old, new)
			}
			fileName = fmt.Sprintf("%s::%s", env.Key, encodedPath)
		}

		data[fileName] = content
	}

	configMap := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapName,
			Namespace: namespace,
			Labels: map[string]string{
				"app":      "flotio-build",
				"build-id": fmt.Sprintf("%d", buildID),
			},
		},
		Data: data,
	}

	_, err := clientset.CoreV1().ConfigMaps(namespace).Create(context.TODO(), configMap, metav1.CreateOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to create ConfigMap: %v", err)
	}

	return configMapName, nil
}

// CreateSecretForKeystore creates a Secret containing the keystore and credentials
func CreateSecretForKeystore(clientset *kubernetes.Clientset, buildID uint, projectID uint, namespace string) (string, error) {
	// Check if database is initialized
	if db.DB == nil {
		// No database connection, skip keystore
		return "", nil
	}

	// Fetch active keystore from database
	var keystore db.Keystore
	if err := db.DB.Where("project_id = ? AND is_active = ?", projectID, true).First(&keystore).Error; err != nil {
		return "", nil // No keystore configured (not an error)
	}

	secretName := fmt.Sprintf("build-%d-keystore", buildID)

	// Decode keystore file from base64
	keystoreData, err := base64.StdEncoding.DecodeString(keystore.KeystoreFile)
	if err != nil {
		return "", fmt.Errorf("failed to decode keystore file: %v", err)
	}

	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: namespace,
			Labels: map[string]string{
				"app":      "flotio-build",
				"build-id": fmt.Sprintf("%d", buildID),
			},
		},
		Type: v1.SecretTypeOpaque,
		Data: map[string][]byte{
			"keystore.jks": keystoreData,
		},
		StringData: map[string]string{
			"store-password": keystore.StorePassword,
			"key-alias":      keystore.KeyAlias,
			"key-password":   keystore.KeyPassword,
		},
	}

	_, err = clientset.CoreV1().Secrets(namespace).Create(context.TODO(), secret, metav1.CreateOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to create Secret: %v", err)
	}

	return secretName, nil
}

// CreatePersistentVolumeClaimForArtifacts creates a PVC for storing build artifacts
func CreatePersistentVolumeClaimForArtifacts(clientset *kubernetes.Clientset, buildID uint, namespace string) (string, error) {
	pvcName := fmt.Sprintf("build-%d-artifacts", buildID)

	storageClassName := "standard" // Adjust based on your cluster
	storage := "5Gi"               // Adjust based on needs

	pvc := &v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pvcName,
			Namespace: namespace,
			Labels: map[string]string{
				"app":      "flotio-build",
				"build-id": fmt.Sprintf("%d", buildID),
			},
		},
		Spec: v1.PersistentVolumeClaimSpec{
			AccessModes: []v1.PersistentVolumeAccessMode{
				v1.ReadWriteOnce,
			},
			StorageClassName: &storageClassName,
			Resources: v1.VolumeResourceRequirements{
				Requests: v1.ResourceList{
					v1.ResourceStorage: parseQuantity(storage),
				},
			},
		},
	}

	_, err := clientset.CoreV1().PersistentVolumeClaims(namespace).Create(context.TODO(), pvc, metav1.CreateOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to create PVC: %v", err)
	}

	return pvcName, nil
}

// DeleteBuildResources deletes all Kubernetes resources associated with a build
func DeleteBuildResources(clientset *kubernetes.Clientset, buildID uint, namespace string) error {
	ctx := context.TODO()
	deletePolicy := metav1.DeletePropagationForeground

	// Delete Pod
	podName := fmt.Sprintf("build-%d", buildID)
	err := clientset.CoreV1().Pods(namespace).Delete(ctx, podName, metav1.DeleteOptions{
		PropagationPolicy: &deletePolicy,
	})
	if err != nil {
		// Log but don't fail if pod doesn't exist
		fmt.Printf("Warning: failed to delete pod %s: %v\n", podName, err)
	}

	// Delete ConfigMap
	configMapName := fmt.Sprintf("build-%d-env-files", buildID)
	err = clientset.CoreV1().ConfigMaps(namespace).Delete(ctx, configMapName, metav1.DeleteOptions{})
	if err != nil {
		fmt.Printf("Warning: failed to delete ConfigMap %s: %v\n", configMapName, err)
	}

	// Delete Secret
	secretName := fmt.Sprintf("build-%d-keystore", buildID)
	err = clientset.CoreV1().Secrets(namespace).Delete(ctx, secretName, metav1.DeleteOptions{})
	if err != nil {
		fmt.Printf("Warning: failed to delete Secret %s: %v\n", secretName, err)
	}

	// Delete PVC
	pvcName := fmt.Sprintf("build-%d-artifacts", buildID)
	err = clientset.CoreV1().PersistentVolumeClaims(namespace).Delete(ctx, pvcName, metav1.DeleteOptions{})
	if err != nil {
		fmt.Printf("Warning: failed to delete PVC %s: %v\n", pvcName, err)
	}

	return nil
}

// Helper functions
func replaceAll(s, old, new string) string {
	result := ""
	for _, char := range s {
		if string(char) == old {
			result += new
		} else {
			result += string(char)
		}
	}
	return result
}

func parseQuantity(s string) resource.Quantity {
	q, _ := resource.ParseQuantity(s)
	return q
}
