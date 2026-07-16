package analytics

import (
	"testing"
	"time"
)

func TestMedianInterval(t *testing.T) {
	if medianInterval(nil) != nil || medianInterval([]time.Time{time.Now()}) != nil {
		t.Fatal("expected nil for <2 times")
	}
	base := time.Now()
	// gaps of 60s, 120s, 180s → median 120.
	times := []time.Time{base, base.Add(60 * time.Second), base.Add(180 * time.Second), base.Add(360 * time.Second)}
	got := medianInterval(times)
	if got == nil || *got != 120 {
		t.Fatalf("median = %v, want 120", got)
	}
}

func TestActivityBuckets(t *testing.T) {
	now := time.Now()
	times := []time.Time{
		now.Add(-10 * time.Minute), // current hour (idx 5)
		now.Add(-20 * time.Minute), // current hour (idx 5)
		now.Add(-90 * time.Minute), // 1h ago (idx 4)
		now.Add(-7 * time.Hour),    // outside 6h window → dropped
	}
	b := activityBuckets(times, now, 6)
	if len(b) != 6 {
		t.Fatalf("len = %d, want 6", len(b))
	}
	if b[5] != 2 {
		t.Errorf("current-hour bucket = %d, want 2", b[5])
	}
	if b[4] != 1 {
		t.Errorf("previous-hour bucket = %d, want 1", b[4])
	}
	sum := 0
	for _, v := range b {
		sum += v
	}
	if sum != 3 {
		t.Errorf("total binned = %d, want 3 (one dropped as out-of-window)", sum)
	}
}
