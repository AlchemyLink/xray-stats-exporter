# Changelog

All notable changes to `xray-stats-exporter` are recorded here.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Documentation
- Privacy & Security section in README, covering listen-address defaults,
  the email-label PII surface, and when to set `--anonymize-secret`.
- Hardening checklist for production deployments.
- `CHANGELOG.md` (this file) backfilled from git history.

## [1.0.3] — 2026-05-06

### Changed
- License formalised: `LICENSE` now contains the full **GNU AGPL-3.0-or-later**
  text. Previously the README badge advertised MIT without an in-tree licence
  file. Code at or before commit `0dfdd4f` is available under either of the
  previously-advertised MIT terms or AGPL-3.0-or-later, at the recipient's
  choice; subsequent commits are AGPL-3.0-or-later only.

## [1.0.2] — 2026-05-02

### Added
- `--anonymize-secret` flag: when set, `email` labels are replaced with a
  stable 16-character hex pseudonym (`hex(sha256(secret + ":" + email))[:16]`).
  Lets operators ship per-user metrics into shared TSDBs without leaking
  raw emails. Default empty (raw emails) for single-operator setups.
  Symmetric — downstream consumers (e.g. raven-dashboard PromQL) must use
  the same secret.

## [1.0.1] — 2026-04-21

### Changed
- TSPU counters for known inbounds are pre-seeded with zero on startup so
  Grafana panels render immediately rather than hide the series until the
  first event arrives.

## [1.0.0] — 2026-04-20

Initial tagged release.

### Added
- Per-user traffic counters via Xray gRPC `StatsService`:
  `xray_user_{up,down}link_bytes_total{email=…}`.
- Per-inbound traffic counters: `xray_inbound_{up,down}link_bytes_total{inbound=…}`.
- Geo enrichment from `access.log` + GeoLite2-City/ASN:
  `xray_user_last_country{email,country,city,lat,lon}`,
  `xray_user_connections_total`, `xray_inbound_connections_total`.
- TSPU / DPI detection from `error.log`: `xray_handshake_failure_total`,
  `xray_connection_reset_total`, `xray_probe_detected_total`, all labelled
  by inbound tag.
- Throughput-degradation detection: `xray_inbound_throughput_bytes_per_second`,
  `xray_throughput_degradation_total` based on a rolling 10-sample baseline.
- Exporter-health metrics: `xray_up`, `xray_scrape_duration_seconds`.
- CI on GitHub Actions: lint (golangci-lint v2), `go vet`, `go test -race`,
  `go mod tidy` check, cross-platform release builds (linux/amd64+arm64,
  darwin/amd64+arm64) on tag push.

[Unreleased]: https://github.com/AlchemyLink/xray-stats-exporter/compare/v1.0.3...HEAD
[1.0.3]: https://github.com/AlchemyLink/xray-stats-exporter/compare/v1.0.2...v1.0.3
[1.0.2]: https://github.com/AlchemyLink/xray-stats-exporter/compare/v1.0.1...v1.0.2
[1.0.1]: https://github.com/AlchemyLink/xray-stats-exporter/compare/v1.0.0...v1.0.1
[1.0.0]: https://github.com/AlchemyLink/xray-stats-exporter/releases/tag/v1.0.0
