package service

import (
	"testing"

	"unitized-curtainwall-silicone-hoist-gate/internal/domain"
)

func TestModel_TerminalDecisionPrecedesLaterAdmissionState(t *testing.T) {
	cases := []struct {
		name           string
		requestType    domain.TerminalType
		decideFirst    bool
		wantCode       domain.ErrorCode
		wantCredential bool
	}{
		{name: "repeat hoist admission", requestType: domain.TerminalHoistAdmitted, decideFirst: true, wantCode: domain.CodeTerminalAlreadyDecided, wantCredential: true},
		{name: "request risk isolation", requestType: domain.TerminalBondRiskIsolated, decideFirst: true, wantCode: domain.CodeTerminalAlreadyDecided, wantCredential: true},
		{name: "request cancellation", requestType: domain.TerminalCancelled, decideFirst: true, wantCode: domain.CodeTerminalAlreadyDecided, wantCredential: true},
		{name: "first admission still checks open rework", requestType: domain.TerminalHoistAdmitted, wantCode: domain.CodeDependencyUnmet},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := newTestService(t)
			driveToAdmission(t, svc)

			var original domain.TerminalDecision
			if tc.decideFirst {
				result, err := svc.DecideTerminal("T-1", TerminalRequest{
					Type: domain.TerminalHoistAdmitted, LogicalTime: 200,
				})
				if err != nil {
					t.Fatalf("initial admission: %v", err)
				}
				original = domain.TerminalDecision{
					Type: result.Type, Credential: result.Credential,
					DecidedAt: result.DecidedAt, BarrierVer: result.BarrierVer,
				}
			}

			if _, err := svc.CreateRework("T-1", ReworkRequest{
				Category: "post_admission_anomaly", RootJoint: "J-1", LogicalTime: 201,
			}); err != nil {
				t.Fatalf("create later rework: %v", err)
			}

			_, err := svc.DecideTerminal("T-1", TerminalRequest{
				Type: tc.requestType, LogicalTime: 202,
			})
			if err == nil || err.Code != tc.wantCode {
				t.Fatalf("terminal request error=%v, want %s", err, tc.wantCode)
			}

			persisted, found, getErr := svc.GetTerminal("T-1")
			if getErr != nil {
				t.Fatalf("get terminal: %v", getErr)
			}
			if found != tc.wantCredential {
				t.Fatalf("terminal found=%v, want %v", found, tc.wantCredential)
			}
			if tc.wantCredential && (persisted.Type != original.Type ||
				persisted.Credential != original.Credential ||
				persisted.DecidedAt != original.DecidedAt ||
				persisted.BarrierVer != original.BarrierVer) {
				t.Fatalf("terminal changed after later rework: got %+v, want %+v", persisted, original)
			}
		})
	}
}
