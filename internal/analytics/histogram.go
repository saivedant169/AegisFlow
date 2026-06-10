package analytics

import (
	"math"
	"sort"
)

// latencyBounds are the inclusive upper bounds (in ms) of the latency
// histogram buckets, with a final implicit overflow bucket for anything larger.
// They grow ~1.6x per step (Fibonacci-like) so relative resolution stays roughly
// constant from sub-millisecond to multi-minute latencies — an HDR-style layout.
var latencyBounds = []int64{
	1, 2, 3, 5, 8, 13, 21, 34, 55, 89,
	144, 233, 377, 610, 987, 1597, 2584, 4181, 6765, 10946,
	17711, 28657, 46368, 75025, 121393, 196418, 317811,
}

// latencyHistogram is a fixed-bucket histogram of request latencies. Recording
// is O(1) and uses bounded memory regardless of request volume, replacing a
// per-bucket slice of every raw latency that had to be copied and sorted on
// every read.
type latencyHistogram struct {
	counts []int64 // len(latencyBounds)+1; final entry is the overflow bucket
	total  int64
	max    int64
}

// observe records one latency sample.
func (h *latencyHistogram) observe(ms int64) {
	if h.counts == nil {
		h.counts = make([]int64, len(latencyBounds)+1)
	}
	idx := sort.Search(len(latencyBounds), func(i int) bool { return latencyBounds[i] >= ms })
	h.counts[idx]++
	h.total++
	if ms > h.max {
		h.max = ms
	}
}

// percentile returns the p-th percentile (0-100) as the upper bound of the
// bucket the percentile falls in, capped at the largest value actually seen.
// Reporting the bucket's upper edge is the conservative HDR choice: it never
// under-reports tail latency. O(number of buckets), no sorting.
func (h *latencyHistogram) percentile(p int) int64 {
	if h.total == 0 {
		return 0
	}
	target := int64(math.Ceil(float64(p) / 100 * float64(h.total)))
	if target < 1 {
		target = 1
	}
	var cum int64
	for i, c := range h.counts {
		cum += c
		if cum >= target {
			rep := h.max
			if i < len(latencyBounds) && latencyBounds[i] < rep {
				rep = latencyBounds[i]
			}
			return rep
		}
	}
	return h.max
}
