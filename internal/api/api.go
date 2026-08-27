// Package api exposes the Go HTTP API: JSON command and query endpoints, the
// stable error envelope, operation-id idempotency control, transaction
// boundaries, a health check and the restart-recovery entry point. It contains
// no user interface or peripheral systems unrelated to the quality loop.
package api

import (
	"encoding/json"
	"log"
	"net/http"

	"unitized-curtainwall-silicone-hoist-gate/internal/domain"
	"unitized-curtainwall-silicone-hoist-gate/internal/service"
)

// ErrorResponse is the stable error envelope: a machine-readable code, sorted
// reasons and a retryability hint. HTTP status reflects only the protocol
// category.
type ErrorResponse struct {
	Code      domain.ErrorCode `json:"code"`
	Reasons   []string         `json:"reasons"`
	Retryable bool             `json:"retryable"`
}

// Server is the HTTP API server bound to an application service.
type Server struct {
	mux *http.ServeMux
	svc *service.Service
}

// NewServer builds a Server with all routes registered against the service.
func NewServer(svc *service.Service) *Server {
	s := &Server{mux: http.NewServeMux(), svc: svc}
	s.routes()
	return s
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /healthz", s.handleHealth)
	s.mux.HandleFunc("POST /v1/tasks/lock", s.handleLock)
	s.mux.HandleFunc("POST /v1/tasks/{task}/commands", s.handleCommand)
	s.mux.HandleFunc("POST /v1/leases/acquire", s.handleLeaseAcquire)
	s.mux.HandleFunc("POST /v1/leases/release", s.handleLeaseRelease)
	s.mux.HandleFunc("POST /v1/device-calls/{call}/results", s.handleDeviceResult)
	s.mux.HandleFunc("POST /v1/device-calls/{call}/retry", s.handleDeviceRetry)
	s.mux.HandleFunc("POST /v1/tasks/{task}/reworks", s.handleRework)
	s.mux.HandleFunc("POST /v1/reworks/{id}/cutout", s.handleReworkCutout)
	s.mux.HandleFunc("POST /v1/reworks/{id}/reinject", s.handleReworkReinject)
	s.mux.HandleFunc("POST /v1/tasks/{task}/reviews", s.handleReview)
	s.mux.HandleFunc("POST /v1/tasks/{task}/terminal", s.handleTerminal)
	s.mux.HandleFunc("GET /v1/tasks/{task}", s.handleGetTask)
	s.mux.HandleFunc("GET /v1/evidence", s.handleGetEvidence)
	s.mux.HandleFunc("GET /v1/mass-balance", s.handleGetMassBalance)
	s.mux.HandleFunc("GET /v1/reworks", s.handleGetReworks)
	s.mux.HandleFunc("GET /v1/terminal", s.handleGetTerminal)
}

// ServeHTTP implements http.Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// decodeBody decodes a JSON request body, returning a 400 envelope on error.
func decodeBody(w http.ResponseWriter, r *http.Request, dst interface{}) bool {
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(dst); err != nil {
		writeError(w, http.StatusBadRequest, domain.NewError(domain.CodeInvalidArgument, false,
			domain.Reason{Message: "invalid JSON body"}))
		return false
	}
	return true
}

// writeError renders a stable error envelope, mapping the error code to an
// HTTP protocol category.
func writeError(w http.ResponseWriter, status int, err *domain.Error) {
	resp := ErrorResponse{Code: err.Code, Retryable: err.Retryable}
	for _, r := range err.Reasons {
		resp.Reasons = append(resp.Reasons, r.Message)
	}
	writeJSON(w, status, resp)
}

// respond maps a service error to the correct HTTP status and writes the
// envelope, or writes the value with 200 on success.
func respond(w http.ResponseWriter, err *domain.Error, value interface{}) {
	if err != nil {
		writeError(w, httpStatus(err), err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func httpStatus(err *domain.Error) int {
	switch err.Code {
	case domain.CodeNotFound:
		return http.StatusNotFound
	case domain.CodeInvalidArgument, domain.CodeIdempotencyConflict,
		domain.CodeTerminalAlreadyDecided, domain.CodeReworkGenerationConflict:
		return http.StatusConflict
	case domain.CodeStaleCompatibility, domain.CodeJointCoverageInvalid,
		domain.CodeFixedPointOverflow, domain.CodeMaterialMismatch,
		domain.CodeMaterialOverdraw, domain.CodeLeaseConflict,
		domain.CodeMixerContaminated, domain.CodeLeaseExpired,
		domain.CodeDependencyUnmet, domain.CodeOpenTimeExpired,
		domain.CodeNoncontiguousPrefix, domain.CodeGenerationMismatch,
		domain.CodeLogicalTimeBackwards, domain.CodeCureGap,
		domain.CodeCalibrationExpired:
		return http.StatusUnprocessableEntity
	default:
		return http.StatusInternalServerError
	}
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("api: write response: %v", err)
	}
}

func errMissingTask() *domain.Error {
	return domain.NewError(domain.CodeInvalidArgument, false,
		domain.Reason{Message: "task query parameter required"})
}

func errNotFound() *domain.Error {
	return domain.NewError(domain.CodeNotFound, false,
		domain.Reason{Message: "terminal not found"})
}
