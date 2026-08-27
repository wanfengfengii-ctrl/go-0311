package service_test

import (
	"path/filepath"
	"reflect"
	"testing"

	"unitized-curtainwall-silicone-hoist-gate/internal/catalog"
	"unitized-curtainwall-silicone-hoist-gate/internal/domain"
	"unitized-curtainwall-silicone-hoist-gate/internal/service"
	"unitized-curtainwall-silicone-hoist-gate/internal/store"
)

func TestModel_EvidenceChainPersistsAcrossRestart(t *testing.T) {
	cases := []struct {
		name          string
		commands      []service.Command
		wantTypes     []domain.EventType
		wantTaskStage string
	}{
		{
			name: "clean links to the lock event",
			commands: []service.Command{
				{OperationID: "op-clean", Kind: service.CommandClean, LogicalTime: 101},
			},
			wantTypes:     []domain.EventType{domain.EventTaskLocked, domain.EventCleaned},
			wantTaskStage: "CLEANED",
		},
		{
			name: "prime links to clean",
			commands: []service.Command{
				{OperationID: "op-clean", Kind: service.CommandClean, LogicalTime: 101},
				{OperationID: "op-prime", Kind: service.CommandPrime, LogicalTime: 102},
			},
			wantTypes:     []domain.EventType{domain.EventTaskLocked, domain.EventCleaned, domain.EventPrimed},
			wantTaskStage: "PRIMED",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dbPath := filepath.Join(t.TempDir(), "evidence.db")
			st, err := store.Open(dbPath)
			if err != nil {
				t.Fatalf("open store: %v", err)
			}
			svc := service.New(st)
			_, lockErr := svc.Lock(catalog.LockRequest{
				TaskID:           "T-chain",
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
					JointID: "J-1", Direction: "E", Start: 0, End: 1000,
					Width: 20, Depth: 10, BondAreaUm2: 200,
					Segments:     []domain.SegmentSpec{{Seq: 1, Start: 0, End: 1000}},
					TrialMapping: map[string]string{"seg1": "sample-1"},
				}},
				Thresholds: domain.Thresholds{MinTensileMPa: 50, MaxBondFailurePct: 10},
				Adjacency:  []string{"P-018"},
				LockedAt:   100,
			})
			if lockErr != nil {
				t.Fatalf("lock task: %v", lockErr)
			}

			var lastResult service.CommandResult
			for _, cmd := range tc.commands {
				lastResult, lockErr = svc.SubmitCommand("T-chain", cmd)
				if lockErr != nil {
					t.Fatalf("submit %s: %v", cmd.Kind, lockErr)
				}
			}
			retried, retryErr := svc.SubmitCommand("T-chain", tc.commands[len(tc.commands)-1])
			if retryErr != nil {
				t.Fatalf("retry last command: %v", retryErr)
			}
			if !reflect.DeepEqual(retried, lastResult) {
				t.Fatalf("idempotent retry result = %#v, want original %#v", retried, lastResult)
			}

			beforeRestart, evidenceErr := svc.GetEvidence("T-chain")
			if evidenceErr != nil {
				t.Fatalf("query evidence before restart: %v", evidenceErr)
			}
			if len(beforeRestart) != len(tc.wantTypes) {
				t.Fatalf("event count after retry = %d, want %d", len(beforeRestart), len(tc.wantTypes))
			}
			if err := st.Close(); err != nil {
				t.Fatalf("close store: %v", err)
			}

			st, err = store.Open(dbPath)
			if err != nil {
				t.Fatalf("reopen store: %v", err)
			}
			t.Cleanup(func() { _ = st.Close() })
			svc = service.New(st)
			afterRestart, evidenceErr := svc.GetEvidence("T-chain")
			if evidenceErr != nil {
				t.Fatalf("query evidence after restart: %v", evidenceErr)
			}
			if !reflect.DeepEqual(afterRestart, beforeRestart) {
				t.Fatalf("persisted evidence changed across restart: before=%#v after=%#v", beforeRestart, afterRestart)
			}

			for i, event := range afterRestart {
				if event.Type != tc.wantTypes[i] {
					t.Fatalf("event[%d].Type = %v, want %v", i, event.Type, tc.wantTypes[i])
				}
				if event.PayloadHash != domain.CanonicalHash(event.Payload) {
					t.Fatalf("event[%d].PayloadHash = %q, want canonical payload hash", i, event.PayloadHash)
				}
				if i == 0 {
					if event.PrevHash != "" {
						t.Fatalf("first event PrevHash = %q, want empty", event.PrevHash)
					}
					continue
				}
				if event.Seq <= afterRestart[i-1].Seq {
					t.Fatalf("event[%d].Seq = %d, want greater than %d", i, event.Seq, afterRestart[i-1].Seq)
				}
				if event.PrevHash == "" || event.PrevHash != afterRestart[i-1].PayloadHash {
					t.Fatalf("event[%d].PrevHash = %q, want previous payload hash %q", i, event.PrevHash, afterRestart[i-1].PayloadHash)
				}
			}

			view, viewErr := svc.GetTask("T-chain")
			if viewErr != nil {
				t.Fatalf("replay task projection after restart: %v", viewErr)
			}
			if view.Stage != tc.wantTaskStage {
				t.Fatalf("replayed task stage = %q, want %q", view.Stage, tc.wantTaskStage)
			}
		})
	}
}
