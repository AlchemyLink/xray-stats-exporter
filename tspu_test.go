package main

import "testing"

func TestExtractInbound(t *testing.T) {
	cases := []struct {
		line string
		want string
	}{
		{
			// explicit [tag=X] format
			`[Warning] [123] proxy/vless/inbound: failed [tag=vless-reality-in]`,
			"vless-reality-in",
		},
		{
			// fallback: inbound/tag: pattern
			`[Warning] proxy/vless-xhttp-in: connection reset by peer`,
			"vless-xhttp-in",
		},
		{
			// fallback: proxy/tag: pattern (no [tag=] present)
			`[Warning] proxy/vless-reality-in: unexpected eof`,
			"vless-reality-in",
		},
		{
			// no inbound info
			`[Info] some unrelated log line`,
			"unknown",
		},
		{
			// [tag=] takes precedence over fallback
			`[Warning] proxy/vless-xhttp-in: failed [tag=vless-reality-in]`,
			"vless-reality-in",
		},
	}
	for _, c := range cases {
		got := extractInbound(c.line)
		if got != c.want {
			t.Errorf("extractInbound(%q) = %q, want %q", c.line, got, c.want)
		}
	}
}

func TestProcessLineHandshakeFailure(t *testing.T) {
	tc := &TSPUCollector{
		handshakeFailures: make(map[string]int64),
		connectionResets:  make(map[string]int64),
		probesDetected:    make(map[string]int64),
	}

	lines := []string{
		`[Warning] tls: handshake failure [tag=vless-in]`,
		`[Warning] tls remote error: tls handshake alert [tag=vless-in]`,
		`[Warning] tls alert: handshake_failure [tag=other-in]`,
		`[Warning] tls timeout waiting for ClientHello [tag=vless-in]`,
	}
	for _, l := range lines {
		tc.processLine(l)
	}

	if tc.handshakeFailures["vless-in"] != 3 {
		t.Errorf("handshakeFailures[vless-in] = %d, want 3", tc.handshakeFailures["vless-in"])
	}
	if tc.handshakeFailures["other-in"] != 1 {
		t.Errorf("handshakeFailures[other-in] = %d, want 1", tc.handshakeFailures["other-in"])
	}
	if len(tc.connectionResets) != 0 || len(tc.probesDetected) != 0 {
		t.Error("unexpected entries in other buckets")
	}
}

func TestProcessLineConnectionReset(t *testing.T) {
	tc := &TSPUCollector{
		handshakeFailures: make(map[string]int64),
		connectionResets:  make(map[string]int64),
		probesDetected:    make(map[string]int64),
	}

	lines := []string{
		`[Warning] read: connection reset by peer [tag=vless-in]`,
		`[Warning] write: broken pipe [tag=vless-in]`,
		`[Warning] wsarecv: connection reset by peer [tag=vless-in]`,
	}
	for _, l := range lines {
		tc.processLine(l)
	}

	if tc.connectionResets["vless-in"] != 3 {
		t.Errorf("connectionResets[vless-in] = %d, want 3", tc.connectionResets["vless-in"])
	}
}

func TestProcessLineProbeDetected(t *testing.T) {
	tc := &TSPUCollector{
		handshakeFailures: make(map[string]int64),
		connectionResets:  make(map[string]int64),
		probesDetected:    make(map[string]int64),
	}

	lines := []string{
		`[Warning] unknown record type [tag=vless-in]`,
		`[Warning] bad record mac [tag=vless-in]`,
		`[Warning] i/o timeout [tag=vless-in]`,
		`[Warning] context deadline exceeded [tag=vless-in]`,
		`[Warning] unexpected eof [tag=vless-in]`,
	}
	for _, l := range lines {
		tc.processLine(l)
	}

	if tc.probesDetected["vless-in"] != 5 {
		t.Errorf("probesDetected[vless-in] = %d, want 5", tc.probesDetected["vless-in"])
	}
}

func TestProcessLineNoMatch(t *testing.T) {
	tc := &TSPUCollector{
		handshakeFailures: make(map[string]int64),
		connectionResets:  make(map[string]int64),
		probesDetected:    make(map[string]int64),
	}

	tc.processLine(`[Info] accepted tcp:example.com:443 from 1.2.3.4:1234`)
	tc.processLine(`[Debug] routing: match rule #1`)

	if len(tc.handshakeFailures) != 0 || len(tc.connectionResets) != 0 || len(tc.probesDetected) != 0 {
		t.Error("expected no counters incremented for non-matching lines")
	}
}

func TestEnsureInboundsSeedsZeros(t *testing.T) {
	tc := &TSPUCollector{
		handshakeFailures: make(map[string]int64),
		connectionResets:  make(map[string]int64),
		probesDetected:    make(map[string]int64),
	}

	tc.EnsureInbounds([]string{"vless-reality-in", "vless-xhttp-in"})

	for _, tag := range []string{"vless-reality-in", "vless-xhttp-in"} {
		if v, ok := tc.handshakeFailures[tag]; !ok || v != 0 {
			t.Errorf("handshakeFailures[%q] = (%d, %v), want (0, true)", tag, v, ok)
		}
		if v, ok := tc.connectionResets[tag]; !ok || v != 0 {
			t.Errorf("connectionResets[%q] = (%d, %v), want (0, true)", tag, v, ok)
		}
		if v, ok := tc.probesDetected[tag]; !ok || v != 0 {
			t.Errorf("probesDetected[%q] = (%d, %v), want (0, true)", tag, v, ok)
		}
	}
}

func TestEnsureInboundsPreservesExistingCounts(t *testing.T) {
	tc := &TSPUCollector{
		handshakeFailures: map[string]int64{"vless-reality-in": 7},
		connectionResets:  map[string]int64{"vless-reality-in": 3},
		probesDetected:    make(map[string]int64),
	}

	tc.EnsureInbounds([]string{"vless-reality-in", "vless-xhttp-in"})

	if tc.handshakeFailures["vless-reality-in"] != 7 {
		t.Errorf("existing handshakeFailures count overwritten: got %d, want 7",
			tc.handshakeFailures["vless-reality-in"])
	}
	if tc.connectionResets["vless-reality-in"] != 3 {
		t.Errorf("existing connectionResets count overwritten: got %d, want 3",
			tc.connectionResets["vless-reality-in"])
	}
	if tc.probesDetected["vless-xhttp-in"] != 0 {
		t.Errorf("new tag not seeded in probesDetected")
	}
}

func TestEnsureInboundsEmptyInputIsNoOp(t *testing.T) {
	tc := &TSPUCollector{
		handshakeFailures: map[string]int64{"vless-reality-in": 5},
		connectionResets:  make(map[string]int64),
		probesDetected:    make(map[string]int64),
	}

	tc.EnsureInbounds(nil)
	tc.EnsureInbounds([]string{})

	if len(tc.handshakeFailures) != 1 || tc.handshakeFailures["vless-reality-in"] != 5 {
		t.Errorf("empty EnsureInbounds mutated state: %+v", tc.handshakeFailures)
	}
}

func TestSnapshotAfterEnsureInboundsEmitsZeroedSeries(t *testing.T) {
	tc := &TSPUCollector{
		handshakeFailures: make(map[string]int64),
		connectionResets:  make(map[string]int64),
		probesDetected:    make(map[string]int64),
	}

	tc.EnsureInbounds([]string{"vless-reality-in"})
	h, r, p := tc.snapshot()

	if _, ok := h[" vless-reality-in"]; ok {
		t.Fatal("unexpected key with leading space")
	}
	if len(h) != 1 || len(r) != 1 || len(p) != 1 {
		t.Errorf("snapshot sizes after seed: h=%d r=%d p=%d, want 1 each", len(h), len(r), len(p))
	}
}

func TestProcessLinePriorityHandshakeOverReset(t *testing.T) {
	// A line containing both handshake and reset keywords hits the first matching branch.
	tc := &TSPUCollector{
		handshakeFailures: make(map[string]int64),
		connectionResets:  make(map[string]int64),
		probesDetected:    make(map[string]int64),
	}

	tc.processLine(`[Warning] tls handshake failure connection reset by peer [tag=vless-in]`)

	if tc.handshakeFailures["vless-in"] != 1 {
		t.Errorf("expected handshake bucket, got handshake=%d reset=%d",
			tc.handshakeFailures["vless-in"], tc.connectionResets["vless-in"])
	}
	if tc.connectionResets["vless-in"] != 0 {
		t.Error("reset bucket should not be incremented when handshake matches first")
	}
}
