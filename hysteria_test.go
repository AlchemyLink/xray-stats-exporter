package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeHysteriaAPI serves the two trafficStats endpoints with a secret check,
// mimicking hysteria's behaviour (plain secret in Authorization, 404 on mismatch).
func fakeHysteriaAPI(t *testing.T, secret string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	auth := func(w http.ResponseWriter, r *http.Request) bool {
		if secret != "" && r.Header.Get("Authorization") != secret {
			w.WriteHeader(http.StatusNotFound)
			return false
		}
		return true
	}
	mux.HandleFunc("/traffic", func(w http.ResponseWriter, r *http.Request) {
		if !auth(w, r) {
			return
		}
		_, _ = w.Write([]byte(`{"alice@example.com":{"tx":1000,"rx":250},"bob@example.com":{"tx":0,"rx":42}}`))
	})
	mux.HandleFunc("/online", func(w http.ResponseWriter, r *http.Request) {
		if !auth(w, r) {
			return
		}
		_, _ = w.Write([]byte(`{"alice@example.com":2}`))
	})
	return httptest.NewServer(mux)
}

func TestQueryHysteriaTraffic(t *testing.T) {
	srv := fakeHysteriaAPI(t, "s3cret")
	defer srv.Close()

	got, err := queryHysteriaTraffic(srv.URL, "s3cret")
	if err != nil {
		t.Fatalf("queryHysteriaTraffic: %v", err)
	}
	if got["alice@example.com"].Tx != 1000 || got["alice@example.com"].Rx != 250 {
		t.Errorf("alice = %+v, want tx=1000 rx=250", got["alice@example.com"])
	}
	if got["bob@example.com"].Rx != 42 {
		t.Errorf("bob.rx = %d, want 42", got["bob@example.com"].Rx)
	}

	if _, err := queryHysteriaTraffic(srv.URL, "wrong"); err == nil {
		t.Error("wrong secret should return an error")
	}
}

func TestWriteHysteriaMetrics(t *testing.T) {
	srv := fakeHysteriaAPI(t, "")
	defer srv.Close()

	var sb strings.Builder
	writeHysteriaMetrics(&sb, srv.URL, "", "")
	out := sb.String()

	for _, want := range []string{
		"hysteria_up 1",
		`hysteria_user_uplink_bytes_total{email="alice@example.com"} 250`,
		`hysteria_user_downlink_bytes_total{email="alice@example.com"} 1000`,
		`hysteria_user_uplink_bytes_total{email="bob@example.com"} 42`,
		`hysteria_user_online{email="alice@example.com"} 2`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q; got:\n%s", want, out)
		}
	}
}

func TestWriteHysteriaMetrics_Anonymized(t *testing.T) {
	srv := fakeHysteriaAPI(t, "")
	defer srv.Close()

	var sb strings.Builder
	writeHysteriaMetrics(&sb, srv.URL, "", "shared-secret")
	out := sb.String()

	// Same contract as obscureEmail's pinned test vector: labels must match
	// what xray_user_* emits for the same email, so the families join in PromQL.
	if !strings.Contains(out, `hysteria_user_uplink_bytes_total{email="501032d5fcab5f1f"} 250`) {
		t.Errorf("anonymized alice label missing; got:\n%s", out)
	}
	if strings.Contains(out, "alice@example.com") {
		t.Errorf("raw email leaked with anonymize secret set:\n%s", out)
	}
}

func TestWriteHysteriaMetrics_APIDown(t *testing.T) {
	var sb strings.Builder
	writeHysteriaMetrics(&sb, "http://127.0.0.1:1", "", "")
	out := sb.String()
	if !strings.Contains(out, "hysteria_up 0") {
		t.Errorf("unreachable API should emit hysteria_up 0; got:\n%s", out)
	}
	if strings.Contains(out, "hysteria_user_") {
		t.Errorf("unreachable API should emit no user series; got:\n%s", out)
	}
}
