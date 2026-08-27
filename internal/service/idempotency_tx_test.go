package service

import (
	"testing"

	"unitized-curtainwall-silicone-hoist-gate/internal/domain"
)

func TestIdempotentRepeatReturnsSameResult(t *testing.T) {
	svc := newTestService(t)
	lockSample(t, svc)

	cmd := Command{OperationID: "op-clean", Kind: CommandClean, LogicalTime: 101}
	r1 := submit(t, svc, "T-1", cmd)
	r2 := submit(t, svc, "T-1", cmd)
	if r1.Stage != r2.Stage || r1.Generation != r2.Generation {
		t.Fatalf("idempotent repeat mismatch: %+v vs %+v", r1, r2)
	}
}

func TestIdempotencyConflictOnDifferentContent(t *testing.T) {
	svc := newTestService(t)
	lockSample(t, svc)

	submit(t, svc, "T-1", Command{OperationID: "op-x", Kind: CommandClean, LogicalTime: 101})
	_, err := svc.SubmitCommand("T-1", Command{OperationID: "op-x", Kind: CommandClean, LogicalTime: 999})
	if err == nil || err.Code != domain.CodeIdempotencyConflict {
		t.Fatalf("idempotency conflict err=%v want IDEMPOTENCY_CONFLICT", err)
	}
}

func TestMassConservationAcrossCategories(t *testing.T) {
	svc := newTestService(t)
	lockSample(t, svc)
	submit(t, svc, "T-1", Command{OperationID: "op-clean", Kind: CommandClean, LogicalTime: 101})
	submit(t, svc, "T-1", Command{OperationID: "op-prime", Kind: CommandPrime, LogicalTime: 102})
	submit(t, svc, "T-1", Command{
		OperationID: "op-mix", Kind: CommandMix, LogicalTime: 103,
		MixerWindow: "W-1", BaseMg: 1000, CatalystMg: 100, PrimerMg: 50,
		TargetRatio: 100, OpenDeadline: 1000, LeaseExpiry: 500, LeaseTokens: mixTokens(),
	})
	// Trial shot deducts trial mass.
	submit(t, svc, "T-1", Command{OperationID: "op-trial", Kind: CommandTrialShot,
		LogicalTime: 104, BaseMg: 20, CatalystMg: 2})

	// The generation must not yet be conserved: base 980, catalyst 98, primer 50
	// remain.
	entries, err := svc.GetMassBalance()
	if err != nil {
		t.Fatalf("mass balance: %v", err)
	}
	var baseIn, baseOut int64
	for _, e := range entries {
		if e.Component != domain.ComponentBase {
			continue
		}
		if e.Direction == domain.MassInput {
			baseIn += int64(e.Amount)
		} else {
			baseOut += int64(e.Amount)
		}
	}
	if baseIn != 1000 || baseOut != 20 {
		t.Fatalf("base in=%d out=%d want in=1000 out=20", baseIn, baseOut)
	}
}

func TestOverdrawRejected(t *testing.T) {
	svc := newTestService(t)
	lockSample(t, svc)
	submit(t, svc, "T-1", Command{OperationID: "op-clean", Kind: CommandClean, LogicalTime: 101})
	submit(t, svc, "T-1", Command{OperationID: "op-prime", Kind: CommandPrime, LogicalTime: 102})
	submit(t, svc, "T-1", Command{
		OperationID: "op-mix", Kind: CommandMix, LogicalTime: 103,
		MixerWindow: "W-1", BaseMg: 100, CatalystMg: 10, PrimerMg: 5,
		TargetRatio: 100, OpenDeadline: 1000, LeaseExpiry: 500, LeaseTokens: mixTokens(),
	})
	// Trial shot exceeding the opened base mass must fail with MATERIAL_OVERDRAW.
	_, err := svc.SubmitCommand("T-1", Command{OperationID: "op-trial", Kind: CommandTrialShot,
		LogicalTime: 104, BaseMg: 500, CatalystMg: 1})
	if err == nil || err.Code != domain.CodeMaterialOverdraw {
		t.Fatalf("overdraw err=%v want MATERIAL_OVERDRAW", err)
	}
}
