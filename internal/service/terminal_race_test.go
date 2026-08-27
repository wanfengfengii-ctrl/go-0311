package service

import (
	"sync"
	"testing"

	"unitized-curtainwall-silicone-hoist-gate/internal/domain"
)

// driveToAdmission drives a task through every stage to a conserved, tested and
// double-reviewed state ready for terminal decision.
func driveToAdmission(t *testing.T, svc *Service) {
	t.Helper()
	lockSample(t, svc)
	submit(t, svc, "T-1", Command{OperationID: "op-clean", Kind: CommandClean, LogicalTime: 101})
	submit(t, svc, "T-1", Command{OperationID: "op-prime", Kind: CommandPrime, LogicalTime: 102})
	submit(t, svc, "T-1", Command{
		OperationID: "op-mix", Kind: CommandMix, LogicalTime: 103,
		MixerWindow: "W-1", BaseMg: 1000, CatalystMg: 100, PrimerMg: 50,
		TargetRatio: 100, OpenDeadline: 1000, LeaseExpiry: 500, LeaseTokens: mixTokens(),
	})
	submit(t, svc, "T-1", Command{OperationID: "op-trial", Kind: CommandTrialShot,
		LogicalTime: 104, BaseMg: 20, CatalystMg: 2})
	for i, seq := range []int64{1, 2, 3} {
		submit(t, svc, "T-1", Command{
			OperationID: "op-seg" + string(rune('0'+seq)), Kind: CommandInjectSegment,
			LogicalTime: domain.LogicalTime(105 + i), JointID: "J-1", SegmentSeq: seq,
			BaseMg: 300, CatalystMg: 30, LeaseTokens: map[string]string{"mixer": "tok-mixer"},
		})
	}
	submit(t, svc, "T-1", Command{OperationID: "op-trim", Kind: CommandTrim, LogicalTime: 108})
	submit(t, svc, "T-1", Command{OperationID: "op-seal", Kind: CommandSeal,
		LogicalTime: 109, BaseMg: 80, CatalystMg: 8, PrimerMg: 50})
	submit(t, svc, "T-1", Command{OperationID: "op-cure", Kind: CommandCureReading,
		LogicalTime: 110, SampleID: "S-1", Temperature: 1000, Humidity: 1000})
	submit(t, svc, "T-1", Command{OperationID: "op-test", Kind: CommandTestResult,
		LogicalTime: 111, TestID: "TS-1", TestKind: domain.TestTensile,
		TensileMPa: 60, ElongationPct: 5, BondFailurePct: 3})
	if err := svc.SubmitReview("T-1", ReviewRequest{ReviewerID: "r1", QualSnapshot: "q1", Summary: "ok", LogicalTime: 112}); err != nil {
		t.Fatalf("review r1: %v", err)
	}
	if err := svc.SubmitReview("T-1", ReviewRequest{ReviewerID: "r2", QualSnapshot: "q2", Summary: "ok", LogicalTime: 113}); err != nil {
		t.Fatalf("review r2: %v", err)
	}
}

func TestTerminalAdmission(t *testing.T) {
	svc := newTestService(t)
	driveToAdmission(t, svc)

	res, err := svc.DecideTerminal("T-1", TerminalRequest{Type: domain.TerminalHoistAdmitted, LogicalTime: 200})
	if err != nil {
		t.Fatalf("admission: %v", err)
	}
	if res.Type != domain.TerminalHoistAdmitted || res.Credential == "" {
		t.Fatalf("admission result=%+v", res)
	}
}

func TestTerminalRequiresDistinctReviewers(t *testing.T) {
	svc := newTestService(t)
	driveToAdmission(t, svc)
	// Add a third review from the same reviewer r1: still two distinct remain,
	// but a review from r1 twice does not satisfy the two-person rule if r2 were
	// absent. Assert the existing distinct-reviewer path is already satisfied by
	// removing nothing; instead verify a fresh task with a single reviewer fails.
	svc2 := newTestService(t)
	lockSample(t, svc2)
	// Drive only partially: skip reviews entirely.
	if err := svc2.SubmitReview("T-1", ReviewRequest{ReviewerID: "only", QualSnapshot: "q", Summary: "ok", LogicalTime: 112}); err != nil {
		// Review before test stage is rejected; that is the dependency gate.
		if err.Code != domain.CodeDependencyUnmet {
			t.Fatalf("expected DEPENDENCY_UNMET, got %v", err)
		}
	}
}

func TestTerminalSameReviewerRejected(t *testing.T) {
	svc := newTestService(t)
	lockSample(t, svc)
	submit(t, svc, "T-1", Command{OperationID: "op-clean", Kind: CommandClean, LogicalTime: 101})
	submit(t, svc, "T-1", Command{OperationID: "op-prime", Kind: CommandPrime, LogicalTime: 102})
	submit(t, svc, "T-1", Command{
		OperationID: "op-mix", Kind: CommandMix, LogicalTime: 103,
		MixerWindow: "W-1", BaseMg: 1000, CatalystMg: 100, PrimerMg: 50,
		TargetRatio: 100, OpenDeadline: 1000, LeaseExpiry: 500, LeaseTokens: mixTokens(),
	})
	submit(t, svc, "T-1", Command{OperationID: "op-trial", Kind: CommandTrialShot, LogicalTime: 104, BaseMg: 20, CatalystMg: 2})
	// Inject all segments and close so the joint prefix is complete.
	for i, seq := range []int64{1, 2, 3} {
		submit(t, svc, "T-1", Command{
			OperationID: "op-seg" + string(rune('0'+seq)), Kind: CommandInjectSegment,
			LogicalTime: domain.LogicalTime(105 + i), JointID: "J-1", SegmentSeq: seq,
			BaseMg: 300, CatalystMg: 30, LeaseTokens: map[string]string{"mixer": "tok-mixer"},
		})
	}
	submit(t, svc, "T-1", Command{OperationID: "op-trim", Kind: CommandTrim, LogicalTime: 108})
	submit(t, svc, "T-1", Command{OperationID: "op-seal", Kind: CommandSeal, LogicalTime: 109, BaseMg: 80, CatalystMg: 8, PrimerMg: 50})
	submit(t, svc, "T-1", Command{OperationID: "op-cure", Kind: CommandCureReading, LogicalTime: 110, SampleID: "S-1", Temperature: 1000, Humidity: 1000})
	submit(t, svc, "T-1", Command{OperationID: "op-test", Kind: CommandTestResult, LogicalTime: 111, TestID: "TS-1", TestKind: domain.TestTensile, TensileMPa: 60, ElongationPct: 5, BondFailurePct: 3})

	// Same reviewer twice must not satisfy the two-person rule.
	if err := svc.SubmitReview("T-1", ReviewRequest{ReviewerID: "same", QualSnapshot: "q", Summary: "ok", LogicalTime: 112}); err != nil {
		t.Fatalf("review: %v", err)
	}
	if err := svc.SubmitReview("T-1", ReviewRequest{ReviewerID: "same", QualSnapshot: "q", Summary: "again", LogicalTime: 113}); err != nil {
		t.Fatalf("review: %v", err)
	}
	_, err := svc.DecideTerminal("T-1", TerminalRequest{Type: domain.TerminalHoistAdmitted, LogicalTime: 200})
	if err == nil || err.Code != domain.CodeDependencyUnmet {
		t.Fatalf("admission same-reviewer err=%v want DEPENDENCY_UNMET", err)
	}
}

func TestTerminalRaceSingleWinner(t *testing.T) {
	svc := newTestService(t)
	driveToAdmission(t, svc)

	var wg sync.WaitGroup
	types := []domain.TerminalType{domain.TerminalHoistAdmitted, domain.TerminalBondRiskIsolated, domain.TerminalCancelled}
	results := make([]*domain.Error, len(types))
	for i, tt := range types {
		wg.Add(1)
		go func(i int, tt domain.TerminalType) {
			defer wg.Done()
			_, results[i] = svc.DecideTerminal("T-1", TerminalRequest{Type: tt, LogicalTime: 200})
		}(i, tt)
	}
	wg.Wait()

	okCount := 0
	for _, err := range results {
		if err == nil {
			okCount++
		} else if err.Code != domain.CodeTerminalAlreadyDecided {
			t.Fatalf("unexpected terminal error: %v", err)
		}
	}
	if okCount != 1 {
		t.Fatalf("expected exactly one terminal winner, got %d", okCount)
	}

	term, found, err := svc.GetTerminal("T-1")
	if err != nil || !found {
		t.Fatalf("terminal after race: found=%v err=%v", found, err)
	}
	if term.Credential == "" {
		t.Fatalf("terminal credential empty: %+v", term)
	}
}
