package s3

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

var client *minio.Client

var cachePurgeRequests atomic.Uint64
var cachePurgedObjects atomic.Uint64
var cacheMetricsRequests atomic.Uint64

type CacheNamespaceMetrics struct {
	Namespace      string     `json:"namespace"`
	Fingerprint    string     `json:"fingerprint,omitempty"`
	Prefix         string     `json:"prefix"`
	ObjectCount    int64      `json:"object_count"`
	TotalSizeBytes int64      `json:"total_size_bytes"`
	LastModifiedAt *time.Time `json:"last_modified_at,omitempty"`
}

type CacheEntry struct {
	Fingerprint    string     `json:"fingerprint"`
	Prefix         string     `json:"prefix"`
	ObjectCount    int64      `json:"object_count"`
	TotalSizeBytes int64      `json:"total_size_bytes"`
	LastModifiedAt *time.Time `json:"last_modified_at,omitempty"`
}

type CacheOperationalMetrics struct {
	PurgeRequests   uint64 `json:"purge_requests"`
	PurgedObjects   uint64 `json:"purged_objects"`
	MetricsRequests uint64 `json:"metrics_requests"`
}

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

// GetCachePrefix returns the S3 prefix/folder for storing dependency caches
func GetCachePrefix() string {
	prefix := os.Getenv("AWS_S3_CACHE_PREFIX")
	if prefix == "" {
		prefix = "build-cache"
	}
	return strings.Trim(prefix, "/")
}

// BuildCacheNamespacePrefix returns the S3 prefix path for a cache namespace.
// Example: build-cache/project-12/main/
func BuildCacheNamespacePrefix(cacheNamespace string) string {
	cacheNamespace = strings.Trim(cacheNamespace, "/")
	if cacheNamespace == "" {
		return GetCachePrefix() + "/"
	}
	return fmt.Sprintf("%s/%s/", GetCachePrefix(), cacheNamespace)
}

func buildCacheScopePrefix(cacheNamespace string, cacheFingerprint string) string {
	basePrefix := BuildCacheNamespacePrefix(cacheNamespace)
	cacheFingerprint = strings.Trim(strings.ToLower(cacheFingerprint), "/")
	if cacheFingerprint == "" {
		return basePrefix
	}
	return fmt.Sprintf("%s%s/", basePrefix, cacheFingerprint)
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

// DeleteBuildArtifacts deletes all artifacts for a specific build from S3
func DeleteBuildArtifacts(buildID uint) error {
	minioClient, err := GetClient()
	if err != nil {
		return err
	}

	bucket := GetBucket()
	prefix := fmt.Sprintf("%s/%d/", GetPrefix(), buildID)
	ctx := context.Background()

	// List all objects with the build prefix
	objectCh := minioClient.ListObjects(ctx, bucket, minio.ListObjectsOptions{
		Prefix:    prefix,
		Recursive: true,
	})

	// Delete each object
	var deleteErrors []error
	for object := range objectCh {
		if object.Err != nil {
			deleteErrors = append(deleteErrors, fmt.Errorf("error listing object: %v", object.Err))
			continue
		}

		err := minioClient.RemoveObject(ctx, bucket, object.Key, minio.RemoveObjectOptions{})
		if err != nil {
			deleteErrors = append(deleteErrors, fmt.Errorf("failed to delete %s: %v", object.Key, err))
		}
	}

	if len(deleteErrors) > 0 {
		return fmt.Errorf("failed to delete some artifacts: %v", deleteErrors)
	}

	return nil
}

// DeleteObject deletes a single object from S3
func DeleteObject(key string) error {
	minioClient, err := GetClient()
	if err != nil {
		return err
	}

	bucket := GetBucket()
	ctx := context.Background()

	err = minioClient.RemoveObject(ctx, bucket, key, minio.RemoveObjectOptions{})
	if err != nil {
		return fmt.Errorf("failed to delete object %s: %v", key, err)
	}

	return nil
}

// GetCacheNamespaceMetrics returns object count and total size for a given cache namespace prefix.
func GetCacheNamespaceMetrics(cacheNamespace string) (CacheNamespaceMetrics, error) {
	return GetCacheScopeMetrics(cacheNamespace, "")
}

// GetCacheScopeMetrics returns object count and total size for a given cache namespace,
// optionally narrowed to a cache fingerprint.
func GetCacheScopeMetrics(cacheNamespace string, cacheFingerprint string) (CacheNamespaceMetrics, error) {
	cacheMetricsRequests.Add(1)

	minioClient, err := GetClient()
	if err != nil {
		return CacheNamespaceMetrics{}, err
	}

	bucket := GetBucket()
	prefix := buildCacheScopePrefix(cacheNamespace, cacheFingerprint)
	ctx := context.Background()

	metrics := CacheNamespaceMetrics{
		Namespace:   cacheNamespace,
		Fingerprint: strings.TrimSpace(cacheFingerprint),
		Prefix:      prefix,
	}

	objectCh := minioClient.ListObjects(ctx, bucket, minio.ListObjectsOptions{
		Prefix:    prefix,
		Recursive: true,
	})

	var latest time.Time
	for object := range objectCh {
		if object.Err != nil {
			return CacheNamespaceMetrics{}, fmt.Errorf("error listing cache objects: %v", object.Err)
		}

		metrics.ObjectCount++
		metrics.TotalSizeBytes += object.Size
		if object.LastModified.After(latest) {
			latest = object.LastModified
		}
	}

	if !latest.IsZero() {
		lastModified := latest
		metrics.LastModifiedAt = &lastModified
	}

	return metrics, nil
}

// DeleteCacheNamespace deletes all objects under a given cache namespace prefix.
func DeleteCacheNamespace(cacheNamespace string) (int, error) {
	return DeleteCacheScope(cacheNamespace, "")
}

// DeleteCacheScope deletes all objects under a cache namespace,
// optionally narrowed to a cache fingerprint.
func DeleteCacheScope(cacheNamespace string, cacheFingerprint string) (int, error) {
	cachePurgeRequests.Add(1)

	minioClient, err := GetClient()
	if err != nil {
		return 0, err
	}

	bucket := GetBucket()
	prefix := buildCacheScopePrefix(cacheNamespace, cacheFingerprint)
	ctx := context.Background()

	objectCh := minioClient.ListObjects(ctx, bucket, minio.ListObjectsOptions{
		Prefix:    prefix,
		Recursive: true,
	})

	deletedCount := 0
	var deleteErrors []error
	for object := range objectCh {
		if object.Err != nil {
			deleteErrors = append(deleteErrors, fmt.Errorf("error listing cache object: %v", object.Err))
			continue
		}

		if err := minioClient.RemoveObject(ctx, bucket, object.Key, minio.RemoveObjectOptions{}); err != nil {
			deleteErrors = append(deleteErrors, fmt.Errorf("failed to delete cache object %s: %v", object.Key, err))
			continue
		}

		deletedCount++
	}

	if deletedCount > 0 {
		cachePurgedObjects.Add(uint64(deletedCount))
	}

	if len(deleteErrors) > 0 {
		return deletedCount, fmt.Errorf("failed to delete some cache objects: %v", deleteErrors)
	}

	return deletedCount, nil
}

// GetCacheOperationalMetrics returns in-memory counters for cache operations.
func GetCacheOperationalMetrics() CacheOperationalMetrics {
	return CacheOperationalMetrics{
		PurgeRequests:   cachePurgeRequests.Load(),
		PurgedObjects:   cachePurgedObjects.Load(),
		MetricsRequests: cacheMetricsRequests.Load(),
	}
}

// GetKeystorePrefix returns the S3 prefix used for storing Android keystores.
func GetKeystorePrefix() string {
	prefix := os.Getenv("AWS_S3_KEYSTORE_PREFIX")
	if prefix == "" {
		prefix = "keystores"
	}
	return strings.Trim(prefix, "/")
}

// GetKeystoreKey returns the S3 object key for a keystore file.
// Pattern: keystores/{projectID}/{keystoreID}.jks
func GetKeystoreKey(projectID uint, keystoreID string) string {
	return fmt.Sprintf("%s/%d/%s.jks", GetKeystorePrefix(), projectID, keystoreID)
}

// UploadKeystore uploads a keystore binary to S3 and returns the S3 object key.
func UploadKeystore(projectID uint, keystoreID string, data []byte) (string, error) {
	minioClient, err := GetClient()
	if err != nil {
		return "", err
	}

	bucket := GetBucket()
	key := GetKeystoreKey(projectID, keystoreID)
	ctx := context.Background()

	_, err = minioClient.PutObject(ctx, bucket, key, bytes.NewReader(data), int64(len(data)), minio.PutObjectOptions{
		ContentType: "application/octet-stream",
	})
	if err != nil {
		return "", fmt.Errorf("failed to upload keystore: %v", err)
	}

	return key, nil
}

// DownloadKeystore downloads a keystore binary from S3.
func DownloadKeystore(key string) ([]byte, error) {
	minioClient, err := GetClient()
	if err != nil {
		return nil, err
	}

	bucket := GetBucket()
	ctx := context.Background()

	obj, err := minioClient.GetObject(ctx, bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get keystore object: %v", err)
	}
	defer obj.Close()

	data, err := io.ReadAll(obj)
	if err != nil {
		return nil, fmt.Errorf("failed to read keystore object: %v", err)
	}

	return data, nil
}

// ListCacheEntries lists cache entries grouped by fingerprint under the given namespace.
func ListCacheEntries(cacheNamespace string) ([]CacheEntry, error) {
	minioClient, err := GetClient()
	if err != nil {
		return nil, err
	}

	bucket := GetBucket()
	prefix := BuildCacheNamespacePrefix(cacheNamespace)
	ctx := context.Background()

	entriesByFingerprint := make(map[string]*CacheEntry)
	objectCh := minioClient.ListObjects(ctx, bucket, minio.ListObjectsOptions{
		Prefix:    prefix,
		Recursive: true,
	})

	for object := range objectCh {
		if object.Err != nil {
			return nil, fmt.Errorf("error listing cache entries: %v", object.Err)
		}

		relativeKey := strings.TrimPrefix(object.Key, prefix)
		if relativeKey == "" {
			continue
		}

		parts := strings.SplitN(relativeKey, "/", 2)
		fingerprint := strings.TrimSpace(parts[0])
		if fingerprint == "" {
			continue
		}

		entry, exists := entriesByFingerprint[fingerprint]
		if !exists {
			entry = &CacheEntry{
				Fingerprint: fingerprint,
				Prefix:      buildCacheScopePrefix(cacheNamespace, fingerprint),
			}
			entriesByFingerprint[fingerprint] = entry
		}

		entry.ObjectCount++
		entry.TotalSizeBytes += object.Size
		if entry.LastModifiedAt == nil || object.LastModified.After(*entry.LastModifiedAt) {
			lastModified := object.LastModified
			entry.LastModifiedAt = &lastModified
		}
	}

	entries := make([]CacheEntry, 0, len(entriesByFingerprint))
	for _, entry := range entriesByFingerprint {
		entries = append(entries, *entry)
	}

	return entries, nil
}
