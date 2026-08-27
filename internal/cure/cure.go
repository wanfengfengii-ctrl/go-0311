// Package cure implements the injection cure and test recorder: it validates
// open time, device calls, logical time and fixed-point metrics; records trial
// shots, injection, trimming, sealing, temperature/humidity trajectories,
// batch peel, hardness and tensile evidence; and manages deterministic retry
// counts.
package cure

import (
	"unitized-curtainwall-silicone-hoist-gate/internal/domain"
)

// EquivalentCureAccum computes the fixed-point equivalent cure accumulation
// from a temperature/humidity trajectory. The deterministic rate formula is:
// rate = (temperature * humidity) / (TemperatureScale), accumulated over the
// logical time delta between consecutive readings. It uses overflow-checked
// arithmetic throughout and returns false on overflow or a negative time gap.
func EquivalentCureAccum(readings []domain.TempHumidity) (int64, bool) {
	const tempScale = 1000
	var accum int64
	var prev *domain.TempHumidity
	for i := range readings {
		r := readings[i]
		if prev != nil {
			delta := int64(r.At - prev.At)
			if delta <= 0 {
				return 0, false
			}
			rate, ok := domain.Mul64(int64(r.Temperature), int64(r.Humidity))
			if !ok {
				return 0, false
			}
			rate, ok = domain.ScaleRoundHalfUp(rate, 1, tempScale)
			if !ok {
				return 0, false
			}
			contrib, ok := domain.Mul64(rate, delta)
			if !ok {
				return 0, false
			}
			accum, ok = domain.Add64(accum, contrib)
			if !ok {
				return 0, false
			}
		}
		prev = &r
	}
	return accum, true
}

// Recorder validates and records cure and test evidence together with
// deterministic device-call retries.
type Recorder interface {
	// RecordCureReading appends a temperature/humidity reading to a cure
	// trajectory, rejecting backward logical time.
	RecordCureReading(sampleID string, reading domain.TempHumidity) *domain.Error

	// RecordTestResult records a fixed-point mechanical test outcome.
	RecordTestResult(result domain.TestResult) *domain.Error

	// RetryAdvance advances the deterministic retry counter of a device call,
	// returning the next attempt number and next logical retry time. A call at
	// its retry ceiling resolves to DeviceFailed and no business reading may be
	// produced.
	RetryAdvance(call domain.DeviceCall) (attempt int, nextAt domain.LogicalTime, failed bool)
}

// NextRetryTime computes the deterministic next retry logical time for a
// device call using a fixed back-off: 1, 2, 4, ... logical units.
func NextRetryTime(call domain.DeviceCall) domain.LogicalTime {
	if call.Attempts <= 0 {
		return call.NextRetryAt
	}
	shift := call.Attempts - 1
	if shift > 60 {
		shift = 60
	}
	return call.NextRetryAt + domain.LogicalTime(int64(1)<<shift)
}
