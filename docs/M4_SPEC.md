# M4 operations and data portability

M4 turns the M3 analytics service into a more operationally complete self-hosted service. The release scope is deployment hardening, database lifecycle tooling, history export, and explicit readiness/observability.

## M4.1 deployment hardening

- supported Docker Compose deployment
- supported rootless Podman Quadlet deployment
- read-only container root filesystem with writable `/tmp`
- non-root UID/GID 1000
- all Linux capabilities dropped
- `no-new-privileges`
- persistent state isolated under `/data`
- bounded PID/stop behavior where supported
- token-file deployment documented

See `M4_1_DEPLOYMENT.md`.

## M4.2 data lifecycle and SQLite maintenance

- optional snapshot retention via `GHCR_STATS_RETENTION`
- retention preserves a boundary snapshot for period baselines
- `integrity` command for SQLite `quick_check`
- consistent `backup` using SQLite `VACUUM INTO`
- offline integrity-checked `restore`
- `maintain` command for integrity, retention, optimize, and vacuum
- runtime qualification for maintenance and persistence

See `M4_2_MAINTENANCE.md`.

## M4.3 history export

Read-only JSON and CSV export is available for package and organization history:

- `/api/v1/packages/{package}/export`
- `/api/v1/org/export`

Exports support `24h`, `7d`, `30d`, `90d`, and `all`, and reuse the same stored history and organization carry-forward semantics as M3 analytics.

See `M4_3_EXPORT.md`.

## M4.4 readiness and observability

`/healthz` remains process liveness. `/readyz` is service readiness and requires:

1. reachable SQLite storage,
2. at least one configured package, and
3. at least one non-stale stored snapshot.

A collector failure does not make the service unready while fresh cached data remains usable.

M4.4 also adds Prometheus service-state gauges:

- `ghcr_stats_info{version,revision}`
- `ghcr_stats_ready{owner}`
- `ghcr_stats_database_up{owner}`
- `ghcr_stats_packages_with_data{owner}`
- `ghcr_stats_fresh_packages{owner}`
- `ghcr_stats_stale_packages{owner}`
- `ghcr_stats_stale_after_seconds{owner}`
- `ghcr_stats_process_uptime_seconds`

The permanent M4.4 runtime workflow verifies readiness transitions and metric exposure in the built container.

## M4.5 release readiness

M4.5 is the release-readiness gate for v0.4.0. It does not add a new product feature. It verifies that:

- `main` is a strict descendant of v0.3.0 with no backwards divergence;
- M4 documentation matches the implemented behavior;
- permanent CI/runtime workflows qualify the final candidate commit;
- no temporary integration helper remains in the release tree;
- the candidate is suitable for the normal multi-architecture OCI release process.

A v0.4.0 tag or GitHub Release is intentionally outside M4.5 and requires a separate release action.
