package main

import "testing"

func TestParseStatName(t *testing.T) {
	cases := []struct {
		name      string
		wantEmail string
		wantDir   string
	}{
		{"user>>>alice@example.com>>>traffic>>>uplink", "alice@example.com", "uplink"},
		{"user>>>bob@domain.com>>>traffic>>>downlink", "bob@domain.com", "downlink"},
		{"inbound>>>vless-reality-in>>>traffic>>>uplink", "", ""},
		{"outbound>>>freedom>>>traffic>>>downlink", "", ""},
		{"user>>>x>>>traffic>>>other", "", ""},
		{"user>>>x>>>other>>>uplink", "", ""},
		{"malformed", "", ""},
		{"user>>>only>>>three", "", ""},
	}
	for _, c := range cases {
		email, dir := parseStatName(c.name)
		if email != c.wantEmail || dir != c.wantDir {
			t.Errorf("parseStatName(%q) = (%q, %q), want (%q, %q)",
				c.name, email, dir, c.wantEmail, c.wantDir)
		}
	}
}

func TestParseInboundStatName(t *testing.T) {
	cases := []struct {
		name    string
		wantTag string
		wantDir string
	}{
		{"inbound>>>vless-reality-in>>>traffic>>>uplink", "vless-reality-in", "uplink"},
		{"inbound>>>vless-xhttp-in>>>traffic>>>downlink", "vless-xhttp-in", "downlink"},
		{"user>>>alice@example.com>>>traffic>>>uplink", "", ""},
		{"outbound>>>freedom>>>traffic>>>uplink", "", ""},
		{"inbound>>>x>>>traffic>>>other", "", ""},
		{"inbound>>>x>>>other>>>uplink", "", ""},
		{"malformed", "", ""},
		{"inbound>>>only>>>three", "", ""},
	}
	for _, c := range cases {
		tag, dir := parseInboundStatName(c.name)
		if tag != c.wantTag || dir != c.wantDir {
			t.Errorf("parseInboundStatName(%q) = (%q, %q), want (%q, %q)",
				c.name, tag, dir, c.wantTag, c.wantDir)
		}
	}
}
