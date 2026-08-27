package service

import (
	"unitized-curtainwall-silicone-hoist-gate/internal/cure"
	"unitized-curtainwall-silicone-hoist-gate/internal/domain"
	"unitized-curtainwall-silicone-hoist-gate/internal/store"
)

// DeviceResult is the outcome of a scripted device receipt.
type DeviceResult struct {
	CallID      string             `json:"call_id"`
	State       string             `json:"state"`
	Attempts    int                `json:"attempts"`
	NextRetryAt domain.LogicalTime `json:"next_retry_at,omitempty"`
	Retryable   bool               `json:"retryable"`
}

// RequestDevice creates a pending device call, used by mix (ratio monitor) and
// by the test flow (tensile machine). It persists the call so its retry state
// survives a restart.
func (s *Service) RequestDevice(callID, requestHash string, calibrated bool, at domain.LogicalTime) *domain.Error {
	return s.runTx(func(tx *store.Tx) *domain.Error {
		if _, found, err := tx.GetDeviceCall(callID); err != nil {
			return txErr(err)
		} else if found {
			return nil // idempotent creation
		}
		if err := tx.SaveDeviceCall(domain.DeviceCall{
			CallID: callID, RequestHash: requestHash, Calibrated: calibrated,
			Attempts: 0, NextRetryAt: at, ResponseState: domain.DevicePending,
		}); err != nil {
			return txErr(err)
		}
		return nil
	})
}

// DeviceResult processes a scripted device receipt. Timeouts, disconnections
// and format errors only advance the deterministic retry state and never
// produce a business reading; an uncalibrated device is rejected outright.
func (s *Service) DeviceResult(callID string, state string, rawSummary string, at domain.LogicalTime) (DeviceResult, *domain.Error) {
	var out DeviceResult
	derr := s.runTx(func(tx *store.Tx) *domain.Error {
		call, found, err := tx.GetDeviceCall(callID)
		if err != nil {
			return txErr(err)
		}
		if !found {
			return domain.NewError(domain.CodeNotFound, false, domain.Reason{Message: "device call not found"})
		}
		if !call.Calibrated {
			return domain.NewError(domain.CodeCalibrationExpired, false,
				domain.Reason{Message: "device calibration expired"})
		}
		switch state {
		case "success":
			call.ResponseState = domain.DeviceSuccess
		case "rejected":
			call.ResponseState = domain.DeviceRejected
		case "timeout":
			call.ResponseState = domain.DeviceTimeout
		case "disconnected":
			call.ResponseState = domain.DeviceDisconnected
		case "bad_format":
			call.ResponseState = domain.DeviceBadFormat
		default:
			return domain.NewError(domain.CodeInvalidArgument, false,
				domain.Reason{Message: "unknown device state"})
		}
		call.RawSummary = rawSummary
		out.CallID = call.CallID
		out.State = state
		if call.ResponseState == domain.DeviceTimeout ||
			call.ResponseState == domain.DeviceDisconnected ||
			call.ResponseState == domain.DeviceBadFormat {
			// Deterministic retry: increment attempts and schedule the next retry.
			attempt, nextAt, failed := cure.RetryAdvance(call)
			call.Attempts = attempt
			call.NextRetryAt = nextAt
			out.Attempts = attempt
			out.NextRetryAt = nextAt
			out.Retryable = !failed
			if failed {
				call.ResponseState = domain.DeviceFailed
				out.State = "failed"
				out.Retryable = false
			}
		} else {
			out.Attempts = call.Attempts
		}
		if err := tx.UpdateDeviceCall(call); err != nil {
			return txErr(err)
		}
		return nil
	})
	if derr != nil {
		return DeviceResult{}, derr
	}
	return out, nil
}

// DeviceRetry advances the deterministic retry counter of a pending device
// call and returns the next attempt number and retry time.
func (s *Service) DeviceRetry(callID string) (DeviceResult, *domain.Error) {
	var out DeviceResult
	derr := s.runTx(func(tx *store.Tx) *domain.Error {
		call, found, err := tx.GetDeviceCall(callID)
		if err != nil {
			return txErr(err)
		}
		if !found {
			return domain.NewError(domain.CodeNotFound, false, domain.Reason{Message: "device call not found"})
		}
		attempt, nextAt, failed := cure.RetryAdvance(call)
		call.Attempts = attempt
		call.NextRetryAt = nextAt
		if failed {
			call.ResponseState = domain.DeviceFailed
		}
		if err := tx.UpdateDeviceCall(call); err != nil {
			return txErr(err)
		}
		out = DeviceResult{CallID: callID, Attempts: attempt, NextRetryAt: nextAt, Retryable: !failed}
		if failed {
			out.State = "failed"
		}
		return nil
	})
	if derr != nil {
		return DeviceResult{}, derr
	}
	return out, nil
}

// GetDeviceCall exposes the persisted device call state for queries and tests.
func (s *Service) GetDeviceCall(callID string) (domain.DeviceCall, bool, *domain.Error) {
	var call domain.DeviceCall
	var found bool
	derr := s.runTx(func(tx *store.Tx) *domain.Error {
		c, ok, err := tx.GetDeviceCall(callID)
		if err != nil {
			return txErr(err)
		}
		call, found = c, ok
		return nil
	})
	if derr != nil {
		return domain.DeviceCall{}, false, derr
	}
	return call, found, nil
}
