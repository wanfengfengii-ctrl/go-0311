// Package service orchestrates the unitized curtain-wall silicone hoist-gate
// business flows on top of the transactional store. It is the only layer that
// composes the rules directory, evidence aggregation, mass conservation, lease
// manager, cure/test recorder and rework/terminal arbiter into the documented
// end-to-end behaviours: task lock, dependent construction, continuous
// injection prefixes, cure, anomaly rework and the single-writer terminal.
package service

import (
	"context"

	"unitized-curtainwall-silicone-hoist-gate/internal/catalog"
	"unitized-curtainwall-silicone-hoist-gate/internal/domain"
	"unitized-curtainwall-silicone-hoist-gate/internal/store"
)

// Service is the application service. It owns the transactional store and the
// rules directory used to validate lock requests.
type Service struct {
	store *store.Store
	dir   *catalog.Directory
}

// New builds a Service over the given store.
func New(st *store.Store) *Service {
	return &Service{store: st, dir: catalog.NewDirectory()}
}

// Store exposes the underlying store for startup recovery and tests.
func (s *Service) Store() *store.Store { return s.store }

// LockResult is the outcome of a successful task lock.
type LockResult struct {
	TaskID       string             `json:"task_id"`
	Generation   domain.Generation  `json:"generation"`
	LockedAt     domain.LogicalTime `json:"locked_at"`
	PayloadHash  string             `json:"payload_hash"`
	JointCount   int                `json:"joint_count"`
	SegmentCount int                `json:"segment_count"`
}

// TaskView is the current projection of a task for the query endpoint.
type TaskView struct {
	TaskID        string                   `json:"task_id"`
	Generation    domain.Generation        `json:"generation"`
	Building      string                   `json:"building"`
	FacadeZone    string                   `json:"facade_zone"`
	Panel         string                   `json:"panel"`
	Stage         string                   `json:"stage"`
	Joints        []JointView              `json:"joints"`
	MassConserved bool                     `json:"mass_conserved"`
	Reviews       []domain.Review          `json:"reviews"`
	Terminal      *domain.TerminalDecision `json:"terminal,omitempty"`
}

// JointView is the rebuildable projection of a single joint.
type JointView struct {
	JointID        string            `json:"joint_id"`
	Stage          string            `json:"stage"`
	ValidPrefixEnd domain.Microns    `json:"valid_prefix_end_um"`
	DesignEnd      domain.Microns    `json:"design_end_um"`
	Generation     domain.Generation `json:"generation"`
	Segments       int               `json:"segments"`
}

// txErr wraps a store/DB error into a stable internal domain error.
func txErr(err error) *domain.Error {
	if err == nil {
		return nil
	}
	if de, ok := err.(*domain.Error); ok {
		return de
	}
	return domain.NewError(domain.CodeInternal, false, domain.Reason{Message: err.Error()})
}

// aggregateID builds the task aggregate id.
func taskAggregate(taskID string) string { return "task:" + taskID }

// jointAggregate builds the joint aggregate id.
func jointAggregate(jointID string) string { return "joint:" + jointID }

// runTx runs fn inside a transaction, converting errors to stable domain
// errors.
func (s *Service) runTx(fn func(*store.Tx) *domain.Error) *domain.Error {
	err := s.store.InTx(context.Background(), func(tx *store.Tx) error {
		if de := fn(tx); de != nil {
			return de
		}
		return nil
	})
	if err != nil {
		return txErr(err)
	}
	return nil
}
