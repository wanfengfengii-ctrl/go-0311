package service

import (
	"testing"

	"unitized-curtainwall-silicone-hoist-gate/internal/catalog"
	"unitized-curtainwall-silicone-hoist-gate/internal/domain"
	"unitized-curtainwall-silicone-hoist-gate/internal/store"
)

// newTestService returns a service backed by a fresh in-memory SQLite store.
func newTestService(t *testing.T) *Service {
	t.Helper()
	st, err := store.Open("")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return New(st)
}

// sampleLockRequest builds a minimal lock request with one joint tiled by three
// contiguous segments, ready for the workflow tests.
func sampleLockRequest() catalog.LockRequest {
	return catalog.LockRequest{
		TaskID:           "T-1",
		Building:         "A",
		FacadeZone:       "E",
		Panel:            "P-017",
		DesignVersion:    "dv-1",
		CompatibilityVer: "cv-1",
		CompatValidUntil: 100000,
		SurfaceSummary:   "glass+aluminium cleaned",
		Batch: domain.MaterialBatch{
			BaseBatch: "B-1", CatalystBatch: "C-1", PrimerBatch: "P-1",
		},
		Joints: []domain.JointSpec{{
			JointID:     "J-1",
			Direction:   "E",
			Start:       0,
			End:         3000,
			Width:       20,
			Depth:       10,
			BondAreaUm2: 200,
			Segments: []domain.SegmentSpec{
				{Seq: 1, Start: 0, End: 1000},
				{Seq: 2, Start: 1000, End: 2000},
				{Seq: 3, Start: 2000, End: 3000},
			},
			TrialMapping: map[string]string{"seg1": "sample-1"},
		}},
		Thresholds: domain.Thresholds{MinTensileMPa: 50, MaxBondFailurePct: 10},
		Adjacency:  []string{"P-018"},
		LockedAt:   100,
	}
}

// lockSample locks the sample task and returns the lock result.
func lockSample(t *testing.T, svc *Service) LockResult {
	t.Helper()
	res, err := svc.Lock(sampleLockRequest())
	if err != nil {
		t.Fatalf("lock: %v", err)
	}
	return res
}

// submit is a convenience wrapper that fails the test on a command error.
func submit(t *testing.T, svc *Service, task string, cmd Command) CommandResult {
	t.Helper()
	res, err := svc.SubmitCommand(task, cmd)
	if err != nil {
		t.Fatalf("command %s: %v", cmd.Kind, err)
	}
	return res
}
