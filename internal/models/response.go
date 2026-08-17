package models

// APIResponse is the standardized success envelope (contract §4.1). It is a
// generic type used at runtime by the envelope response writers.
type APIResponse[T any] struct {
	Status  string `json:"status"`
	Code    int    `json:"code"`
	Message string `json:"message"`
	Details T      `json:"details,omitempty"`
}

// APIResponseDoc is the documentation-only mirror of APIResponse[T] with an
// untyped details field. swag names generic instantiations with a type-argument
// suffix (e.g. "APIResponse-X"), so this concrete struct guarantees a single
// definition named exactly "APIResponse" (contract §4.1 / AC-16).
type APIResponseDoc struct {
	Status  string      `json:"status"`
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Details interface{} `json:"details,omitempty"`
} // @name APIResponse

// APIErrorResponse is the standardized error envelope (contract §4.1). The
// Details field is documentation-only: it is never populated at runtime, so
// wire bodies contain exactly {status, code, message} (contract D2 / AC-10);
// it exists so the generated definition carries the "details" property the
// contract suite (AC-16) expects on both envelopes.
type APIErrorResponse struct {
	Status  string      `json:"status"`
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Details interface{} `json:"details,omitempty"`
} // @name APIErrorResponse
