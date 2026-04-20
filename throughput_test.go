package main

import (
	"testing"
	"time"
)

func TestRateWindowAvgRequiresFullWindow(t *testing.T) {
	w := &rateWindow{}
	for i := 0; i < windowSize-1; i++ {
		w.push(float64(i + 1))
		if _, ok := w.avg(); ok {
			t.Fatalf("avg returned ok after only %d pushes (need %d)", i+1, windowSize)
		}
	}
	w.push(1000)
	if _, ok := w.avg(); !ok {
		t.Fatal("avg not ok after windowSize pushes")
	}
}

func TestRateWindowAvgCorrect(t *testing.T) {
	w := &rateWindow{}
	for i := 0; i < windowSize; i++ {
		w.push(100)
	}
	avg, ok := w.avg()
	if !ok {
		t.Fatal("expected ok")
	}
	if avg != 100 {
		t.Errorf("avg = %g, want 100", avg)
	}
}

func TestRateWindowRingOverwrite(t *testing.T) {
	w := &rateWindow{}
	// fill window with 100s, then overwrite with 200s
	for i := 0; i < windowSize; i++ {
		w.push(100)
	}
	for i := 0; i < windowSize; i++ {
		w.push(200)
	}
	avg, ok := w.avg()
	if !ok {
		t.Fatal("expected ok")
	}
	if avg != 200 {
		t.Errorf("avg after overwrite = %g, want 200", avg)
	}
}

func makeInbounds(uplink, downlink int64) map[string]*trafficStats {
	return map[string]*trafficStats{
		"vless-in": {uplink: uplink, downlink: downlink},
	}
}

// fillBaseline calls Update windowSize+1 times with growing values so the
// tracker has a full baseline window of ~ratePerSec bytes/sec before the test.
func fillBaseline(tt *ThroughputTracker, ratePerSec int64) {
	const interval = 15 * time.Second
	base := time.Now().Add(-time.Duration(windowSize+2) * interval)

	var cumBytes int64
	for i := 0; i <= windowSize+1; i++ {
		cumBytes += ratePerSec * int64(interval.Seconds())
		ts := base.Add(time.Duration(i) * interval)
		inbounds := map[string]*trafficStats{
			"vless-in": {uplink: cumBytes / 2, downlink: cumBytes / 2},
		}
		tt.mu.Lock()
		tt.prev["vless-in"] = &inboundSample{
			uplink:   inbounds["vless-in"].uplink,
			downlink: inbounds["vless-in"].downlink,
			at:       ts,
		}
		tt.mu.Unlock()

		if i > 0 {
			// compute rate manually to push into window
			prev := (cumBytes - ratePerSec*int64(interval.Seconds()))
			rate := float64(cumBytes-prev) / interval.Seconds()
			tt.mu.Lock()
			if _, ok := tt.windows["vless-in"]; !ok {
				tt.windows["vless-in"] = &rateWindow{}
			}
			tt.windows["vless-in"].push(rate)
			tt.mu.Unlock()
		}
	}
}

func TestThroughputDegradationDetected(t *testing.T) {
	tt := newThroughputTracker()

	const baselineRate = int64(500 * 1024) // 500 KB/s — well above minBaselineRate

	// fill window: windowSize samples at baselineRate
	for i := 0; i < windowSize; i++ {
		tt.mu.Lock()
		if _, ok := tt.windows["vless-in"]; !ok {
			tt.windows["vless-in"] = &rateWindow{}
		}
		tt.windows["vless-in"].push(float64(baselineRate))
		tt.mu.Unlock()
	}

	// Set a prev sample so the next Update computes a delta
	before := time.Now().Add(-15 * time.Second)
	tt.mu.Lock()
	tt.prev["vless-in"] = &inboundSample{uplink: 1_000_000, downlink: 1_000_000, at: before}
	tt.mu.Unlock()

	// Inject a near-zero traffic sample — rate will be far below 30% of baseline
	tt.Update(map[string]*trafficStats{
		"vless-in": {uplink: 1_000_001, downlink: 1_000_001},
	})

	_, degradations := tt.Snapshot()
	if degradations["vless-in"] != 1 {
		t.Errorf("expected 1 degradation, got %d", degradations["vless-in"])
	}
}

func TestThroughputNoDegradationBelowMinBaseline(t *testing.T) {
	tt := newThroughputTracker()

	// baseline below minBaselineRate (50 KB/s < 100 KB/s)
	for i := 0; i < windowSize; i++ {
		tt.mu.Lock()
		if _, ok := tt.windows["vless-in"]; !ok {
			tt.windows["vless-in"] = &rateWindow{}
		}
		tt.windows["vless-in"].push(50 * 1024)
		tt.mu.Unlock()
	}

	before := time.Now().Add(-15 * time.Second)
	tt.mu.Lock()
	tt.prev["vless-in"] = &inboundSample{uplink: 1_000_000, downlink: 1_000_000, at: before}
	tt.mu.Unlock()

	tt.Update(map[string]*trafficStats{
		"vless-in": {uplink: 1_000_001, downlink: 1_000_001},
	})

	_, degradations := tt.Snapshot()
	if degradations["vless-in"] != 0 {
		t.Errorf("expected no degradation for idle inbound, got %d", degradations["vless-in"])
	}
}

func TestThroughputNegativeDeltaSkipped(t *testing.T) {
	tt := newThroughputTracker()

	before := time.Now().Add(-15 * time.Second)
	tt.mu.Lock()
	tt.prev["vless-in"] = &inboundSample{uplink: 5_000_000, downlink: 5_000_000, at: before}
	tt.mu.Unlock()

	// Counters decreased (Xray restart) — should not panic or produce a degradation
	tt.Update(map[string]*trafficStats{
		"vless-in": {uplink: 100, downlink: 100},
	})

	rates, degradations := tt.Snapshot()
	if _, ok := rates["vless-in"]; ok {
		t.Error("expected no rate entry after negative delta")
	}
	if degradations["vless-in"] != 0 {
		t.Errorf("expected no degradation after counter reset, got %d", degradations["vless-in"])
	}
}

func TestThroughputWindowNotFullNoDegradation(t *testing.T) {
	tt := newThroughputTracker()

	// Only push windowSize-1 samples — baseline not yet available
	for i := 0; i < windowSize-1; i++ {
		tt.mu.Lock()
		if _, ok := tt.windows["vless-in"]; !ok {
			tt.windows["vless-in"] = &rateWindow{}
		}
		tt.windows["vless-in"].push(500 * 1024)
		tt.mu.Unlock()
	}

	before := time.Now().Add(-15 * time.Second)
	tt.mu.Lock()
	tt.prev["vless-in"] = &inboundSample{uplink: 1_000_000, downlink: 1_000_000, at: before}
	tt.mu.Unlock()

	tt.Update(map[string]*trafficStats{
		"vless-in": {uplink: 1_000_001, downlink: 1_000_001},
	})

	_, degradations := tt.Snapshot()
	if degradations["vless-in"] != 0 {
		t.Errorf("expected no degradation while window is filling, got %d", degradations["vless-in"])
	}
}
