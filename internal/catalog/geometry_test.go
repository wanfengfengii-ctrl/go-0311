package catalog

import (
	"testing"

	"unitized-curtainwall-silicone-hoist-gate/internal/domain"
)

// Panel P-017 on tower A east facade with three contiguous micron segments.
func sampleJoint(segments []domain.SegmentSpec) domain.JointSpec {
	return domain.JointSpec{
		JointID: "J-017",

		Start:       0,
		End:         3000,
		Width:       20,
		Depth:       10,
		BondAreaUm2: 200,
		Segments:    segments,
	}
}

func TestValidateSegmentsContiguous(t *testing.T) {
	j := sampleJoint([]domain.SegmentSpec{
		{Seq: 1, Start: 0, End: 1000},
		{Seq: 2, Start: 1000, End: 2000},
		{Seq: 3, Start: 2000, End: 3000},
	})
	if seq, err := ValidateSegments(j); err != nil || seq != 0 {
		t.Fatalf("ValidateSegments contiguous seq=%d err=%v", seq, err)
	}
}

func TestValidateSegmentsGap(t *testing.T) {
	j := sampleJoint([]domain.SegmentSpec{
		{Seq: 1, Start: 0, End: 1000},
		{Seq: 2, Start: 1500, End: 2000},
		{Seq: 3, Start: 2000, End: 3000},
	})
	seq, err := ValidateSegments(j)
	if err == nil || err.Code != domain.CodeJointCoverageInvalid || seq != 2 {
		t.Fatalf("gap: seq=%d err=%v", seq, err)
	}
}

func TestValidateSegmentsOverlap(t *testing.T) {
	j := sampleJoint([]domain.SegmentSpec{
		{Seq: 1, Start: 0, End: 1000},
		{Seq: 2, Start: 500, End: 2000},
		{Seq: 3, Start: 2000, End: 3000},
	})
	seq, err := ValidateSegments(j)
	if err == nil || err.Code != domain.CodeJointCoverageInvalid || seq != 2 {
		t.Fatalf("overlap: seq=%d err=%v", seq, err)
	}
}

func TestValidateSegmentsDegenerate(t *testing.T) {
	j := sampleJoint([]domain.SegmentSpec{
		{Seq: 1, Start: 0, End: 1000},
		{Seq: 2, Start: 1000, End: 1000},
		{Seq: 3, Start: 1000, End: 3000},
	})
	seq, err := ValidateSegments(j)
	if err == nil || err.Code != domain.CodeJointCoverageInvalid || seq != 2 {
		t.Fatalf("degenerate: seq=%d err=%v", seq, err)
	}
}

func TestValidateSegmentsNegative(t *testing.T) {
	j := sampleJoint([]domain.SegmentSpec{
		{Seq: 1, Start: -100, End: 1000},
		{Seq: 2, Start: 1000, End: 2000},
		{Seq: 3, Start: 2000, End: 3000},
	})
	_, err := ValidateSegments(j)
	if err == nil || err.Code != domain.CodeJointCoverageInvalid {
		t.Fatalf("negative: err=%v", err)
	}
}

func TestValidateGeometryBondAreaOverflow(t *testing.T) {
	j := sampleJoint([]domain.SegmentSpec{{Seq: 1, Start: 0, End: 3000}})
	j.Width = domain.Microns(1 << 40)
	j.Depth = domain.Microns(1 << 40)
	err := ValidateGeometry(j)
	if err == nil || err.Code != domain.CodeFixedPointOverflow {
		t.Fatalf("bond area overflow: err=%v", err)
	}
}

func TestValidateGeometryBondAreaMismatch(t *testing.T) {
	j := sampleJoint([]domain.SegmentSpec{{Seq: 1, Start: 0, End: 3000}})
	j.BondAreaUm2 = 999
	err := ValidateGeometry(j)
	if err == nil || err.Code != domain.CodeJointCoverageInvalid {
		t.Fatalf("bond area mismatch: err=%v", err)
	}
}
