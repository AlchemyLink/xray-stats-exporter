package main

// Hysteria2 Traffic Stats API source (https://v2.hysteria.network/docs/advanced/Traffic-Stats/).
//
// The hysteria server exposes a localhost HTTP API when trafficStats is configured:
//
//	GET /traffic → {"<auth id>": {"tx": N, "rx": N}}   cumulative bytes since process start
//	GET /online  → {"<auth id>": <connection count>}
//
// Requests carry "Authorization: <secret>" when trafficStats.secret is set (plain
// secret, not a Bearer scheme). We never call /traffic?clear=1 — counters must stay
// cumulative so Prometheus increase() handles process restarts as ordinary resets.
//
// With auth.type:http the <auth id> is the username Raven's auth-backend returned,
// i.e. the same email that labels xray_user_* — the two metric families join on the
// email label (after the same anonymize-secret hashing).
//
// Direction mapping follows the xray_user_* convention: hysteria rx (client→server)
// = uplink, tx (server→client) = downlink. The bytes stay a SEPARATE metric family
// (hysteria_user_*) rather than being folded into xray_user_*: the two underlying
// counters reset independently (xray restart vs hysteria restart), and a merged
// counter would partially decrease on either reset, corrupting increase() and the
// dashboard quota accounting built on it.

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

type hysteriaTraffic struct {
	Tx int64 `json:"tx"`
	Rx int64 `json:"rx"`
}

var hysteriaClient = &http.Client{Timeout: 5 * time.Second}

// hysteriaGet fetches endpoint+path and decodes the JSON object into out.
func hysteriaGet(endpoint, secret, path string, out any) error {
	req, err := http.NewRequest(http.MethodGet, endpoint+path, nil)
	if err != nil {
		return err
	}
	if secret != "" {
		req.Header.Set("Authorization", secret)
	}
	resp, err := hysteriaClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("hysteria API %s: status %d", path, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return err
	}
	return json.Unmarshal(body, out)
}

func queryHysteriaTraffic(endpoint, secret string) (map[string]hysteriaTraffic, error) {
	out := make(map[string]hysteriaTraffic)
	if err := hysteriaGet(endpoint, secret, "/traffic", &out); err != nil {
		return nil, err
	}
	return out, nil
}

func queryHysteriaOnline(endpoint, secret string) (map[string]int64, error) {
	out := make(map[string]int64)
	if err := hysteriaGet(endpoint, secret, "/online", &out); err != nil {
		return nil, err
	}
	return out, nil
}

// writeHysteriaMetrics queries the hysteria trafficStats API and writes the
// hysteria_* metric families. Called from serveMetrics when --hysteria-endpoint
// is set. API failure emits hysteria_up 0 and nothing else (mirrors xray_up).
func writeHysteriaMetrics(w io.Writer, endpoint, secret, anonSecret string) {
	traffic, trafficErr := queryHysteriaTraffic(endpoint, secret)
	online, onlineErr := queryHysteriaOnline(endpoint, secret)

	up := 1
	if trafficErr != nil && onlineErr != nil {
		up = 0
	}
	fmt.Fprintf(w, "# HELP hysteria_up 1 if the hysteria trafficStats API is reachable\n")
	fmt.Fprintf(w, "# TYPE hysteria_up gauge\n")
	fmt.Fprintf(w, "hysteria_up %d\n", up)
	if up == 0 {
		log.Printf("hysteria stats error: %v", trafficErr)
		return
	}

	if trafficErr == nil {
		fmt.Fprintf(w, "# HELP hysteria_user_uplink_bytes_total Cumulative client-to-server bytes per user via the Hysteria2 reserve (rx)\n")
		fmt.Fprintf(w, "# TYPE hysteria_user_uplink_bytes_total counter\n")
		for id, t := range traffic {
			fmt.Fprintf(w, "hysteria_user_uplink_bytes_total{email=%q} %d\n", obscureEmail(anonSecret, id), t.Rx)
		}
		fmt.Fprintf(w, "# HELP hysteria_user_downlink_bytes_total Cumulative server-to-client bytes per user via the Hysteria2 reserve (tx)\n")
		fmt.Fprintf(w, "# TYPE hysteria_user_downlink_bytes_total counter\n")
		for id, t := range traffic {
			fmt.Fprintf(w, "hysteria_user_downlink_bytes_total{email=%q} %d\n", obscureEmail(anonSecret, id), t.Tx)
		}
	} else {
		log.Printf("hysteria traffic error: %v", trafficErr)
	}

	if onlineErr == nil {
		fmt.Fprintf(w, "# HELP hysteria_user_online Current connection count per user on the Hysteria2 reserve\n")
		fmt.Fprintf(w, "# TYPE hysteria_user_online gauge\n")
		for id, n := range online {
			fmt.Fprintf(w, "hysteria_user_online{email=%q} %d\n", obscureEmail(anonSecret, id), n)
		}
	} else {
		log.Printf("hysteria online error: %v", onlineErr)
	}
}
