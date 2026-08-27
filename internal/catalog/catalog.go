package catalog

import (
	"unitized-curtainwall-silicone-hoist-gate/internal/domain"
)

// LockRequest is the full, once-validated set required to lock a task.
type LockRequest struct {
	TaskID           string               `json:"task_id"`
	Building         string               `json:"building"`
	FacadeZone       string               `json:"facade_zone"`
	Panel            string               `json:"panel"`
	DesignVersion    string               `json:"design_version"`
	CompatibilityVer string               `json:"compatibility_ver"`
	CompatValidUntil domain.LogicalTime   `json:"compat_valid_until"`
	SurfaceSummary   string               `json:"surface_summary"`
	Batch            domain.MaterialBatch `json:"batch"`
	Joints           []domain.JointSpec   `json:"joints"`
	Thresholds       domain.Thresholds    `json:"thresholds"`
	Adjacency        []string             `json:"adjacency"` // load-bearing neighbour panel ids
	LockedAt         domain.LogicalTime   `json:"locked_at"`
}

// Catalog is the rules directory: it validates and locks the immutable task
// snapshot. Implementations must reject any stale or inconsistent input as a
// whole, leaving no partial records.
type Catalog interface {
	// Lock validates the full lock request and returns the generated design
	// lock plus its canonical payload hash. On any inconsistency it returns a
	// stable *domain.Error and must not persist partial state.
	Lock(req LockRequest) (domain.DesignLock, string, *domain.Error)
}

// ValidateJoint validates a single joint's geometry and segment coverage,
// returning a stable error on failure.
func ValidateJoint(joint domain.JointSpec) *domain.Error {
	if err := ValidateGeometry(joint); err != nil {
		return err
	}
	if _, err := ValidateSegments(joint); err != nil {
		return err
	}
	return nil
}
