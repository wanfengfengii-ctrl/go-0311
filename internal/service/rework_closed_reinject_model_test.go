package service

import (
	"reflect"
	"testing"

	"unitized-curtainwall-silicone-hoist-gate/internal/domain"
)

func TestModel_ReworkReinjectClosedCannotBeBookedAgain(t *testing.T) {
	valid := ReinjectRequest{BaseMg: 500, CatalystMg: 50, PrimerMg: 25, LogicalTime: 202}
	tests := []struct {
		name        string
		first       ReinjectRequest
		firstCode   domain.ErrorCode
		retry       *ReinjectRequest
		retryCode   domain.ErrorCode
		wantClosed  bool
		wantEntries int
		wantEvents  int
	}{
		{
			name:        "failed material write rolls back the whole reinjection",
			first:       ReinjectRequest{BaseMg: 500, CatalystMg: -1, PrimerMg: 25, LogicalTime: 202},
			firstCode:   domain.CodeMaterialOverdraw,
			wantClosed:  false,
			wantEntries: 0,
			wantEvents:  0,
		},
		{
			name:        "same timed-out request after close is rejected without duplicate booking",
			first:       valid,
			retry:       &valid,
			retryCode:   domain.CodeReworkGenerationConflict,
			wantClosed:  true,
			wantEntries: 3,
			wantEvents:  1,
		},
		{
			name:  "changed request after close is rejected without changing generation",
			first: valid,
			retry: &ReinjectRequest{
				BaseMg: 900, CatalystMg: 90, PrimerMg: 45, LogicalTime: 999,
			},
			retryCode:   domain.CodeReworkGenerationConflict,
			wantClosed:  true,
			wantEntries: 3,
			wantEvents:  1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := newTestService(t)
			lockSample(t, svc)
			rw, derr := svc.CreateRework("T-1", ReworkRequest{
				Category: "ratio_drift", RootJoint: "J-1", LogicalTime: 200,
			})
			if derr != nil {
				t.Fatalf("create rework: %v", derr)
			}
			if derr := svc.ReworkCutout(rw.CaseID, CutoutRequest{
				CutoutMass: 300, Destination: "recycle", LogicalTime: 201,
			}); derr != nil {
				t.Fatalf("cutout: %v", derr)
			}

			derr = svc.ReworkReinject(rw.CaseID, tc.first)
			if tc.firstCode == "" {
				if derr != nil {
					t.Fatalf("first reinject: %v", derr)
				}
			} else if derr == nil || derr.Code != tc.firstCode {
				t.Fatalf("first reinject error=%v, want code %s", derr, tc.firstCode)
			}

			beforeMass, derr := svc.GetMassBalance()
			if derr != nil {
				t.Fatalf("mass before retry: %v", derr)
			}
			beforeEvidence, derr := svc.GetEvidence("T-1")
			if derr != nil {
				t.Fatalf("evidence before retry: %v", derr)
			}
			beforeReworks, derr := svc.GetReworks()
			if derr != nil {
				t.Fatalf("reworks before retry: %v", derr)
			}

			if tc.retry != nil {
				derr = svc.ReworkReinject(rw.CaseID, *tc.retry)
				if derr == nil || derr.Code != tc.retryCode {
					t.Fatalf("retry error=%v, want code %s", derr, tc.retryCode)
				}
			}

			afterMass, derr := svc.GetMassBalance()
			if derr != nil {
				t.Fatalf("mass after retry: %v", derr)
			}
			afterEvidence, derr := svc.GetEvidence("T-1")
			if derr != nil {
				t.Fatalf("evidence after retry: %v", derr)
			}
			afterReworks, derr := svc.GetReworks()
			if derr != nil {
				t.Fatalf("reworks after retry: %v", derr)
			}

			if !reflect.DeepEqual(afterMass, beforeMass) ||
				!reflect.DeepEqual(afterEvidence, beforeEvidence) ||
				!reflect.DeepEqual(afterReworks, beforeReworks) {
				t.Fatalf("reinject retry changed committed state\nbefore mass=%+v\nafter mass=%+v\nbefore evidence=%+v\nafter evidence=%+v\nbefore reworks=%+v\nafter reworks=%+v",
					beforeMass, afterMass, beforeEvidence, afterEvidence, beforeReworks, afterReworks)
			}

			var generationEntries, reinjectEvents int
			for _, entry := range afterMass {
				if entry.Generation == rw.NewGeneration {
					generationEntries++
				}
			}
			for _, event := range afterEvidence {
				if event.Generation == rw.NewGeneration && event.Type == domain.EventReworkReinject {
					reinjectEvents++
				}
			}
			if generationEntries != tc.wantEntries || reinjectEvents != tc.wantEvents {
				t.Fatalf("generation %d has %d material entries and %d reinject events, want %d and %d",
					rw.NewGeneration, generationEntries, reinjectEvents, tc.wantEntries, tc.wantEvents)
			}
			if tc.wantEntries == 3 {
				wantComponents := []domain.Component{domain.ComponentBase, domain.ComponentCatalyst, domain.ComponentPrimer}
				wantAmounts := []domain.Milligrams{valid.BaseMg, valid.CatalystMg, valid.PrimerMg}
				var index int
				for _, entry := range afterMass {
					if entry.Generation != rw.NewGeneration {
						continue
					}
					if entry.Component != wantComponents[index] || entry.Amount != wantAmounts[index] ||
						entry.Direction != domain.MassInput || entry.Category != domain.MassStock || entry.Evidence != rw.CaseID {
						t.Fatalf("generation %d material entry[%d]=%+v, want component=%s amount=%d stock input for %s",
							rw.NewGeneration, index, entry, wantComponents[index], wantAmounts[index], rw.CaseID)
					}
					index++
				}
			}
			if len(afterReworks) != 1 || afterReworks[0].Closed != tc.wantClosed {
				t.Fatalf("rework state=%+v, want one case with closed=%t", afterReworks, tc.wantClosed)
			}
			if !tc.wantClosed && afterReworks[0].ReinjectGen != 0 {
				t.Fatalf("failed reinject changed generation to %d", afterReworks[0].ReinjectGen)
			}
		})
	}
}
