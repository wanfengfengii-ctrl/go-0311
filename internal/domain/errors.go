package domain

import (
	"fmt"
	"sort"
	"strings"
)

// ErrorCode is a stable machine-readable error code. HTTP status reflects only
// the protocol-layer category; the code carries the business meaning.
type ErrorCode string

const (
	CodeOK ErrorCode = "OK"

	// Locking boundary.
	CodeStaleCompatibility   ErrorCode = "STALE_COMPATIBILITY"
	CodeJointCoverageInvalid ErrorCode = "JOINT_COVERAGE_INVALID"
	CodeFixedPointOverflow   ErrorCode = "FIXED_POINT_OVERFLOW"
	CodeMaterialMismatch     ErrorCode = "MATERIAL_MISMATCH"

	// Material and lease boundary.
	CodeMaterialOverdraw  ErrorCode = "MATERIAL_OVERDRAW"
	CodeLeaseConflict     ErrorCode = "LEASE_CONFLICT"
	CodeMixerContaminated ErrorCode = "MIXER_CONTAMINATED"
	CodeLeaseExpired      ErrorCode = "LEASE_EXPIRED"

	// Construction state boundary.
	CodeDependencyUnmet     ErrorCode = "DEPENDENCY_UNMET"
	CodeOpenTimeExpired     ErrorCode = "OPEN_TIME_EXPIRED"
	CodeNoncontiguousPrefix ErrorCode = "NONCONTIGUOUS_PREFIX"
	CodeGenerationMismatch  ErrorCode = "GENERATION_MISMATCH"

	// Time and device boundary.
	CodeLogicalTimeBackwards ErrorCode = "LOGICAL_TIME_BACKWARDS"
	CodeCureGap              ErrorCode = "CURE_GAP"
	CodeCalibrationExpired   ErrorCode = "CALIBRATION_EXPIRED"

	// Rework boundary.
	CodeReworkGenerationConflict ErrorCode = "REWORK_GENERATION_CONFLICT"

	// Idempotency and terminal boundary.
	CodeIdempotencyConflict    ErrorCode = "IDEMPOTENCY_CONFLICT"
	CodeTerminalAlreadyDecided ErrorCode = "TERMINAL_ALREADY_DECIDED"

	// Generic.
	CodeInvalidArgument ErrorCode = "INVALID_ARGUMENT"
	CodeNotFound        ErrorCode = "NOT_FOUND"
	CodeInternal        ErrorCode = "INTERNAL"
)

// Reason is one sortable explanation line. Ordering follows building, facade
// zone, panel, joint, segment, material generation and rework generation.
type Reason struct {
	Building    string
	FacadeZone  string
	Panel       string
	Joint       string
	Segment     int64
	MaterialGen Generation
	ReworkGen   Generation
	Message     string
}

// sortKey renders a deterministic, comparable key for a reason.
func (r Reason) sortKey() string {
	return fmt.Sprintf("%s|%s|%s|%s|%020d|%020d|%020d|%s",
		r.Building, r.FacadeZone, r.Panel, r.Joint, r.Segment,
		int64(r.MaterialGen), int64(r.ReworkGen), r.Message)
}

// Error is a stable business error carrying a code, sorted reasons and a
// retryability hint.
type Error struct {
	Code      ErrorCode
	Reasons   []Reason
	Retryable bool
}

func (e *Error) Error() string {
	rs := make([]string, 0, len(e.Reasons))
	for _, r := range e.Reasons {
		rs = append(rs, r.Message)
	}
	return fmt.Sprintf("%s: %s", e.Code, strings.Join(rs, "; "))
}

// NewError builds an Error with reasons sorted by the documented stable order.
func NewError(code ErrorCode, retryable bool, reasons ...Reason) *Error {
	sort.SliceStable(reasons, func(i, j int) bool {
		return reasons[i].sortKey() < reasons[j].sortKey()
	})
	return &Error{Code: code, Reasons: reasons, Retryable: retryable}
}

// Reasonf builds a single reason from a formatted message.
func Reasonf(msg string, args ...interface{}) Reason {
	return Reason{Message: fmt.Sprintf(msg, args...)}
}
