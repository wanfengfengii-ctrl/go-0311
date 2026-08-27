package store

import (
	"database/sql"
	"errors"

	"unitized-curtainwall-silicone-hoist-gate/internal/domain"
)

// AppendEvent records one evidence event, assigning the next monotonic sequence
// number and chaining the previous hash. It returns the assigned sequence and
// the resulting chain hash so the caller can journal the event range.
func (t *Tx) AppendEvent(ev domain.EvidenceEvent) (int64, string, error) {
	prev, err := t.lastHash(ev.AggregateID)
	if err != nil {
		return 0, "", err
	}
	if ev.PrevHash == "" {
		ev.PrevHash = prev
	}
	ev.WrittenAt = ev.LogicalTime
	if ev.PayloadHash == "" {
		ev.PayloadHash = domain.CanonicalHash(ev.Payload)
	}
	res, err := t.tx.Exec(
		`INSERT INTO events(aggregate_id, generation, type, payload, payload_hash, logical_time, written_at, prev_hash)
		 VALUES(?,?,?,?,?,?,?,?)`,
		ev.AggregateID, int64(ev.Generation), int64(ev.Type), ev.Payload, ev.PayloadHash,
		int64(ev.LogicalTime), int64(ev.WrittenAt), ev.PrevHash)
	if err != nil {
		return 0, "", err
	}
	seq, err := res.LastInsertId()
	if err != nil {
		return 0, "", err
	}
	return seq, ev.PrevHash, nil
}

func (t *Tx) lastHash(aggregateID string) (string, error) {
	var h string
	err := t.tx.QueryRow(
		`SELECT prev_hash FROM events WHERE aggregate_id=? ORDER BY seq DESC LIMIT 1`,
		aggregateID).Scan(&h)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return h, nil
}

// ReplayEvents returns the ordered event chain for an aggregate.
func (t *Tx) ReplayEvents(aggregateID string) ([]domain.EvidenceEvent, error) {
	rows, err := t.tx.Query(
		`SELECT seq, aggregate_id, generation, type, payload, payload_hash, logical_time, written_at, prev_hash
		 FROM events WHERE aggregate_id=? ORDER BY seq ASC`, aggregateID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.EvidenceEvent
	for rows.Next() {
		ev, err := scanEvent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, ev)
	}
	return out, rows.Err()
}

// AllEvents returns every committed event ordered by sequence, used by the
// evidence query endpoint.
func (t *Tx) AllEvents() ([]domain.EvidenceEvent, error) {
	rows, err := t.tx.Query(
		`SELECT seq, aggregate_id, generation, type, payload, payload_hash, logical_time, written_at, prev_hash
		 FROM events ORDER BY seq ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.EvidenceEvent
	for rows.Next() {
		ev, err := scanEvent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, ev)
	}
	return out, rows.Err()
}

type eventScanner interface {
	Scan(dest ...interface{}) error
}

func scanEvent(s eventScanner) (domain.EvidenceEvent, error) {
	var ev domain.EvidenceEvent
	var typ int64
	var gen, lt, wt int64
	if err := s.Scan(&ev.Seq, &ev.AggregateID, &gen, &typ, &ev.Payload,
		&ev.PayloadHash, &lt, &wt, &ev.PrevHash); err != nil {
		return ev, err
	}
	ev.Generation = domain.Generation(gen)
	ev.Type = domain.EventType(typ)
	ev.LogicalTime = domain.LogicalTime(lt)
	ev.WrittenAt = domain.LogicalTime(wt)
	return ev, nil
}

// JournalPrepared records that a transaction has prepared (about to append
// events). JournalCommitted marks it committed. On restart the service drops
// any prepared-but-never-committed event range.
func (t *Tx) JournalPrepared(txnID string, eventFrom, eventTo int64) error {
	_, err := t.tx.Exec(
		`INSERT INTO tx_journal(txn_id, prepared, committed, event_from, event_to)
		 VALUES(?,1,0,?,?) ON CONFLICT(txn_id) DO UPDATE SET prepared=1, event_from=?, event_to=?`,
		txnID, eventFrom, eventTo, eventFrom, eventTo)
	return err
}

// JournalCommitted marks a prepared transaction committed.
func (t *Tx) JournalCommitted(txnID string) error {
	_, err := t.tx.Exec(`UPDATE tx_journal SET committed=1 WHERE txn_id=?`, txnID)
	return err
}

// IncompleteJournal returns prepared transactions that never committed.
func (t *Tx) IncompleteJournal() ([]domain.TransactionJournal, error) {
	rows, err := t.tx.Query(
		`SELECT txn_id, prepared, committed, event_from, event_to FROM tx_journal WHERE committed=0`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.TransactionJournal
	for rows.Next() {
		var j domain.TransactionJournal
		if err := rows.Scan(&j.TxnID, &j.Prepared, &j.Committed, &j.EventFrom, &j.EventTo); err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}
