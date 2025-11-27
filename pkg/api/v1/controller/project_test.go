package controller

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProjectCreateHandler_Unauthorized(t *testing.T) {
	req := httptest.NewRequest("POST", "/projects", bytes.NewReader([]byte("{}")))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	ProjectCreateHandler(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401, got %d", w.Code)
	}

	var response map[string]string
	json.Unmarshal(w.Body.Bytes(), &response)

	if response["error"] != "Unauthorized" {
		t.Errorf("Expected error message 'Unauthorized', got %s", response["error"])
	}
}

func TestProjectGetHandler_Unauthorized(t *testing.T) {
	req := httptest.NewRequest("GET", "/projects/1", nil)

	w := httptest.NewRecorder()
	ProjectGetHandler(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401, got %d", w.Code)
	}

	var response map[string]string
	json.Unmarshal(w.Body.Bytes(), &response)

	if response["error"] != "Unauthorized" {
		t.Errorf("Expected error message 'Unauthorized', got %s", response["error"])
	}
}
