package catalog

import (
	"encoding/json"

	"unitized-curtainwall-silicone-hoist-gate/internal/domain"
)

// Directory is the concrete rules-directory implementation: it validates the
// complete lock request as a whole and returns the immutable snapshot plus its
// canonical payload hash, or a stable error with no partial state.
type Directory struct{}

// NewDirectory returns a rules directory.
func NewDirectory() *Directory { return &Directory{} }

// Lock validates the full lock request. It rejects stale compatibility
// summaries, missing design versions, missing material batches, empty joint
// sets, invalid geometry or segment coverage, missing trial mapping, and
// duplicate joint ids. On success it returns the design lock for the next task
// generation together with a canonical payload hash used for idempotency and
// tamper evidence.
func (d *Directory) Lock(req LockRequest) (domain.DesignLock, string, *domain.Error) {
	if req.TaskID == "" || req.Building == "" || req.FacadeZone == "" || req.Panel == "" {
		return domain.DesignLock{}, "", domain.NewError(domain.CodeInvalidArgument, false,
			domain.Reason{Message: "task, building, facade zone and panel are required"})
	}
	if req.DesignVersion == "" {
		return domain.DesignLock{}, "", domain.NewError(domain.CodeInvalidArgument, false,
			domain.Reason{Message: "design version required"})
	}
	if req.CompatibilityVer == "" {
		return domain.DesignLock{}, "", domain.NewError(domain.CodeStaleCompatibility, false,
			domain.Reason{Message: "compatibility summary version required"})
	}
	if req.CompatValidUntil < req.LockedAt {
		return domain.DesignLock{}, "", domain.NewError(domain.CodeStaleCompatibility, false,
			domain.Reason{Message: "compatibility summary has expired"})
	}
	if req.Batch.BaseBatch == "" || req.Batch.CatalystBatch == "" || req.Batch.PrimerBatch == "" {
		return domain.DesignLock{}, "", domain.NewError(domain.CodeMaterialMismatch, false,
			domain.Reason{Message: "base, catalyst and primer batches are required"})
	}
	if len(req.Joints) == 0 {
		return domain.DesignLock{}, "", domain.NewError(domain.CodeJointCoverageInvalid, false,
			domain.Reason{Message: "at least one joint is required"})
	}
	seen := make(map[string]struct{}, len(req.Joints))
	for _, j := range req.Joints {
		if _, dup := seen[j.JointID]; dup {
			return domain.DesignLock{}, "", domain.NewError(domain.CodeJointCoverageInvalid, false,
				domain.Reason{Joint: j.JointID, Message: "duplicate joint id"})
		}
		seen[j.JointID] = struct{}{}
		if len(j.TrialMapping) == 0 {
			return domain.DesignLock{}, "", domain.NewError(domain.CodeJointCoverageInvalid, false,
				domain.Reason{Joint: j.JointID, Message: "trial mapping required"})
		}
		if err := ValidateJoint(j); err != nil {
			return domain.DesignLock{}, "", err
		}
	}

	lock := domain.DesignLock{
		TaskID:           req.TaskID,
		Generation:       1,
		Building:         req.Building,
		FacadeZone:       req.FacadeZone,
		Panel:            req.Panel,
		DesignVersion:    req.DesignVersion,
		CompatibilityVer: req.CompatibilityVer,
		CompatValidUntil: req.CompatValidUntil,
		SurfaceSummary:   req.SurfaceSummary,
		Thresholds:       req.Thresholds,
		LockedAt:         req.LockedAt,
	}
	payload, err := json.Marshal(req)
	if err != nil {
		return domain.DesignLock{}, "", domain.NewError(domain.CodeInternal, false,
			domain.Reason{Message: "encode lock payload"})
	}
	return lock, domain.CanonicalHash(string(payload)), nil
}
