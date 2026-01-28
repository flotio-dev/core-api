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

func RespondWithError(w http.ResponseWriter, opts *ResponseOptions) {
	httpCode := http.StatusBadRequest
	status := StatusInvalidArgs
	message := ""

	if opts != nil {
		if opts.HTTPCode != 0 {
			httpCode = opts.HTTPCode
		}
		if opts.Status != "" {
			status = opts.Status
		} else {
			switch httpCode {
			case http.StatusUnauthorized:
				status = StatusUnauthorized
			case http.StatusNotFound:
				status = StatusNotFound
			case http.StatusBadGateway:
				status = StatusBadGateway
			case http.StatusInternalServerError:
				status = StatusInternalError
			case http.StatusMethodNotAllowed:
				status = StatusMethodNotAllowed
			default:
				status = StatusInvalidArgs
			}
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
