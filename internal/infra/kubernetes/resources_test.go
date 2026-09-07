package kubernetes

import (
	"encoding/base64"
	"os"
	"testing"

	"github.com/flotio-dev/core-api/internal/common/crypto"
	dbEngine "github.com/flotio-dev/core-api/internal/common/database"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func setKubeCryptoKey(t *testing.T) {
	t.Helper()
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 5)
	}
	os.Setenv("SECRETS_ENCRYPTION_KEY", base64.StdEncoding.EncodeToString(key))
	_ = crypto.Init()
}

func setupKubeDB(t *testing.T) *gorm.DB {
	t.Helper()
	setKubeCryptoKey(t)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("sqlite open error: %v", err)
	}
	_ = db.AutoMigrate(
		&dbEngine.Env{},
		&dbEngine.ProjectConfig{},
		&dbEngine.Keystore{},
		&dbEngine.GooglePlayCredentials{},
	)
	dbEngine.DB = db
	return db
}

func TestReplaceAll(t *testing.T) {
	got := replaceAll("a/b/c", "/", "__")
	if got != "a__b__c" {
		t.Errorf("expected a__b__c, got %s", got)
	}
}

func TestPodHelpers(t *testing.T) {
	// PodName
	name := GetPodName(42)
	if name == "" {
		t.Error("expected non-empty pod name")
	}

	// Namespace
	os.Unsetenv("K8S_NAMESPACE")
	if getNamespace() != "default" {
		t.Errorf("expected default, got %s", getNamespace())
	}
	os.Setenv("K8S_NAMESPACE", "custom-ns")
	if getNamespace() != "custom-ns" {
		t.Errorf("expected custom-ns, got %s", getNamespace())
	}

	// Image and PullPolicy
	os.Unsetenv("FLUTTER_BUILD_IMAGE")
	if getFlutterBuildImage() == "" {
		t.Error("expected non-empty image")
	}
	os.Setenv("FLUTTER_BUILD_IMAGE", "custom/flutter:latest")
	if getFlutterBuildImage() != "custom/flutter:latest" {
		t.Errorf("expected custom/flutter:latest, got %s", getFlutterBuildImage())
	}

	os.Unsetenv("FLUTTER_BUILD_IMAGE_PULL_POLICY")
	if getFlutterBuildImagePullPolicy() != v1.PullAlways {
		t.Errorf("expected PullAlways as default")
	}
	os.Setenv("FLUTTER_BUILD_IMAGE_PULL_POLICY", "IfNotPresent")
	if getFlutterBuildImagePullPolicy() != v1.PullIfNotPresent {
		t.Errorf("expected PullIfNotPresent")
	}
	os.Setenv("FLUTTER_BUILD_IMAGE_PULL_POLICY", "Never")
	if getFlutterBuildImagePullPolicy() != v1.PullNever {
		t.Errorf("expected PullNever")
	}

	// Build mode
	if getBuildMode("") != "release" || getBuildMode("debug") != "debug" {
		t.Errorf("unexpected build mode")
	}

	// Build target
	if getBuildTarget("android", "") != "apk" {
		t.Errorf("expected apk default for android")
	}
	if getBuildTarget("android", "aab") != "aab" {
		t.Errorf("expected aab")
	}
	if getBuildTarget("ios", "") != "ios" {
		t.Errorf("expected ios default for ios")
	}
	if getBuildTarget("web", "") != "web" {
		t.Errorf("expected web default for web")
	}
	if getBuildTarget("custom", "bin") != "bin" {
		t.Errorf("expected bin for custom platform")
	}

	// Channel
	if getFlutterChannel("") != "stable" || getFlutterChannel("beta") != "beta" {
		t.Errorf("unexpected flutter channel")
	}

	// S3 env helpers
	os.Unsetenv("AWS_S3_BUCKET")
	if getAWSS3Bucket() != "flotio-builds" {
		t.Errorf("expected default bucket")
	}
	os.Setenv("AWS_S3_BUCKET", "b1")
	if getAWSS3Bucket() != "b1" {
		t.Errorf("expected b1")
	}

	os.Unsetenv("AWS_REGION")
	if getAWSRegion() != "garage" {
		t.Errorf("expected default region garage")
	}
	os.Setenv("AWS_REGION", "us-east-1")
	if getAWSRegion() != "us-east-1" {
		t.Errorf("expected us-east-1")
	}

	os.Unsetenv("AWS_S3_ENDPOINT")
	if getAWSS3Endpoint() != "" {
		t.Errorf("expected empty default endpoint")
	}
	os.Setenv("AWS_S3_ENDPOINT", "http://minio:9000")
	if getAWSS3Endpoint() != "http://minio:9000" {
		t.Errorf("expected http://minio:9000")
	}

	os.Unsetenv("AWS_S3_PREFIX")
	if getAWSS3Prefix() != "builds" {
		t.Errorf("expected builds prefix")
	}
	os.Setenv("AWS_S3_PREFIX", "custom-p")
	if getAWSS3Prefix() != "custom-p" {
		t.Errorf("expected custom-p")
	}

	os.Unsetenv("AWS_S3_CACHE_PREFIX")
	if getAWSS3CachePrefix() != "build-cache" {
		t.Errorf("expected build-cache")
	}
	os.Setenv("AWS_S3_CACHE_PREFIX", "c1")
	if getAWSS3CachePrefix() != "c1" {
		t.Errorf("expected c1")
	}

	// Quantity & pointers
	q := parseQuantity("4Gi")
	expectedQ := resource.MustParse("4Gi")
	if (&q).Value() != (&expectedQ).Value() {
		t.Errorf("unexpected quantity")
	}
	p := int32Ptr(10)
	if *p != 10 {
		t.Errorf("expected 10")
	}

	// Node ready
	nodeReady := v1.Node{
		Status: v1.NodeStatus{
			Conditions: []v1.NodeCondition{
				{Type: v1.NodeReady, Status: v1.ConditionTrue},
			},
		},
	}
	if !isNodeReady(nodeReady) {
		t.Error("expected node to be ready")
	}
	nodeNotReady := v1.Node{
		Status: v1.NodeStatus{
			Conditions: []v1.NodeCondition{
				{Type: v1.NodeReady, Status: v1.ConditionFalse},
			},
		},
	}
	if isNodeReady(nodeNotReady) {
		t.Error("expected node not ready")
	}

	// Configured max concurrent pods
	os.Unsetenv("BUILD_MAX_CONCURRENT_PODS")
	limit, ok, err := getConfiguredMaxConcurrentBuildPods()
	if ok || err != nil || limit != 0 {
		t.Errorf("expected ok=false for unset env")
	}
	os.Setenv("BUILD_MAX_CONCURRENT_PODS", "8")
	limit, ok, err = getConfiguredMaxConcurrentBuildPods()
	if !ok || err != nil || limit != 8 {
		t.Errorf("expected 8, got %d", limit)
	}

	// Artifact URL
	os.Setenv("AWS_S3_ENDPOINT", "http://minio:9000")
	url := GetArtifactURL(42, "app.apk")
	if url != "http://minio:9000/b1/custom-p/42/app.apk" {
		t.Errorf("unexpected url: %s", url)
	}

	os.Unsetenv("AWS_S3_ENDPOINT")
	urlAWS := GetArtifactURL(42, "app.apk")
	if urlAWS != "https://b1.s3.us-east-1.amazonaws.com/custom-p/42/app.apk" {
		t.Errorf("unexpected urlAWS: %s", urlAWS)
	}

	// Schedule cleanup
	ScheduleBuildPodCleanup(12345)
}

func TestBuildEnvironmentVariables(t *testing.T) {
	cfg := BuildConfig{
		BuildID:     42,
		Platform:    "android",
		BuildMode:   "release",
		BuildTarget: "apk",
		GitBranch:   "main",
		GitUsername: "gituser",
		GitToken:    "gittoken",
	}
	envs := buildEnvironmentVariables(cfg)
	if len(envs) == 0 {
		t.Error("expected environment variables generated")
	}
}

func TestGenerateBuildRunnerScript_Full(t *testing.T) {
	cfg := BuildConfig{
		BuildID:              1,
		Platform:             "android",
		BuildMode:            "release",
		BuildTarget:          "aab",
		VersionName:          "1.0.0",
		VersionCode:          10,
		FlutterChannel:       "stable",
		GitBranch:            "main",
		CacheEnabled:         true,
		CacheUploadOnSuccess: true,
		CacheNamespace:       "proj/main",
		CacheTTLHours:        24,
	}
	projConfig := &dbEngine.ProjectConfig{
		Test:                       true,
		EnableFlutterAnalyze:       true,
		EnableFlutterTest:          true,
		EnableFlutterDriver:        true,
		FlutterDriverTargets:       []string{"integration_test/app_test.dart"},
		EnableAndroidCodeSigning:   true,
		EnableGooglePlayPublishing: true,
		GooglePlayTrack:            "internal",
		PostCloneScript:            "echo post-clone",
		PreTestScript:              "echo pre-test",
		PostTestScript:             "echo post-test",
		PreBuildScript:             "echo pre-build",
		PostBuildScript:            "echo post-build",
		PrePublishScript:           "echo pre-publish",
		DependencyCaching:          true,
		DependencyDirs:             []string{".pub-cache", ".gradle"},
	}

	script := GenerateBuildRunnerScript(cfg, projConfig)
	if script == "" {
		t.Fatal("expected non-empty script")
	}
}

func TestResources_NilDBAndEmptyConfigs(t *testing.T) {
	ns := "default"

	// 1. CreateConfigMapForEnvFiles with nil DB
	dbEngine.DB = nil
	nilCm, err := CreateConfigMapForEnvFiles(nil, 42, 10, ns)
	if err != nil || nilCm != "" {
		t.Errorf("expected empty cm on nil db")
	}

	// 2. CreateSecretForKeystore with nil DB
	ksName, err := CreateSecretForKeystore(nil, 42, 10, ns)
	if err != nil || ksName != "" {
		t.Errorf("expected empty secret on nil db")
	}

	// 3. CreateSecretForGooglePlay with nil DB
	gpName, err := CreateSecretForGooglePlay(nil, 42, 10, ns)
	if err != nil || gpName != "" {
		t.Errorf("expected empty secret on nil db")
	}

	// Setup DB with empty config (no keystore, no google play)
	db := setupKubeDB(t)
	dbEngine.DB = db

	pConfig := dbEngine.ProjectConfig{
		ProjectID: 99,
	}
	db.Create(&pConfig)

	// No files for env
	cmEmpty, err := CreateConfigMapForEnvFiles(nil, 42, 99, ns)
	if err != nil || cmEmpty != "" {
		t.Errorf("expected empty cm when no env files exist")
	}

	// No keystore ID
	ksEmpty, err := CreateSecretForKeystore(nil, 42, 99, ns)
	if err != nil || ksEmpty != "" {
		t.Errorf("expected empty keystore secret when KeystoreID is nil")
	}

	// No google play ID
	gpEmpty, err := CreateSecretForGooglePlay(nil, 42, 99, ns)
	if err != nil || gpEmpty != "" {
		t.Errorf("expected empty gp secret when GooglePlayCredentialsID is nil")
	}
}

func TestGenerateBuildRunnerScript_Platforms(t *testing.T) {
	// 1. iOS platform
	iosCfg := BuildConfig{
		BuildID:     2,
		Platform:    "ios",
		BuildMode:   "debug",
		BuildTarget: "ios",
		GitBranch:   "dev",
	}
	iosProjConfig := &dbEngine.ProjectConfig{
		IosBuildArgs:   "--no-codesign",
		XcodeVersion:   "15.0",
		ProjectPath:    "subfolder/app",
		FlutterVersion: "3.24.0",
	}
	iosScript := GenerateBuildRunnerScript(iosCfg, iosProjConfig)
	if iosScript == "" {
		t.Error("expected ios script generated")
	}

	// 2. Web platform
	webCfg := BuildConfig{
		BuildID:     3,
		Platform:    "web",
		BuildMode:   "release",
		BuildTarget: "web",
	}
	webProjConfig := &dbEngine.ProjectConfig{
		WebBuildArgs: "--wasm",
	}
	webScript := GenerateBuildRunnerScript(webCfg, webProjConfig)
	if webScript == "" {
		t.Error("expected web script generated")
	}

	// 3. Nil projectConfig
	nilProjScript := GenerateBuildRunnerScript(BuildConfig{Platform: "android"}, nil)
	if nilProjScript == "" {
		t.Error("expected script with nil projectConfig")
	}

	// 4. Android with custom build args, no test, no caching
	androidCfg := BuildConfig{
		BuildID:     4,
		Platform:    "android",
		BuildMode:   "profile",
		BuildTarget: "apk",
	}
	androidProjConfig := &dbEngine.ProjectConfig{
		AndroidBuildArgs:         "--split-per-abi",
		Test:                     false,
		EnableAndroidCodeSigning: false,
	}
	androidScript := GenerateBuildRunnerScript(androidCfg, androidProjConfig)
	if androidScript == "" {
		t.Error("expected android script generated")
	}
}

func TestPodOperations_NoCluster(t *testing.T) {
	os.Setenv("KUBECONFIG", "/nonexistent/path/kubeconfig")

	// 1. getKubernetesConfig
	_, err := getKubernetesConfig()
	if err == nil {
		t.Error("expected error for nonexistent kubeconfig")
	}

	// 2. CreateBuildPod
	err = CreateBuildPod(BuildConfig{BuildID: 99})
	if err == nil {
		t.Error("expected error for CreateBuildPod without cluster")
	}

	// 3. HasBuildPodCapacity
	_, err = HasBuildPodCapacity()
	if err == nil {
		t.Error("expected error for HasBuildPodCapacity without cluster")
	}

	// 4. GetPodStatus
	_, err = GetPodStatus(99)
	if err == nil {
		t.Error("expected error for GetPodStatus without cluster")
	}

	// 5. DeleteBuildPod
	err = DeleteBuildPod(99)
	if err == nil {
		t.Error("expected error for DeleteBuildPod without cluster")
	}

	// 6. GetPodLogs
	_, err = GetPodLogs(99)
	if err == nil {
		t.Error("expected error for GetPodLogs without cluster")
	}

	// 7. StreamPodLogs
	ch := make(chan string, 1)
	err = StreamPodLogs(99, ch)
	if err == nil {
		t.Error("expected error for StreamPodLogs without cluster")
	}
}
