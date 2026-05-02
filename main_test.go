package main

import (
	"strings"
	"testing"
)

func TestObscureEmail(t *testing.T) {
	cases := []struct {
		name   string
		secret string
		email  string
		want   string // exact value when secret is empty (passthrough); regex of length when set
	}{
		{name: "empty secret passes email through", secret: "", email: "alice@example.com", want: "alice@example.com"},
		{name: "empty email returns empty regardless of secret", secret: "s", email: "", want: ""},
		{name: "set secret pseudonymises", secret: "shared-secret", email: "alice@example.com", want: ""}, // checked separately below
	}
	for _, c := range cases {
		got := obscureEmail(c.secret, c.email)
		if c.secret == "" || c.email == "" {
			if got != c.want {
				t.Errorf("%s: got %q, want %q", c.name, got, c.want)
			}
			continue
		}
		// With secret set: must be 16 hex chars and must NOT contain the original email.
		if len(got) != 16 {
			t.Errorf("%s: hash length = %d, want 16", c.name, len(got))
		}
		if strings.Contains(got, c.email) || strings.Contains(got, "@") {
			t.Errorf("%s: hash %q leaks email", c.name, got)
		}
	}
}

func TestObscureEmail_Deterministic(t *testing.T) {
	a := obscureEmail("k", "user@x.io")
	b := obscureEmail("k", "user@x.io")
	if a != b {
		t.Errorf("same secret+email gives different hashes: %q vs %q", a, b)
	}
}

func TestObscureEmail_DifferentSecretsDiverge(t *testing.T) {
	a := obscureEmail("secret-A", "user@x.io")
	b := obscureEmail("secret-B", "user@x.io")
	if a == b {
		t.Errorf("different secrets gave identical hash: %q", a)
	}
}

// Cross-binary symmetry: raven-dashboard's obscureEmail must produce the same
// bytes for the same (secret, email) — otherwise its PromQL queries miss every
// pseudonymised series we emit. Pin the exact contract here so a refactor on
// either side that drifts the formula fails CI immediately.
func TestObscureEmail_ContractStable(t *testing.T) {
	// python3 -c "import hashlib; print(hashlib.sha256(b'shared-secret:alice@example.com').hexdigest()[:16])"
	const want = "501032d5fcab5f1f"
	got := obscureEmail("shared-secret", "alice@example.com")
	if got != want {
		t.Errorf("contract drift: got %q, want %q (raven-dashboard's obscureEmail will desync)", got, want)
	}
}

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
