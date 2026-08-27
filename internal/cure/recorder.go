package cure

import (
	"sync"

	"unitized-curtainwall-silicone-hoist-gate/internal/domain"
)

// MaxRetries is the deterministic retry ceiling for a device call. A call that
// reaches this ceiling resolves to a failure and must never produce a business
// reading, advance a prefix or release a cure/test barrier.
const MaxRetries = 3

// RetryAdvance advances the deterministic retry counter of a device call. It
// returns the next attempt number (1-based), the next logical retry time and
// whether the ceiling has been reached. A reached ceiling resolves to
// DeviceFailed.
func RetryAdvance(call domain.DeviceCall) (attempt int, nextAt domain.LogicalTime, failed bool) {
	attempt = call.Attempts + 1
	nextAt = NextRetryTime(domain.DeviceCall{Attempts: attempt, NextRetryAt: call.NextRetryAt})
	failed = attempt >= MaxRetries
	return attempt, nextAt, failed
}

// ValidateTestResult checks a fixed-point mechanical test outcome against the
// locked thresholds. It rejects a pending verdict, a sub-threshold tensile
// strength, an over-threshold elongation or bond-failure ratio, using integer
// comparison on the already-scaled values.
func ValidateTestResult(result domain.TestResult, th domain.Thresholds) *domain.Error {
	if result.Verdict == domain.VerdictPending {
		return domain.NewError(domain.CodeDependencyUnmet, false,
			domain.Reason{Message: "test verdict is still pending"})
	}
	if result.Verdict == domain.VerdictFail {
		return domain.NewError(domain.CodeDependencyUnmet, false,
			domain.Reason{Message: "test failed"})
	}
	if th.MinTensileMPa > 0 && result.TensileMPa < th.MinTensileMPa {
		return domain.NewError(domain.CodeDependencyUnmet, false,
			domain.Reason{Message: "tensile strength below threshold"})
	}
	if th.MaxElongationPct > 0 && result.ElongationPct > th.MaxElongationPct {
		return domain.NewError(domain.CodeDependencyUnmet, false,
			domain.Reason{Message: "elongation above threshold"})
	}
	if th.MaxBondFailurePct > 0 && result.BondFailurePct > th.MaxBondFailurePct {
		return domain.NewError(domain.CodeDependencyUnmet, false,
			domain.Reason{Message: "bond failure above threshold"})
	}
	return nil
}

// MemoryRecorder is an in-memory Recorder implementation used by unit tests and
// as a reference for the store-backed recorder. It stores cure trajectories and
// test results in memory only.
type MemoryRecorder struct {
	mu       sync.Mutex
	readings map[string][]domain.TempHumidity
	tests    map[string][]domain.TestResult
}

// NewMemoryRecorder returns an empty in-memory recorder.
func NewMemoryRecorder() *MemoryRecorder {
	return &MemoryRecorder{
		readings: make(map[string][]domain.TempHumidity),
		tests:    make(map[string][]domain.TestResult),
	}
}

// RecordCureReading appends a temperature/humidity reading, rejecting backward
// logical time relative to the last reading of the same sample.
func (m *MemoryRecorder) RecordCureReading(sampleID string, reading domain.TempHumidity) *domain.Error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if cur := m.readings[sampleID]; len(cur) > 0 && reading.At <= cur[len(cur)-1].At {
		return domain.NewError(domain.CodeLogicalTimeBackwards, false,
			domain.Reason{Message: "cure reading time went backwards"})
	}
	m.readings[sampleID] = append(m.readings[sampleID], reading)
	return nil
}

// RecordTestResult appends a fixed-point mechanical test outcome.
func (m *MemoryRecorder) RecordTestResult(result domain.TestResult) *domain.Error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tests[result.TestID] = append(m.tests[result.TestID], result)
	return nil
}

// RetryAdvance delegates to the package-level deterministic retry function.
func (m *MemoryRecorder) RetryAdvance(call domain.DeviceCall) (int, domain.LogicalTime, bool) {
	return RetryAdvance(call)
}

// Readings returns the recorded trajectory for a sample.
func (m *MemoryRecorder) Readings(sampleID string) []domain.TempHumidity {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]domain.TempHumidity, len(m.readings[sampleID]))
	copy(out, m.readings[sampleID])
	return out
}

// Tests returns the recorded tests for a test id.
func (m *MemoryRecorder) Tests(testID string) []domain.TestResult {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]domain.TestResult, len(m.tests[testID]))
	copy(out, m.tests[testID])
	return out
}
