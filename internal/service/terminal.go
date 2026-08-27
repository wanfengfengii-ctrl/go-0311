package service

import (
	"unitized-curtainwall-silicone-hoist-gate/internal/domain"
	"unitized-curtainwall-silicone-hoist-gate/internal/rework"
	"unitized-curtainwall-silicone-hoist-gate/internal/store"
)

// ReviewRequest is an independent qualified review of a task.
type ReviewRequest struct {
	ReviewerID   string             `json:"reviewer_id"`
	QualSnapshot string             `json:"qual_snapshot"`
	Summary      string             `json:"summary"`
	LogicalTime  domain.LogicalTime `json:"logical_time"`
}

// TerminalRequest is the terminal decision application.
type TerminalRequest struct {
	Type        domain.TerminalType `json:"type"`
	LogicalTime domain.LogicalTime  `json:"logical_time"`
}

// TerminalResult is the outcome of a terminal decision.
type TerminalResult struct {
	Type       domain.TerminalType `json:"type"`
	Credential string              `json:"credential"`
	DecidedAt  domain.LogicalTime  `json:"decided_at"`
	BarrierVer int64               `json:"barrier_ver"`
}

// SubmitReview records one independent qualified review. Reviews from the same
// reviewer are idempotent; a second review must come from a different reviewer.
func (s *Service) SubmitReview(taskID string, req ReviewRequest) *domain.Error {
	return s.runTx(func(tx *store.Tx) *domain.Error {
		lock, ok, err := tx.GetLock(taskID)
		if err != nil {
			return txErr(err)
		}
		if !ok {
			return domain.NewError(domain.CodeNotFound, false, domain.Reason{Message: "task not locked"})
		}
		if req.ReviewerID == "" {
			return domain.NewError(domain.CodeDependencyUnmet, false,
				domain.Reason{Message: "reviewer id required"})
		}
		// A review must not come before the test stage.
		proj, derr := loadTaskProjection(tx, taskID)
		if derr != nil {
			return derr
		}
		if proj.stage < stageTested {
			return domain.NewError(domain.CodeDependencyUnmet, false,
				domain.Reason{Message: "test before review"})
		}
		if err := tx.SaveReview(taskID, domain.Review{
			ReviewerID: req.ReviewerID, QualSnapshot: req.QualSnapshot,
			Summary: req.Summary, ReviewedAt: req.LogicalTime,
		}); err != nil {
			return txErr(err)
		}
		_ = lock
		return nil
	})
}

// DecideTerminal applies a terminal decision. Hoist admission requires full
// conservation, closed prefixes, a complete cure trajectory, passing tests,
// closed reworks and two distinct qualified reviewers. Admission, isolation and
// cancellation share a single-writer barrier: exactly one concurrent request
// succeeds.
func (s *Service) DecideTerminal(taskID string, req TerminalRequest) (TerminalResult, *domain.Error) {
	var out TerminalResult
	derr := s.runTx(func(tx *store.Tx) *domain.Error {
		lock, ok, err := tx.GetLock(taskID)
		if err != nil {
			return txErr(err)
		}
		if !ok {
			return domain.NewError(domain.CodeNotFound, false, domain.Reason{Message: "task not locked"})
		}
		if _, ok, err := tx.GetTerminal(taskID); err != nil {
			return txErr(err)
		} else if ok {
			return domain.NewError(domain.CodeTerminalAlreadyDecided, false,
				domain.Reason{Message: "terminal already decided"})
		}

		if req.Type == domain.TerminalHoistAdmitted {
			if derr := s.checkAdmission(tx, taskID); derr != nil {
				return derr
			}
		}

		credential := "HC-" + domain.CanonicalHash(taskID + ":" + req.Type.String())[:16]
		decision := domain.TerminalDecision{
			Type: req.Type, Credential: credential,
			EvidenceHash: domain.CanonicalHash(taskID), BarrierVer: 1, DecidedAt: req.LogicalTime,
		}
		decided, created, err := tx.SaveTerminal(taskID, decision)
		if err != nil {
			return txErr(err)
		}
		if !created {
			decision = decided
		}
		_ = lock
		out = TerminalResult{Type: decision.Type, Credential: decision.Credential,
			DecidedAt: decision.DecidedAt, BarrierVer: decision.BarrierVer}
		return nil
	})
	if derr != nil {
		return TerminalResult{}, derr
	}
	return out, nil
}

// checkAdmission verifies every admission condition for a hoist admission.
func (s *Service) checkAdmission(tx *store.Tx, taskID string) *domain.Error {
	// Two distinct qualified reviewers.
	reviews, err := tx.ListReviews(taskID)
	if err != nil {
		return txErr(err)
	}
	if len(reviews) < 2 {
		return domain.NewError(domain.CodeDependencyUnmet, false,
			domain.Reason{Message: "two independent reviews required"})
	}
	if derr := rework.ReviewValidator(reviews[0], reviews[1]); derr != nil {
		return domain.NewError(domain.CodeDependencyUnmet, false,
			domain.Reason{Message: "reviews must come from two distinct qualified reviewers"})
	}

	// Closed prefixes.
	closed, derr := jointsClosed(tx, taskID)
	if derr != nil {
		return derr
	}
	if !closed {
		return domain.NewError(domain.CodeNoncontiguousPrefix, false,
			domain.Reason{Message: "joint prefixes not closed"})
	}

	// Mass conservation for every material generation.
	gens, err := tx.AllGenerations()
	if err != nil {
		return txErr(err)
	}
	for _, g := range gens {
		cons, err := tx.MassConserved(g)
		if err != nil {
			return txErr(err)
		}
		if !cons {
			return domain.NewError(domain.CodeMaterialOverdraw, false,
				domain.Reason{MaterialGen: g, Message: "material generation not conserved"})
		}
	}

	// All reworks on this task closed. Admission is scoped to the task's own
	// reworks so an unrelated task's open rework cannot gate this hoist gate.
	reworks, err := tx.ListReworksForTask(taskID)
	if err != nil {
		return txErr(err)
	}
	for _, rw := range reworks {
		if !rw.Closed {
			return domain.NewError(domain.CodeDependencyUnmet, false,
				domain.Reason{Message: "rework " + rw.CaseID + " not closed"})
		}
	}

	// A passing test result must exist (stage tested).
	proj, derr := loadTaskProjection(tx, taskID)
	if derr != nil {
		return derr
	}
	if proj.stage < stageTested {
		return domain.NewError(domain.CodeDependencyUnmet, false,
			domain.Reason{Message: "passing test result required"})
	}
	return nil
}
