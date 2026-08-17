package server

import (
	"encoding/json"
	"log"
	"net/http"

	models "github.com/flotio-dev/core-api/internal/models"
)

func WriteJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("Error encoding JSON: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

func ReadJSON(r *http.Request, v interface{}) error {
	return json.NewDecoder(r.Body).Decode(v)
}

// WriteErrorJSON serializes the standardized error envelope
// models.APIErrorResponse {status, code, message} (contract D2 / §4.3).
func WriteErrorJSON(w http.ResponseWriter, message string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	resp := models.APIErrorResponse{
		Status:  string(statusTypeForHTTP(status)),
		Code:    status,
		Message: message,
	}
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Printf("Error encoding error JSON: %v", err)
		// Don't call http.Error here since we've already written the status code
	}
}
