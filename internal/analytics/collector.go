package analytics

import (
	"math"
	"sort"
	"sync"
	"time"
)

// DataPoint is recorded by the gateway after each request.
type DataPoint struct {
	TenantID      string
	Model         string
	Provider      string
	StatusCode    int
	LatencyMs     int64
	Tokens        int64
	EstimatedCost float64
	Timestamp     time.Time
}

// MetricBucket holds aggregated metrics for a 1-minute window.
type MetricBucket struct {
	Timestamp     time.Time
	Requests      int64
	Errors        int64
	Latencies     []int64
	Tokens        int64
	EstimatedCost float64
}

// BucketSummary is a computed snapshot of a bucket (no raw latencies).
type BucketSummary struct {
	Timestamp     time.Time `json:"timestamp"`
	Requests      int64     `json:"requests"`
	Errors        int64     `json:"errors"`
	ErrorRate     float64   `json:"error_rate"`
	P50Latency    int64     `json:"p50_latency"`
	P95Latency    int64     `json:"p95_latency"`
	P99Latency    int64     `json:"p99_latency"`
	Tokens        int64     `json:"tokens"`
	EstimatedCost float64   `json:"estimated_cost"`
}

// TimeSeries is a ring buffer of 1-minute metric buckets for a single dimension.
type TimeSeries struct {
	mu      sync.RWMutex
	buckets []MetricBucket
	size    int
	pos     int
	count   int
}

func NewTimeSeries(retentionMinutes int) *TimeSeries {
	return &TimeSeries{
		buckets: make([]MetricBucket, retentionMinutes),
		size:    retentionMinutes,
	}
}

func (ts *TimeSeries) Record(dp DataPoint) {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	bucketTime := dp.Timestamp.Truncate(time.Minute)
	// Check if current bucket matches
	if ts.count > 0 {
		currentIdx := (ts.pos - 1 + ts.size) % ts.size
		if ts.buckets[currentIdx].Timestamp.Equal(bucketTime) {
			b := &ts.buckets[currentIdx]
			b.Requests++
			if dp.StatusCode >= 500 {
				b.Errors++
			}
			b.Latencies = append(b.Latencies, dp.LatencyMs)
			b.Tokens += dp.Tokens
			b.EstimatedCost += dp.EstimatedCost
			return
		}
	}

	// New bucket
	ts.buckets[ts.pos] = MetricBucket{
		Timestamp:     bucketTime,
		Requests:      1,
		Errors:        boolToInt64(dp.StatusCode >= 500),
		Latencies:     []int64{dp.LatencyMs},
		Tokens:        dp.Tokens,
		EstimatedCost: dp.EstimatedCost,
	}
	ts.pos = (ts.pos + 1) % ts.size
	if ts.count < ts.size {
		ts.count++
	}
}

func (ts *TimeSeries) RecentBuckets(n int) []BucketSummary {
	ts.mu.RLock()
	defer ts.mu.RUnlock()

	if n > ts.count {
		n = ts.count
	}
	result := make([]BucketSummary, 0, n)
	for i := 0; i < n; i++ {
		idx := (ts.pos - n + i + ts.size) % ts.size
		b := ts.buckets[idx]
		result = append(result, summarize(b))
	}
	return result
}

func (ts *TimeSeries) AllBuckets() []BucketSummary {
	return ts.RecentBuckets(ts.count)
}

func summarize(b MetricBucket) BucketSummary {
	s := BucketSummary{
		Timestamp:     b.Timestamp,
		Requests:      b.Requests,
		Errors:        b.Errors,
		Tokens:        b.Tokens,
		EstimatedCost: b.EstimatedCost,
	}
	if b.Requests > 0 {
		s.ErrorRate = float64(b.Errors) / float64(b.Requests) * 100
	}
	if len(b.Latencies) > 0 {
		sorted := make([]int64, len(b.Latencies))
		copy(sorted, b.Latencies)
		sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
		s.P50Latency = percentile(sorted, 50)
		s.P95Latency = percentile(sorted, 95)
		s.P99Latency = percentile(sorted, 99)
	}
	return s
}

func percentile(sorted []int64, p int) int64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(math.Ceil(float64(p)/100*float64(len(sorted)))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

func boolToInt64(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

// Collector manages time-series data for all dimensions.
type Collector struct {
	mu        sync.RWMutex
	series    map[string]*TimeSeries
	retention int // minutes
}

func NewCollector(retentionHours int) *Collector {
	return &Collector{
		series:    make(map[string]*TimeSeries),
		retention: retentionHours * 60,
	}
}

func (c *Collector) Record(dp DataPoint) {
	if dp.Timestamp.IsZero() {
		dp.Timestamp = time.Now()
	}
	dims := []string{
		"tenant:" + dp.TenantID,
		"model:" + dp.Model,
		"provider:" + dp.Provider,
		"global",
	}

	// Collect all TimeSeries references under a single lock, then record outside.
	// This avoids unlock/relock inside the loop which could race with map mutations.
	c.mu.Lock()
	targets := make([]*TimeSeries, len(dims))
	for i, dim := range dims {
		ts, ok := c.series[dim]
		if !ok {
			ts = NewTimeSeries(c.retention)
			c.series[dim] = ts
		}
		targets[i] = ts
	}
	c.mu.Unlock()

	for _, ts := range targets {
		ts.Record(dp)
	}
}

func (c *Collector) GetSeries(dimension string) *TimeSeries {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.series[dimension]
}

func (c *Collector) Dimensions() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	dims := make([]string, 0, len(c.series))
	for k := range c.series {
		dims = append(dims, k)
	}
	return dims
}

// RealtimeSummary returns the last 5 minutes of data for all dimensions.
func (c *Collector) RealtimeSummary() map[string][]BucketSummary {
	c.mu.RLock()
	defer c.mu.RUnlock()
	result := make(map[string][]BucketSummary)
	for dim, ts := range c.series {
		result[dim] = ts.RecentBuckets(5)
	}
	return result
}
