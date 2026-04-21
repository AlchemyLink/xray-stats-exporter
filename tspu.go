package main

// TSPUCollector tails the Xray error log and counts events that indicate
// active DPI interference or probe attempts. Three counter families:
//
//   xray_handshake_failure_total{inbound}  — TLS handshake rejected by peer
//   xray_connection_reset_total{inbound}   — TCP RST / forced close (TSPU block)
//   xray_probe_detected_total{inbound}     — unexpected/garbage data or timeout (active probe)
//
// All counters use the inbound tag extracted from the log line, or "unknown"
// when the tag is not present in that log line.
//
// Log rotation: on each poll the file inode is compared to the original.
// If the file was replaced (logrotate), processLog returns and tailLog reopens.

import (
	"bufio"
	"log"
	"os"
	"regexp"
	"sync"
	"time"
)

// errorLogRe extracts the inbound tag from Xray error log lines.
// Example: [Warning] [123] proxy/vless/inbound: ... [tag=vless-reality-in]
// Fallback: ... vless-reality-in: failed ...
var errorTagRe = regexp.MustCompile(`\[tag=([^\]]+)\]`)

// fallback: "tcp_inbound/vless-reality-in: ..."
var errorTagFallbackRe = regexp.MustCompile(`(?:inbound|proxy)[^:]*?/([a-zA-Z0-9_-]+):\s`)

// TSPU event patterns in Xray error log.
var (
	// TLS handshake aborted by the remote side — classic TSPU RST-during-handshake.
	patHandshakeFailure = regexp.MustCompile(`(?i)handshake.failure|tls.*remote.error.*tls|tls.*alert.*handshake|tls.*timeout`)

	// TCP RST or forced pipe close — TSPU drops the connection forcibly.
	patConnectionReset = regexp.MustCompile(`(?i)connection.reset.by.peer|read:.connection.reset|wsarecv.*connection.reset|broken.pipe|write:.broken.pipe`)

	// Active probe / DPI interference signatures:
	//   - garbage/invalid TLS record sent by TSPU prober
	//   - i/o timeout: TSPU throttles the stream to detect protocol
	//   - context deadline exceeded: firewall-induced timeout mid-session
	patProbe = regexp.MustCompile(`(?i)unknown.record.type|bad.record.mac|unexpected.alpn|unexpected.eof|failed.to.unpack|not.tls.handshake|invalid.header|i/o.timeout|context.deadline.exceeded`)
)

// TSPUCollector aggregates error-log TSPU event counters.
type TSPUCollector struct {
	mu      sync.RWMutex
	logPath string

	handshakeFailures map[string]int64 // inbound → count
	connectionResets  map[string]int64
	probesDetected    map[string]int64
}

func newTSPUCollector(logPath string) *TSPUCollector {
	t := &TSPUCollector{
		logPath:           logPath,
		handshakeFailures: make(map[string]int64),
		connectionResets:  make(map[string]int64),
		probesDetected:    make(map[string]int64),
	}
	go t.tailLog()
	return t
}

// extractInbound tries to pull an inbound tag from an error log line.
func extractInbound(line string) string {
	if m := errorTagRe.FindStringSubmatch(line); m != nil {
		return m[1]
	}
	if m := errorTagFallbackRe.FindStringSubmatch(line); m != nil {
		return m[1]
	}
	return "unknown"
}

func (t *TSPUCollector) processLine(line string) {
	var bucket map[string]int64
	switch {
	case patHandshakeFailure.MatchString(line):
		bucket = t.handshakeFailures
	case patConnectionReset.MatchString(line):
		bucket = t.connectionResets
	case patProbe.MatchString(line):
		bucket = t.probesDetected
	default:
		return
	}
	inbound := extractInbound(line)
	t.mu.Lock()
	bucket[inbound]++
	t.mu.Unlock()
}

// EnsureInbounds guarantees that each given inbound tag has a zero-valued entry
// in all three TSPU counter maps. Called by the scrape handler with the list
// of inbounds known to Xray so that Grafana renders "0 events" rather than
// "No data" on quiet inbounds that never triggered a TSPU pattern.
func (t *TSPUCollector) EnsureInbounds(tags []string) {
	if len(tags) == 0 {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, tag := range tags {
		if _, ok := t.handshakeFailures[tag]; !ok {
			t.handshakeFailures[tag] = 0
		}
		if _, ok := t.connectionResets[tag]; !ok {
			t.connectionResets[tag] = 0
		}
		if _, ok := t.probesDetected[tag]; !ok {
			t.probesDetected[tag] = 0
		}
	}
}

func (t *TSPUCollector) tailLog() {
	for {
		if err := t.processLog(); err != nil {
			log.Printf("tspu: error log tail error: %v, retrying in 10s", err)
		}
		time.Sleep(10 * time.Second)
	}
}

func (t *TSPUCollector) processLog() error {
	f, err := os.Open(t.logPath)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	// Remember the inode of the file we just opened so we can detect rotation.
	openedFi, err := f.Stat()
	if err != nil {
		return err
	}

	// Tail from end — only process new lines appended after startup.
	if _, err := f.Seek(0, 2); err != nil {
		return err
	}

	scanner := bufio.NewScanner(f)
	for {
		for scanner.Scan() {
			t.processLine(scanner.Text())
		}
		if err := scanner.Err(); err != nil {
			return err
		}
		time.Sleep(2 * time.Second)

		// Detect log rotation: if the path now points to a different inode,
		// the file was replaced by logrotate. Return nil so tailLog reopens.
		if curFi, err := os.Stat(t.logPath); err != nil || !os.SameFile(openedFi, curFi) {
			return nil
		}

		scanner = bufio.NewScanner(f)
	}
}

// snapshot returns copies of the three counters for safe metric emission.
func (t *TSPUCollector) snapshot() (handshake, resets, probes map[string]int64) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	cp := func(src map[string]int64) map[string]int64 {
		dst := make(map[string]int64, len(src))
		for k, v := range src {
			dst[k] = v
		}
		return dst
	}
	return cp(t.handshakeFailures), cp(t.connectionResets), cp(t.probesDetected)
}
