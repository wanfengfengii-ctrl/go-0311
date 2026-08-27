package service

import (
	"testing"

	"unitized-curtainwall-silicone-hoist-gate/internal/domain"
	"unitized-curtainwall-silicone-hoist-gate/internal/store"
)

func TestDeviceRetrySequence(t *testing.T) {
	svc := newTestService(t)
	// Create a ratio-monitor device call via mix.
	lockSample(t, svc)
	submit(t, svc, "T-1", Command{OperationID: "op-clean", Kind: CommandClean, LogicalTime: 101})
	submit(t, svc, "T-1", Command{OperationID: "op-prime", Kind: CommandPrime, LogicalTime: 102})
	submit(t, svc, "T-1", Command{
		OperationID: "op-mix", Kind: CommandMix, LogicalTime: 103,
		MixerWindow: "W-1", BaseMg: 1000, CatalystMg: 100, PrimerMg: 50,
		TargetRatio: 100, OpenDeadline: 1000, LeaseExpiry: 500, LeaseTokens: mixTokens(),
	})

	callID := ratioCallID("T-1", 1)

	// Timeout then bad format then success; assert deterministic attempts.
	r1, err := svc.DeviceResult(callID, "timeout", "no response", 110)
	if err != nil {
		t.Fatalf("timeout result: %v", err)
	}
	if !r1.Retryable || r1.Attempts != 1 {
		t.Fatalf("timeout attempts=%d retryable=%v want 1/true", r1.Attempts, r1.Retryable)
	}

	r2, err := svc.DeviceResult(callID, "bad_format", "garbled", 120)
	if err != nil {
		t.Fatalf("bad_format result: %v", err)
	}
	if !r2.Retryable || r2.Attempts != 2 {
		t.Fatalf("bad_format attempts=%d want 2", r2.Attempts)
	}

	r3, err := svc.DeviceResult(callID, "success", "ratio ok", 130)
	if err != nil {
		t.Fatalf("success result: %v", err)
	}
	if r3.State != "success" {
		t.Fatalf("success state=%q want success", r3.State)
	}
}

func TestDeviceCalibrationExpired(t *testing.T) {
	svc := newTestService(t)
	if err := svc.RequestDevice("tensile-1", "req", false, 10); err != nil {
		t.Fatalf("request device: %v", err)
	}
	_, err := svc.DeviceResult("tensile-1", "success", "ok", 20)
	if err == nil || err.Code != domain.CodeCalibrationExpired {
		t.Fatalf("calibration err=%v want CALIBRATION_EXPIRED", err)
	}
}

func TestDeviceRetryPersistsAcrossRestart(t *testing.T) {
	path := t.TempDir() + "/hoist.db"

	st, err := store.Open(path)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	svc := New(st)
	if derr := svc.RequestDevice("ratio-1", "req", true, 10); derr != nil {
		t.Fatalf("request device: %v", derr)
	}
	if _, derr := svc.DeviceResult("ratio-1", "timeout", "t/o", 20); derr != nil {
		t.Fatalf("timeout: %v", derr)
	}
	_ = st.Close()

	// Simulate a process restart: re-open the same database file.
	st2, err := store.Open(path)
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	defer st2.Close()
	svc2 := New(st2)
	call, found, derr := svc2.GetDeviceCall("ratio-1")
	if derr != nil {
		t.Fatalf("get device call after restart: %v", derr)
	}
	if !found || call.Attempts != 1 || call.ResponseState != domain.DeviceTimeout {
		t.Fatalf("persisted call after restart=%+v", call)
	}
}
