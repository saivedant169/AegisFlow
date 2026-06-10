package analytics

import "testing"

func TestHistogramEmpty(t *testing.T) {
	var h latencyHistogram
	if got := h.percentile(95); got != 0 {
		t.Fatalf("empty histogram p95 should be 0, got %d", got)
	}
}

// The reported percentile must never under-report the true value at that rank
// (HDR conservative tail), and must be capped at the largest value observed.
func TestHistogramConservativeTail(t *testing.T) {
	var h latencyHistogram
	for i := 0; i < 100; i++ {
		h.observe(50)
	}
	h.observe(10000) // a single outlier

	// True p99 (rank 100 of 101) is 50ms; the histogram may round up to the
	// bucket edge but must be >= 50 and not absurdly large.
	if p99 := h.percentile(99); p99 < 50 || p99 > 100 {
		t.Fatalf("p99 should be just above 50ms, got %d", p99)
	}
	// p100 includes the outlier and is capped at the observed max.
	if p100 := h.percentile(100); p100 != 10000 {
		t.Fatalf("p100 should equal the observed max 10000, got %d", p100)
	}
}

func TestHistogramMonotonic(t *testing.T) {
	var h latencyHistogram
	for i := int64(1); i <= 1000; i++ {
		h.observe(i)
	}
	p50, p95, p99 := h.percentile(50), h.percentile(95), h.percentile(99)
	if !(p50 <= p95 && p95 <= p99) {
		t.Fatalf("percentiles must be monotonic, got p50=%d p95=%d p99=%d", p50, p95, p99)
	}
	// p50 of a uniform 1..1000 spread should land near the middle, not the tail.
	if p50 < 400 || p50 > 700 {
		t.Fatalf("p50 of uniform 1..1000 should be mid-range, got %d", p50)
	}
}

// Memory stays bounded no matter how many samples are recorded.
func TestHistogramBoundedMemory(t *testing.T) {
	var h latencyHistogram
	for i := 0; i < 100000; i++ {
		h.observe(int64(i % 5000))
	}
	if want := len(latencyBounds) + 1; len(h.counts) != want {
		t.Fatalf("histogram should use %d fixed buckets, got %d", want, len(h.counts))
	}
	if h.total != 100000 {
		t.Fatalf("expected total 100000, got %d", h.total)
	}
}

func TestHistogramSingleValue(t *testing.T) {
	var h latencyHistogram
	h.observe(10000)
	for _, p := range []int{50, 95, 99, 100} {
		if got := h.percentile(p); got != 10000 {
			t.Fatalf("single-sample p%d should be the observed value 10000, got %d", p, got)
		}
	}
}
