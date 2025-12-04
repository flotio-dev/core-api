package s3

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

var client *minio.Client

// GetClient returns a singleton MinIO/S3 client
func GetClient() (*minio.Client, error) {
	if client != nil {
		return client, nil
	}

	endpoint := getEndpoint()
	accessKey := os.Getenv("AWS_ACCESS_KEY_ID")
	secretKey := os.Getenv("AWS_SECRET_ACCESS_KEY")
	useSSL := getUseSSL()

	var err error
	client, err = minio.New(endpoint, &minio.Options{
		Creds:        credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure:       useSSL,
		Region:       getRegion(),
		BucketLookup: minio.BucketLookupAuto,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create S3 client: %v", err)
	}

	return client, nil
}

// GetBucket returns the S3 bucket name
func GetBucket() string {
	bucket := os.Getenv("AWS_S3_BUCKET")
	if bucket == "" {
		bucket = "flotio-builds"
	}
	return bucket
}

// GetPrefix returns the S3 prefix/folder for storing build artifacts
func GetPrefix() string {
	prefix := os.Getenv("AWS_S3_PREFIX")
	if prefix == "" {
		prefix = "builds"
	}
	return prefix
}

// getEndpoint returns the S3 endpoint (without protocol)
func getEndpoint() string {
	endpoint := os.Getenv("AWS_S3_ENDPOINT")
	if endpoint == "" {
		// Default to AWS S3
		region := getRegion()
		return fmt.Sprintf("s3.%s.amazonaws.com", region)
	}
	// Remove protocol prefix if present
	if len(endpoint) > 8 && endpoint[:8] == "https://" {
		return endpoint[8:]
	}
	if len(endpoint) > 7 && endpoint[:7] == "http://" {
		return endpoint[7:]
	}
	return endpoint
}

// getRegion returns the AWS region
func getRegion() string {
	region := os.Getenv("AWS_REGION")
	if region == "" {
		region = "garage"
	}
	return region
}

// getUseSSL determines whether to use SSL based on endpoint
func getUseSSL() bool {
	endpoint := os.Getenv("AWS_S3_ENDPOINT")
	// If endpoint starts with http://, don't use SSL
	if len(endpoint) > 7 && endpoint[:7] == "http://" {
		return false
	}
	// Default to SSL for production
	useSSL := os.Getenv("AWS_S3_USE_SSL")
	if useSSL == "false" {
		return false
	}
	return true
}

// GetBuildArtifactKey generates the S3 key for a build artifact
func GetBuildArtifactKey(buildID uint, artifactName string) string {
	return fmt.Sprintf("%s/%d/%s", GetPrefix(), buildID, artifactName)
}

// ListBuildArtifacts lists all artifacts for a specific build in S3
func ListBuildArtifacts(buildID uint) ([]string, error) {
	minioClient, err := GetClient()
	if err != nil {
		return nil, err
	}

	bucket := GetBucket()
	prefix := fmt.Sprintf("%s/%d/", GetPrefix(), buildID)

	var artifacts []string
	ctx := context.Background()

	objectCh := minioClient.ListObjects(ctx, bucket, minio.ListObjectsOptions{
		Prefix:    prefix,
		Recursive: true,
	})

	for object := range objectCh {
		if object.Err != nil {
			return nil, fmt.Errorf("error listing objects: %v", object.Err)
		}
		artifacts = append(artifacts, object.Key)
	}

	return artifacts, nil
}

// FindPrimaryArtifactKey finds the main artifact key for a build (APK, AAB, or IPA)
func FindPrimaryArtifactKey(buildID uint, platform string) (string, error) {
	artifacts, err := ListBuildArtifacts(buildID)
	if err != nil {
		return "", err
	}

	// Define artifact priority based on platform
	var priorityExtensions []string
	switch platform {
	case "android":
		priorityExtensions = []string{".aab", ".apk"}
	case "ios":
		priorityExtensions = []string{".ipa"}
	case "web":
		priorityExtensions = []string{".zip", ".tar.gz"}
	default:
		priorityExtensions = []string{".aab", ".apk", ".ipa", ".zip"}
	}

	// Search for artifacts in priority order
	for _, ext := range priorityExtensions {
		for _, artifact := range artifacts {
			if len(artifact) >= len(ext) && artifact[len(artifact)-len(ext):] == ext {
				return artifact, nil
			}
		}
	}

	// If no primary artifact found but artifacts exist, return the first one
	if len(artifacts) > 0 {
		return artifacts[0], nil
	}

	return "", fmt.Errorf("no artifacts found for build %d", buildID)
}

// ObjectExists checks if an object exists in S3
func ObjectExists(key string) (bool, error) {
	minioClient, err := GetClient()
	if err != nil {
		return false, err
	}

	bucket := GetBucket()
	ctx := context.Background()

	_, err = minioClient.StatObject(ctx, bucket, key, minio.StatObjectOptions{})
	if err != nil {
		errResponse := minio.ToErrorResponse(err)
		if errResponse.Code == "NoSuchKey" {
			return false, nil
		}
		return false, err
	}

	return true, nil
}

// GetPresignedURL generates a presigned URL for downloading an artifact
func GetPresignedURL(key string, expirySeconds int) (string, error) {
	minioClient, err := GetClient()
	if err != nil {
		return "", err
	}

	bucket := GetBucket()
	ctx := context.Background()

	expiry := time.Duration(expirySeconds) * time.Second
	presignedURL, err := minioClient.PresignedGetObject(ctx, bucket, key, expiry, nil)
	if err != nil {
		return "", fmt.Errorf("failed to generate presigned URL: %v", err)
	}

	return presignedURL.String(), nil
}
