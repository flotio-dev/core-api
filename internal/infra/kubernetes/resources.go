package kubernetes

import (
	"context"
	"encoding/base64"
	"fmt"

	appCrypto "github.com/flotio-dev/core-api/internal/common/crypto"
	dbEngine "github.com/flotio-dev/core-api/internal/common/database"
	s3Client "github.com/flotio-dev/core-api/internal/infra/s3"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// CreateConfigMapForEnvFiles creates a ConfigMap containing environment files for a build
// @Summary		Create ConfigMap for environment files
// @Description	Creates a Kubernetes ConfigMap containing environment files for a build
// @Tags			kubernetes
// @Accept		json
// @Produce		json
// @Param		buildID		path		int		true	"Build ID"
// @Param		projectID	path		int		true	"Project ID"
// @Success		200	{string}	string	"ConfigMap name"
// @Failure		500	{object}	map[string]string
// @Router		/internal/kubernetes/configmap [post]
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

// CreateSecretForKeystore creates a Secret containing the keystore and credentials
// @Summary		Create Secret for keystore
// @Description	Creates a Kubernetes Secret containing the Android keystore and credentials for signing
// @Tags			kubernetes
// @Accept		json
// @Produce		json
// @Param		buildID		path		int		true	"Build ID"
// @Param		projectID	path		int		true	"Project ID"
// @Success		200	{string}	string	"Secret name"
// @Failure		500	{object}	map[string]string
// @Router		/internal/kubernetes/secret [post]
func CreateSecretForKeystore(clientset *kubernetes.Clientset, buildID uint, projectID uint, namespace string) (string, error) {
	if dbEngine.DB == nil {
		return "", nil
	}

	// Prefer "release" signing config, fall back to "debug"
	var config dbEngine.AndroidSigningConfig
	if err := dbEngine.DB.Where("project_id = ? AND build_type = ?", projectID, "release").First(&config).Error; err != nil {
		if err2 := dbEngine.DB.Where("project_id = ? AND build_type = ?", projectID, "debug").First(&config).Error; err2 != nil {
			return "", nil // No signing config configured (not an error)
		}
	}

	// Download keystore binary from S3
	keystoreData, err := s3Client.DownloadKeystore(config.KeystorePath)
	if err != nil {
		return "", fmt.Errorf("failed to download keystore from storage: %v", err)
	}

	// Decrypt passwords
	keystorePassword, err := appCrypto.Decrypt(config.KeystorePasswordEncrypted)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt keystore password: %v", err)
	}
	keyPassword, err := appCrypto.Decrypt(config.KeyPasswordEncrypted)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt key password: %v", err)
	}

	secretName := fmt.Sprintf("build-%d-keystore", buildID)

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
			"store-password": keystorePassword,
			"key-alias":      config.KeyAlias,
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
// @Summary		Delete build resources
// @Description	Deletes all Kubernetes resources (Pod, ConfigMap, Secret) associated with a build
// @Tags			kubernetes
// @Accept		json
// @Produce		json
// @Param		buildID	path		int	true	"Build ID"
// @Success		200
// @Failure		500	{object}	map[string]string
// @Router		/internal/kubernetes/build/{buildID}/resources [delete]
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
