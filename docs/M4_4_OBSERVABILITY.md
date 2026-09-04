# M4.4 readiness and observability

M4.4 separates process liveness from service readiness and exposes service-level Prometheus gauges.

## Liveness

`GET /healthz` remains a pure process/liveness check. Transient GitHub or collector failures do not fail liveness.

## Readiness

`GET /readyz` returns `200 OK` only when all of these are true:

1. SQLite is reachable;
2. at least one package is configured;
3. at least one configured package has a stored snapshot that is not stale.

It returns `503 Service Unavailable` otherwise. `HEAD` is supported with the same status semantics. Unsupported methods return `405 Method Not Allowed`.

Collector availability is deliberately not an independent readiness requirement. If the upstream collector is temporarily failing but ghcr-stats still has usable fresh cached data, the service remains ready.

## Prometheus

M4.4 adds:

- `ghcr_stats_info{version,revision}`
- `ghcr_stats_ready{owner}`
- `ghcr_stats_database_up{owner}`
- `ghcr_stats_packages_with_data{owner}`
- `ghcr_stats_fresh_packages{owner}`
- `ghcr_stats_stale_packages{owner}`
- `ghcr_stats_stale_after_seconds{owner}`
- `ghcr_stats_process_uptime_seconds`

These complement the existing collector, discovery, package, and organization metrics.

## Qualification

`.github/workflows/m44-observability.yml` builds the actual container and verifies the unready-to-ready transition plus metric exposure. This keeps readiness semantics covered at runtime rather than only by Go unit tests.
