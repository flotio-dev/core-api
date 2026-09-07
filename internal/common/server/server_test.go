package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	models "github.com/flotio-dev/core-api/internal/models"
)

func TestWriteJSON(t *testing.T) {
	w := httptest.NewRecorder()
	payload := map[string]string{"foo": "bar"}
	WriteJSON(w, payload)

	if w.Header().Get("Content-Type") != "application/json" {
		t.Errorf("expected application/json, got %s", w.Header().Get("Content-Type"))
	}
	var res map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if res["foo"] != "bar" {
		t.Errorf("expected bar, got %s", res["foo"])
	}
}

func TestWriteJSON_Error(t *testing.T) {
	w := httptest.NewRecorder()
	// channels cannot be serialized to JSON
	invalidPayload := map[string]chan int{"unsupported": make(chan int)}
	WriteJSON(w, invalidPayload)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 on json encode failure, got %d", w.Code)
	}
}

func TestReadJSON(t *testing.T) {
	body := strings.NewReader(`{"name":"test"}`)
	req := httptest.NewRequest("POST", "/test", body)

	var target struct {
		Name string `json:"name"`
	}
	err := ReadJSON(req, &target)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if target.Name != "test" {
		t.Errorf("expected 'test', got %s", target.Name)
	}
}

func TestWriteErrorJSON(t *testing.T) {
	testCases := []struct {
		status     int
		wantStatus StatusType
	}{
		{http.StatusUnauthorized, StatusUnauthorized},
		{http.StatusNotFound, StatusNotFound},
		{http.StatusBadGateway, StatusBadGateway},
		{http.StatusInternalServerError, StatusInternalError},
		{http.StatusMethodNotAllowed, StatusMethodNotAllowed},
		{http.StatusBadRequest, StatusBadRequest},
		{http.StatusUnprocessableEntity, StatusInvalidArgs},
	}

	for _, tc := range testCases {
		w := httptest.NewRecorder()
		WriteErrorJSON(w, "something went wrong", tc.status)

		if w.Code != tc.status {
			t.Errorf("expected HTTP %d, got %d", tc.status, w.Code)
		}
		if w.Header().Get("Content-Type") != "application/json" {
			t.Errorf("expected application/json header")
		}

		var resp models.APIErrorResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to decode error response: %v", err)
		}
		if resp.Code != tc.status {
			t.Errorf("expected code %d, got %d", tc.status, resp.Code)
		}
		if resp.Status != string(tc.wantStatus) {
			t.Errorf("expected status %s, got %s", tc.wantStatus, resp.Status)
		}
		if resp.Message != "something went wrong" {
			t.Errorf("expected message 'something went wrong', got %s", resp.Message)
		}
	}
}

func TestRespondWithSuccess(t *testing.T) {
	// Default options and nil data
	w := httptest.NewRecorder()
	RespondWithSuccess[string](w, nil, nil)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	var res models.APIResponse[string]
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if res.Status != string(StatusOK) || res.Code != 200 {
		t.Errorf("unexpected envelope: %+v", res)
	}

	// Custom options and data
	w2 := httptest.NewRecorder()
	val := "hello-world"
	RespondWithSuccess(w2, &val, &ResponseOptions{
		HTTPCode: http.StatusCreated,
		Status:   StatusCreated,
		Message:  "Resource created",
	})
	if w2.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", w2.Code)
	}
	var res2 models.APIResponse[string]
	if err := json.Unmarshal(w2.Body.Bytes(), &res2); err != nil {
		t.Fatal(err)
	}
	if res2.Details != val || res2.Message != "Resource created" || res2.Status != string(StatusCreated) {
		t.Errorf("unexpected envelope: %+v", res2)
	}
}

func TestRespondWithError(t *testing.T) {
	// Nil options (defaults)
	w := httptest.NewRecorder()
	RespondWithError(w, nil)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
	var res models.APIErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if res.Status != string(StatusInvalidArgs) || res.Code != 400 {
		t.Errorf("unexpected error envelope: %+v", res)
	}

	// Custom options with explicit Status
	w2 := httptest.NewRecorder()
	RespondWithError(w2, &ResponseOptions{
		HTTPCode: http.StatusNotFound,
		Status:   StatusNotFound,
		Message:  "Not found",
	})
	if w2.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w2.Code)
	}
	var res2 models.APIErrorResponse
	_ = json.Unmarshal(w2.Body.Bytes(), &res2)
	if res2.Status != string(StatusNotFound) || res2.Message != "Not found" {
		t.Errorf("unexpected error envelope: %+v", res2)
	}

	// Custom options without Status (inferred from HTTPCode)
	w3 := httptest.NewRecorder()
	RespondWithError(w3, &ResponseOptions{
		HTTPCode: http.StatusUnauthorized,
		Message:  "Token missing",
	})
	if w3.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w3.Code)
	}
	var res3 models.APIErrorResponse
	_ = json.Unmarshal(w3.Body.Bytes(), &res3)
	if res3.Status != string(StatusUnauthorized) {
		t.Errorf("expected inferred status %s, got %s", StatusUnauthorized, res3.Status)
	}
}
