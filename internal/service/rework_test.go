package service

import (
	"testing"

	"unitized-curtainwall-silicone-hoist-gate/internal/domain"
)

func TestReworkImpactUniqueAndSorted(t *testing.T) {
	svc := newTestService(t)
	runWorkflow(t, svc)

	rw, err := svc.CreateRework("T-1", ReworkRequest{
		Category: "ratio_drift", RootJoint: "J-1", LogicalTime: 200,
	})
	if err != nil {
		t.Fatalf("create rework: %v", err)
	}
	if rw.NewGeneration != 2 {
		t.Fatalf("new generation=%d want 2", rw.NewGeneration)
	}
	if len(rw.Affected) != 1 || rw.Affected[0] != "J-1" {
		t.Fatalf("affected=%v want [J-1]", rw.Affected)
	}

	// A repeated anomaly returns the original case unchanged.
	rw2, err := svc.CreateRework("T-1", ReworkRequest{
		Category: "ratio_drift", RootJoint: "J-1", LogicalTime: 201,
	})
	if err != nil {
		t.Fatalf("repeat rework: %v", err)
	}
	if rw2.CaseID != rw.CaseID {
		t.Fatalf("repeat rework case=%s want %s", rw2.CaseID, rw.CaseID)
	}
}

func TestReworkCutoutAndReinject(t *testing.T) {
	svc := newTestService(t)
	runWorkflow(t, svc)

	rw, err := svc.CreateRework("T-1", ReworkRequest{
		Category: "ratio_drift", RootJoint: "J-1", LogicalTime: 200,
	})
	if err != nil {
		t.Fatalf("create rework: %v", err)
	}

	// Reinject before cutout must fail.
	if err := svc.ReworkReinject(rw.CaseID, ReinjectRequest{
		BaseMg: 500, CatalystMg: 50, PrimerMg: 25, LogicalTime: 202,
	}); err == nil || err.Code != domain.CodeDependencyUnmet {
		t.Fatalf("reinject before cutout err=%v want DEPENDENCY_UNMET", err)
	}

	if err := svc.ReworkCutout(rw.CaseID, CutoutRequest{
		CutoutMass: 300, Destination: "recycle", LogicalTime: 201,
	}); err != nil {
		t.Fatalf("cutout: %v", err)
	}

	if err := svc.ReworkReinject(rw.CaseID, ReinjectRequest{
		BaseMg: 500, CatalystMg: 50, PrimerMg: 25, LogicalTime: 202,
	}); err != nil {
		t.Fatalf("reinject: %v", err)
	}

	reworks, err := svc.GetReworks()
	if err != nil {
		t.Fatalf("get reworks: %v", err)
	}
	if len(reworks) != 1 || !reworks[0].Closed {
		t.Fatalf("rework not closed: %+v", reworks)
	}
	if reworks[0].CutoutDest != "recycle" || reworks[0].CutoutMass != 300 {
		t.Fatalf("cutout not recorded: %+v", reworks[0])
	}
}

func TestReworkOldEvidenceImmutable(t *testing.T) {
	svc := newTestService(t)
	runWorkflow(t, svc)

	before, err := svc.GetEvidence("T-1")
	if err != nil {
		t.Fatalf("evidence before: %v", err)
	}
	nBefore := len(before)

	_, err = svc.CreateRework("T-1", ReworkRequest{
		Category: "ratio_drift", RootJoint: "J-1", LogicalTime: 200,
	})
	if err != nil {
		t.Fatalf("create rework: %v", err)
	}
	if err := svc.ReworkCutout("rework:T-1:2", CutoutRequest{
		CutoutMass: 300, Destination: "recycle", LogicalTime: 201,
	}); err != nil {
		t.Fatalf("cutout: %v", err)
	}
	if err := svc.ReworkReinject("rework:T-1:2", ReinjectRequest{
		BaseMg: 500, CatalystMg: 50, PrimerMg: 25, LogicalTime: 202,
	}); err != nil {
		t.Fatalf("reinject: %v", err)
	}

	// Old evidence is only appended, never mutated.
	after, err := svc.GetEvidence("T-1")
	if err != nil {
		t.Fatalf("evidence after: %v", err)
	}
	if len(after) <= nBefore {
		t.Fatalf("expected new events, got %d before %d after", nBefore, len(after))
	}
	for i := 0; i < nBefore; i++ {
		if after[i].Seq != before[i].Seq || after[i].PayloadHash != before[i].PayloadHash {
			t.Fatalf("old evidence mutated at index %d", i)
		}
	}
}
