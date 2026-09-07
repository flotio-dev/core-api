package server

import (
	"encoding/json"
	"net/http"

	models "github.com/flotio-dev/core-api/internal/models"
)

type StatusType string

const (
	StatusOK               StatusType = "OK"
	StatusCreated          StatusType = "Created"
	StatusInvalidArgs      StatusType = "InvalidArguments"
	StatusUnauthorized     StatusType = "Unauthorized"
	StatusNotFound         StatusType = "NotFound"
	StatusInternalError    StatusType = "InternalError"
	StatusBadGateway       StatusType = "BadGateway"
	StatusBadRequest       StatusType = "BadRequest"
	StatusMethodNotAllowed StatusType = "MethodNotAllowed"
)

type ResponseOptions struct {
	HTTPCode int
	Status   StatusType
	Message  string
}

// statusTypeForHTTP maps an HTTP status code to the StatusType vocabulary used
// in the API envelopes (contract §4.3).
func statusTypeForHTTP(httpCode int) StatusType {
	switch httpCode {
	case http.StatusUnauthorized:
		return StatusUnauthorized
	case http.StatusNotFound:
		return StatusNotFound
	case http.StatusBadGateway:
		return StatusBadGateway
	case http.StatusInternalServerError:
		return StatusInternalError
	case http.StatusMethodNotAllowed:
		return StatusMethodNotAllowed
	case http.StatusBadRequest:
		return StatusBadRequest
	default:
		return StatusInvalidArgs
	}
}

func RespondWithSuccess[T any](w http.ResponseWriter, data *T, opts *ResponseOptions) {
	httpCode := http.StatusOK
	status := StatusOK
	message := ""

	if opts != nil {
		if opts.HTTPCode != 0 {
			httpCode = opts.HTTPCode
		}
		if opts.Status != "" {
			status = opts.Status
		}
		if opts.Message != "" {
			message = opts.Message
		}
	}

	resp := models.APIResponse[T]{
		Status:  string(status),
		Code:    httpCode,
		Message: message,
	}

	if data != nil {
		resp.Details = *data
	}

	w.WriteHeader(httpCode)
	json.NewEncoder(w).Encode(resp)
}

// httpForStatusType maps a StatusType vocabulary to an HTTP status code.
func httpForStatusType(status StatusType) int {
	switch status {
	case StatusUnauthorized:
		return http.StatusUnauthorized
	case StatusNotFound:
		return http.StatusNotFound
	case StatusBadGateway:
		return http.StatusBadGateway
	case StatusInternalError:
		return http.StatusInternalServerError
	case StatusMethodNotAllowed:
		return http.StatusMethodNotAllowed
	case StatusBadRequest, StatusInvalidArgs:
		return http.StatusBadRequest
	default:
		return http.StatusBadRequest
	}
}

func RespondWithError(w http.ResponseWriter, opts *ResponseOptions) {
	httpCode := http.StatusBadRequest
	status := StatusInvalidArgs
	message := ""

	if opts != nil {
		if opts.HTTPCode != 0 {
			httpCode = opts.HTTPCode
		} else if opts.Status != "" {
			httpCode = httpForStatusType(opts.Status)
		}
		if opts.Status != "" {
			status = opts.Status
		} else {
			status = statusTypeForHTTP(httpCode)
		}
		if opts.Message != "" {
			message = opts.Message
		}
	}

	resp := models.APIErrorResponse{
		Status:  string(status),
		Code:    httpCode,
		Message: message,
	}

	w.WriteHeader(httpCode)
	json.NewEncoder(w).Encode(resp)
}
