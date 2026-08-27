package service

import (
	"testing"

	"unitized-curtainwall-silicone-hoist-gate/internal/domain"
)

// mixTokens returns the lease tokens expected by the mix command.
func mixTokens() map[string]string {
	return map[string]string{
		"mixer":           "tok-mixer",
		"metering_pump":   "tok-pump",
		"injection_table": "tok-table",
	}
}

// runWorkflow drives the task through clean, prime, mix, trial and three
// contiguous segment injections, returning the final task view.
func runWorkflow(t *testing.T, svc *Service) {
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
}

func TestWorkflowPrefixGrowth(t *testing.T) {
	svc := newTestService(t)
	runWorkflow(t, svc)

	// Inject the three segments in order and assert the prefix grows each time.
	for i, seg := range []struct {
		op  string
		seq int64
		end int64
	}{
		{"op-seg1", 1, 1000},
		{"op-seg2", 2, 2000},
		{"op-seg3", 3, 3000},
	} {
		res := submit(t, svc, "T-1", Command{
			OperationID: seg.op, Kind: CommandInjectSegment, LogicalTime: domain.LogicalTime(105 + i),
			JointID: "J-1", SegmentSeq: seg.seq, LeaseTokens: map[string]string{"mixer": "tok-mixer"},
		})
		if res.PrefixEnd != domain.Microns(seg.end) {
			t.Fatalf("segment %d prefix=%d want %d", seg.seq, res.PrefixEnd, seg.end)
		}
	}
}

func TestWorkflowSkipSegmentRejected(t *testing.T) {
	svc := newTestService(t)
	runWorkflow(t, svc)

	// Attempt segment 2 before segment 1: skipped ahead of the prefix.
	_, err := svc.SubmitCommand("T-1", Command{
		OperationID: "op-skip", Kind: CommandInjectSegment, LogicalTime: 105,
		JointID: "J-1", SegmentSeq: 2, LeaseTokens: map[string]string{"mixer": "tok-mixer"},
	})
	if err == nil || err.Code != domain.CodeNoncontiguousPrefix {
		t.Fatalf("skip err=%v want NONCONTIGUOUS_PREFIX", err)
	}
}

func TestWorkflowReverseSegmentRejected(t *testing.T) {
	svc := newTestService(t)
	runWorkflow(t, svc)
	submit(t, svc, "T-1", Command{OperationID: "op-seg1", Kind: CommandInjectSegment,
		LogicalTime: 105, JointID: "J-1", SegmentSeq: 1, LeaseTokens: map[string]string{"mixer": "tok-mixer"}})

	// Re-inject segment 1: reversed, already covered.
	_, err := svc.SubmitCommand("T-1", Command{
		OperationID: "op-rev", Kind: CommandInjectSegment, LogicalTime: 106,
		JointID: "J-1", SegmentSeq: 1, LeaseTokens: map[string]string{"mixer": "tok-mixer"},
	})
	if err == nil || err.Code != domain.CodeNoncontiguousPrefix {
		t.Fatalf("reverse err=%v want NONCONTIGUOUS_PREFIX", err)
	}
}

func TestWorkflowWrongGenerationRejected(t *testing.T) {
	svc := newTestService(t)
	runWorkflow(t, svc)

	_, err := svc.SubmitCommand("T-1", Command{
		OperationID: "op-gen", Kind: CommandInjectSegment, ExpectedGen: 99, LogicalTime: 105,
		JointID: "J-1", SegmentSeq: 1, LeaseTokens: map[string]string{"mixer": "tok-mixer"},
	})
	if err == nil || err.Code != domain.CodeGenerationMismatch {
		t.Fatalf("generation err=%v want GENERATION_MISMATCH", err)
	}
}

func TestWorkflowOpenTimeExpired(t *testing.T) {
	svc := newTestService(t)
	lockSample(t, svc)
	submit(t, svc, "T-1", Command{OperationID: "op-clean", Kind: CommandClean, LogicalTime: 101})
	submit(t, svc, "T-1", Command{OperationID: "op-prime", Kind: CommandPrime, LogicalTime: 102})

	// Mix with an open deadline already in the past.
	_, err := svc.SubmitCommand("T-1", Command{
		OperationID: "op-mix", Kind: CommandMix, LogicalTime: 103,
		MixerWindow: "W-1", BaseMg: 1000, CatalystMg: 100, PrimerMg: 50,
		TargetRatio: 100, OpenDeadline: 50, LeaseExpiry: 500, LeaseTokens: mixTokens(),
	})
	if err == nil || err.Code != domain.CodeOpenTimeExpired {
		t.Fatalf("open time err=%v want OPEN_TIME_EXPIRED", err)
	}
}

func TestWorkflowExpiredLeaseRejected(t *testing.T) {
	svc := newTestService(t)
	runWorkflow(t, svc)

	// Inject with a lease token that is not held.
	_, err := svc.SubmitCommand("T-1", Command{
		OperationID: "op-exp", Kind: CommandInjectSegment, LogicalTime: 105,
		JointID: "J-1", SegmentSeq: 1, LeaseTokens: map[string]string{"mixer": "wrong-token"},
	})
	if err == nil || err.Code != domain.CodeLeaseConflict {
		t.Fatalf("lease err=%v want LEASE_CONFLICT", err)
	}
}

func TestWorkflowDependencyUnmet(t *testing.T) {
	svc := newTestService(t)
	lockSample(t, svc)

	// Prime before clean.
	_, err := svc.SubmitCommand("T-1", Command{OperationID: "op-p", Kind: CommandPrime, LogicalTime: 101})
	if err == nil || err.Code != domain.CodeDependencyUnmet {
		t.Fatalf("dependency err=%v want DEPENDENCY_UNMET", err)
	}
}
