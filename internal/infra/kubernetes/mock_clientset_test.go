package kubernetes

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	dbEngine "github.com/flotio-dev/core-api/internal/common/database"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

func setupMockK8sServer(t *testing.T) (*kubernetes.Clientset, *httptest.Server) {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Log streaming endpoint
		if strings.HasSuffix(r.URL.Path, "/log") {
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("build log line 1\nbuild log line 2\n"))
			return
		}

		w.Header().Set("Content-Type", "application/json")

		// 1. Nodes list
		if r.Method == "GET" && r.URL.Path == "/api/v1/nodes" {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"items": []map[string]interface{}{
					{
						"status": map[string]interface{}{
							"allocatable": map[string]string{"memory": "16Gi"},
							"conditions": []map[string]string{
								{"type": "Ready", "status": "True"},
							},
						},
					},
				},
			})
			return
		}

		// 2. Pods get single
		if r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/api/v1/namespaces/flotio/pods/build-") {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"metadata": map[string]string{"name": "build-1"},
				"status":   map[string]string{"phase": "Running"},
			})
			return
		}

		// 3. Pods list
		if r.Method == "GET" && strings.Contains(r.URL.Path, "/pods") {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"items": []map[string]interface{}{
					{
						"metadata": map[string]string{"name": "build-1"},
						"status":   map[string]string{"phase": "Running"},
					},
				},
			})
			return
		}

		// 4. Create Pod / ConfigMap / Secret
		if r.Method == "POST" {
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"metadata": map[string]string{"name": "created-resource"},
			})
			return
		}

		// 5. Delete Pod / ConfigMap / Secret
		if r.Method == "DELETE" {
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"status": "Success"})
			return
		}

		http.NotFound(w, r)
	}))

	t.Setenv("KUBECTL_API", ts.URL)
	t.Setenv("KUBECTL_TOKEN", "mock-token")
	t.Setenv("K8S_NAMESPACE", "flotio")

	cs, err := kubernetes.NewForConfig(&rest.Config{
		Host: ts.URL,
	})
	if err != nil {
		t.Fatalf("failed to build clientset for mock: %v", err)
	}

	return cs, ts
}

func TestKubeResources_WithMockK8s(t *testing.T) {
	cs, ts := setupMockK8sServer(t)
	defer ts.Close()

	db := setupKubeDB(t)
	ns := "flotio"

	// 1. CreateConfigMapForRunScript
	cmScript, err := CreateConfigMapForRunScript(cs, 10, "echo build", ns)
	if err != nil || cmScript != "build-run-script-10" {
		t.Errorf("CreateConfigMapForRunScript failed: %v, %s", err, cmScript)
	}

	// 2. CreateConfigMapForEnvFiles with records
	envFile := dbEngine.Env{
		ProjectID: &[]uint{5}[0],
		Type:      "file",
		Key:       "google-services.json",
		Value:     "{}",
		Path:      "android/app/google-services.json",
	}
	db.Create(&envFile)
	cmEnv, err := CreateConfigMapForEnvFiles(cs, 10, 5, ns)
	if err != nil || cmEnv != "build-10-env-files" {
		t.Errorf("CreateConfigMapForEnvFiles failed: %v, %s", err, cmEnv)
	}

	// 3. CreateSecretForKeystore with records
	keystore := dbEngine.Keystore{
		Name:          "my-ks",
		KeystoreFile:  "ZmlsZQ==", // "file" in base64
		StorePassword: "pass",
		KeyPassword:   "pass",
		KeyAlias:      "alias",
	}
	db.Create(&keystore)
	pConfig := dbEngine.ProjectConfig{
		ProjectID:  5,
		KeystoreID: &keystore.ID,
	}
	db.Create(&pConfig)

	ksName, err := CreateSecretForKeystore(cs, 10, 5, ns)
	if err != nil || ksName != "build-10-keystore" {
		t.Errorf("CreateSecretForKeystore failed: %v, %s", err, ksName)
	}

	// 4. CreateSecretForGooglePlay with records
	gpCreds := dbEngine.GooglePlayCredentials{
		Name:        "gp",
		Credentials: `{"type":"service_account"}`,
	}
	db.Create(&gpCreds)
	pConfig.GooglePlayCredentialsID = &gpCreds.ID
	db.Save(&pConfig)

	gpName, err := CreateSecretForGooglePlay(cs, 10, 5, ns)
	if err != nil || gpName != "build-10-google-play" {
		t.Errorf("CreateSecretForGooglePlay failed: %v, %s", err, gpName)
	}

	// 5. DeleteBuildResources
	err = DeleteBuildResources(cs, 10, ns)
	if err != nil {
		t.Errorf("DeleteBuildResources failed: %v", err)
	}

	// 6. getTotalReadyNodeAllocatableMemory
	mem, err := getTotalReadyNodeAllocatableMemory(cs)
	if err != nil || mem == 0 {
		t.Errorf("getTotalReadyNodeAllocatableMemory failed: %v, %d", err, mem)
	}

	// 7. countActiveBuildPods
	active, err := countActiveBuildPods(cs, ns)
	if err != nil || active != 1 {
		t.Errorf("countActiveBuildPods failed: %v, active=%d", err, active)
	}
}

func TestUpdateBuildStatusFromPod_And_Artifacts(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "pod-success") {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"status": map[string]string{"phase": "Succeeded"},
			})
			return
		}
		if strings.Contains(r.URL.Path, "pod-fail") {
			_ = json.NewEncoder(w).Encode(map[string]string{"phase": "Failed"},
			)
			return
		}
		http.NotFound(w, r)
	}))
	defer ts.Close()

	cs, err := kubernetes.NewForConfig(&rest.Config{Host: ts.URL})
	if err != nil {
		t.Fatal(err)
	}

	db := setupKubeDB(t)
	_ = db.AutoMigrate(&dbEngine.Build{})
	ns := "flotio"

	b1 := dbEngine.Build{Status: "running"}
	b2 := dbEngine.Build{Status: "running"}
	b3 := dbEngine.Build{Status: "cancelled"}
	db.Create(&b1)
	db.Create(&b2)
	db.Create(&b3)

	// 1. Pod succeeded
	updateBuildStatusFromPod(cs, ns, "pod-success", b1.ID)

	// 2. Pod failed
	updateBuildStatusFromPod(cs, ns, "pod-fail", b2.ID)

	// 3. Build already cancelled
	updateBuildStatusFromPod(cs, ns, "pod-success", b3.ID)

	// 4. Pod not found
	updateBuildStatusFromPod(cs, ns, "pod-missing", b1.ID)

	// 5. Build not in DB
	updateBuildStatusFromPod(cs, ns, "pod-success", 99999)

	// 6. GetBuildArtifacts
	arts, err := GetBuildArtifacts(42)
	if err != nil || len(arts) != 3 {
		t.Errorf("GetBuildArtifacts failed: %v, %v", err, arts)
	}
}

func TestPodOperations_WithMock(t *testing.T) {
	_, ts := setupMockK8sServer(t)
	defer ts.Close()

	db := setupKubeDB(t)

	// Create supporting models for project 5
	keystore := dbEngine.Keystore{
		Name:          "mock-ks",
		KeystoreFile:  "ZmlsZQ==",
		StorePassword: "pass",
		KeyPassword:   "pass",
		KeyAlias:      "alias",
	}
	db.Create(&keystore)

	gpCreds := dbEngine.GooglePlayCredentials{
		Name:        "mock-gp",
		Credentials: `{"type":"service_account"}`,
	}
	db.Create(&gpCreds)

	pConfig := dbEngine.ProjectConfig{
		ProjectID:               5,
		KeystoreID:              &keystore.ID,
		GooglePlayCredentialsID: &gpCreds.ID,
	}
	db.Create(&pConfig)

	proj := dbEngine.Project{
		Name: "test-proj",
	}
	proj.ID = 5
	db.Create(&proj)

	bConfig := BuildConfig{
		BuildID:       1,
		Project:       proj,
		Platform:      "android",
		ProjectConfig: &pConfig,
		BuildMode:     "release",
		FlutterChannel: "stable",
	}

	// 1. CreateBuildPod
	err := CreateBuildPod(bConfig)
	if err != nil {
		t.Errorf("CreateBuildPod failed: %v", err)
	}

	// 2. HasBuildPodCapacity
	cap, err := HasBuildPodCapacity()
	if err != nil || !cap {
		t.Errorf("HasBuildPodCapacity failed: %v, cap=%v", err, cap)
	}

	// 3. GetPodLogs
	logs, err := GetPodLogs(1)
	if err != nil || len(logs) == 0 {
		t.Errorf("GetPodLogs failed: %v, logs=%v", err, logs)
	}

	// 4. StreamPodLogs
	ch := make(chan string, 5)
	err = StreamPodLogs(1, ch)
	if err != nil {
		t.Errorf("StreamPodLogs failed: %v", err)
	}
	var streamLogs []string
	for l := range ch {
		streamLogs = append(streamLogs, l)
	}
	if len(streamLogs) == 0 {
		t.Errorf("StreamPodLogs produced no logs")
	}

	// 5. GetPodStatus
	status, err := GetPodStatus(1)
	if err != nil || status != "Running" {
		t.Errorf("GetPodStatus failed: %v, status=%s", err, status)
	}

	// 6. DeleteBuildPod
	err = DeleteBuildPod(1)
	if err != nil {
		t.Errorf("DeleteBuildPod failed: %v", err)
	}

	// 7. ScheduleBuildPodCleanup
	ScheduleBuildPodCleanup(1)
	ScheduleBuildPodCleanup(1) // Duplicate call should be ignored immediately
}

