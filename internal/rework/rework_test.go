package rework

import (
	"testing"

	"unitized-curtainwall-silicone-hoist-gate/internal/domain"
)

func TestUniqueImpactSortAndDedup(t *testing.T) {
	keys := []ImpactKey{
		{Building: "A", Panel: "P-2", Joint: "J-2", Segment: 1, MaterialGen: 1},
		{Building: "A", Panel: "P-1", Joint: "J-1", Segment: 2, MaterialGen: 1},
		{Building: "A", Panel: "P-1", Joint: "J-1", Segment: 1, MaterialGen: 1},
		{Building: "A", Panel: "P-1", Joint: "J-1", Segment: 1, MaterialGen: 1},
	}
	got := UniqueImpact(keys)
	if len(got) != 3 {
		t.Fatalf("expected 3 unique impacts, got %d", len(got))
	}
	// Sorted order: (P-1,J-1,S1), (P-1,J-1,S2), (P-2,J-2,S1).
	if got[0].Segment != 1 || got[0].Joint != "J-1" {
		t.Fatalf("first impact wrong: %+v", got[0])
	}
	if got[1].Segment != 2 || got[1].Joint != "J-1" {
		t.Fatalf("second impact wrong: %+v", got[1])
	}
	if got[2].Panel != "P-2" {
		t.Fatalf("third impact wrong: %+v", got[2])
	}
}

func TestReviewValidator(t *testing.T) {
	if err := ReviewValidator(domain.Review{ReviewerID: "a"}, domain.Review{ReviewerID: "a"}); err == nil || err.Code != domain.CodeDependencyUnmet {
		t.Fatalf("same reviewer err=%v want DEPENDENCY_UNMET", err)
	}
	if err := ReviewValidator(domain.Review{ReviewerID: "a"}, domain.Review{ReviewerID: "b"}); err != nil {
		t.Fatalf("distinct reviewers err=%v want nil", err)
	}
	if err := ReviewValidator(domain.Review{ReviewerID: ""}, domain.Review{ReviewerID: "b"}); err == nil || err.Code != domain.CodeDependencyUnmet {
		t.Fatalf("missing reviewer err=%v want DEPENDENCY_UNMET", err)
	}
}
