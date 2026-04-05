package main

// TSPUCollector tails the Xray error log and counts events that indicate
// active DPI interference or probe attempts. Three counter families:
//
//   xray_handshake_failure_total{inbound}  — TLS handshake rejected by peer
//   xray_connection_reset_total{inbound}   — TCP RST received (TSPU block)
//   xray_probe_detected_total{inbound}     — unexpected/garbage data (active probe)
//
// All counters use the inbound tag extracted from the log line, or "unknown"
// when the tag is not present in that log line.

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
	patHandshakeFailure = regexp.MustCompile(`(?i)handshake.failure|tls.*remote.error.*tls|tls.*alert.*handshake`)
	patConnectionReset  = regexp.MustCompile(`(?i)connection.reset.by.peer|read:.connection.reset|wsarecv.*connection.reset`)
	patProbe            = regexp.MustCompile(`(?i)unknown.record.type|bad.record.mac|unexpected.alpn|unexpected.eof|failed.to.unpack|not.tls.handshake|invalid.header`)
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
	defer f.Close()

	// Tail from end — only new lines
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
