package models

type APIResponse[T any] struct {
	Status  string `json:"status" example:"success"`
	Code    int    `json:"code" example:"200"`
	Message string `json:"message,omitempty"`
	Details T      `json:"details,omitempty"`
}

type APIErrorResponse struct {
	Status  string `json:"status" example:"error"`
	Code    int    `json:"code" example:"400"`
	Message string `json:"message,omitempty" example:"Bad request"`
}
