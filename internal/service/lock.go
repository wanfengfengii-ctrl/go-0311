package service

import (
	"encoding/json"

	"unitized-curtainwall-silicone-hoist-gate/internal/catalog"
	"unitized-curtainwall-silicone-hoist-gate/internal/domain"
	"unitized-curtainwall-silicone-hoist-gate/internal/store"
)

// Lock validates the complete lock request and persists the immutable snapshot,
// joint specs and the task-locked evidence event atomically. Any inconsistency
// rejects the whole transaction with no partial state.
func (s *Service) Lock(req catalog.LockRequest) (LockResult, *domain.Error) {
	lock, payloadHash, derr := s.dir.Lock(req)
	if derr != nil {
		return LockResult{}, derr
	}
	// A re-lock of the same task advances the generation; evidence for the old
	// generation remains untouched.
	var result LockResult
	err := s.runTx(func(tx *store.Tx) *domain.Error {
		maxGen, err := tx.MaxGeneration(req.TaskID)
		if err != nil {
			return txErr(err)
		}
		lock.Generation = maxGen + 1
		payload, err := json.Marshal(req)
		if err != nil {
			return domain.NewError(domain.CodeInternal, false, domain.Reason{Message: "encode lock"})
		}
		if err := tx.SaveLock(lock); err != nil {
			return txErr(err)
		}
		for _, j := range req.Joints {
			if err := tx.SaveJoint(req.TaskID, j); err != nil {
				return txErr(err)
			}
		}
		if err := tx.SaveAdjacency(req.TaskID, req.Adjacency); err != nil {
			return txErr(err)
		}
		_, _, err = tx.AppendEvent(domain.EvidenceEvent{
			AggregateID: taskAggregate(req.TaskID),
			Generation:  lock.Generation,
			Type:        domain.EventTaskLocked,
			Payload:     string(payload),
			PayloadHash: payloadHash,
			LogicalTime: req.LockedAt,
		})
		if err != nil {
			return txErr(err)
		}
		result = LockResult{
			TaskID:      req.TaskID,
			Generation:  lock.Generation,
			LockedAt:    req.LockedAt,
			PayloadHash: payloadHash,
			JointCount:  len(req.Joints),
		}
		for _, j := range req.Joints {
			result.SegmentCount += len(j.Segments)
		}
		return nil
	})
	if err != nil {
		return LockResult{}, err
	}
	return result, nil
}
