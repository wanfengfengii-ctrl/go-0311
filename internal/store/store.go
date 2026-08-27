// Package store provides the SQLite-backed persistence layer for the unitized
// curtain-wall hoist gate. It owns the relational schema, the transaction
// boundary and every typed read/write operation used by the service layer. It
// is deliberately the only package that imports the SQL driver, so the rest of
// the system depends on an abstract, transaction-scoped store rather than on a
// specific database.
package store

import (
	"context"
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

// Store is a handle to the backing database and its schema.
type Store struct {
	db *sql.DB
}

// Open opens (or creates) the SQLite database at path and ensures the schema
// exists. An empty path selects an in-memory database.
func Open(path string) (*Store, error) {
	dsn := path
	if dsn == "" {
		dsn = ":memory:"
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	// A single writer serializes transactions, which matches the append-only
	// single-writer semantics of the terminal barrier and the mass ledger.
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

// Close releases the underlying database handle.
func (s *Store) Close() error { return s.db.Close() }

// InTx runs fn inside a single database transaction, committing on success and
// rolling back on any error or panic. Every business change is required to run
// through this boundary so that a failure never leaves partial state.
func (s *Store) InTx(ctx context.Context, fn func(*Tx) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	stx := &Tx{tx: tx}
	if err := fn(stx); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// Tx is a transaction-scoped store handle. All methods operate on the current
// transaction and are atomic together.
type Tx struct {
	tx *sql.Tx
}

func (s *Store) migrate() error {
	const schema = `
CREATE TABLE IF NOT EXISTS locks (
  task_id            TEXT PRIMARY KEY,
  generation         INTEGER NOT NULL,
  building           TEXT NOT NULL,
  facade_zone        TEXT NOT NULL,
  panel              TEXT NOT NULL,
  design_version     TEXT NOT NULL,
  compatibility_ver  TEXT NOT NULL,
  compat_valid_until INTEGER NOT NULL,
  surface_summary    TEXT NOT NULL,
  thresholds_json    TEXT NOT NULL,
  locked_at          INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS joints (
  task_id   TEXT NOT NULL,
  joint_id  TEXT NOT NULL,
  spec_json TEXT NOT NULL,
  PRIMARY KEY (task_id, joint_id)
);
CREATE TABLE IF NOT EXISTS events (
  seq          INTEGER PRIMARY KEY AUTOINCREMENT,
  aggregate_id TEXT NOT NULL,
  generation   INTEGER NOT NULL,
  type         INTEGER NOT NULL,
  payload      TEXT NOT NULL,
  payload_hash TEXT NOT NULL,
  logical_time INTEGER NOT NULL,
  written_at   INTEGER NOT NULL,
  prev_hash    TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_events_aggregate ON events(aggregate_id, seq);
CREATE TABLE IF NOT EXISTS mass_entries (
  seq        INTEGER PRIMARY KEY AUTOINCREMENT,
  generation INTEGER NOT NULL,
  component  INTEGER NOT NULL,
  direction  INTEGER NOT NULL,
  category   INTEGER NOT NULL,
  amount     INTEGER NOT NULL,
  evidence   TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_mass_generation ON mass_entries(generation, component);
CREATE TABLE IF NOT EXISTS leases (
  resource_type INTEGER NOT NULL,
  resource_id   TEXT NOT NULL,
  token         TEXT NOT NULL,
  holder_op     TEXT NOT NULL,
  acquired_at   INTEGER NOT NULL,
  expires_at    INTEGER NOT NULL,
  PRIMARY KEY (resource_type, resource_id)
);
CREATE TABLE IF NOT EXISTS device_calls (
  call_id        TEXT PRIMARY KEY,
  request_hash   TEXT NOT NULL,
  calibrated     INTEGER NOT NULL,
  attempts       INTEGER NOT NULL,
  next_retry_at  INTEGER NOT NULL,
  response_state INTEGER NOT NULL,
  raw_summary    TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS reviews (
  task_id       TEXT NOT NULL,
  reviewer_id   TEXT NOT NULL,
  qual_snapshot TEXT NOT NULL,
  summary       TEXT NOT NULL,
  reviewed_at   INTEGER NOT NULL,
  PRIMARY KEY (task_id, reviewer_id)
);
CREATE TABLE IF NOT EXISTS reworks (
  case_id        TEXT PRIMARY KEY,
  task_id        TEXT NOT NULL,
  category       TEXT NOT NULL,
  root_evidence  TEXT NOT NULL,
  impact_summary TEXT NOT NULL,
  affected_json  TEXT NOT NULL,
  cutout_mass    INTEGER NOT NULL,
  cutout_dest    TEXT NOT NULL,
  new_generation INTEGER NOT NULL,
  reinject_gen   INTEGER NOT NULL,
  closed         INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS terminals (
  task_id       TEXT NOT NULL,
  generation    INTEGER NOT NULL,
  type          INTEGER NOT NULL,
  credential    TEXT NOT NULL,
  evidence_hash TEXT NOT NULL,
  barrier_ver   INTEGER NOT NULL,
  decided_at    INTEGER NOT NULL,
  PRIMARY KEY (task_id, generation)
);
CREATE TABLE IF NOT EXISTS idempotency (
  task_id      TEXT NOT NULL,
  operation_id TEXT NOT NULL,
  endpoint     TEXT NOT NULL,
  request_hash TEXT NOT NULL,
  response     TEXT NOT NULL,
  event_range  TEXT NOT NULL,
  PRIMARY KEY (task_id, operation_id)
);
CREATE TABLE IF NOT EXISTS tx_journal (
  txn_id     TEXT PRIMARY KEY,
  prepared   INTEGER NOT NULL,
  committed  INTEGER NOT NULL,
  event_from INTEGER NOT NULL,
  event_to   INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS task_adjacency (
  task_id        TEXT PRIMARY KEY,
  adjacency_json TEXT NOT NULL
);
`
	if _, err := s.db.Exec(schema); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	return nil
}
