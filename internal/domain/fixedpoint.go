package domain

import "math/bits"

// RatioScale is the documented fixed-point scale for ratios and percentages
// expressed in per-mil (1/1000). All scaled integer arithmetic uses this
// scale to keep results platform-independent.
const RatioScale int64 = 1000

// Mul64 performs an overflow-checked signed 64-bit multiply. It returns false
// when the product does not fit in an int64.
func Mul64(a, b int64) (int64, bool) {
	hi, lo := bits.Mul64(uint64(a), uint64(b))
	// Recompute the sign of the true product to detect signed overflow.
	sign := (a < 0) != (b < 0)
	if sign {
		// Negative product: valid only if the high bits are all ones up to the
		// sign bit of lo, i.e. the two's-complement of (hi,lo) fits in int64.
		neg, carry := bits.Add64(^lo, 1, 0)
		_ = neg
		if carry != 0 {
			hi = ^hi
		} else {
			hi = ^hi + 1
		}
		if hi != 0 {
			return 0, false
		}
	} else if hi != 0 {
		return 0, false
	}
	return int64(lo), true
}

// Add64 performs an overflow-checked signed 64-bit add.
func Add64(a, b int64) (int64, bool) {
	sum := a + b
	if (b > 0 && sum < a) || (b < 0 && sum > a) {
		return 0, false
	}
	return sum, true
}

// ScaleRoundHalfUp multiplies a value by numerator and divides by denominator
// with round-half-up (ties away from zero) semantics, checking for overflow at
// every step. It is the single entry point for every documented fixed-point
// metric so that sign, rounding and boundary behavior are consistent.
func ScaleRoundHalfUp(value, numerator, denominator int64) (int64, bool) {
	if denominator == 0 {
		return 0, false
	}
	prod, ok := Mul64(value, numerator)
	if !ok {
		return 0, false
	}
	// Go integer division truncates toward zero.
	q := prod / denominator
	r := prod % denominator
	if r != 0 && 2*abs64(r) >= abs64(denominator) {
		// Round away from zero on ties and above.
		if prod > 0 {
			q++
		} else {
			q--
		}
	}
	return q, true
}

func abs64(x int64) int64 {
	if x < 0 {
		return -x
	}
	return x
}

// RatioPermil computes numerator/denominator expressed in per-mil with
// half-up rounding and overflow checks.
func RatioPermil(numerator, denominator int64) (int64, bool) {
	return ScaleRoundHalfUp(numerator, RatioScale, denominator)
}
