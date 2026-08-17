package kubernetes

import (
	"context"
	"encoding/base64"
	"fmt"
	"strconv"

	"github.com/flotio-dev/core-api/internal/common/crypto"
	dbEngine "github.com/flotio-dev/core-api/internal/common/database"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// CreateConfigMapForEnvFiles creates a ConfigMap containing environment files for a build
func CreateConfigMapForEnvFiles(clientset *kubernetes.Clientset, buildID uint, projectID uint, namespace string) (string, error) {
	// Check if database is initialized
	if dbEngine.DB == nil {
		// No database connection, skip environment files
		return "", nil
	}

	// Fetch environment files from database
	var envs []dbEngine.Env
	if err := dbEngine.DB.Where("project_id = ? AND type = ?", projectID, "file").Find(&envs).Error; err != nil {
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

// CreateConfigMapForRunScript creates a ConfigMap containing the modular build runner script
func CreateConfigMapForRunScript(clientset *kubernetes.Clientset, buildID uint, script string, namespace string) (string, error) {
	configMapName := fmt.Sprintf("build-run-script-%d", buildID)

	configMap := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapName,
			Namespace: namespace,
			Labels: map[string]string{
				"app":      "flotio-build",
				"build-id": strconv.Itoa(int(buildID)),
			},
		},
		Data: map[string]string{
			"build.sh": script,
		},
	}

	_, err := clientset.CoreV1().ConfigMaps(namespace).Create(context.TODO(), configMap, metav1.CreateOptions{})
	if err != nil {
		if apierrors.IsAlreadyExists(err) {
			return configMapName, nil
		}
		return "", err
	}

	return configMapName, nil
}

// CreateSecretForKeystore creates a Secret containing the keystore and credentials
func CreateSecretForKeystore(clientset *kubernetes.Clientset, buildID uint, projectID uint, namespace string) (string, error) {
	// Check if database is initialized
	if dbEngine.DB == nil {
		// No database connection, skip keystore
		return "", nil
	}

	// Fetch active keystore from database
	var config dbEngine.ProjectConfig
	if err := dbEngine.DB.Where("project_id = ?", projectID).First(&config).Error; err != nil {
		return "", nil // No config, no keystore (not an error)
	}

	if config.KeystoreID == nil {
		return "", nil // No keystore configured (not an error)
	}

	var keystore dbEngine.Keystore
	if err := dbEngine.DB.First(&keystore, *config.KeystoreID).Error; err != nil {
		return "", nil // Keystore not found (not an error)
	}

	secretName := fmt.Sprintf("build-%d-keystore", buildID)

	// Decrypt secrets at rest before use. Decrypt passes through legacy
	// plaintext (values without the "enc:v1:" prefix), so this keeps working
	// for keystores stored before encryption was rolled out.
	keystoreFile, err := crypto.Decrypt(keystore.KeystoreFile)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt keystore file: %v", err)
	}
	storePassword, err := crypto.Decrypt(keystore.StorePassword)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt store password: %v", err)
	}
	keyPassword, err := crypto.Decrypt(keystore.KeyPassword)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt key password: %v", err)
	}

	// The keystore file is stored as base64; decode it to raw bytes.
	keystoreData, err := base64.StdEncoding.DecodeString(keystoreFile)
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
			"store-password": storePassword,
			"key-alias":      keystore.KeyAlias,
			"key-password":   keyPassword,
		},
	}

	_, err = clientset.CoreV1().Secrets(namespace).Create(context.TODO(), secret, metav1.CreateOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to create Secret: %v", err)
	}

	return secretName, nil
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

	return nil
}

// Helper function
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
