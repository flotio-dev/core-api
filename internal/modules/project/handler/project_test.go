package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	userService "github.com/flotio-dev/core-api/internal/modules/user/service"
)

// assertEnvelope401 asserts the contract-mandated error envelope (D2/AC-10):
// the body is exactly {status, code, message} — no "error" key — with
// status "Unauthorized" and HTTP 401.
func assertEnvelope401(t *testing.T, w *httptest.ResponseRecorder) {
	t.Helper()
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("Expected status 401, got %d", w.Code)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if _, hasError := response["error"]; hasError {
		t.Errorf("error envelope must not contain an \"error\" key (contract D2), got %v", response["error"])
	}
	if response["status"] != "Unauthorized" {
		t.Errorf("Expected status 'Unauthorized', got %v", response["status"])
	}
	if response["code"] != float64(http.StatusUnauthorized) {
		t.Errorf("Expected code %d, got %v", http.StatusUnauthorized, response["code"])
	}
	if msg, _ := response["message"].(string); msg == "" {
		t.Errorf("Expected a non-empty message, got %v", response["message"])
	}
}

func TestProjectCreateHandler_Unauthorized(t *testing.T) {
	req := httptest.NewRequest("POST", "/projects", bytes.NewReader([]byte("{}")))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	projectCtrl := NewProjectController(&userService.UserService{})
	projectCtrl.ProjectCreateHandler(w, req)

	assertEnvelope401(t, w)
}

func TestProjectGetHandler_Unauthorized(t *testing.T) {
	req := httptest.NewRequest("GET", "/projects/1", nil)

	w := httptest.NewRecorder()
	projectCtrl := NewProjectController(&userService.UserService{})
	projectCtrl.ProjectGetHandler(w, req)

	assertEnvelope401(t, w)
}
