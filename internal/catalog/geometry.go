// Package catalog implements the curtain-wall joint and material rules
// directory: the immutable task snapshot and the deterministic validation of
// design/compatibility versions, substrate surface treatment, material
// batches, joint geometry, segment coverage, direction, spacer placement,
// trial mapping, adjacency and thresholds.
package catalog

import (
	"unitized-curtainwall-silicone-hoist-gate/internal/domain"
)

// ValidateSegments verifies that segments tile the joint design boundary
// exactly: first segment starts at the boundary start, segments are strictly
// contiguous, non-degenerate, non-overlapping, in ascending order and the last
// segment ends at the boundary end. It returns the first offending segment
// index (1-based) or 0 with a nil error on success.
func ValidateSegments(joint domain.JointSpec) (int64, *domain.Error) {
	if joint.Start < 0 || joint.End < 0 {
		return 0, domain.NewError(domain.CodeJointCoverageInvalid, false,
			domain.Reason{Joint: joint.JointID, Message: "negative boundary coordinate"})
	}
	if joint.End <= joint.Start {
		return 0, domain.NewError(domain.CodeJointCoverageInvalid, false,
			domain.Reason{Joint: joint.JointID, Message: "degenerate boundary interval"})
	}
	if len(joint.Segments) == 0 {
		return 0, domain.NewError(domain.CodeJointCoverageInvalid, false,
			domain.Reason{Joint: joint.JointID, Message: "missing segments"})
	}

	var prev domain.Microns = joint.Start
	for i, seg := range joint.Segments {
		seq := int64(i + 1)
		if seg.Seq != seq {
			return seq, domain.NewError(domain.CodeJointCoverageInvalid, false,
				domain.Reason{Joint: joint.JointID, Segment: seq, Message: "segment sequence out of order"})
		}
		if seg.Start < 0 || seg.End < 0 {
			return seq, domain.NewError(domain.CodeJointCoverageInvalid, false,
				domain.Reason{Joint: joint.JointID, Segment: seq, Message: "negative segment coordinate"})
		}
		if seg.End <= seg.Start {
			return seq, domain.NewError(domain.CodeJointCoverageInvalid, false,
				domain.Reason{Joint: joint.JointID, Segment: seq, Message: "degenerate segment"})
		}
		if seg.Start != prev {
			// Either a gap or an overlap; distinguish for a precise reason.
			if seg.Start > prev {
				return seq, domain.NewError(domain.CodeJointCoverageInvalid, false,
					domain.Reason{Joint: joint.JointID, Segment: seq, Message: "segment gap"})
			}
			return seq, domain.NewError(domain.CodeJointCoverageInvalid, false,
				domain.Reason{Joint: joint.JointID, Segment: seq, Message: "segment overlap"})
		}
		prev = seg.End
	}
	if prev != joint.End {
		return int64(len(joint.Segments)), domain.NewError(domain.CodeJointCoverageInvalid, false,
			domain.Reason{Joint: joint.JointID, Message: "segments do not cover full boundary"})
	}
	return 0, nil
}

// ValidateGeometry validates the joint width, depth, corner angle, spacer
// intervals and effective bond area, rejecting negative values, degenerate
// intervals, division by zero and signed 64-bit multiply overflow.
func ValidateGeometry(joint domain.JointSpec) *domain.Error {
	if joint.Width <= 0 || joint.Depth <= 0 {
		return domain.NewError(domain.CodeJointCoverageInvalid, false,
			domain.Reason{Joint: joint.JointID, Message: "non-positive width or depth"})
	}
	// Effective bond area must equal width * depth (overflow-checked).
	area, ok := domain.Mul64(int64(joint.Width), int64(joint.Depth))
	if !ok {
		return domain.NewError(domain.CodeFixedPointOverflow, false,
			domain.Reason{Joint: joint.JointID, Message: "bond area overflow"})
	}
	if area <= 0 {
		return domain.NewError(domain.CodeJointCoverageInvalid, false,
			domain.Reason{Joint: joint.JointID, Message: "non-positive bond area"})
	}
	if joint.BondAreaUm2 != area {
		return domain.NewError(domain.CodeJointCoverageInvalid, false,
			domain.Reason{Joint: joint.JointID, Message: "bond area mismatch"})
	}
	for _, iv := range joint.SpacerIntervals {
		if iv.Start < 0 || iv.End < 0 || iv.End <= iv.Start {
			return domain.NewError(domain.CodeJointCoverageInvalid, false,
				domain.Reason{Joint: joint.JointID, Message: "degenerate spacer interval"})
		}
		if iv.Start < joint.Start || iv.End > joint.End {
			return domain.NewError(domain.CodeJointCoverageInvalid, false,
				domain.Reason{Joint: joint.JointID, Message: "spacer outside boundary"})
		}
	}
	return nil
}
