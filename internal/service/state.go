package service

import (
	"encoding/json"

	"unitized-curtainwall-silicone-hoist-gate/internal/domain"
	"unitized-curtainwall-silicone-hoist-gate/internal/store"
)

// taskStage is the ordered milestone progression of a task, derived solely
// from the append-only event chain.
type taskStage int

const (
	stageLocked taskStage = iota
	stageCleaned
	stagePrimed
	stageMixed
	stageTrialed
	stageInjected
	stageTrimmed
	stageSealed
	stageCured
	stageTested
)

func (s taskStage) String() string {
	switch s {
	case stageCleaned:
		return "CLEANED"
	case stagePrimed:
		return "PRIMED"
	case stageMixed:
		return "MIXED"
	case stageTrialed:
		return "TRIALED"
	case stageInjected:
		return "INJECTED"
	case stageTrimmed:
		return "TRIMMED"
	case stageSealed:
		return "SEALED"
	case stageCured:
		return "CURED"
	case stageTested:
		return "TESTED"
	default:
		return "LOCKED"
	}
}

// segmentPayload is the canonical JSON body of a SegmentInjected event, used to
// rebuild the valid prefix without any in-memory state.
type segmentPayload struct {
	JointID      string `json:"joint_id"`
	SegmentSeq   int64  `json:"segment_seq"`
	SegmentStart int64  `json:"segment_start"`
	SegmentEnd   int64  `json:"segment_end"`
	Generation   int64  `json:"generation"`
}

// curePayload is the canonical JSON body of a CureReading event.
type curePayload struct {
	SampleID    string `json:"sample_id"`
	At          int64  `json:"at"`
	Temperature int64  `json:"temperature"`
	Humidity    int64  `json:"humidity"`
}

// testPayload is the canonical JSON body of a TestResult event.
type testPayload struct {
	TestID        string `json:"test_id"`
	Kind          int    `json:"kind"`
	Hardness      int64  `json:"hardness"`
	TensileMPa    int64  `json:"tensile_mpa"`
	ElongationPct int64  `json:"elongation_pct"`
	BondFailure   int64  `json:"bond_failure_pct"`
}

// taskProjection is the rebuildable current view of a task.
type taskProjection struct {
	generation domain.Generation
	stage      taskStage
}

// loadTaskProjection replays the task event chain and folds it into a
// taskProjection. It is the single source of truth for dependency checks.
func loadTaskProjection(tx *store.Tx, taskID string) (taskProjection, *domain.Error) {
	events, err := tx.ReplayEvents(taskAggregate(taskID))
	if err != nil {
		return taskProjection{}, txErr(err)
	}
	p := taskProjection{stage: stageLocked}
	for _, ev := range events {
		if ev.Generation > p.generation {
			p.generation = ev.Generation
		}
		switch ev.Type {
		case domain.EventCleaned:
			if p.stage < stageCleaned {
				p.stage = stageCleaned
			}
		case domain.EventPrimed:
			if p.stage < stagePrimed {
				p.stage = stagePrimed
			}
		case domain.EventMixed:
			if p.stage < stageMixed {
				p.stage = stageMixed
			}
		case domain.EventTrialShot:
			if p.stage < stageTrialed {
				p.stage = stageTrialed
			}
		case domain.EventSegmentInjected:
			if p.stage < stageInjected {
				p.stage = stageInjected
			}
		case domain.EventTrimmed:
			if p.stage < stageTrimmed {
				p.stage = stageTrimmed
			}
		case domain.EventSealed:
			if p.stage < stageSealed {
				p.stage = stageSealed
			}
		case domain.EventCureReading:
			if p.stage < stageCured {
				p.stage = stageCured
			}
		case domain.EventTestResult:
			if p.stage < stageTested {
				p.stage = stageTested
			}
		}
	}
	return p, nil
}

// jointPrefix rebuilds the valid continuous prefix of a joint from its
// SegmentInjected events. It returns the furthest segment end reached, the
// current generation and the number of segments injected.
func jointPrefix(tx *store.Tx, jointID string, gen domain.Generation) (domain.Microns, int, *domain.Error) {
	events, err := tx.ReplayEvents(jointAggregate(jointID))
	if err != nil {
		return 0, 0, txErr(err)
	}
	var prefix domain.Microns
	count := 0
	for _, ev := range events {
		if ev.Type != domain.EventSegmentInjected {
			continue
		}
		if gen > 0 && ev.Generation != gen {
			continue
		}
		var sp segmentPayload
		if err := json.Unmarshal([]byte(ev.Payload), &sp); err != nil {
			return 0, 0, domain.NewError(domain.CodeInternal, false, domain.Reason{Message: "bad segment payload"})
		}
		if domain.Microns(sp.SegmentEnd) > prefix {
			prefix = domain.Microns(sp.SegmentEnd)
		}
		count++
	}
	return prefix, count, nil
}

// jointsClosed reports whether every joint of the task has had its full design
// boundary covered by an injected prefix under its latest generation.
func jointsClosed(tx *store.Tx, taskID string) (bool, *domain.Error) {
	joints, err := tx.ListJoints(taskID)
	if err != nil {
		return false, txErr(err)
	}
	for _, j := range joints {
		prefix, _, derr := jointMaxPrefix(tx, j.JointID)
		if derr != nil {
			return false, derr
		}
		if prefix < j.End {
			return false, nil
		}
	}
	return true, nil
}

// jointMaxPrefix returns the furthest prefix reached under the latest injected
// generation and that generation number.
func jointMaxPrefix(tx *store.Tx, jointID string) (domain.Microns, domain.Generation, *domain.Error) {
	events, err := tx.ReplayEvents(jointAggregate(jointID))
	if err != nil {
		return 0, 0, txErr(err)
	}
	var maxGen domain.Generation
	for _, ev := range events {
		if ev.Type == domain.EventSegmentInjected && ev.Generation > maxGen {
			maxGen = ev.Generation
		}
	}
	prefix, _, derr := jointPrefix(tx, jointID, maxGen)
	if derr != nil {
		return 0, 0, derr
	}
	return prefix, maxGen, nil
}
