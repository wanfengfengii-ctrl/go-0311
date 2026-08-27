package cure

import (
	"testing"

	"unitized-curtainwall-silicone-hoist-gate/internal/domain"
)

func TestEquivalentCureAccum(t *testing.T) {
	readings := []domain.TempHumidity{
		{At: 0, Temperature: 1000, Humidity: 1000},
		{At: 10, Temperature: 2000, Humidity: 500},
		{At: 20, Temperature: 1000, Humidity: 1000},
	}
	got, ok := EquivalentCureAccum(readings)
	if !ok {
		t.Fatal("expected success")
	}
	// rate = T*H/1000. contrib = rate * delta.
	// seg1: 1000*1000/1000=1000; *10 = 10000
	// seg2: 2000*500/1000=1000; *10 = 10000
	want := int64(20000)
	if got != want {
		t.Fatalf("EquivalentCureAccum=%d want %d", got, want)
	}
}

func TestEquivalentCureAccumBackwardsTime(t *testing.T) {
	readings := []domain.TempHumidity{
		{At: 10, Temperature: 1000, Humidity: 1000},
		{At: 5, Temperature: 1000, Humidity: 1000},
	}
	if _, ok := EquivalentCureAccum(readings); ok {
		t.Fatal("expected failure on backwards time")
	}
}

func TestNextRetryTimeBackoff(t *testing.T) {
	c := domain.DeviceCall{Attempts: 1, NextRetryAt: 100}
	if got := NextRetryTime(c); got != 101 {
		t.Fatalf("NextRetryTime attempt1=%d want 101", got)
	}
	c.Attempts = 2
	if got := NextRetryTime(c); got != 102 {
		t.Fatalf("NextRetryTime attempt2=%d want 102", got)
	}
	c.Attempts = 3
	if got := NextRetryTime(c); got != 104 {
		t.Fatalf("NextRetryTime attempt3=%d want 104", got)
	}
}
