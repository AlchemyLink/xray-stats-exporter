# xray-stats-exporter

[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Go Report Card](https://goreportcard.com/badge/github.com/alchemylink/xray-stats-exporter)](https://goreportcard.com/report/github.com/alchemylink/xray-stats-exporter)

Prometheus exporter for [Xray-core](https://github.com/XTLS/Xray-core) — exposes per-user and per-inbound traffic metrics via the Xray gRPC StatsService API.

Built for self-hosted VPN operators who want to monitor user bandwidth and protocol usage in Grafana.

---

## Metrics

| Metric | Labels | Description |
|--------|--------|-------------|
| `xray_user_uplink_bytes_total` | `email` | Cumulative bytes sent by user (client → server) |
| `xray_user_downlink_bytes_total` | `email` | Cumulative bytes received by user (server → client) |
| `xray_inbound_uplink_bytes_total` | `inbound` | Cumulative uplink bytes per inbound |
| `xray_inbound_downlink_bytes_total` | `inbound` | Cumulative downlink bytes per inbound |
| `xray_user_last_country` | `email`, `country`, `city` | Last seen geo location per user (gauge=1, requires access.log + GeoLite2) |
| `xray_user_connections_total` | `email`, `country`, `city` | Connection count per user per location |
| `xray_inbound_connections_total` | `inbound`, `country` | Connection count per inbound per country |
| `xray_up` | — | 1 if Xray gRPC API is reachable, 0 otherwise |
| `xray_scrape_duration_seconds` | — | Time taken to scrape the Xray API |

Geo metrics (`xray_user_last_country`, `xray_user_connections_total`, `xray_inbound_connections_total`) require `--log-path` and `--geo-city-db` to be set.

---

## Requirements

- Xray-core with `StatsService` and `HandlerService` enabled in the API config
- Xray API must be accessible (default: `127.0.0.1:10085`)
- Per-user stats enabled in Xray (`policy.levels` with `statsUserUplink`/`statsUserDownlink`)

**For geo metrics (optional):**
- Xray `access.log` with real client IPs
- [GeoLite2-City.mmdb](https://dev.maxmind.com/geoip/geolite2-free-geolocation-data) from MaxMind

---

## Quick Start

### 1. Build

```bash
git clone https://github.com/AlchemyLink/xray-stats-exporter.git
cd xray-stats-exporter
go build -o xray-stats-exporter .
sudo mv xray-stats-exporter /usr/local/bin/
```

### 2. Run

```bash
xray-stats-exporter \
  --listen=127.0.0.1:9551 \
  --xray-endpoint=127.0.0.1:10085
```

With geo metrics:

```bash
xray-stats-exporter \
  --listen=127.0.0.1:9551 \
  --xray-endpoint=127.0.0.1:10085 \
  --log-path=/var/log/Xray/access.log \
  --geo-city-db=/var/lib/xray-exporter/GeoLite2-City.mmdb \
  --geo-asn-db=/var/lib/xray-exporter/GeoLite2-ASN.mmdb
```

### 3. Verify

```bash
curl http://127.0.0.1:9551/metrics | grep xray_
```

---

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--listen` | `127.0.0.1:9551` | Address to expose metrics on |
| `--metrics-path` | `/metrics` | HTTP path for the metrics endpoint |
| `--xray-endpoint` | `127.0.0.1:10085` | Xray gRPC API address |
| `--log-path` | `""` | Path to Xray `access.log` for geo metrics (empty = disabled) |
| `--geo-city-db` | `""` | Path to `GeoLite2-City.mmdb` (empty = geo disabled) |
| `--geo-asn-db` | `""` | Path to `GeoLite2-ASN.mmdb` (empty = ASN label disabled) |

---

## Systemd Service

```ini
[Unit]
Description=Xray Stats Prometheus Exporter
After=network.target xray.service

[Service]
User=xrayuser
Group=xrayuser
ExecStart=/usr/local/bin/xray-stats-exporter \
  --listen=127.0.0.1:9551 \
  --xray-endpoint=127.0.0.1:10085 \
  --log-path=/var/log/Xray/access.log \
  --geo-city-db=/var/lib/xray-exporter/GeoLite2-City.mmdb \
  --geo-asn-db=/var/lib/xray-exporter/GeoLite2-ASN.mmdb
Restart=on-failure
RestartSec=5s

[Install]
WantedBy=multi-user.target
```

---

## Xray Config Requirements

Xray must have stats and API enabled. Minimal required config fragments:

**`010-stats.json`** — enable stats collection:
```json
{
  "stats": {},
  "policy": {
    "levels": {"0": {"statsUserUplink": true, "statsUserDownlink": true}},
    "system": {"statsInboundUplink": true, "statsInboundDownlink": true}
  }
}
```

**`050-api.json`** — expose gRPC API:
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

Users must have an `email` field in the inbound config for per-user metrics:

```json
{
  "clients": [
    {"id": "uuid-here", "email": "alice@example.com", "flow": "xtls-rprx-vision"}
  ]
}
```

---

## Prometheus Scrape Config

```yaml
scrape_configs:
  - job_name: xray-stats
    static_configs:
      - targets: ['127.0.0.1:9551']
```

---

## Grafana Dashboard

Works out of the box with [Raven-server-install](https://github.com/AlchemyLink/Raven-server-install) which includes a pre-built Grafana dashboard with:

- Per-user upload/download timeseries
- Top users by traffic (bar gauge)
- Per-inbound traffic breakdown (vless-reality-in vs vless-xhttp-in)

---

## Related Projects

- [Raven-server-install](https://github.com/AlchemyLink/Raven-server-install) — Ansible playbooks that deploy this exporter alongside Xray + Raven-subscribe
- [Raven-subscribe](https://github.com/AlchemyLink/Raven-subscribe) — subscription server for Xray users
- [Xray-core](https://github.com/XTLS/Xray-core) — the VPN core

---

## License

[MIT](LICENSE) © AlchemyLink
