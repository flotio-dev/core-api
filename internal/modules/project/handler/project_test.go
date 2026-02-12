package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	userService "github.com/flotio-dev/core-api/internal/modules/user/service"
)

func TestProjectCreateHandler_Unauthorized(t *testing.T) {
	req := httptest.NewRequest("POST", "/projects", bytes.NewReader([]byte("{}")))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	projectCtrl := NewProjectController(&userService.UserService{})
	projectCtrl.ProjectCreateHandler(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401, got %d", w.Code)
	}

	var response map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if response["error"] != "Unauthorized" {
		t.Errorf("Expected error message 'Unauthorized', got %s", response["error"])
	}
}

func TestProjectGetHandler_Unauthorized(t *testing.T) {
	req := httptest.NewRequest("GET", "/projects/1", nil)

	w := httptest.NewRecorder()
	projectCtrl := NewProjectController(&userService.UserService{})
	projectCtrl.ProjectGetHandler(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401, got %d", w.Code)
	}

	var response map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if response["error"] != "Unauthorized" {
		t.Errorf("Expected error message 'Unauthorized', got %s", response["error"])
	}
}
