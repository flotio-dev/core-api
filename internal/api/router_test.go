package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHealthzHandler(t *testing.T) {
	req := httptest.NewRequest("GET", "/healthz", nil)
	w := httptest.NewRecorder()

	HealthzHandler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if w.Body.String() != "ok" {
		t.Errorf("expected 'ok', got %q", w.Body.String())
	}
}

func TestRouter_PublicAndProtectedRoutes(t *testing.T) {
	r := Router()

	tests := []struct {
		name           string
		method         string
		path           string
		body           string
		expectedStatus int
	}{
		{"healthz", "GET", "/healthz", "", http.StatusOK},
		{"auth_register_invalid_json", "POST", "/auth/register", "not-json", http.StatusBadRequest},
		{"auth_login_invalid_json", "POST", "/auth/login", "not-json", http.StatusBadRequest},
		{"auth_logout_empty", "POST", "/auth/logout", "{}", http.StatusOK},
		{"flutter_versions_unauthorized", "GET", "/flutter/versions", "", http.StatusUnauthorized},
		{"me_get_unauthorized", "GET", "/auth/@me", "", http.StatusUnauthorized},
		{"env_get_unauthorized", "GET", "/env", "", http.StatusUnauthorized},
		{"envs_get_unauthorized", "GET", "/envs", "", http.StatusUnauthorized},
		{"project_get_unauthorized", "GET", "/project", "", http.StatusUnauthorized},
		{"project_id_get_unauthorized", "GET", "/project/1", "", http.StatusUnauthorized},
		{"keystore_get_unauthorized", "GET", "/keystore", "", http.StatusUnauthorized},
		{"google_play_get_unauthorized", "GET", "/google-play-credentials", "", http.StatusUnauthorized},
		{"not_found", "GET", "/non-existent-route-xyz", "", http.StatusNotFound},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var bodyReader *strings.Reader
			if tc.body != "" {
				bodyReader = strings.NewReader(tc.body)
			} else {
				bodyReader = strings.NewReader("")
			}
			req := httptest.NewRequest(tc.method, tc.path, bodyReader)
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			r.ServeHTTP(w, req)

			if w.Code != tc.expectedStatus {
				t.Errorf("%s %s: expected %d, got %d (body: %s)", tc.method, tc.path, tc.expectedStatus, w.Code, w.Body.String())
			}
		})
	}
}
