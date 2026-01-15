package httpclient

import (
	"sort"
	"sync"
	"time"
)

// LatencyTracker tracks request latencies per endpoint for adaptive hedging.
//
// It maintains a sliding window of latency samples per endpoint and can
// calculate approximate percentiles (e.g., P95, P99) for hedge delay calculation.
//
// The tracker is safe for concurrent use.
type LatencyTracker struct {
	mu         sync.RWMutex
	endpoints  map[string]*latencyWindow
	windowSize int
	minSamples int
}

// latencyWindow holds a circular buffer of latency samples.
type latencyWindow struct {
	samples []time.Duration
	head    int
	count   int
}

// NewLatencyTracker creates a new latency tracker.
//
// windowSize determines how many samples are kept per endpoint.
// minSamples is the minimum number of samples required before percentile calculation.
func NewLatencyTracker(windowSize, minSamples int) *LatencyTracker {
	if windowSize <= 0 {
		windowSize = 100
	}
	if minSamples <= 0 {
		minSamples = 10
	}
	return &LatencyTracker{
		endpoints:  make(map[string]*latencyWindow),
		windowSize: windowSize,
		minSamples: minSamples,
	}
}

// Record adds a latency sample for the given endpoint.
func (t *LatencyTracker) Record(endpoint string, latency time.Duration) {
	t.mu.Lock()
	defer t.mu.Unlock()

	window, ok := t.endpoints[endpoint]
	if !ok {
		window = &latencyWindow{
			samples: make([]time.Duration, t.windowSize),
		}
		t.endpoints[endpoint] = window
	}

	window.samples[window.head] = latency
	window.head = (window.head + 1) % t.windowSize
	if window.count < t.windowSize {
		window.count++
	}
}

// Percentile returns the approximate percentile latency for an endpoint.
//
// p should be between 0 and 1 (e.g., 0.95 for P95).
// Returns false if insufficient samples are available.
func (t *LatencyTracker) Percentile(endpoint string, p float64) (time.Duration, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	window, ok := t.endpoints[endpoint]
	if !ok || window.count < t.minSamples {
		return 0, false
	}

	// Copy samples for sorting (avoid modifying circular buffer)
	samples := make([]time.Duration, window.count)
	copy(samples, window.samples[:window.count])
	sort.Slice(samples, func(i, j int) bool {
		return samples[i] < samples[j]
	})

	// Calculate percentile index
	idx := int(float64(len(samples)-1) * p)
	if idx >= len(samples) {
		idx = len(samples) - 1
	}

	return samples[idx], true
}

// Count returns the number of samples for an endpoint.
func (t *LatencyTracker) Count(endpoint string) int {
	t.mu.RLock()
	defer t.mu.RUnlock()

	window, ok := t.endpoints[endpoint]
	if !ok {
		return 0
	}
	return window.count
}

// Reset clears all tracked data.
func (t *LatencyTracker) Reset() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.endpoints = make(map[string]*latencyWindow)
}

// defaultLatencyTracker is the global tracker used when not explicitly provided.
var defaultLatencyTracker = NewLatencyTracker(100, 10)

// DefaultLatencyTracker returns the global latency tracker.
func DefaultLatencyTracker() *LatencyTracker {
	return defaultLatencyTracker
}
