// xray-stats-exporter: Prometheus exporter for per-user and per-inbound traffic via Xray StatsService gRPC API.
//
// Metrics exposed:
//   xray_user_uplink_bytes_total{email="..."}       — cumulative uplink bytes per user
//   xray_user_downlink_bytes_total{email="..."}     — cumulative downlink bytes per user
//   xray_inbound_uplink_bytes_total{inbound="..."}  — cumulative uplink bytes per inbound
//   xray_inbound_downlink_bytes_total{inbound="..."} — cumulative downlink bytes per inbound
//   xray_scrape_duration_seconds                    — scrape latency
//   xray_up                                         — 1 if xray API reachable, 0 otherwise
//
// Usage:
//   xray-stats-exporter [--listen=127.0.0.1:9551] [--xray-endpoint=127.0.0.1:10085]
//                       [--metrics-path=/metrics]
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	statscommand "github.com/xtls/xray-core/app/stats/command"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var (
	listen       = flag.String("listen", "127.0.0.1:9551", "Listen address")
	metricsPath  = flag.String("metrics-path", "/metrics", "HTTP path for metrics")
	xrayEndpoint = flag.String("xray-endpoint", "127.0.0.1:10085", "Xray gRPC API address")
)

const dialTimeout = 5 * time.Second

// queryStats calls Xray StatsService.QueryStats with the given pattern and returns raw stats.
func queryStats(endpoint, pattern string) ([]*statscommand.Stat, error) {
	ctx, cancel := context.WithTimeout(context.Background(), dialTimeout)
	defer cancel()

	conn, err := grpc.NewClient(endpoint, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("dial: %w", err)
	}
	defer func() { _ = conn.Close() }()

	client := statscommand.NewStatsServiceClient(conn)
	resp, err := client.QueryStats(ctx, &statscommand.QueryStatsRequest{
		Pattern: pattern,
		Reset_:  false,
	})
	if err != nil {
		return nil, fmt.Errorf("QueryStats: %w", err)
	}
	return resp.GetStat(), nil
}

// parseUserStatName parses "user>>>email@domain>>>traffic>>>uplink" → (email, direction).
// Returns ("", "") if not a user traffic stat.
func parseUserStatName(name string) (email, direction string) {
	// format: user>>>EMAIL>>>traffic>>>uplink|downlink
	parts := strings.Split(name, ">>>")
	if len(parts) != 4 || parts[0] != "user" || parts[2] != "traffic" {
		return "", ""
	}
	dir := parts[3]
	if dir != "uplink" && dir != "downlink" {
		return "", ""
	}
	return parts[1], dir
}

// parseStatName is an alias kept for backward compatibility with tests.
func parseStatName(name string) (email, direction string) {
	return parseUserStatName(name)
}

// parseInboundStatName parses "inbound>>>tag>>>traffic>>>uplink" → (tag, direction).
// Returns ("", "") if not an inbound traffic stat.
func parseInboundStatName(name string) (tag, direction string) {
	// format: inbound>>>TAG>>>traffic>>>uplink|downlink
	parts := strings.Split(name, ">>>")
	if len(parts) != 4 || parts[0] != "inbound" || parts[2] != "traffic" {
		return "", ""
	}
	dir := parts[3]
	if dir != "uplink" && dir != "downlink" {
		return "", ""
	}
	return parts[1], dir
}

func serveMetrics(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	userStats, userErr := queryStats(*xrayEndpoint, "user>>>")
	inboundStats, inboundErr := queryStats(*xrayEndpoint, "inbound>>>")

	elapsed := time.Since(start).Seconds()

	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")

	// If both queries failed, report xray as down
	if userErr != nil && inboundErr != nil {
		log.Printf("scrape error: %v", userErr)
		fmt.Fprintf(w, "# HELP xray_up 1 if Xray API is reachable\n")
		fmt.Fprintf(w, "# TYPE xray_up gauge\n")
		fmt.Fprintf(w, "xray_up 0\n")
		fmt.Fprintf(w, "# HELP xray_scrape_duration_seconds Duration of last scrape\n")
		fmt.Fprintf(w, "# TYPE xray_scrape_duration_seconds gauge\n")
		fmt.Fprintf(w, "xray_scrape_duration_seconds %g\n", elapsed)
		return
	}

	// Collect per-user values
	type trafficStats struct {
		uplink   int64
		downlink int64
	}

	users := make(map[string]*trafficStats)
	if userErr == nil {
		for _, s := range userStats {
			email, dir := parseUserStatName(s.GetName())
			if email == "" {
				continue
			}
			if _, ok := users[email]; !ok {
				users[email] = &trafficStats{}
			}
			val := s.GetValue()
			switch dir {
			case "uplink":
				users[email].uplink = val
			case "downlink":
				users[email].downlink = val
			}
		}
	} else {
		log.Printf("user stats error: %v", userErr)
	}

	// Collect per-inbound values
	inbounds := make(map[string]*trafficStats)
	if inboundErr == nil {
		for _, s := range inboundStats {
			tag, dir := parseInboundStatName(s.GetName())
			if tag == "" {
				continue
			}
			if _, ok := inbounds[tag]; !ok {
				inbounds[tag] = &trafficStats{}
			}
			val := s.GetValue()
			switch dir {
			case "uplink":
				inbounds[tag].uplink = val
			case "downlink":
				inbounds[tag].downlink = val
			}
		}
	} else {
		log.Printf("inbound stats error: %v", inboundErr)
	}

	// Write per-user metrics
	fmt.Fprintf(w, "# HELP xray_user_uplink_bytes_total Cumulative uplink bytes per user\n")
	fmt.Fprintf(w, "# TYPE xray_user_uplink_bytes_total counter\n")
	for email, u := range users {
		fmt.Fprintf(w, "xray_user_uplink_bytes_total{email=%q} %d\n", email, u.uplink)
	}

	fmt.Fprintf(w, "# HELP xray_user_downlink_bytes_total Cumulative downlink bytes per user\n")
	fmt.Fprintf(w, "# TYPE xray_user_downlink_bytes_total counter\n")
	for email, u := range users {
		fmt.Fprintf(w, "xray_user_downlink_bytes_total{email=%q} %d\n", email, u.downlink)
	}

	// Write per-inbound metrics
	fmt.Fprintf(w, "# HELP xray_inbound_uplink_bytes_total Cumulative uplink bytes per inbound\n")
	fmt.Fprintf(w, "# TYPE xray_inbound_uplink_bytes_total counter\n")
	for tag, ib := range inbounds {
		fmt.Fprintf(w, "xray_inbound_uplink_bytes_total{inbound=%q} %d\n", tag, ib.uplink)
	}

	fmt.Fprintf(w, "# HELP xray_inbound_downlink_bytes_total Cumulative downlink bytes per inbound\n")
	fmt.Fprintf(w, "# TYPE xray_inbound_downlink_bytes_total counter\n")
	for tag, ib := range inbounds {
		fmt.Fprintf(w, "xray_inbound_downlink_bytes_total{inbound=%q} %d\n", tag, ib.downlink)
	}

	fmt.Fprintf(w, "# HELP xray_up 1 if Xray API is reachable\n")
	fmt.Fprintf(w, "# TYPE xray_up gauge\n")
	fmt.Fprintf(w, "xray_up 1\n")

	fmt.Fprintf(w, "# HELP xray_scrape_duration_seconds Duration of last scrape\n")
	fmt.Fprintf(w, "# TYPE xray_scrape_duration_seconds gauge\n")
	fmt.Fprintf(w, "xray_scrape_duration_seconds %g\n", elapsed)
}

func main() {
	flag.Parse()

	mux := http.NewServeMux()
	mux.HandleFunc(*metricsPath, serveMetrics)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `<html><body><a href="%s">Metrics</a></body></html>`, *metricsPath)
	})

	log.Printf("xray-stats-exporter listening on %s, metrics at %s", *listen, *metricsPath)
	log.Printf("Xray API endpoint: %s", *xrayEndpoint)

	if err := http.ListenAndServe(*listen, mux); err != nil {
		log.Fatalf("listen: %v", err)
	}
}
