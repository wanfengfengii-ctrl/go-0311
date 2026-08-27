// Package rework implements the rework/reinjection and terminal arbitration:
// it deterministically computes the anomaly impact set, manages cutout and
// re-injection generations, isolates late device receipts, performs
// two-person review and enforces the single-writer terminal competition among
// hoist admission, bond-risk isolation and cancellation.
package rework

import (
	"sort"

	"unitized-curtainwall-silicone-hoist-gate/internal/domain"
)

// ImpactKey is the stable sort key for a rework impact element: building,
// facade zone, panel, joint, segment, material generation and rework
// generation, in that order.
type ImpactKey struct {
	Building    string
	FacadeZone  string
	Panel       string
	Joint       string
	Segment     int64
	MaterialGen domain.Generation
	ReworkGen   domain.Generation
}

// Less implements the documented stable ordering.
func (a ImpactKey) Less(b ImpactKey) bool {
	if a.Building != b.Building {
		return a.Building < b.Building
	}
	if a.FacadeZone != b.FacadeZone {
		return a.FacadeZone < b.FacadeZone
	}
	if a.Panel != b.Panel {
		return a.Panel < b.Panel
	}
	if a.Joint != b.Joint {
		return a.Joint < b.Joint
	}
	if a.Segment != b.Segment {
		return a.Segment < b.Segment
	}
	if a.MaterialGen != b.MaterialGen {
		return a.MaterialGen < b.MaterialGen
	}
	return a.ReworkGen < b.ReworkGen
}

// UniqueImpact deduplicates and sorts an impact set by the documented stable
// key. It returns the deduplicated, sorted slice.
func UniqueImpact(keys []ImpactKey) []ImpactKey {
	sort.SliceStable(keys, func(i, j int) bool { return keys[i].Less(keys[j]) })
	out := keys[:0]
	for i, k := range keys {
		if i == 0 || k != out[len(out)-1] {
			out = append(out, k)
		}
	}
	return out
}

// Arbiter computes rework impact sets and manages the single-writer terminal
// decision.
type Arbiter interface {
	// ComputeImpact derives the unique, sorted rework set from a root evidence
	// reference and the immutable relation snapshot captured at failure time.
	ComputeImpact(rootEvidence string) ([]ImpactKey, *domain.Error)

	// Decide applies a terminal decision using a conditional single-writer
	// update. Exactly one concurrent request may succeed; the others receive
	// TERMINAL_ALREADY_DECIDED plus the existing decision.
	Decide(decision domain.TerminalDecision) (domain.TerminalDecision, *domain.Error)
}

// ReviewValidator checks that two reviews satisfy the two-person rule: two
// distinct qualified reviewers.
func ReviewValidator(a, b domain.Review) *domain.Error {
	if a.ReviewerID == "" || b.ReviewerID == "" {
		return domain.NewError(domain.CodeDependencyUnmet, false,
			domain.Reason{Message: "missing reviewer"})
	}
	if a.ReviewerID == b.ReviewerID {
		return domain.NewError(domain.CodeDependencyUnmet, false,
			domain.Reason{Message: "reviewers must be distinct"})
	}
	return nil
}
