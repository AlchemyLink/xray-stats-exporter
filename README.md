# xray-stats-exporter

[![License: AGPL v3](https://img.shields.io/badge/License-AGPL_v3-blue.svg)](LICENSE)
[![Go Report Card](https://goreportcard.com/badge/github.com/alchemylink/xray-stats-exporter)](https://goreportcard.com/report/github.com/alchemylink/xray-stats-exporter)
[![CI](https://github.com/AlchemyLink/xray-stats-exporter/actions/workflows/ci.yml/badge.svg)](https://github.com/AlchemyLink/xray-stats-exporter/actions/workflows/ci.yml)
[![Status](https://img.shields.io/badge/Status-Alpha%20Testing-orange)](https://github.com/AlchemyLink/xray-stats-exporter)

Prometheus exporter for [Xray-core](https://github.com/XTLS/Xray-core) — exposes per-user and per-inbound traffic metrics, geo location data, and **TSPU/DPI interference detection** via the Xray gRPC StatsService API.

Built for self-hosted VPN operators who need to monitor user bandwidth, detect censorship events, and track throughput degradation in Grafana.

> [!WARNING]
> **Alpha testing.** Core metrics are stable, but TSPU detection patterns and throughput degradation logic are being tuned against real-world DPI events. API and flag names may change before v1.0.

---

## Table of Contents

- [Metrics](#metrics)
- [Requirements](#requirements)
- [Installation](#installation)
- [Flags](#flags)
- [Systemd Service](#systemd-service)
- [Xray Config Requirements](#xray-config-requirements)
- [TSPU / DPI Detection](#tspu--dpi-detection)
- [Throughput Degradation Detection](#throughput-degradation-detection)
- [Geo Metrics](#geo-metrics)
- [Prometheus Scrape Config](#prometheus-scrape-config)
- [Grafana Dashboard](#grafana-dashboard)
- [Related Projects](#related-projects)

---

## Metrics

### Traffic (gRPC StatsService)

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `xray_user_uplink_bytes_total` | counter | `email` | Cumulative bytes sent by user (client → server) |
| `xray_user_downlink_bytes_total` | counter | `email` | Cumulative bytes received by user (server → client) |
| `xray_inbound_uplink_bytes_total` | counter | `inbound` | Cumulative uplink bytes per inbound tag |
| `xray_inbound_downlink_bytes_total` | counter | `inbound` | Cumulative downlink bytes per inbound tag |

### Throughput (computed per scrape)

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `xray_inbound_throughput_bytes_per_second` | gauge | `inbound` | Current bytes/sec per inbound (rolling, combined up+down) |
| `xray_throughput_degradation_total` | counter | `inbound` | Scrapes where rate dropped >70% below rolling 10-sample baseline |

### TSPU / DPI Detection (error log tail)

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `xray_handshake_failure_total` | counter | `inbound` | TLS handshake failures — classic TSPU RST-during-handshake |
| `xray_connection_reset_total` | counter | `inbound` | TCP RST / broken pipe events — TSPU forcibly drops connections |
| `xray_probe_detected_total` | counter | `inbound` | Unexpected data / i/o timeout — active DPI probe signatures |

Requires `--error-log-path`. See [TSPU / DPI Detection](#tspu--dpi-detection) for details.

### Geo Location (access log tail)

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `xray_user_last_country` | gauge | `email`, `country`, `city`, `lat`, `lon` | Last seen geo location per user (gauge=1) |
| `xray_user_connections_total` | counter | `email`, `country`, `city` | Connections per user per location |
| `xray_inbound_connections_total` | counter | `inbound`, `country` | Connections per inbound per country |

Requires `--log-path` and `--geo-city-db`. See [Geo Metrics](#geo-metrics).

### Exporter Health

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `xray_up` | gauge | — | 1 if Xray gRPC API is reachable, 0 otherwise |
| `xray_scrape_duration_seconds` | gauge | — | Time taken to scrape the Xray gRPC API |

---

## Requirements

- **Xray-core** with `StatsService` enabled in the API config
- Xray gRPC API accessible at `127.0.0.1:10085` (configurable)
- Per-user stats enabled in Xray (`policy.levels` with `statsUserUplink` / `statsUserDownlink`)
- Users must have an `email` field in the inbound client config

**For TSPU detection (optional):**
- Xray `error.log` with `Warning` level or higher

**For geo metrics (optional):**
- Xray `access.log` with real client IPs (not PROXY protocol without passthrough)
- [GeoLite2-City.mmdb](https://dev.maxmind.com/geoip/geolite2-free-geolocation-data) from MaxMind (free registration required)
- Optionally: `GeoLite2-ASN.mmdb` for ASN labels

---

## Installation

### Download binary (recommended)

```bash
curl -LO https://github.com/AlchemyLink/xray-stats-exporter/releases/latest/download/xray-stats-exporter-linux-amd64
chmod +x xray-stats-exporter-linux-amd64
sudo mv xray-stats-exporter-linux-amd64 /usr/local/bin/xray-stats-exporter
```

### Build from source

```bash
git clone https://github.com/AlchemyLink/xray-stats-exporter.git
cd xray-stats-exporter
go build -o xray-stats-exporter .
sudo mv xray-stats-exporter /usr/local/bin/
```

### Run (minimal)

```bash
xray-stats-exporter \
  --listen=127.0.0.1:9551 \
  --xray-endpoint=127.0.0.1:10085
```

### Run (full — geo + TSPU)

```bash
xray-stats-exporter \
  --listen=127.0.0.1:9551 \
  --xray-endpoint=127.0.0.1:10085 \
  --log-path=/var/log/Xray/access.log \
  --error-log-path=/var/log/Xray/error.log \
  --geo-city-db=/var/lib/xray-exporter/GeoLite2-City.mmdb \
  --geo-asn-db=/var/lib/xray-exporter/GeoLite2-ASN.mmdb
```

### Verify

```bash
curl -s http://127.0.0.1:9551/metrics | grep xray_
```

---

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--listen` | `127.0.0.1:9551` | Address and port to expose metrics on |
| `--metrics-path` | `/metrics` | HTTP path for the metrics endpoint |
| `--xray-endpoint` | `127.0.0.1:10085` | Xray gRPC API address |
| `--log-path` | `""` | Path to Xray `access.log` for geo metrics (empty = disabled) |
| `--error-log-path` | `""` | Path to Xray `error.log` for TSPU detection metrics (empty = disabled) |
| `--geo-city-db` | `""` | Path to `GeoLite2-City.mmdb` (empty = geo metrics disabled) |
| `--geo-asn-db` | `""` | Path to `GeoLite2-ASN.mmdb` (empty = ASN label disabled) |

---

## Systemd Service

Create `/etc/systemd/system/xray-stats-exporter.service`:

```ini
[Unit]
Description=Xray Stats Prometheus Exporter
After=network.target xray.service

[Service]
User=nobody
Group=nogroup
ExecStart=/usr/local/bin/xray-stats-exporter \
  --listen=127.0.0.1:9551 \
  --xray-endpoint=127.0.0.1:10085 \
  --log-path=/var/log/Xray/access.log \
  --error-log-path=/var/log/Xray/error.log \
  --geo-city-db=/var/lib/xray-exporter/GeoLite2-City.mmdb \
  --geo-asn-db=/var/lib/xray-exporter/GeoLite2-ASN.mmdb
Restart=on-failure
RestartSec=5s
NoNewPrivileges=true
ProtectSystem=strict
ReadOnlyPaths=/var/log/Xray
ReadWritePaths=

[Install]
WantedBy=multi-user.target
```

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now xray-stats-exporter
sudo systemctl status xray-stats-exporter
```

---

## Xray Config Requirements

Xray must have stats and API enabled. Minimal required config fragments:

**`010-stats.json`** — enable stats collection:

```json
{
  "stats": {},
  "policy": {
    "levels": {
      "0": {
        "statsUserUplink": true,
        "statsUserDownlink": true
      }
    },
    "system": {
      "statsInboundUplink": true,
      "statsInboundDownlink": true
    }
  }
}
```

**`050-api.json`** — expose gRPC API on localhost:

```json
{
  "inbounds": [{
    "listen": "127.0.0.1",
    "port": 10085,
    "protocol": "dokodemo-door",
    "settings": {"address": "127.0.0.1"},
    "tag": "api-inbound"
  }],
  "api": {
    "tag": "api-inbound",
    "services": ["StatsService", "HandlerService", "ReflectionService"]
  }
}
```

**Users must have an `email` field** for per-user metrics:

```json
{
  "clients": [
    {"id": "uuid-here", "email": "alice@example.com", "flow": "xtls-rprx-vision"}
  ]
}
```

Without the `email` field, the user contributes to inbound totals only — no per-user metrics are emitted.

---

## TSPU / DPI Detection

When `--error-log-path` is set, the exporter tails the Xray error log in real time and classifies lines into three counter families:

| Event | Metric | What it indicates |
|-------|--------|-------------------|
| `handshake failure` / `tls alert` | `xray_handshake_failure_total` | TSPU injects RST before TLS completes |
| `connection reset by peer` / `broken pipe` | `xray_connection_reset_total` | TSPU forcibly terminates established connections |
| `unknown record type` / `i/o timeout` / `context deadline exceeded` | `xray_probe_detected_total` | Active DPI probe or firewall-induced timeout |

Each counter is labelled by `inbound` tag (extracted from `[tag=X]` in the log line, fallback to the proxy path component, or `"unknown"`).

**Alerting example** — fire when 10+ connection resets hit any inbound in 5 minutes:

```yaml
groups:
  - name: xray-tspu
    rules:
      - alert: XrayTSPUBlock
        expr: increase(xray_connection_reset_total[5m]) > 10
        for: 0m
        labels:
          severity: warning
        annotations:
          summary: "TSPU block suspected on inbound {{ $labels.inbound }}"
```

---

## Throughput Degradation Detection

The exporter computes a rolling `bytes/sec` rate per inbound on every scrape and compares it against a 10-sample baseline. A degradation event is counted when:

- Current rate < 30% of baseline average (70% drop), **and**
- Baseline average > 100 KB/s (ignores idle inbounds)

This detects the DPI throttling pattern where an active inbound suddenly drops traffic (TSPU rate-limits or drops the flow while probing), distinct from natural low-usage periods.

| Parameter | Value | Meaning |
|-----------|-------|---------|
| Window size | 10 scrapes | ~2.5 min history at 15s scrape interval |
| Degradation threshold | 30% of baseline | Triggers on ≥70% traffic drop |
| Minimum baseline | 100 KB/s | Ignores idle inbounds |

Counter resets (Xray restart) are automatically skipped — negative delta produces no sample.

---

## Geo Metrics

When `--log-path` and `--geo-city-db` are set, the exporter tails the Xray access log and resolves source IPs against GeoLite2 databases:

```
2026/03/28 10:41:22 from 1.2.3.4:56789 accepted tcp:host:443 [vless-reality-in -> direct] email: alice@example.com
```

**GeoLite2 database setup:**

```bash
# Register at https://www.maxmind.com/en/geolite2/signup
# Download from: https://download.maxmind.com/app/geoip_download
mkdir -p /var/lib/xray-exporter
mv GeoLite2-City.mmdb /var/lib/xray-exporter/
mv GeoLite2-ASN.mmdb /var/lib/xray-exporter/
```

Loopback IPs (`127.0.0.x`) are skipped. Entries without an `email:` field are counted in inbound stats only.

---

## Prometheus Scrape Config

```yaml
scrape_configs:
  - job_name: xray-stats
    scrape_interval: 15s
    static_configs:
      - targets: ['127.0.0.1:9551']
```

---

## Grafana Dashboard

Works out of the box with [Raven-server-install](https://github.com/AlchemyLink/Raven-server-install) which includes a pre-built Grafana dashboard with:

- Per-user upload/download timeseries
- Top users by traffic (bar gauge)
- Per-inbound traffic breakdown
- TSPU event counters (handshake failures, RST, probes) with threshold alerting
- Throughput degradation event timeline

---

## Related Projects

- [Raven-server-install](https://github.com/AlchemyLink/Raven-server-install) — Ansible playbooks that deploy this exporter alongside Xray + Raven-subscribe
- [Raven-subscribe](https://github.com/AlchemyLink/Raven-subscribe) — subscription server for Xray users
- [Xray-core](https://github.com/XTLS/Xray-core) — the VPN core

---

## License

[GNU Affero General Public License v3.0 or later](LICENSE) © AlchemyLink contributors

The project's README badge previously indicated MIT; on 2026-05-06 a formal
LICENSE file was added under AGPL-3.0-or-later. Code at or before commit
`0dfdd4f268917c17a333e37b6bd1ef7f7d42efb5` is available under either the previously-advertised
MIT terms or AGPL-3.0-or-later, at the recipient's choice; subsequent commits
are AGPL-3.0-or-later only.
