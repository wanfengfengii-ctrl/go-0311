package service

import (
	"encoding/json"
	"strconv"

	"unitized-curtainwall-silicone-hoist-gate/internal/domain"
	"unitized-curtainwall-silicone-hoist-gate/internal/store"
)

// ratioCallID derives the deterministic ratio-monitor device call id for a task
// generation.
func ratioCallID(taskID string, gen domain.Generation) string {
	return "ratio:" + taskID + ":" + strconv.FormatInt(int64(gen), 10)
}

// CommandKind enumerates the unified command kinds accepted by the command
// endpoint.
type CommandKind string

const (
	CommandClean         CommandKind = "clean"
	CommandPrime         CommandKind = "prime"
	CommandMix           CommandKind = "mix"
	CommandTrialShot     CommandKind = "trial_shot"
	CommandInjectSegment CommandKind = "inject_segment"
	CommandTrim          CommandKind = "trim"
	CommandSeal          CommandKind = "seal"
	CommandCureReading   CommandKind = "cure_reading"
	CommandTestResult    CommandKind = "test_result"
)

// Command is the unified command payload submitted to POST
// /v1/tasks/{task}/commands. It carries the operation id, the expected task
// generation, the logical time and the command-specific fields for the
// documented construction steps.
type Command struct {
	OperationID string             `json:"operation_id"`
	Kind        CommandKind        `json:"kind"`
	ExpectedGen domain.Generation  `json:"expected_generation"`
	LogicalTime domain.LogicalTime `json:"logical_time"`

	// Mix fields.
	MixerWindow  string             `json:"mixer_window,omitempty"`
	BaseMg       domain.Milligrams  `json:"base_mg,omitempty"`
	CatalystMg   domain.Milligrams  `json:"catalyst_mg,omitempty"`
	PrimerMg     domain.Milligrams  `json:"primer_mg,omitempty"`
	TargetRatio  int64              `json:"target_ratio,omitempty"`
	OpenDeadline domain.LogicalTime `json:"open_deadline,omitempty"`

	// Lease tokens: resource-type string -> token.
	LeaseTokens map[string]string  `json:"lease_tokens,omitempty"`
	LeaseExpiry domain.LogicalTime `json:"lease_expiry,omitempty"`

	// Injection fields.
	JointID    string `json:"joint_id,omitempty"`
	SegmentSeq int64  `json:"segment_seq,omitempty"`

	// Cure reading fields.
	SampleID    string `json:"sample_id,omitempty"`
	Temperature int64  `json:"temperature,omitempty"`
	Humidity    int64  `json:"humidity,omitempty"`

	// Test result fields.
	TestID         string          `json:"test_id,omitempty"`
	TestKind       domain.TestKind `json:"test_kind,omitempty"`
	Hardness       int64           `json:"hardness,omitempty"`
	TensileMPa     int64           `json:"tensile_mpa,omitempty"`
	ElongationPct  int64           `json:"elongation_pct,omitempty"`
	BondFailurePct int64           `json:"bond_failure_pct,omitempty"`
}

// CommandResult is the deterministic response to a command submission.
type CommandResult struct {
	OperationID string             `json:"operation_id"`
	Kind        CommandKind        `json:"kind"`
	TaskID      string             `json:"task_id"`
	Generation  domain.Generation  `json:"generation"`
	LogicalTime domain.LogicalTime `json:"logical_time"`
	Stage       string             `json:"stage"`
	JointID     string             `json:"joint_id,omitempty"`
	SegmentSeq  int64              `json:"segment_seq,omitempty"`
	PrefixEnd   domain.Microns     `json:"prefix_end_um,omitempty"`
}

// canonical returns the canonical JSON form of the command used for idempotency
// hashing and conflict detection.
func (c Command) canonical() (string, error) {
	b, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// SubmitCommand dispatches a unified command against a locked task, enforcing
// dependency order, generation match, lease validity, open-time limits and
// idempotency inside a single transaction.
func (s *Service) SubmitCommand(taskID string, cmd Command) (CommandResult, *domain.Error) {
	canon, err := cmd.canonical()
	if err != nil {
		return CommandResult{}, domain.NewError(domain.CodeInvalidArgument, false,
			domain.Reason{Message: "bad command payload"})
	}
	hash := domain.CanonicalHash(canon)

	var result CommandResult
	derr := s.runTx(func(tx *store.Tx) *domain.Error {
		// Idempotency: same operation id + same content returns the original
		// result; different content conflicts.
		if rec, found, err := tx.GetIdempotency(cmd.OperationID); err != nil {
			return txErr(err)
		} else if found {
			if rec.RequestHash != hash {
				return domain.NewError(domain.CodeIdempotencyConflict, false,
					domain.Reason{Message: "operation id reused with different content"})
			}
			if err := json.Unmarshal([]byte(rec.Response), &result); err != nil {
				return domain.NewError(domain.CodeInternal, false, domain.Reason{Message: "bad cached response"})
			}
			return nil
		}

		lock, ok, err := tx.GetLock(taskID)
		if err != nil {
			return txErr(err)
		}
		if !ok {
			return domain.NewError(domain.CodeNotFound, false, domain.Reason{Message: "task not locked"})
		}
		if cmd.ExpectedGen != 0 && cmd.ExpectedGen != lock.Generation {
			return domain.NewError(domain.CodeGenerationMismatch, false,
				domain.Reason{Message: "expected generation does not match lock"})
		}

		proj, derr := loadTaskProjection(tx, taskID)
		if derr != nil {
			return derr
		}
		if cmd.LogicalTime <= lock.LockedAt {
			return domain.NewError(domain.CodeLogicalTimeBackwards, false,
				domain.Reason{Message: "logical time must advance past lock"})
		}

		switch cmd.Kind {
		case CommandClean:
			derr = s.doClean(tx, taskID, lock, proj, cmd, &result)
		case CommandPrime:
			derr = s.doPrime(tx, taskID, lock, proj, cmd, &result)
		case CommandMix:
			derr = s.doMix(tx, taskID, lock, proj, cmd, &result)
		case CommandTrialShot:
			derr = s.doTrialShot(tx, taskID, lock, proj, cmd, &result)
		case CommandInjectSegment:
			derr = s.doInjectSegment(tx, taskID, lock, proj, cmd, &result)
		case CommandTrim:
			derr = s.doTrim(tx, taskID, lock, proj, cmd, &result)
		case CommandSeal:
			derr = s.doSeal(tx, taskID, lock, proj, cmd, &result)
		case CommandCureReading:
			derr = s.doCureReading(tx, taskID, lock, proj, cmd, &result)
		case CommandTestResult:
			derr = s.doTestResult(tx, taskID, lock, proj, cmd, &result)
		default:
			derr = domain.NewError(domain.CodeInvalidArgument, false,
				domain.Reason{Message: "unknown command kind"})
		}
		if derr != nil {
			return derr
		}

		// Persist the idempotency record with the canonical response.
		respJSON, err := json.Marshal(result)
		if err != nil {
			return domain.NewError(domain.CodeInternal, false, domain.Reason{Message: "encode result"})
		}
		if err := tx.SaveIdempotency(domain.IdempotencyRecord{
			OperationID: cmd.OperationID,
			Endpoint:    "commands",
			RequestHash: hash,
			Response:    string(respJSON),
		}); err != nil {
			return txErr(err)
		}
		return nil
	})
	if derr != nil {
		return CommandResult{}, derr
	}
	return result, nil
}

func (s *Service) doClean(tx *store.Tx, taskID string, lock domain.DesignLock, proj taskProjection, cmd Command, result *CommandResult) *domain.Error {
	if proj.stage >= stageCleaned {
		return nil
	}
	_, _, err := tx.AppendEvent(domain.EvidenceEvent{
		AggregateID: taskAggregate(taskID), Generation: lock.Generation, Type: domain.EventCleaned,
		Payload: "{}", LogicalTime: cmd.LogicalTime,
	})
	if err != nil {
		return txErr(err)
	}
	*result = CommandResult{OperationID: cmd.OperationID, Kind: cmd.Kind, TaskID: taskID,
		Generation: lock.Generation, LogicalTime: cmd.LogicalTime, Stage: "CLEANED"}
	return nil
}

func (s *Service) doPrime(tx *store.Tx, taskID string, lock domain.DesignLock, proj taskProjection, cmd Command, result *CommandResult) *domain.Error {
	if proj.stage < stageCleaned {
		return domain.NewError(domain.CodeDependencyUnmet, false,
			domain.Reason{Message: "clean before prime"})
	}
	if proj.stage >= stagePrimed {
		return nil
	}
	_, _, err := tx.AppendEvent(domain.EvidenceEvent{
		AggregateID: taskAggregate(taskID), Generation: lock.Generation, Type: domain.EventPrimed,
		Payload: "{}", LogicalTime: cmd.LogicalTime,
	})
	if err != nil {
		return txErr(err)
	}
	*result = CommandResult{OperationID: cmd.OperationID, Kind: cmd.Kind, TaskID: taskID,
		Generation: lock.Generation, LogicalTime: cmd.LogicalTime, Stage: "PRIMED"}
	return nil
}

func (s *Service) doMix(tx *store.Tx, taskID string, lock domain.DesignLock, proj taskProjection, cmd Command, result *CommandResult) *domain.Error {
	if proj.stage < stagePrimed {
		return domain.NewError(domain.CodeDependencyUnmet, false,
			domain.Reason{Message: "prime before mix"})
	}
	if cmd.OpenDeadline <= cmd.LogicalTime {
		return domain.NewError(domain.CodeOpenTimeExpired, false,
			domain.Reason{Message: "open deadline already passed"})
	}
	if cmd.MixerWindow == "" || cmd.BaseMg <= 0 || cmd.CatalystMg <= 0 || cmd.PrimerMg < 0 {
		return domain.NewError(domain.CodeInvalidArgument, false,
			domain.Reason{Message: "mixer window, base and catalyst mass required"})
	}
	gen := lock.Generation

	// Atomically acquire the mixer, metering pump and injection table leases.
	leases := []domain.ResourceLease{
		{ResourceType: domain.ResourceMixer, ResourceID: "mixer", Token: cmd.LeaseTokens["mixer"],
			HolderOp: cmd.OperationID, AcquiredAt: cmd.LogicalTime, ExpiresAt: cmd.LeaseExpiry},
		{ResourceType: domain.ResourceMeteringPump, ResourceID: "pump", Token: cmd.LeaseTokens["metering_pump"],
			HolderOp: cmd.OperationID, AcquiredAt: cmd.LogicalTime, ExpiresAt: cmd.LeaseExpiry},
		{ResourceType: domain.ResourceInjectionTable, ResourceID: "table", Token: cmd.LeaseTokens["injection_table"],
			HolderOp: cmd.OperationID, AcquiredAt: cmd.LogicalTime, ExpiresAt: cmd.LeaseExpiry},
	}
	for _, l := range leases {
		if l.Token == "" {
			return domain.NewError(domain.CodeLeaseConflict, false,
				domain.Reason{Message: "missing lease token"})
		}
		if err := tx.AcquireLease(l); err != nil {
			return txErr(err)
		}
	}

	// Open the two-component stock as input to the material generation.
	inputs := []domain.MassEntry{
		{Generation: gen, Component: domain.ComponentBase, Direction: domain.MassInput, Category: domain.MassStock, Amount: cmd.BaseMg, Evidence: cmd.OperationID},
		{Generation: gen, Component: domain.ComponentCatalyst, Direction: domain.MassInput, Category: domain.MassStock, Amount: cmd.CatalystMg, Evidence: cmd.OperationID},
		{Generation: gen, Component: domain.ComponentPrimer, Direction: domain.MassInput, Category: domain.MassStock, Amount: cmd.PrimerMg, Evidence: cmd.OperationID},
	}
	for _, e := range inputs {
		if err := tx.PostMass(e); err != nil {
			return txErr(err)
		}
	}

	payload, _ := json.Marshal(map[string]interface{}{
		"mixer_window": cmd.MixerWindow, "target_ratio": cmd.TargetRatio, "open_deadline": cmd.OpenDeadline,
	})
	_, _, err := tx.AppendEvent(domain.EvidenceEvent{
		AggregateID: taskAggregate(taskID), Generation: gen, Type: domain.EventMixed,
		Payload: string(payload), LogicalTime: cmd.LogicalTime,
	})
	if err != nil {
		return txErr(err)
	}
	// Create the ratio-monitor device call that verifies the mixed ratio. Its
	// retry state persists so a restart can resume the same deterministic retry.
	ratioCallID := ratioCallID(taskID, gen)
	if err := tx.SaveDeviceCall(domain.DeviceCall{
		CallID: ratioCallID, RequestHash: domain.CanonicalHash(cmd.MixerWindow + ":" + strconv.FormatInt(cmd.TargetRatio, 10)),
		Calibrated: true, Attempts: 0, NextRetryAt: cmd.LogicalTime, ResponseState: domain.DevicePending,
	}); err != nil {
		return txErr(err)
	}
	*result = CommandResult{OperationID: cmd.OperationID, Kind: cmd.Kind, TaskID: taskID,
		Generation: gen, LogicalTime: cmd.LogicalTime, Stage: "MIXED"}
	return nil
}

func (s *Service) doTrialShot(tx *store.Tx, taskID string, lock domain.DesignLock, proj taskProjection, cmd Command, result *CommandResult) *domain.Error {
	if proj.stage < stageMixed {
		return domain.NewError(domain.CodeDependencyUnmet, false,
			domain.Reason{Message: "mix before trial shot"})
	}
	gen := lock.Generation
	// Deduct the trial-shot mass carried by the command from each component.
	trial := []domain.MassEntry{
		{Generation: gen, Component: domain.ComponentBase, Direction: domain.MassOutput, Category: domain.MassTrialShot, Amount: cmd.BaseMg, Evidence: cmd.OperationID},
		{Generation: gen, Component: domain.ComponentCatalyst, Direction: domain.MassOutput, Category: domain.MassTrialShot, Amount: cmd.CatalystMg, Evidence: cmd.OperationID},
	}
	for _, e := range trial {
		if err := tx.PostMass(e); err != nil {
			return txErr(err)
		}
	}
	_, _, err := tx.AppendEvent(domain.EvidenceEvent{
		AggregateID: taskAggregate(taskID), Generation: gen, Type: domain.EventTrialShot,
		Payload: "{}", LogicalTime: cmd.LogicalTime,
	})
	if err != nil {
		return txErr(err)
	}
	*result = CommandResult{OperationID: cmd.OperationID, Kind: cmd.Kind, TaskID: taskID,
		Generation: gen, LogicalTime: cmd.LogicalTime, Stage: "TRIALED"}
	return nil
}

func (s *Service) doInjectSegment(tx *store.Tx, taskID string, lock domain.DesignLock, proj taskProjection, cmd Command, result *CommandResult) *domain.Error {
	if proj.stage < stageTrialed {
		return domain.NewError(domain.CodeDependencyUnmet, false,
			domain.Reason{Message: "trial shot before injection"})
	}
	if cmd.JointID == "" {
		return domain.NewError(domain.CodeInvalidArgument, false,
			domain.Reason{Message: "joint id required"})
	}
	joint, ok, err := tx.GetJoint(taskID, cmd.JointID)
	if err != nil {
		return txErr(err)
	}
	if !ok {
		return domain.NewError(domain.CodeNotFound, false,
			domain.Reason{Message: "joint not found"})
	}
	gen := lock.Generation

	// Validate the mixer lease is still held under the presented token.
	if err := checkLease(tx, domain.ResourceMixer, "mixer", cmd.LeaseTokens["mixer"], cmd.LogicalTime); err != nil {
		return err
	}

	prefix, count, derr := jointPrefix(tx, cmd.JointID, gen)
	if derr != nil {
		return derr
	}
	// Find the segment being injected.
	var seg *domain.SegmentSpec
	for i := range joint.Segments {
		if joint.Segments[i].Seq == cmd.SegmentSeq {
			seg = &joint.Segments[i]
			break
		}
	}
	if seg == nil {
		return domain.NewError(domain.CodeNoncontiguousPrefix, false,
			domain.Reason{Joint: cmd.JointID, Segment: cmd.SegmentSeq, Message: "unknown segment"})
	}
	// The segment must be the immediate next one after the current prefix.
	if seg.Start != prefix {
		if seg.Start < prefix {
			return domain.NewError(domain.CodeNoncontiguousPrefix, false,
				domain.Reason{Joint: cmd.JointID, Segment: cmd.SegmentSeq, Message: "segment already covered or reversed"})
		}
		return domain.NewError(domain.CodeNoncontiguousPrefix, false,
			domain.Reason{Joint: cmd.JointID, Segment: cmd.SegmentSeq, Message: "segment skipped ahead of prefix"})
	}

	// Deduct the in-joint mass carried by the command.
	if cmd.BaseMg > 0 || cmd.CatalystMg > 0 {
		for _, e := range []domain.MassEntry{
			{Generation: gen, Component: domain.ComponentBase, Direction: domain.MassOutput, Category: domain.MassInJoint, Amount: cmd.BaseMg, Evidence: cmd.OperationID},
			{Generation: gen, Component: domain.ComponentCatalyst, Direction: domain.MassOutput, Category: domain.MassInJoint, Amount: cmd.CatalystMg, Evidence: cmd.OperationID},
		} {
			_ = tx.PostMass(e)
		}
	}

	payload, _ := json.Marshal(segmentPayload{
		JointID: cmd.JointID, SegmentSeq: cmd.SegmentSeq,
		SegmentStart: int64(seg.Start), SegmentEnd: int64(seg.End), Generation: int64(gen),
	})
	_, _, err = tx.AppendEvent(domain.EvidenceEvent{
		AggregateID: jointAggregate(cmd.JointID), Generation: gen, Type: domain.EventSegmentInjected,
		Payload: string(payload), LogicalTime: cmd.LogicalTime,
	})
	if err != nil {
		return txErr(err)
	}
	_ = count
	*result = CommandResult{OperationID: cmd.OperationID, Kind: cmd.Kind, TaskID: taskID,
		Generation: gen, LogicalTime: cmd.LogicalTime, Stage: "INJECTED", JointID: cmd.JointID,
		SegmentSeq: cmd.SegmentSeq, PrefixEnd: seg.End}
	return nil
}

func (s *Service) doTrim(tx *store.Tx, taskID string, lock domain.DesignLock, proj taskProjection, cmd Command, result *CommandResult) *domain.Error {
	if proj.stage < stageTrialed {
		return domain.NewError(domain.CodeDependencyUnmet, false,
			domain.Reason{Message: "trial shot before trim"})
	}
	closed, derr := jointsClosed(tx, taskID)
	if derr != nil {
		return derr
	}
	if !closed {
		return domain.NewError(domain.CodeNoncontiguousPrefix, false,
			domain.Reason{Message: "all joints must be fully injected before trim"})
	}
	_, _, err := tx.AppendEvent(domain.EvidenceEvent{
		AggregateID: taskAggregate(taskID), Generation: lock.Generation, Type: domain.EventTrimmed,
		Payload: "{}", LogicalTime: cmd.LogicalTime,
	})
	if err != nil {
		return txErr(err)
	}
	*result = CommandResult{OperationID: cmd.OperationID, Kind: cmd.Kind, TaskID: taskID,
		Generation: lock.Generation, LogicalTime: cmd.LogicalTime, Stage: "TRIMMED"}
	return nil
}

func (s *Service) doSeal(tx *store.Tx, taskID string, lock domain.DesignLock, proj taskProjection, cmd Command, result *CommandResult) *domain.Error {
	if proj.stage < stageTrimmed {
		return domain.NewError(domain.CodeDependencyUnmet, false,
			domain.Reason{Message: "trim before seal"})
	}
	// Record any leftover material as remainder so the generation can balance.
	if cmd.BaseMg > 0 || cmd.CatalystMg > 0 || cmd.PrimerMg > 0 {
		for _, e := range []domain.MassEntry{
			{Generation: lock.Generation, Component: domain.ComponentBase, Direction: domain.MassOutput, Category: domain.MassRemainder, Amount: cmd.BaseMg, Evidence: cmd.OperationID},
			{Generation: lock.Generation, Component: domain.ComponentCatalyst, Direction: domain.MassOutput, Category: domain.MassRemainder, Amount: cmd.CatalystMg, Evidence: cmd.OperationID},
			{Generation: lock.Generation, Component: domain.ComponentPrimer, Direction: domain.MassOutput, Category: domain.MassRemainder, Amount: cmd.PrimerMg, Evidence: cmd.OperationID},
		} {
			if err := tx.PostMass(e); err != nil {
				return txErr(err)
			}
		}
	}
	_, _, err := tx.AppendEvent(domain.EvidenceEvent{
		AggregateID: taskAggregate(taskID), Generation: lock.Generation, Type: domain.EventSealed,
		Payload: "{}", LogicalTime: cmd.LogicalTime,
	})
	if err != nil {
		return txErr(err)
	}
	*result = CommandResult{OperationID: cmd.OperationID, Kind: cmd.Kind, TaskID: taskID,
		Generation: lock.Generation, LogicalTime: cmd.LogicalTime, Stage: "SEALED"}
	return nil
}

func (s *Service) doCureReading(tx *store.Tx, taskID string, lock domain.DesignLock, proj taskProjection, cmd Command, result *CommandResult) *domain.Error {
	if proj.stage < stageSealed {
		return domain.NewError(domain.CodeDependencyUnmet, false,
			domain.Reason{Message: "seal before cure reading"})
	}
	payload, _ := json.Marshal(curePayload{
		SampleID: cmd.SampleID, At: int64(cmd.LogicalTime),
		Temperature: cmd.Temperature, Humidity: cmd.Humidity,
	})
	_, _, err := tx.AppendEvent(domain.EvidenceEvent{
		AggregateID: taskAggregate(taskID), Generation: lock.Generation, Type: domain.EventCureReading,
		Payload: string(payload), LogicalTime: cmd.LogicalTime,
	})
	if err != nil {
		return txErr(err)
	}
	*result = CommandResult{OperationID: cmd.OperationID, Kind: cmd.Kind, TaskID: taskID,
		Generation: lock.Generation, LogicalTime: cmd.LogicalTime, Stage: "CURED"}
	return nil
}

func (s *Service) doTestResult(tx *store.Tx, taskID string, lock domain.DesignLock, proj taskProjection, cmd Command, result *CommandResult) *domain.Error {
	if proj.stage < stageCured {
		return domain.NewError(domain.CodeDependencyUnmet, false,
			domain.Reason{Message: "cure before test result"})
	}
	payload, _ := json.Marshal(testPayload{
		TestID: cmd.TestID, Kind: int(cmd.TestKind), Hardness: cmd.Hardness,
		TensileMPa: cmd.TensileMPa, ElongationPct: cmd.ElongationPct, BondFailure: cmd.BondFailurePct,
	})
	_, _, err := tx.AppendEvent(domain.EvidenceEvent{
		AggregateID: taskAggregate(taskID), Generation: lock.Generation, Type: domain.EventTestResult,
		Payload: string(payload), LogicalTime: cmd.LogicalTime,
	})
	if err != nil {
		return txErr(err)
	}
	*result = CommandResult{OperationID: cmd.OperationID, Kind: cmd.Kind, TaskID: taskID,
		Generation: lock.Generation, LogicalTime: cmd.LogicalTime, Stage: "TESTED"}
	return nil
}

func checkLease(tx *store.Tx, rt domain.ResourceType, resourceID, token string, at domain.LogicalTime) *domain.Error {
	holder, ok, err := tx.LeaseHolder(rt, resourceID, at)
	if err != nil {
		return txErr(err)
	}
	if !ok {
		return domain.NewError(domain.CodeLeaseExpired, false,
			domain.Reason{Message: "lease expired or absent"})
	}
	if holder != token {
		return domain.NewError(domain.CodeLeaseConflict, false,
			domain.Reason{Message: "lease token mismatch"})
	}
	return nil
}
