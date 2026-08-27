package domain

import "testing"

func TestScaleRoundHalfUp(t *testing.T) {
	cases := []struct {
		name         string
		value, num   int64
		den          int64
		want         int64
		wantOverflow bool
	}{
		{"exact", 1000, 1, 4, 250, false},
		{"half up positive", 5, 1, 2, 3, false},
		{"half up negative", -5, 1, 2, -3, false},
		{"less than half", 4, 1, 2, 2, false},
		{"per mil", 1, RatioScale, 3, 333, false},
		{"two thirds round up", 2, RatioScale, 3, 667, false},
		{"divide by zero", 1, 1, 0, 0, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := ScaleRoundHalfUp(c.value, c.num, c.den)
			if ok == c.wantOverflow {
				t.Fatalf("ScaleRoundHalfUp(%d,%d,%d) ok=%v want overflow=%v (got=%d)", c.value, c.num, c.den, ok, c.wantOverflow, got)
			}
			if !c.wantOverflow && got != c.want {
				t.Fatalf("ScaleRoundHalfUp(%d,%d,%d)=%d want %d", c.value, c.num, c.den, got, c.want)
			}
		})
	}
}

func TestMul64Overflow(t *testing.T) {
	if v, ok := Mul64(1<<30, 1<<30); !ok || v != 1<<60 {
		t.Fatalf("Mul64(2^30,2^30)=%d,%v want 2^60,true", v, ok)
	}
	if _, ok := Mul64(1<<32, 1<<32); ok {
		t.Fatal("expected overflow for 2^32 * 2^32")
	}
	if _, ok := Mul64(-1<<40, 1<<40); ok {
		t.Fatal("expected overflow for negative product")
	}
}

func TestAdd64Overflow(t *testing.T) {
	if _, ok := Add64(1<<62, 1<<62); ok {
		t.Fatal("expected overflow on positive add")
	}
	// min int64 exactly fits: -2^62 + -2^62 == -2^63, no overflow.
	if v, ok := Add64(-1<<62, -1<<62); !ok || v != -1<<63 {
		t.Fatalf("Add64(-2^62,-2^62)=%d,%v want min int64,true", v, ok)
	}
	if _, ok := Add64(-1<<62, -1<<62-1); ok {
		t.Fatal("expected overflow on negative add past min int64")
	}
	if v, ok := Add64(10, -5); !ok || v != 5 {
		t.Fatalf("Add64(10,-5)=%d,%v want 5,true", v, ok)
	}
}

func TestRatioPermil(t *testing.T) {
	got, ok := RatioPermil(1, 4)
	if !ok || got != 250 {
		t.Fatalf("RatioPermil(1,4)=%d,%v want 250,true", got, ok)
	}
}
