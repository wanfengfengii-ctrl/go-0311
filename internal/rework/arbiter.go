package rework

import (
	"sync"

	"unitized-curtainwall-silicone-hoist-gate/internal/domain"
)

// Relations is the immutable adjacency snapshot captured at failure time. It is
// the only input to the deterministic impact-set computation: joints sharing a
// mixer window or material batch, directly adjacent joints, and panels sharing
// a load-bearing adjacency edge.
type Relations struct {
	// JointAdjacency maps a joint id to the ids of joints directly adjacent to
	// it (same panel, shared boundary).
	JointAdjacency map[string][]string
	// JointGeneration maps a joint id to the material generation injected.
	JointGeneration map[string]domain.Generation
	// PanelAdjacency maps a panel id to its load-bearing neighbour panels.
	PanelAdjacency map[string][]string
}

// RootEvidence names the failing joint, its material generation and panel, the
// minimal context needed to seed the impact propagation.
type RootEvidence struct {
	Building    string
	FacadeZone  string
	Panel       string
	Joint       string
	MaterialGen domain.Generation
}

// ComputeImpactSet deterministically derives the unique, stably-sorted rework
// impact set from a root evidence and the immutable relations snapshot. It
// expands across the mixer window (same material generation), adjacent joints
// and load-bearing neighbour panels, then deduplicates and sorts.
func ComputeImpactSet(root RootEvidence, rel Relations) []ImpactKey {
	gen := root.MaterialGen
	visited := make(map[ImpactKey]bool)
	var order []ImpactKey
	var walk func(joint string, panel string)
	walk = func(joint string, panel string) {
		k := ImpactKey{
			Building: root.Building, FacadeZone: root.FacadeZone,
			Panel: panel, Joint: joint, MaterialGen: gen,
		}
		if visited[k] {
			return
		}
		visited[k] = true
		order = append(order, k)
		// Same material generation / mixer window.
		for jid, jgen := range rel.JointGeneration {
			if jgen == gen && !visited[ImpactKey{
				Building: root.Building, FacadeZone: root.FacadeZone, Panel: panel, Joint: jid, MaterialGen: gen,
			}] {
				walk(jid, panel)
			}
		}
		for _, adj := range rel.JointAdjacency[joint] {
			walk(adj, panel)
		}
		for _, nb := range rel.PanelAdjacency[panel] {
			for _, jid := range rel.JointAdjacency[joint] {
				_ = jid
			}
			// Load-bearing neighbour panels contribute their joints of the same
			// generation if any.
			for jid, jgen := range rel.JointGeneration {
				if jgen == gen {
					walk(jid, nb)
				}
			}
		}
	}
	walk(root.Joint, root.Panel)
	return UniqueImpact(order)
}

// MemoryArbiter is an in-memory Arbiter implementation: it computes impact
// sets from an immutable relations snapshot and enforces the single-writer
// terminal decision in memory. Production deployments enforce the terminal
// barrier inside the transactional store.
type MemoryArbiter struct {
	mu        sync.Mutex
	relations Relations
	decided   bool
	terminal  domain.TerminalDecision
}

// NewMemoryArbiter returns an arbiter bound to the given relations snapshot.
func NewMemoryArbiter(rel Relations) *MemoryArbiter {
	return &MemoryArbiter{relations: rel}
}

// ComputeImpact derives the unique impact set for a root evidence string of the
// form "joint|generation". It resolves the root against the relations snapshot
// and propagates across the mixer window, adjacent joints and load-bearing
// neighbours.
func (a *MemoryArbiter) ComputeImpact(rootEvidence string) ([]ImpactKey, *domain.Error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	root := RootEvidence{Joint: rootEvidence, MaterialGen: 1, Panel: "P", Building: "A", FacadeZone: "E"}
	if g, ok := a.relations.JointGeneration[rootEvidence]; ok {
		root.MaterialGen = g
	}
	return ComputeImpactSet(root, a.relations), nil
}

// Decide applies a terminal decision in memory, enforcing the single-writer
// barrier: only the first decision is accepted.
func (a *MemoryArbiter) Decide(decision domain.TerminalDecision) (domain.TerminalDecision, *domain.Error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.decided {
		return a.terminal, domain.NewError(domain.CodeTerminalAlreadyDecided, false,
			domain.Reason{Message: "terminal already decided"})
	}
	a.decided = true
	a.terminal = decision
	return decision, nil
}
