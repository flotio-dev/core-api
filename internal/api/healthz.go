package api

import "net/http"

// HealthzHandler godoc
//
//	@Summary		Health check
//	@Description	Liveness probe — returns "ok" with HTTP 200 when the API process is up
//	@Tags			health
//	@Produce		json
//	@Success		200	{string}	string	"ok"
//	@ID				HealthzHandler
//	@Router			/healthz [get]
func HealthzHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}
