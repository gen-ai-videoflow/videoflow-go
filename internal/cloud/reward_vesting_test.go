package cloud

import (
	"testing"
	"time"
)

func TestRewardVestingBoundaries(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		elapsed time.Duration
		want    int64
	}{{-time.Second, 0}, {0, 0}, {15 * 24 * time.Hour, 50}, {30*24*time.Hour - time.Nanosecond, 100}, {30 * 24 * time.Hour, 101}, {60 * 24 * time.Hour, 101}} {
		got, err := VestedCents(101, start, start.Add(tc.elapsed), 30*24*time.Hour)
		if err != nil || got != tc.want {
			t.Fatalf("%v: got %d err %v want %d", tc.elapsed, got, err, tc.want)
		}
	}
	if _, err := VestedCents(-1, start, start, time.Hour); err == nil {
		t.Fatal("negative principal accepted")
	}
	if _, err := VestedCents(1, start, start, 0); err == nil {
		t.Fatal("zero duration accepted")
	}
	max := int64(^uint64(0) >> 1)
	if n, err := VestedCents(max, start, start.Add(15*24*time.Hour), 30*24*time.Hour); err != nil || n != max/2 {
		t.Fatalf("overflow: %d %v", n, err)
	}
}
