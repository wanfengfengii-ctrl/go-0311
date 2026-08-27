package service

import (
	"testing"

	"unitized-curtainwall-silicone-hoist-gate/internal/domain"
)

func TestModel_HoistAdmissionScopesReworksToTargetTask(t *testing.T) {
	tests := []struct {
		name string
		run  func(t *testing.T, svc *Service)
	}{
		{
			name: "another task open rework does not block admission",
			run: func(t *testing.T, svc *Service) {
				driveToAdmission(t, svc)

				other := sampleLockRequest()
				other.TaskID = "T-2"
				other.Panel = "P-018"
				other.Joints[0].JointID = "J-2"
				if _, err := svc.Lock(other); err != nil {
					t.Fatalf("lock other task: %v", err)
				}
				if _, err := svc.CreateRework("T-2", ReworkRequest{
					Category: "bubble", RootJoint: "J-2", LogicalTime: 150,
				}); err != nil {
					t.Fatalf("create other task rework: %v", err)
				}

				result, err := svc.DecideTerminal("T-1", TerminalRequest{
					Type: domain.TerminalHoistAdmitted, LogicalTime: 200,
				})
				if err != nil {
					t.Fatalf("unrelated open rework blocked admission: %v", err)
				}
				if result.Type != domain.TerminalHoistAdmitted || result.Credential == "" {
					t.Fatalf("admission result=%+v", result)
				}
			},
		},
		{
			name: "target task open rework still blocks admission",
			run: func(t *testing.T, svc *Service) {
				driveToAdmission(t, svc)
				if _, err := svc.CreateRework("T-1", ReworkRequest{
					Category: "ratio-drift", RootJoint: "J-1", LogicalTime: 150,
				}); err != nil {
					t.Fatalf("create target task rework: %v", err)
				}

				_, err := svc.DecideTerminal("T-1", TerminalRequest{
					Type: domain.TerminalHoistAdmitted, LogicalTime: 200,
				})
				if err == nil || err.Code != domain.CodeDependencyUnmet {
					t.Fatalf("admission error=%v want %s", err, domain.CodeDependencyUnmet)
				}
			},
		},
		{
			name: "closing target task rework permits admission",
			run: func(t *testing.T, svc *Service) {
				driveToAdmission(t, svc)
				rw, err := svc.CreateRework("T-1", ReworkRequest{
					Category: "ratio-drift", RootJoint: "J-1", LogicalTime: 150,
				})
				if err != nil {
					t.Fatalf("create target task rework: %v", err)
				}
				if err := svc.ReworkCutout(rw.CaseID, CutoutRequest{
					Destination: "quarantine", LogicalTime: 151,
				}); err != nil {
					t.Fatalf("record cutout: %v", err)
				}
				if err := svc.ReworkReinject(rw.CaseID, ReinjectRequest{
					LogicalTime: 152,
				}); err != nil {
					t.Fatalf("close rework: %v", err)
				}

				result, err := svc.DecideTerminal("T-1", TerminalRequest{
					Type: domain.TerminalHoistAdmitted, LogicalTime: 200,
				})
				if err != nil {
					t.Fatalf("closed target rework blocked admission: %v", err)
				}
				if result.Type != domain.TerminalHoistAdmitted || result.Credential == "" {
					t.Fatalf("admission result=%+v", result)
				}
			},
		},
		{
			name: "global management query remains complete and ordered",
			run: func(t *testing.T, svc *Service) {
				first := sampleLockRequest()
				if _, err := svc.Lock(first); err != nil {
					t.Fatalf("lock first task: %v", err)
				}
				second := sampleLockRequest()
				second.TaskID = "T-2"
				second.Panel = "P-018"
				second.Joints[0].JointID = "J-2"
				if _, err := svc.Lock(second); err != nil {
					t.Fatalf("lock second task: %v", err)
				}
				if _, err := svc.CreateRework("T-2", ReworkRequest{
					Category: "bubble", RootJoint: "J-2", LogicalTime: 150,
				}); err != nil {
					t.Fatalf("create second task rework: %v", err)
				}
				if _, err := svc.CreateRework("T-1", ReworkRequest{
					Category: "ratio-drift", RootJoint: "J-1", LogicalTime: 151,
				}); err != nil {
					t.Fatalf("create first task rework: %v", err)
				}

				reworks, err := svc.GetReworks()
				if err != nil {
					t.Fatalf("get global reworks: %v", err)
				}
				if len(reworks) != 2 {
					t.Fatalf("global rework count=%d want 2", len(reworks))
				}
				want := []string{"rework:T-1:2", "rework:T-2:2"}
				for i := range want {
					if reworks[i].CaseID != want[i] {
						t.Fatalf("global reworks[%d]=%q want %q", i, reworks[i].CaseID, want[i])
					}
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.run(t, newTestService(t))
		})
	}
}
