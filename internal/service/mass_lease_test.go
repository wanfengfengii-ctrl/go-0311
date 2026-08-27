package service

import (
	"sync"
	"testing"

	"unitized-curtainwall-silicone-hoist-gate/internal/domain"
)

// TestConcurrentMixSingleWinner runs two concurrent mix commands competing for
// the same mixer lease and stock, asserting exactly one succeeds and the loser
// leaves no mass or lease behind.
func TestConcurrentMixSingleWinner(t *testing.T) {
	svc := newTestService(t)
	lockSample(t, svc)
	submit(t, svc, "T-1", Command{OperationID: "op-clean", Kind: CommandClean, LogicalTime: 101})
	submit(t, svc, "T-1", Command{OperationID: "op-prime", Kind: CommandPrime, LogicalTime: 102})

	var wg sync.WaitGroup
	errs := make([]*domain.Error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, errs[i] = svc.SubmitCommand("T-1", Command{
				OperationID: "op-mix-" + string(rune('A'+i)), Kind: CommandMix, LogicalTime: 103,
				MixerWindow: "W-1", BaseMg: 1000, CatalystMg: 100, PrimerMg: 50,
				TargetRatio: 100, OpenDeadline: 1000, LeaseExpiry: 500,
				LeaseTokens: map[string]string{
					"mixer":           "tok-" + string(rune('A'+i)),
					"metering_pump":   "tok-" + string(rune('A'+i)),
					"injection_table": "tok-" + string(rune('A'+i)),
				},
			})
		}(i)
	}
	wg.Wait()

	okCount := 0
	conflictCount := 0
	for _, err := range errs {
		switch {
		case err == nil:
			okCount++
		case err.Code == domain.CodeLeaseConflict:
			conflictCount++
		}
	}
	if okCount != 1 {
		t.Fatalf("expected exactly one successful mix, got %d (errs=%v)", okCount, errs)
	}
	if conflictCount != 1 {
		t.Fatalf("expected exactly one lease conflict, got %d", conflictCount)
	}

	// Only one material generation's stock should have been opened.
	entries, err := svc.GetMassBalance()
	if err != nil {
		t.Fatalf("mass balance: %v", err)
	}
	var baseIn int64
	for _, e := range entries {
		if e.Component == domain.ComponentBase && e.Direction == domain.MassInput {
			baseIn += int64(e.Amount)
		}
	}
	if baseIn != 1000 {
		t.Fatalf("base input=%d want 1000 (loser must not deduct)", baseIn)
	}
}
