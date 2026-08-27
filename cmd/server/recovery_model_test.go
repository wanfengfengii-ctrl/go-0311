package main

import (
	"context"
	"testing"

	"unitized-curtainwall-silicone-hoist-gate/internal/catalog"
	"unitized-curtainwall-silicone-hoist-gate/internal/domain"
	"unitized-curtainwall-silicone-hoist-gate/internal/service"
	"unitized-curtainwall-silicone-hoist-gate/internal/store"
)

func TestModel_RecoveryUsesLogicalTimeForLeaseExpiry(t *testing.T) {
	tests := []struct {
		name            string
		leaseExpiry     domain.LogicalTime
		lastLogicalTime domain.LogicalTime
		wantInjection   bool
	}{
		{name: "lease valid in business time survives restart", leaseExpiry: 500, lastLogicalTime: 104, wantInjection: true},
		{name: "lease expired in business time is removed", leaseExpiry: 104, lastLogicalTime: 105, wantInjection: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := t.TempDir() + "/hoist.db"
			st, err := store.Open(path)
			if err != nil {
				t.Fatalf("open store: %v", err)
			}
			svc := service.New(st)
			_, derr := svc.Lock(catalog.LockRequest{
				TaskID: "T-1", Building: "A", FacadeZone: "E", Panel: "P-017",
				DesignVersion: "dv-1", CompatibilityVer: "cv-1", CompatValidUntil: 1000,
				SurfaceSummary: "clean", LockedAt: 100,
				Batch: domain.MaterialBatch{BaseBatch: "B-1", CatalystBatch: "C-1", PrimerBatch: "P-1"},
				Joints: []domain.JointSpec{{
					JointID: "J-1", Direction: "E", Start: 0, End: 1000, Width: 20, Depth: 10,
					BondAreaUm2: 200, Segments: []domain.SegmentSpec{{Seq: 1, Start: 0, End: 1000}},
					TrialMapping: map[string]string{"seg1": "sample-1"},
				}},
				Thresholds: domain.Thresholds{MinTensileMPa: 50, MaxBondFailurePct: 10},
			})
			if derr != nil {
				t.Fatalf("lock: %v", derr)
			}
			commands := []service.Command{
				{OperationID: "op-clean", Kind: service.CommandClean, LogicalTime: 101},
				{OperationID: "op-prime", Kind: service.CommandPrime, LogicalTime: 102},
				{
					OperationID: "op-mix", Kind: service.CommandMix, LogicalTime: 103,
					MixerWindow: "W-1", BaseMg: 1000, CatalystMg: 100, PrimerMg: 50,
					TargetRatio: 100, OpenDeadline: 1000, LeaseExpiry: tt.leaseExpiry,
					LeaseTokens: map[string]string{"mixer": "tok-mixer", "metering_pump": "tok-pump", "injection_table": "tok-table"},
				},
				{OperationID: "op-trial", Kind: service.CommandTrialShot, LogicalTime: tt.lastLogicalTime, BaseMg: 20, CatalystMg: 2},
			}
			for _, cmd := range commands {
				if _, derr := svc.SubmitCommand("T-1", cmd); derr != nil {
					t.Fatalf("submit %s: %v", cmd.Kind, derr)
				}
			}
			if err := st.Close(); err != nil {
				t.Fatalf("close before restart: %v", err)
			}

			restarted, err := store.Open(path)
			if err != nil {
				t.Fatalf("reopen store: %v", err)
			}
			t.Cleanup(func() { _ = restarted.Close() })
			if err := recover(restarted); err != nil {
				t.Fatalf("recover: %v", err)
			}

			if tt.wantInjection {
				_, derr := service.New(restarted).SubmitCommand("T-1", service.Command{
					OperationID: "op-inject", Kind: service.CommandInjectSegment, LogicalTime: 105,
					JointID: "J-1", SegmentSeq: 1, LeaseTokens: map[string]string{"mixer": "tok-mixer"},
				})
				if derr != nil {
					t.Fatalf("inject with pre-restart token: %v", derr)
				}
				return
			}

			err = restarted.InTx(context.Background(), func(tx *store.Tx) error {
				return tx.ReleaseLease(domain.ResourceMixer, "mixer", "tok-mixer", 103)
			})
			if derr, ok := err.(*domain.Error); !ok || derr.Code != domain.CodeLeaseExpired {
				t.Fatalf("release recovered expired lease err=%v, want LEASE_EXPIRED for absent lease", err)
			}
		})
	}
}
