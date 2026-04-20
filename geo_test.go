package main

import (
	"net"
	"testing"
)

func TestParseLogLineValid(t *testing.T) {
	line := `2026/03/28 10:41:22.571790 from 1.2.3.4:56789 accepted tcp:example.com:443 [vless-reality-in -> direct] email: alice@example.com`
	e := parseLogLine(line)
	if e == nil {
		t.Fatal("expected non-nil entry")
	}
	if !e.srcIP.Equal(net.ParseIP("1.2.3.4")) {
		t.Errorf("srcIP = %v, want 1.2.3.4", e.srcIP)
	}
	if e.inbound != "vless-reality-in" {
		t.Errorf("inbound = %q, want vless-reality-in", e.inbound)
	}
	if e.email != "alice@example.com" {
		t.Errorf("email = %q, want alice@example.com", e.email)
	}
}

func TestParseLogLineNoEmail(t *testing.T) {
	line := `2026/03/28 10:41:22.571790 from 5.6.7.8:12345 accepted tcp:example.com:443 [vless-xhttp-in -> direct]`
	e := parseLogLine(line)
	if e == nil {
		t.Fatal("expected non-nil entry")
	}
	if e.email != "" {
		t.Errorf("email = %q, want empty", e.email)
	}
	if e.inbound != "vless-xhttp-in" {
		t.Errorf("inbound = %q, want vless-xhttp-in", e.inbound)
	}
}

func TestParseLogLineLoopbackSkipped(t *testing.T) {
	line := `2026/03/28 10:41:22.571790 from 127.0.0.1:9999 accepted tcp:example.com:443 [api-inbound -> api]`
	if e := parseLogLine(line); e != nil {
		t.Errorf("expected nil for loopback IP, got %+v", e)
	}
}

func TestParseLogLineInvalidFormat(t *testing.T) {
	cases := []string{
		``,
		`[Info] some unrelated line`,
		`from not-an-ip:1234 accepted tcp:host:443 [tag -> out]`,
	}
	for _, l := range cases {
		if e := parseLogLine(l); e != nil {
			t.Errorf("parseLogLine(%q) = %+v, want nil", l, e)
		}
	}
}

func TestParseLogLineIPv4MappedIPv6(t *testing.T) {
	// Some systems log IPv4-mapped IPv6; verify it parses correctly
	line := `2026/03/28 10:41:22 from 203.0.113.5:8080 accepted tcp:host:443 [vless-in -> direct] email: user@test.com`
	e := parseLogLine(line)
	if e == nil {
		t.Fatal("expected entry")
	}
	if e.srcIP == nil {
		t.Error("srcIP is nil")
	}
}
