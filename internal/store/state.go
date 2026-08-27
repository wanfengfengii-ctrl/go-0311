package store

import (
	"database/sql"
	"encoding/json"

	"unitized-curtainwall-silicone-hoist-gate/internal/domain"
)

// SaveLock persists an immutable design lock snapshot.
func (t *Tx) SaveLock(lock domain.DesignLock) error {
	th, err := json.Marshal(lock.Thresholds)
	if err != nil {
		return err
	}
	_, err = t.tx.Exec(
		`INSERT INTO locks(task_id, generation, building, facade_zone, panel, design_version,
		   compatibility_ver, compat_valid_until, surface_summary, thresholds_json, locked_at)
		 VALUES(?,?,?,?,?,?,?,?,?,?,?)`,
		lock.TaskID, int64(lock.Generation), lock.Building, lock.FacadeZone, lock.Panel,
		lock.DesignVersion, lock.CompatibilityVer, int64(lock.CompatValidUntil),
		lock.SurfaceSummary, string(th), int64(lock.LockedAt))
	return err
}

// GetLock loads a design lock snapshot by task id.
func (t *Tx) GetLock(taskID string) (domain.DesignLock, bool, error) {
	var l domain.DesignLock
	var gen, cvu, lt int64
	var th string
	err := t.tx.QueryRow(
		`SELECT task_id, generation, building, facade_zone, panel, design_version,
		   compatibility_ver, compat_valid_until, surface_summary, thresholds_json, locked_at
		 FROM locks WHERE task_id=?`, taskID).Scan(
		&l.TaskID, &gen, &l.Building, &l.FacadeZone, &l.Panel, &l.DesignVersion,
		&l.CompatibilityVer, &cvu, &l.SurfaceSummary, &th, &lt)
	if err == sql.ErrNoRows {
		return l, false, nil
	}
	if err != nil {
		return l, false, err
	}
	l.Generation = domain.Generation(gen)
	l.CompatValidUntil = domain.LogicalTime(cvu)
	l.LockedAt = domain.LogicalTime(lt)
	if err := json.Unmarshal([]byte(th), &l.Thresholds); err != nil {
		return l, false, err
	}
	return l, true, nil
}

// MaxGeneration returns the highest task generation locked so far, or zero.
func (t *Tx) MaxGeneration(taskID string) (domain.Generation, error) {
	var n sql.NullInt64
	err := t.tx.QueryRow(`SELECT MAX(generation) FROM locks WHERE task_id=?`, taskID).Scan(&n)
	if err != nil {
		return 0, err
	}
	return domain.Generation(n.Int64), nil
}

// SaveJoint persists the immutable joint spec of a locked task.
func (t *Tx) SaveJoint(taskID string, j domain.JointSpec) error {
	b, err := json.Marshal(j)
	if err != nil {
		return err
	}
	_, err = t.tx.Exec(
		`INSERT INTO joints(task_id, joint_id, spec_json) VALUES(?,?,?)`,
		taskID, j.JointID, string(b))
	return err
}

// GetJoint loads a joint spec.
func (t *Tx) GetJoint(taskID, jointID string) (domain.JointSpec, bool, error) {
	var j domain.JointSpec
	var raw string
	err := t.tx.QueryRow(`SELECT spec_json FROM joints WHERE task_id=? AND joint_id=?`,
		taskID, jointID).Scan(&raw)
	if err == sql.ErrNoRows {
		return j, false, nil
	}
	if err != nil {
		return j, false, err
	}
	if err := json.Unmarshal([]byte(raw), &j); err != nil {
		return j, false, err
	}
	return j, true, nil
}

// ListJoints returns all joint specs of a task ordered by joint id.
func (t *Tx) ListJoints(taskID string) ([]domain.JointSpec, error) {
	rows, err := t.tx.Query(`SELECT spec_json FROM joints WHERE task_id=? ORDER BY joint_id ASC`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.JointSpec
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var j domain.JointSpec
		if err := json.Unmarshal([]byte(raw), &j); err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

// SaveReview records one independent review. It is idempotent per reviewer.
func (t *Tx) SaveReview(taskID string, r domain.Review) error {
	_, err := t.tx.Exec(
		`INSERT INTO reviews(task_id, reviewer_id, qual_snapshot, summary, reviewed_at)
		 VALUES(?,?,?,?,?) ON CONFLICT(task_id, reviewer_id) DO UPDATE SET
		   qual_snapshot=?, summary=?, reviewed_at=?`,
		taskID, r.ReviewerID, r.QualSnapshot, r.Summary, int64(r.ReviewedAt),
		r.QualSnapshot, r.Summary, int64(r.ReviewedAt))
	return err
}

// ListReviews returns reviews for a task ordered by reviewer id.
func (t *Tx) ListReviews(taskID string) ([]domain.Review, error) {
	rows, err := t.tx.Query(
		`SELECT reviewer_id, qual_snapshot, summary, reviewed_at FROM reviews
		 WHERE task_id=? ORDER BY reviewer_id ASC`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Review
	for rows.Next() {
		var r domain.Review
		var lt int64
		if err := rows.Scan(&r.ReviewerID, &r.QualSnapshot, &r.Summary, &lt); err != nil {
			return nil, err
		}
		r.ReviewedAt = domain.LogicalTime(lt)
		out = append(out, r)
	}
	return out, rows.Err()
}

// SaveRework persists or updates a rework case.
func (t *Tx) SaveRework(rw domain.ReworkCase) error {
	b, err := json.Marshal(rw.Affected)
	if err != nil {
		return err
	}
	_, err = t.tx.Exec(
		`INSERT INTO reworks(case_id, task_id, category, root_evidence, impact_summary,
		   affected_json, cutout_mass, cutout_dest, new_generation, reinject_gen, closed)
		 VALUES(?,?,?,?,?,?,?,?,?,?,?)
		 ON CONFLICT(case_id) DO UPDATE SET cutout_mass=?, cutout_dest=?, closed=?`,
		rw.CaseID, rw.TaskID, rw.Category, rw.RootEvidence, rw.ImpactSummary, string(b),
		int64(rw.CutoutMass), rw.CutoutDest, int64(rw.NewGeneration), int64(rw.ReinjectGen),
		boolInt(rw.Closed), int64(rw.CutoutMass), rw.CutoutDest, boolInt(rw.Closed))
	return err
}

// GetRework loads a rework case by id.
func (t *Tx) GetRework(caseID string) (domain.ReworkCase, bool, error) {
	var rw domain.ReworkCase
	var raw string
	var cutout, ng, rg int64
	var closed int
	err := t.tx.QueryRow(
		`SELECT case_id, task_id, category, root_evidence, impact_summary, affected_json,
		   cutout_mass, cutout_dest, new_generation, reinject_gen, closed
		 FROM reworks WHERE case_id=?`, caseID).Scan(
		&rw.CaseID, &rw.TaskID, &rw.Category, &rw.RootEvidence, &rw.ImpactSummary, &raw,
		&cutout, &rw.CutoutDest, &ng, &rg, &closed)
	if err == sql.ErrNoRows {
		return rw, false, nil
	}
	if err != nil {
		return rw, false, err
	}
	rw.CutoutMass = domain.Milligrams(cutout)
	rw.NewGeneration = domain.Generation(ng)
	rw.ReinjectGen = domain.Generation(rg)
	rw.Closed = closed != 0
	if err := json.Unmarshal([]byte(raw), &rw.Affected); err != nil {
		return rw, false, err
	}
	return rw, true, nil
}

// ListReworks returns reworks for a task ordered by case id.
func (t *Tx) ListReworks() ([]domain.ReworkCase, error) {
	rows, err := t.tx.Query(
		`SELECT case_id, task_id, category, root_evidence, impact_summary, affected_json,
		   cutout_mass, cutout_dest, new_generation, reinject_gen, closed
		 FROM reworks ORDER BY case_id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.ReworkCase
	for rows.Next() {
		var rw domain.ReworkCase
		var raw string
		var cutout, ng, rg int64
		var closed int
		if err := rows.Scan(&rw.CaseID, &rw.TaskID, &rw.Category, &rw.RootEvidence, &rw.ImpactSummary, &raw,
			&cutout, &rw.CutoutDest, &ng, &rg, &closed); err != nil {
			return nil, err
		}
		rw.CutoutMass = domain.Milligrams(cutout)
		rw.NewGeneration = domain.Generation(ng)
		rw.ReinjectGen = domain.Generation(rg)
		rw.Closed = closed != 0
		if err := json.Unmarshal([]byte(raw), &rw.Affected); err != nil {
			return nil, err
		}
		out = append(out, rw)
	}
	return out, rows.Err()
}

// HasReworkFor reports whether a rework case already exists for the given root
// evidence, so a repeated anomaly returns the original set instead of a new one.
func (t *Tx) HasReworkFor(rootEvidence string) (string, bool, error) {
	var id string
	err := t.tx.QueryRow(`SELECT case_id FROM reworks WHERE root_evidence=? LIMIT 1`, rootEvidence).Scan(&id)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return id, true, nil
}

// SaveTerminal attempts a conditional single-writer terminal decision. Exactly
// one concurrent request may succeed; a conflicting insert returns
// TERMINAL_ALREADY_DECIDED together with the existing decision.
func (t *Tx) SaveTerminal(taskID string, d domain.TerminalDecision) (domain.TerminalDecision, bool, error) {
	res, err := t.tx.Exec(
		`INSERT INTO terminals(task_id, generation, type, credential, evidence_hash, barrier_ver, decided_at)
		 VALUES(?,?,?,?,?,?,?) ON CONFLICT(task_id, generation) DO NOTHING`,
		taskID, int64(0), int64(d.Type), d.Credential, d.EvidenceHash, d.BarrierVer, int64(d.DecidedAt))
	if err != nil {
		return d, false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return d, false, err
	}
	if n == 0 {
		existing, _, err := t.GetTerminal(taskID)
		return existing, false, err
	}
	return d, true, nil
}

// GetTerminal loads the current terminal decision for a task, if any.
func (t *Tx) GetTerminal(taskID string) (domain.TerminalDecision, bool, error) {
	var d domain.TerminalDecision
	var typ, gen, bv, lt int64
	err := t.tx.QueryRow(
		`SELECT type, credential, evidence_hash, barrier_ver, decided_at, generation
		 FROM terminals WHERE task_id=? ORDER BY barrier_ver DESC LIMIT 1`, taskID).Scan(
		&typ, &d.Credential, &d.EvidenceHash, &bv, &lt, &gen)
	if err == sql.ErrNoRows {
		return d, false, nil
	}
	if err != nil {
		return d, false, err
	}
	d.Type = domain.TerminalType(typ)
	d.BarrierVer = bv
	d.DecidedAt = domain.LogicalTime(lt)
	return d, true, nil
}

// SaveDeviceCall inserts a device call row.
func (t *Tx) SaveDeviceCall(c domain.DeviceCall) error {
	_, err := t.tx.Exec(
		`INSERT INTO device_calls(call_id, request_hash, calibrated, attempts, next_retry_at, response_state, raw_summary)
		 VALUES(?,?,?,?,?,?,?)`,
		c.CallID, c.RequestHash, boolInt(c.Calibrated), c.Attempts, int64(c.NextRetryAt),
		int64(c.ResponseState), c.RawSummary)
	return err
}

// GetDeviceCall loads a device call row.
func (t *Tx) GetDeviceCall(callID string) (domain.DeviceCall, bool, error) {
	var c domain.DeviceCall
	var cal int
	var nat int64
	var rs int64
	err := t.tx.QueryRow(
		`SELECT call_id, request_hash, calibrated, attempts, next_retry_at, response_state, raw_summary
		 FROM device_calls WHERE call_id=?`, callID).Scan(
		&c.CallID, &c.RequestHash, &cal, &c.Attempts, &nat, &rs, &c.RawSummary)
	if err == sql.ErrNoRows {
		return c, false, nil
	}
	if err != nil {
		return c, false, err
	}
	c.Calibrated = cal != 0
	c.NextRetryAt = domain.LogicalTime(nat)
	c.ResponseState = domain.DeviceResponse(rs)
	return c, true, nil
}

// UpdateDeviceCall mutates the retry state of a device call.
func (t *Tx) UpdateDeviceCall(c domain.DeviceCall) error {
	_, err := t.tx.Exec(
		`UPDATE device_calls SET attempts=?, next_retry_at=?, response_state=?, raw_summary=? WHERE call_id=?`,
		c.Attempts, int64(c.NextRetryAt), int64(c.ResponseState), c.RawSummary, c.CallID)
	return err
}

// GetIdempotency loads a cached idempotency record by operation id.
func (t *Tx) GetIdempotency(operationID string) (domain.IdempotencyRecord, bool, error) {
	var rec domain.IdempotencyRecord
	err := t.tx.QueryRow(
		`SELECT operation_id, endpoint, request_hash, response, event_range FROM idempotency WHERE operation_id=?`,
		operationID).Scan(&rec.OperationID, &rec.Endpoint, &rec.RequestHash, &rec.Response, &rec.EventRange)
	if err == sql.ErrNoRows {
		return rec, false, nil
	}
	if err != nil {
		return rec, false, err
	}
	return rec, true, nil
}

// SaveIdempotency caches a canonical response keyed by operation id.
func (t *Tx) SaveIdempotency(rec domain.IdempotencyRecord) error {
	_, err := t.tx.Exec(
		`INSERT INTO idempotency(operation_id, endpoint, request_hash, response, event_range)
		 VALUES(?,?,?,?,?) ON CONFLICT(operation_id) DO NOTHING`,
		rec.OperationID, rec.Endpoint, rec.RequestHash, rec.Response, rec.EventRange)
	return err
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// SaveAdjacency persists the load-bearing panel adjacency list of a task.
func (t *Tx) SaveAdjacency(taskID string, adjacency []string) error {
	b, err := json.Marshal(adjacency)
	if err != nil {
		return err
	}
	_, err = t.tx.Exec(
		`INSERT INTO task_adjacency(task_id, adjacency_json) VALUES(?,?)
		 ON CONFLICT(task_id) DO UPDATE SET adjacency_json=?`,
		taskID, string(b), string(b))
	return err
}

// GetAdjacency loads the load-bearing panel adjacency list of a task.
func (t *Tx) GetAdjacency(taskID string) ([]string, error) {
	var raw string
	err := t.tx.QueryRow(`SELECT adjacency_json FROM task_adjacency WHERE task_id=?`, taskID).Scan(&raw)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, err
	}
	return out, nil
}
