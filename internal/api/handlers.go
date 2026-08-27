package api

import (
	"net/http"

	"unitized-curtainwall-silicone-hoist-gate/internal/catalog"
	"unitized-curtainwall-silicone-hoist-gate/internal/domain"
	"unitized-curtainwall-silicone-hoist-gate/internal/service"
)

func (s *Server) handleLock(w http.ResponseWriter, r *http.Request) {
	var req catalog.LockRequest
	if !decodeBody(w, r, &req) {
		return
	}
	result, err := s.svc.Lock(req)
	respond(w, err, result)
}

func (s *Server) handleCommand(w http.ResponseWriter, r *http.Request) {
	task := r.PathValue("task")
	var cmd service.Command
	if !decodeBody(w, r, &cmd) {
		return
	}
	result, err := s.svc.SubmitCommand(task, cmd)
	respond(w, err, result)
}

func (s *Server) handleLeaseAcquire(w http.ResponseWriter, r *http.Request) {
	var req service.LeaseRequest
	if !decodeBody(w, r, &req) {
		return
	}
	respond(w, s.svc.AcquireLease(req), map[string]string{"status": "acquired"})
}

func (s *Server) handleLeaseRelease(w http.ResponseWriter, r *http.Request) {
	var req service.LeaseReleaseRequest
	if !decodeBody(w, r, &req) {
		return
	}
	respond(w, s.svc.ReleaseLease(req), map[string]string{"status": "released"})
}

func (s *Server) handleDeviceResult(w http.ResponseWriter, r *http.Request) {
	call := r.PathValue("call")
	var req struct {
		State      string             `json:"state"`
		RawSummary string             `json:"raw_summary"`
		At         domain.LogicalTime `json:"at"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	result, err := s.svc.DeviceResult(call, req.State, req.RawSummary, req.At)
	respond(w, err, result)
}

func (s *Server) handleDeviceRetry(w http.ResponseWriter, r *http.Request) {
	call := r.PathValue("call")
	result, err := s.svc.DeviceRetry(call)
	respond(w, err, result)
}

func (s *Server) handleRework(w http.ResponseWriter, r *http.Request) {
	task := r.PathValue("task")
	var req service.ReworkRequest
	if !decodeBody(w, r, &req) {
		return
	}
	result, err := s.svc.CreateRework(task, req)
	respond(w, err, result)
}

func (s *Server) handleReworkCutout(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req service.CutoutRequest
	if !decodeBody(w, r, &req) {
		return
	}
	respond(w, s.svc.ReworkCutout(id, req), map[string]string{"status": "cutout recorded"})
}

func (s *Server) handleReworkReinject(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req service.ReinjectRequest
	if !decodeBody(w, r, &req) {
		return
	}
	respond(w, s.svc.ReworkReinject(id, req), map[string]string{"status": "reinjected"})
}

func (s *Server) handleReview(w http.ResponseWriter, r *http.Request) {
	task := r.PathValue("task")
	var req service.ReviewRequest
	if !decodeBody(w, r, &req) {
		return
	}
	respond(w, s.svc.SubmitReview(task, req), map[string]string{"status": "reviewed"})
}

func (s *Server) handleTerminal(w http.ResponseWriter, r *http.Request) {
	task := r.PathValue("task")
	var req struct {
		Type        string             `json:"type"`
		LogicalTime domain.LogicalTime `json:"logical_time"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	tt, ok := terminalTypeFromString(req.Type)
	if !ok {
		writeError(w, http.StatusBadRequest, domain.NewError(domain.CodeInvalidArgument, false,
			domain.Reason{Message: "unknown terminal type"}))
		return
	}
	result, err := s.svc.DecideTerminal(task, service.TerminalRequest{Type: tt, LogicalTime: req.LogicalTime})
	respond(w, err, result)
}

func terminalTypeFromString(s string) (domain.TerminalType, bool) {
	switch s {
	case "HOIST_ADMITTED":
		return domain.TerminalHoistAdmitted, true
	case "BOND_RISK_ISOLATED":
		return domain.TerminalBondRiskIsolated, true
	case "CANCELLED":
		return domain.TerminalCancelled, true
	default:
		return 0, false
	}
}
