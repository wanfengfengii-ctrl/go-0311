package service

import (
	"encoding/json"
	"strconv"

	"unitized-curtainwall-silicone-hoist-gate/internal/domain"
	"unitized-curtainwall-silicone-hoist-gate/internal/rework"
	"unitized-curtainwall-silicone-hoist-gate/internal/store"
)

// ReworkRequest names an anomaly and its root joint to seed impact propagation.
type ReworkRequest struct {
	Category    string             `json:"category"`
	RootJoint   string             `json:"root_joint"`
	LogicalTime domain.LogicalTime `json:"logical_time"`
}

// ReworkResult is the deterministic outcome of creating a rework case.
type ReworkResult struct {
	CaseID        string            `json:"case_id"`
	NewGeneration domain.Generation `json:"new_generation"`
	Affected      []string          `json:"affected"`
	ImpactSummary string            `json:"impact_summary"`
}

// CutoutRequest records the mass and destination of the removed old adhesive.
type CutoutRequest struct {
	CutoutMass  domain.Milligrams  `json:"cutout_mass"`
	Destination string             `json:"destination"`
	LogicalTime domain.LogicalTime `json:"logical_time"`
}

// ReinjectRequest opens a new material generation for re-injection.
type ReinjectRequest struct {
	BaseMg      domain.Milligrams  `json:"base_mg"`
	CatalystMg  domain.Milligrams  `json:"catalyst_mg"`
	PrimerMg    domain.Milligrams  `json:"primer_mg"`
	LogicalTime domain.LogicalTime `json:"logical_time"`
}

// CreateRework computes the unique, stably-sorted rework impact set from the
// immutable relations snapshot and persists the rework case. A repeated
// anomaly for the same root evidence returns the original case unchanged.
func (s *Service) CreateRework(taskID string, req ReworkRequest) (ReworkResult, *domain.Error) {
	var out ReworkResult
	derr := s.runTx(func(tx *store.Tx) *domain.Error {
		lock, ok, err := tx.GetLock(taskID)
		if err != nil {
			return txErr(err)
		}
		if !ok {
			return domain.NewError(domain.CodeNotFound, false, domain.Reason{Message: "task not locked"})
		}
		proj, derr := loadTaskProjection(tx, taskID)
		if derr != nil {
			return derr
		}
		rootGen := proj.generation
		if rootGen == 0 {
			rootGen = lock.Generation
		}

		// Existing rework for the same root evidence returns the original case.
		if existingID, found, err := tx.HasReworkFor(req.RootJoint + ":" + req.Category); err != nil {
			return txErr(err)
		} else if found {
			rw, _, err := tx.GetRework(existingID)
			if err != nil {
				return txErr(err)
			}
			out = ReworkResult{CaseID: rw.CaseID, NewGeneration: rw.NewGeneration,
				Affected: rw.Affected, ImpactSummary: rw.ImpactSummary}
			return nil
		}

		rels, derr := s.relations(tx, taskID, lock)
		if derr != nil {
			return derr
		}
		impact := rework.ComputeImpactSet(rework.RootEvidence{
			Building: lock.Building, FacadeZone: lock.FacadeZone, Panel: lock.Panel,
			Joint: req.RootJoint, MaterialGen: rootGen,
		}, rels)

		affected := make([]string, 0, len(impact))
		seen := make(map[string]bool)
		for _, k := range impact {
			if !seen[k.Joint] {
				seen[k.Joint] = true
				affected = append(affected, k.Joint)
			}
		}
		if len(affected) == 0 {
			affected = []string{req.RootJoint}
		}
		newGen := rootGen + 1
		caseID := "rework:" + taskID + ":" + strconv.FormatInt(int64(newGen), 10)
		summary := req.Category + "@" + req.RootJoint

		rw := domain.ReworkCase{
			CaseID: caseID, TaskID: taskID, Category: req.Category, RootEvidence: req.RootJoint + ":" + req.Category,
			ImpactSummary: summary, Affected: affected, NewGeneration: newGen,
		}
		if err := tx.SaveRework(rw); err != nil {
			return txErr(err)
		}
		out = ReworkResult{CaseID: caseID, NewGeneration: newGen, Affected: affected, ImpactSummary: summary}
		return nil
	})
	if derr != nil {
		return ReworkResult{}, derr
	}
	return out, nil
}

// ReworkCutout records the mass and destination of the removed old adhesive.
func (s *Service) ReworkCutout(reworkID string, req CutoutRequest) *domain.Error {
	return s.runTx(func(tx *store.Tx) *domain.Error {
		rw, found, err := tx.GetRework(reworkID)
		if err != nil {
			return txErr(err)
		}
		if !found {
			return domain.NewError(domain.CodeNotFound, false, domain.Reason{Message: "rework not found"})
		}
		if rw.Closed {
			return domain.NewError(domain.CodeReworkGenerationConflict, false,
				domain.Reason{Message: "rework already closed"})
		}
		rw.CutoutMass = req.CutoutMass
		rw.CutoutDest = req.Destination
		if err := tx.SaveRework(rw); err != nil {
			return txErr(err)
		}
		return nil
	})
}

// ReworkReinject opens a new material generation for the reworked area, marks
// the rework closed and advances the task's current generation. Old evidence
// remains immutable; the new generation re-covers the affected segments.
func (s *Service) ReworkReinject(reworkID string, req ReinjectRequest) *domain.Error {
	return s.runTx(func(tx *store.Tx) *domain.Error {
		rw, found, err := tx.GetRework(reworkID)
		if err != nil {
			return txErr(err)
		}
		if !found {
			return domain.NewError(domain.CodeNotFound, false, domain.Reason{Message: "rework not found"})
		}
		// A closed rework must not be re-injected. A timed-out client that
		// resends the same reinject request would otherwise post a duplicate
		// mass entry and a duplicate reinject event, double-counting the
		// generation's base, catalyst and primer inputs.
		if rw.Closed {
			return domain.NewError(domain.CodeReworkGenerationConflict, false,
				domain.Reason{Message: "rework already closed"})
		}
		if rw.CutoutDest == "" {
			return domain.NewError(domain.CodeDependencyUnmet, false,
				domain.Reason{Message: "cutout must be recorded before reinject"})
		}
		gen := rw.NewGeneration
		inputs := []domain.MassEntry{
			{Generation: gen, Component: domain.ComponentBase, Direction: domain.MassInput, Category: domain.MassStock, Amount: req.BaseMg, Evidence: reworkID},
			{Generation: gen, Component: domain.ComponentCatalyst, Direction: domain.MassInput, Category: domain.MassStock, Amount: req.CatalystMg, Evidence: reworkID},
			{Generation: gen, Component: domain.ComponentPrimer, Direction: domain.MassInput, Category: domain.MassStock, Amount: req.PrimerMg, Evidence: reworkID},
		}
		for _, e := range inputs {
			if err := tx.PostMass(e); err != nil {
				return txErr(err)
			}
		}
		payload, _ := json.Marshal(map[string]interface{}{"rework": reworkID, "generation": gen})
		_, _, err = tx.AppendEvent(domain.EvidenceEvent{
			AggregateID: taskAggregate(rw.TaskID), Generation: gen, Type: domain.EventReworkReinject,
			Payload: string(payload), LogicalTime: req.LogicalTime,
		})
		if err != nil {
			return txErr(err)
		}
		rw.Closed = true
		rw.ReinjectGen = gen
		if err := tx.SaveRework(rw); err != nil {
			return txErr(err)
		}
		return nil
	})
}

// relations builds the immutable relations snapshot from the lock and joints.
func (s *Service) relations(tx *store.Tx, taskID string, lock domain.DesignLock) (rework.Relations, *domain.Error) {
	joints, err := tx.ListJoints(taskID)
	if err != nil {
		return rework.Relations{}, txErr(err)
	}
	adjacency, err := tx.GetAdjacency(taskID)
	if err != nil {
		return rework.Relations{}, txErr(err)
	}
	rel := rework.Relations{
		JointAdjacency:  make(map[string][]string),
		JointGeneration: make(map[string]domain.Generation),
		PanelAdjacency:  map[string][]string{lock.Panel: adjacency},
	}
	ids := make([]string, 0, len(joints))
	for _, j := range joints {
		ids = append(ids, j.JointID)
		rel.JointGeneration[j.JointID] = lock.Generation
	}
	for _, id := range ids {
		for _, other := range ids {
			if id != other {
				rel.JointAdjacency[id] = append(rel.JointAdjacency[id], other)
			}
		}
	}
	return rel, nil
}
