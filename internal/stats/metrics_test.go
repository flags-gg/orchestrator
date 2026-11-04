package stats

import (
	"testing"
	"time"
)

func TestLatencyBucketAndPercentile(t *testing.T) {
	if b := latencyBucket(3 * time.Millisecond); b != 1 {
		t.Errorf("3ms bucket = %d, want 1", b)
	}
	if b := latencyBucket(2 * time.Second); b != 9 {
		t.Errorf("2s bucket = %d, want 9 (overflow)", b)
	}
	// 90 fast requests (<=10ms), 10 slow (<=250ms)
	buckets := []int{0, 90, 0, 0, 0, 10, 0, 0, 0}
	if p := percentile(buckets, 0.5); p != 10 {
		t.Errorf("p50 = %d, want 10", p)
	}
	if p := percentile(buckets, 0.95); p != 250 {
		t.Errorf("p95 = %d, want 250", p)
	}
	if p := percentile([]int{0, 0, 0, 0, 0, 0, 0, 0, 5}, 0.5); p != 1001 {
		t.Errorf("overflow p50 = %d, want 1001", p)
	}
	if p := percentile(nil, 0.5); p != 0 {
		t.Errorf("empty p50 = %d, want 0", p)
	}
}
