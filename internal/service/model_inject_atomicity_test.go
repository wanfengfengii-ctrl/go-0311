package service

import (
	"reflect"
	"testing"

	"unitized-curtainwall-silicone-hoist-gate/internal/domain"
)

func TestModel_InjectSegmentComponentOverdrawIsAtomic(t *testing.T) {
	cases := []struct {
		name           string
		failedBase     domain.Milligrams
		failedCatalyst domain.Milligrams
	}{
		{name: "base overdraw", failedBase: 71, failedCatalyst: 1},
		{name: "catalyst overdraw after base posting", failedBase: 10, failedCatalyst: 8},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := newTestService(t)
			lockSample(t, svc)
			submit(t, svc, "T-1", Command{OperationID: "op-clean", Kind: CommandClean, LogicalTime: 101})
			submit(t, svc, "T-1", Command{OperationID: "op-prime", Kind: CommandPrime, LogicalTime: 102})
			submit(t, svc, "T-1", Command{
				OperationID: "op-mix", Kind: CommandMix, LogicalTime: 103,
				MixerWindow: "W-1", BaseMg: 100, CatalystMg: 10, PrimerMg: 0,
				TargetRatio: 100, OpenDeadline: 1000, LeaseExpiry: 500, LeaseTokens: mixTokens(),
			})
			submit(t, svc, "T-1", Command{
				OperationID: "op-trial", Kind: CommandTrialShot, LogicalTime: 104,
				BaseMg: 10, CatalystMg: 1,
			})
			first := submit(t, svc, "T-1", Command{
				OperationID: "op-seg-1", Kind: CommandInjectSegment, LogicalTime: 105,
				JointID: "J-1", SegmentSeq: 1, BaseMg: 20, CatalystMg: 2,
				LeaseTokens: map[string]string{"mixer": "tok-mixer"},
			})
			if first.PrefixEnd != 1000 {
				t.Fatalf("first legal segment prefix=%d want 1000", first.PrefixEnd)
			}

			massBefore, derr := svc.GetMassBalance()
			if derr != nil {
				t.Fatalf("mass before failed segment: %v", derr)
			}
			evidenceBefore, derr := svc.GetAllEvidence()
			if derr != nil {
				t.Fatalf("evidence before failed segment: %v", derr)
			}

			failed := Command{
				OperationID: "op-seg-2", Kind: CommandInjectSegment, LogicalTime: 106,
				JointID: "J-1", SegmentSeq: 2,
				BaseMg: tc.failedBase, CatalystMg: tc.failedCatalyst,
				LeaseTokens: map[string]string{"mixer": "tok-mixer"},
			}
			if _, err := svc.SubmitCommand("T-1", failed); err == nil || err.Code != domain.CodeMaterialOverdraw {
				t.Fatalf("failed segment error=%v want MATERIAL_OVERDRAW", err)
			}

			massAfter, derr := svc.GetMassBalance()
			if derr != nil {
				t.Fatalf("mass after failed segment: %v", derr)
			}
			if !reflect.DeepEqual(massAfter, massBefore) {
				t.Fatalf("failed segment changed mass ledger:\n before=%+v\n after=%+v", massBefore, massAfter)
			}
			evidenceAfter, derr := svc.GetAllEvidence()
			if derr != nil {
				t.Fatalf("evidence after failed segment: %v", derr)
			}
			if !reflect.DeepEqual(evidenceAfter, evidenceBefore) {
				t.Fatalf("failed segment changed evidence:\n before=%+v\n after=%+v", evidenceBefore, evidenceAfter)
			}
			view, derr := svc.GetTask("T-1")
			if derr != nil {
				t.Fatalf("task after failed segment: %v", derr)
			}
			if len(view.Joints) != 1 || view.Joints[0].ValidPrefixEnd != 1000 {
				t.Fatalf("prefix after failed segment=%+v want 1000", view.Joints)
			}

			failed.BaseMg, failed.CatalystMg = 10, 1
			res, err := svc.SubmitCommand("T-1", failed)
			if err != nil {
				t.Fatalf("corrected retry with same operation id: %v", err)
			}
			if res.PrefixEnd != 2000 {
				t.Fatalf("corrected contiguous segment prefix=%d want 2000", res.PrefixEnd)
			}
		})
	}
}
