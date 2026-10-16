package cloud

import (
	"net/http"
	apicontract "videoflow-go/api"
)

// The route lives under adminAccess: role, enrollment and recent MFA still apply.
func (s *Server) adminAPIContract(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(apicontract.OpenAPI)
}
