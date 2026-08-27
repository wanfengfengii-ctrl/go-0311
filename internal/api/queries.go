package api

import "net/http"

func (s *Server) handleGetTask(w http.ResponseWriter, r *http.Request) {
	task := r.PathValue("task")
	view, err := s.svc.GetTask(task)
	respond(w, err, view)
}

func (s *Server) handleGetEvidence(w http.ResponseWriter, r *http.Request) {
	events, err := s.svc.GetAllEvidence()
	respond(w, err, events)
}

func (s *Server) handleGetMassBalance(w http.ResponseWriter, r *http.Request) {
	entries, err := s.svc.GetMassBalance()
	respond(w, err, entries)
}

func (s *Server) handleGetReworks(w http.ResponseWriter, r *http.Request) {
	reworks, err := s.svc.GetReworks()
	respond(w, err, reworks)
}

func (s *Server) handleGetTerminal(w http.ResponseWriter, r *http.Request) {
	task := r.URL.Query().Get("task")
	if task == "" {
		writeError(w, http.StatusBadRequest, errMissingTask())
		return
	}
	term, found, err := s.svc.GetTerminal(task)
	if err != nil {
		respond(w, err, nil)
		return
	}
	if !found {
		respond(w, errNotFound(), nil)
		return
	}
	respond(w, nil, term)
}
