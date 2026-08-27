package service

import (
	"unitized-curtainwall-silicone-hoist-gate/internal/domain"
	"unitized-curtainwall-silicone-hoist-gate/internal/store"
)

// GetTask returns the current projection of a task: its stage, per-joint valid
// prefixes, mass conservation, reviews and terminal decision.
func (s *Service) GetTask(taskID string) (TaskView, *domain.Error) {
	var view TaskView
	derr := s.runTx(func(tx *store.Tx) *domain.Error {
		lock, ok, err := tx.GetLock(taskID)
		if err != nil {
			return txErr(err)
		}
		if !ok {
			return domain.NewError(domain.CodeNotFound, false, domain.Reason{Message: "task not found"})
		}
		proj, derr := loadTaskProjection(tx, taskID)
		if derr != nil {
			return derr
		}
		view = TaskView{
			TaskID: lock.TaskID, Generation: lock.Generation, Building: lock.Building,
			FacadeZone: lock.FacadeZone, Panel: lock.Panel, Stage: proj.stage.String(),
		}
		joints, err := tx.ListJoints(taskID)
		if err != nil {
			return txErr(err)
		}
		for _, j := range joints {
			prefix, maxGen, derr := jointMaxPrefix(tx, j.JointID)
			if derr != nil {
				return derr
			}
			view.Joints = append(view.Joints, JointView{
				JointID: j.JointID, Stage: jointStageName(prefix, j.End),
				ValidPrefixEnd: prefix, DesignEnd: j.End, Generation: maxGen, Segments: len(j.Segments),
			})
		}
		cons, err := tx.MassConserved(lock.Generation)
		if err != nil {
			return txErr(err)
		}
		view.MassConserved = cons
		reviews, err := tx.ListReviews(taskID)
		if err != nil {
			return txErr(err)
		}
		view.Reviews = reviews
		if term, ok, err := tx.GetTerminal(taskID); err != nil {
			return txErr(err)
		} else if ok {
			view.Terminal = &term
		}
		return nil
	})
	if derr != nil {
		return TaskView{}, derr
	}
	return view, nil
}

// GetEvidence returns the complete ordered event chain for a task aggregate.
func (s *Service) GetEvidence(taskID string) ([]domain.EvidenceEvent, *domain.Error) {
	var events []domain.EvidenceEvent
	derr := s.runTx(func(tx *store.Tx) *domain.Error {
		evs, err := tx.ReplayEvents(taskAggregate(taskID))
		if err != nil {
			return txErr(err)
		}
		events = evs
		return nil
	})
	if derr != nil {
		return nil, derr
	}
	return events, nil
}

// GetAllEvidence returns every committed event across all aggregates, ordered
// by sequence number.
func (s *Service) GetAllEvidence() ([]domain.EvidenceEvent, *domain.Error) {
	var events []domain.EvidenceEvent
	derr := s.runTx(func(tx *store.Tx) *domain.Error {
		evs, err := tx.AllEvents()
		if err != nil {
			return txErr(err)
		}
		events = evs
		return nil
	})
	if derr != nil {
		return nil, derr
	}
	return events, nil
}

// GetMassBalance returns the full ordered mass ledger across all generations.
func (s *Service) GetMassBalance() ([]domain.MassEntry, *domain.Error) {
	var out []domain.MassEntry
	derr := s.runTx(func(tx *store.Tx) *domain.Error {
		gens, err := tx.AllGenerations()
		if err != nil {
			return txErr(err)
		}
		for _, g := range gens {
			entries, err := tx.MassEntries(g)
			if err != nil {
				return txErr(err)
			}
			out = append(out, entries...)
		}
		return nil
	})
	if derr != nil {
		return nil, derr
	}
	return out, nil
}

// GetReworks returns every rework case ordered by case id.
func (s *Service) GetReworks() ([]domain.ReworkCase, *domain.Error) {
	var out []domain.ReworkCase
	derr := s.runTx(func(tx *store.Tx) *domain.Error {
		rws, err := tx.ListReworks()
		if err != nil {
			return txErr(err)
		}
		out = rws
		return nil
	})
	if derr != nil {
		return nil, derr
	}
	return out, nil
}

// GetTerminal returns the current terminal decision for a task, if any.
func (s *Service) GetTerminal(taskID string) (domain.TerminalDecision, bool, *domain.Error) {
	var out domain.TerminalDecision
	var found bool
	derr := s.runTx(func(tx *store.Tx) *domain.Error {
		term, ok, err := tx.GetTerminal(taskID)
		if err != nil {
			return txErr(err)
		}
		out, found = term, ok
		return nil
	})
	if derr != nil {
		return domain.TerminalDecision{}, false, derr
	}
	return out, found, nil
}

// jointStageName derives a per-joint stage string from its prefix coverage.
func jointStageName(prefix, end domain.Microns) string {
	if prefix <= 0 {
		return "LOCKED"
	}
	if prefix >= end {
		return "CLOSED"
	}
	return "INJECTED"
}
