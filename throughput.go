package main

// ThroughputTracker detects per-inbound throughput degradation by comparing
// each scrape's bytes/sec against a rolling baseline of recent scrapes.
//
// Metrics produced:
//   xray_inbound_throughput_bytes_per_second{inbound} — current rate (gauge)
//   xray_throughput_degradation_total{inbound}        — events where rate < 30% of baseline
//
// Detection logic:
//   1. On each scrape, compute rate = deltaBytes / deltaTime for each inbound.
//   2. Compare rate against rolling average of the last windowSize samples.
//   3. If rate < degradationRatio * avg AND avg > minBaselineRate → increment counter.
//   4. Push rate into window (baseline does not include current sample).
//
// False positive mitigations:
//   - minBaselineRate: skip inbounds with low baseline (idle, no active users).
//   - windowSize: short spikes don't shift baseline fast enough to mask a block.
//   - Negative delta (Xray restart): treated as rate=0, not counted as degradation.

import (
	"sync"
	"time"
)

const (
	// windowSize is the number of scrapes used to compute the baseline average.
	// At a 15s scrape interval this is ~2.5 minutes of history.
	windowSize = 10

	// degradationRatio: trigger when current rate drops below this fraction of baseline.
	// 0.3 = 70% drop required — avoids false positives on normal usage valleys.
	degradationRatio = 0.3

	// minBaselineRate is the minimum baseline (bytes/sec) to bother checking.
	// Inbounds below this are considered idle. 100 KB/s ≈ 1 active VPN user.
	minBaselineRate = 100 * 1024
)

type rateWindow struct {
	rates [windowSize]float64
	count int // samples filled so far (capped at windowSize)
	next  int // ring buffer write index
}

func (w *rateWindow) push(rate float64) {
	w.rates[w.next] = rate
	w.next = (w.next + 1) % windowSize
	if w.count < windowSize {
		w.count++
	}
}

// avg returns the mean rate and whether the window is full.
// Returns (0, false) until windowSize samples have been collected.
func (w *rateWindow) avg() (float64, bool) {
	if w.count < windowSize {
		return 0, false
	}
	var sum float64
	for _, r := range w.rates {
		sum += r
	}
	return sum / windowSize, true
}

type inboundSample struct {
	uplink   int64
	downlink int64
	at       time.Time
}

// ThroughputTracker maintains per-inbound rate history and degradation counters.
type ThroughputTracker struct {
	mu           sync.Mutex
	prev         map[string]*inboundSample
	windows      map[string]*rateWindow
	degradations map[string]int64
	currentRates map[string]float64
}

func newThroughputTracker() *ThroughputTracker {
	return &ThroughputTracker{
		prev:         make(map[string]*inboundSample),
		windows:      make(map[string]*rateWindow),
		degradations: make(map[string]int64),
		currentRates: make(map[string]float64),
	}
}

// Update processes a new scrape's inbound traffic totals.
// Must be called once per scrape, after collecting inbound stats from gRPC.
func (t *ThroughputTracker) Update(inbounds map[string]*trafficStats) {
	now := time.Now()
	t.mu.Lock()
	defer t.mu.Unlock()

	for tag, stats := range inbounds {
		prev, hasPrev := t.prev[tag]
		t.prev[tag] = &inboundSample{
			uplink:   stats.uplink,
			downlink: stats.downlink,
			at:       now,
		}

		if !hasPrev {
			continue
		}

		dt := now.Sub(prev.at).Seconds()
		if dt < 0.5 {
			continue
		}

		delta := (stats.uplink + stats.downlink) - (prev.uplink + prev.downlink)
		if delta < 0 {
			// Xray restarted — counters reset. Skip this sample.
			continue
		}

		rate := float64(delta) / dt
		t.currentRates[tag] = rate

		if _, ok := t.windows[tag]; !ok {
			t.windows[tag] = &rateWindow{}
		}
		w := t.windows[tag]

		// Check against historical baseline BEFORE pushing current rate.
		baseline, hasBaseline := w.avg()
		w.push(rate)

		if !hasBaseline || baseline < minBaselineRate {
			continue
		}
		if rate < baseline*degradationRatio {
			t.degradations[tag]++
		}
	}
}

// Snapshot returns copies of current rates and degradation counters.
func (t *ThroughputTracker) Snapshot() (rates map[string]float64, degradations map[string]int64) {
	t.mu.Lock()
	defer t.mu.Unlock()

	ratesCopy := make(map[string]float64, len(t.currentRates))
	for k, v := range t.currentRates {
		ratesCopy[k] = v
	}
	degCopy := make(map[string]int64, len(t.degradations))
	for k, v := range t.degradations {
		degCopy[k] = v
	}
	return ratesCopy, degCopy
}
