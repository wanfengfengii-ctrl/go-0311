// Package domain holds the stable domain types shared across the
// unitized curtain-wall silicone hoist-gate backend. These types mirror the
// approved data model and must remain free of persistence or transport
// concerns.
package domain

// LogicalTime is a monotonically increasing, integer logical clock used to
// order all business evidence. It is measured in logical microseconds.
type LogicalTime int64

// Microns is a signed 64-bit integer micro-meter measure used for all joint
// and segment geometry.
type Microns int64

// Milligrams is a signed 64-bit integer milligram measure used for all mass
// accounting.
type Milligrams int64

// Component identifies one component of the two-part structural silicone and
// its primer. Base and catalyst components may never offset each other.
type Component int

const (
	ComponentBase Component = iota + 1
	ComponentCatalyst
	ComponentPrimer
)

func (c Component) String() string {
	switch c {
	case ComponentBase:
		return "base"
	case ComponentCatalyst:
		return "catalyst"
	case ComponentPrimer:
		return "primer"
	default:
		return "unknown"
	}
}

// MassDirection records whether a mass entry is an input (stock) or an
// output (consumed, sampled, cut out or lost).
type MassDirection int

const (
	MassInput MassDirection = iota + 1
	MassOutput
)

// MassCategory is the documented source or destination of a mass entry.
type MassCategory int

const (
	MassStock MassCategory = iota + 1
	MassTrialShot
	MassInJoint
	MassSample
	MassCutout
	MassRemainder
	MassLoss
)

// ResourceType enumerates the mutually exclusive, time-limited resources that
// may be leased.
type ResourceType int

const (
	ResourceMeteringPump ResourceType = iota + 1
	ResourceRatioMonitor
	ResourceInjectionTable
	ResourceMixer
	ResourceCureRack
	ResourceTensileMachine
	ResourceCutoutStation
)

func (r ResourceType) String() string {
	switch r {
	case ResourceMeteringPump:
		return "metering_pump"
	case ResourceRatioMonitor:
		return "ratio_monitor"
	case ResourceInjectionTable:
		return "injection_table"
	case ResourceMixer:
		return "mixer"
	case ResourceCureRack:
		return "cure_rack"
	case ResourceTensileMachine:
		return "tensile_machine"
	case ResourceCutoutStation:
		return "cutout_station"
	default:
		return "unknown"
	}
}

// Generation is an immutable task or material generation counter.
type Generation int64

// DesignLock is the immutable snapshot produced once a task is locked. After
// locking, none of its referenced collections may be modified in place; a
// change requires a new generation.
type DesignLock struct {
	TaskID           string
	Generation       Generation
	Building         string
	FacadeZone       string
	Panel            string
	DesignVersion    string
	CompatibilityVer string
	CompatValidUntil LogicalTime
	SurfaceSummary   string
	Thresholds       Thresholds
	LockedAt         LogicalTime
}

// Thresholds is the strength and geometry threshold snapshot recorded at lock
// time.
type Thresholds struct {
	MinTensileMPa        int64 `json:"min_tensile_mpa"`
	MaxElongationPct     int64 `json:"max_elongation_pct"`
	MaxBondFailurePct    int64 `json:"max_bond_failure_pct"`
	MinFillRatePermil    int64 `json:"min_fill_rate_permil"`
	MaxAspectRatioPermil int64 `json:"max_aspect_ratio_permil"`
}

// JointSpec describes one sealant joint with its continuous segments.
type JointSpec struct {
	JointID         string            `json:"joint_id"`
	Direction       string            `json:"direction"`
	Start           Microns           `json:"start"`
	End             Microns           `json:"end"`
	Width           Microns           `json:"width"`
	Depth           Microns           `json:"depth"`
	CornerAngle     int64             `json:"corner_angle"`
	SpacerIntervals []Interval        `json:"spacer_intervals"`
	BondAreaUm2     int64             `json:"bond_area_um2"`
	Segments        []SegmentSpec     `json:"segments"`
	TrialMapping    map[string]string `json:"trial_mapping"`
}

// Interval is a half-open [Start, End) integer micro-meter interval used for
// spacer placement and design boundaries.
type Interval struct {
	Start Microns `json:"start"`
	End   Microns `json:"end"`
}

// SegmentSpec is one contiguous segment of a joint. Segments must tile the
// joint boundary exactly, in order, with no gaps or overlap.
type SegmentSpec struct {
	Seq   int64   `json:"seq"`
	Start Microns `json:"start"`
	End   Microns `json:"end"`
}

// MaterialBatch identifies physical stock of base, catalyst and primer.
type MaterialBatch struct {
	BaseBatch     string `json:"base_batch"`
	CatalystBatch string `json:"catalyst_batch"`
	PrimerBatch   string `json:"primer_batch"`
}

// MaterialGeneration is a single mixed-material window bound to a mixer.
type MaterialGeneration struct {
	Generation   Generation
	Batch        MaterialBatch
	MixerWindow  string
	TargetRatio  int64 // base:Catalyst, scaled by RatioScale
	OpenDeadline LogicalTime
	Status       GenerationStatus
}

// GenerationStatus is the lifecycle state of a material generation.
type GenerationStatus int

const (
	GenerationOpen GenerationStatus = iota + 1
	GenerationClosed
	GenerationSuperseded
)

// MassEntry is a single double-entry ledger row for one component.
type MassEntry struct {
	Seq        int64
	Generation Generation
	Component  Component
	Direction  MassDirection
	Category   MassCategory
	Amount     Milligrams
	Evidence   string
}

// EventType enumerates the append-only evidence event kinds.
type EventType int

const (
	EventTaskLocked EventType = iota + 1
	EventCleaned
	EventPrimed
	EventMixed
	EventTrialShot
	EventSegmentInjected
	EventTrimmed
	EventSealed
	EventCureReading
	EventTestResult
	EventReworkCutout
	EventReworkReinject
	EventReviewed
	EventTerminalDecided
)

// EvidenceEvent is a single monotonic append-only record. Payload carries the
// canonical JSON body of the command or observation so projections can be
// rebuilt without any in-memory state; PayloadHash is the tamper-evident digest
// of that payload.
type EvidenceEvent struct {
	Seq         int64
	AggregateID string
	Generation  Generation
	Type        EventType
	Payload     string
	PayloadHash string
	LogicalTime LogicalTime
	WrittenAt   LogicalTime
	PrevHash    string
}

// JointProjection is the rebuildable current view of one joint, maintained
// solely from its event chain.
type JointProjection struct {
	JointID        string
	Generation     Generation
	Stage          Stage
	ValidPrefixEnd Microns
	CurrentGen     Generation
}

// Stage is the ordered dependency stage of a joint.
type Stage int

const (
	StageLocked Stage = iota + 1
	StageCleaned
	StagePrimed
	StageInjected
	StageSealed
	StageCured
	StageTested
)

// ResourceLease is a single exclusive, time-limited hold on a resource.
type ResourceLease struct {
	ResourceType ResourceType
	ResourceID   string
	Token        string
	HolderOp     string
	AcquiredAt   LogicalTime
	ExpiresAt    LogicalTime
}

// DeviceCall is a scripted device invocation and its deterministic retry
// state.
type DeviceCall struct {
	CallID        string
	RequestHash   string
	Calibrated    bool
	Attempts      int
	NextRetryAt   LogicalTime
	ResponseState DeviceResponse
	RawSummary    string
}

// DeviceResponse records the resolved state of a device call.
type DeviceResponse int

const (
	DevicePending DeviceResponse = iota + 1
	DeviceSuccess
	DeviceRejected
	DeviceTimeout
	DeviceDisconnected
	DeviceBadFormat
	DeviceFailed
)

// CureSample is the cure trajectory plus a fixed-point accumulated equivalent
// cure value.
type CureSample struct {
	CureRack       string
	Readings       []TempHumidity
	EquivCureAccum int64
	SampleID       string
	SampleMass     Milligrams
}

// TempHumidity is one probe reading at a logical time.
type TempHumidity struct {
	At          LogicalTime
	Temperature int64
	Humidity    int64
}

// TestResult is one fixed-point mechanical test outcome.
type TestResult struct {
	TestID         string
	Kind           TestKind
	Hardness       int64
	TensileMPa     int64
	ElongationPct  int64
	BondFailurePct int64
	Verdict        TestVerdict
}

// TestKind enumerates the documented tests.
type TestKind int

const (
	TestButterfly TestKind = iota + 1
	TestSnap
	TestPeel
	TestHardness
	TestTensile
)

// TestVerdict is the pass/fail conclusion of a test.
type TestVerdict int

const (
	VerdictPending TestVerdict = iota + 1
	VerdictPass
	VerdictFail
)

// ReworkCase is a single deterministically derived rework/reinjection set.
type ReworkCase struct {
	CaseID        string
	TaskID        string
	Category      string
	RootEvidence  string
	ImpactSummary string
	Affected      []string
	CutoutMass    Milligrams
	CutoutDest    string
	NewGeneration Generation
	ReinjectGen   Generation
	Closed        bool
}

// Review is one independent qualified review of a task.
type Review struct {
	ReviewerID   string      `json:"reviewer_id"`
	QualSnapshot string      `json:"qual_snapshot"`
	Summary      string      `json:"summary"`
	ReviewedAt   LogicalTime `json:"reviewed_at"`
}

// TerminalType is the single-writer terminal decision kind.
type TerminalType int

const (
	TerminalHoistAdmitted TerminalType = iota + 1
	TerminalBondRiskIsolated
	TerminalCancelled
)

func (t TerminalType) String() string {
	switch t {
	case TerminalHoistAdmitted:
		return "HOIST_ADMITTED"
	case TerminalBondRiskIsolated:
		return "BOND_RISK_ISOLATED"
	case TerminalCancelled:
		return "CANCELLED"
	default:
		return "UNKNOWN"
	}
}

// TerminalDecision is the unique final verdict for one task generation.
type TerminalDecision struct {
	Type         TerminalType `json:"type"`
	Credential   string       `json:"credential"`
	EvidenceHash string       `json:"evidence_hash"`
	BarrierVer   int64        `json:"barrier_ver"`
	DecidedAt    LogicalTime  `json:"decided_at"`
}

// IdempotencyRecord caches a canonical response keyed by operation id.
type IdempotencyRecord struct {
	OperationID string
	Endpoint    string
	RequestHash string
	Response    string
	EventRange  string
}

// TransactionJournal records prepared/committed batches for crash recovery.
type TransactionJournal struct {
	TxnID     string
	Prepared  bool
	Committed bool
	EventFrom int64
	EventTo   int64
}
