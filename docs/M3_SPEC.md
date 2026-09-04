# M3 — Historical analytics and dashboard

M3 turns ghcr-stats from a collector/API into a read-only analytics application while keeping the same single Go binary, SQLite database and OCI image.

## Scope

- package history API
- organization history API
- 24h, 7d, 30d and 90d analytics
- all-time ranking support
- package rankings by period
- built-in organization dashboard
- built-in package detail page
- period selection in the dashboard and package detail views
- dependency-free Canvas history graphs
- carry-forward organization history aggregation for non-aligned package snapshots
- explicit bounded-period organization baselines
- collector health/staleness integrated into rankings and UI
- hardened GHCR HTML parser with explicit `Total downloads` semantics
- collection-state schema initialized during `OpenStore`
- HTTP runtime qualification in CI

## API

- `GET /api/v1/packages/{package}/history?period=24h|7d|30d|90d|all`
- `GET /api/v1/org/history?period=24h|7d|30d|90d|all`
- `GET /api/v1/rankings?period=24h|7d|30d|90d|all`

Existing package JSON remains backwards compatible and additionally exposes `downloads_24h` and `downloads_90d` while retaining the existing 7d/30d fields and collector health state.

## Period semantics

Package period deltas use the contract established before M3: the baseline is the newest snapshot at or before the period boundary. If no baseline exists yet, the delta is zero rather than an invented increase. Counter regressions also produce zero delta.

The `all` ranking period compares the latest snapshot with the first stored snapshot.

Organization history uses carry-forward aggregation. Each event in the organization series updates one package while retaining the latest known value for all other packages. This avoids artificial total drops when packages are collected at slightly different timestamps. For bounded periods, a synthetic period-boundary baseline is emitted when prior package state exists.

## UI

`/` is the organization overview. It shows total pulls, 24h/7d/30d/90d deltas, a selected-period organization graph, selected-period ranking and package health.

`/package/{package}` is the package detail page. It shows totals/deltas, collector health and a selected-period history graph.

Supported UI periods are `24h`, `7d`, `30d`, `90d` and `all`. Invalid or missing dashboard period values normalize to `30d`.

The graph uses the browser Canvas API. No Node.js, npm, React or external chart library is required.

No-data, stale and collector-error states are presented explicitly in the UI rather than being silently treated as fresh statistics.

## Internal cleanup completed in M3.3

- removed obsolete pre-M3.2 dashboard templates
- removed legacy broad `N downloads` parser patterns
- replaced parser `init()` override behavior with explicit `parseDownloadCount`
- kept only `Total downloads`-anchored extraction semantics
- moved `collection_state` schema creation to `OpenStore` rather than lazy read/write DDL

These changes are internal and do not alter the public API contract.

## Qualification

`m3_test.go` covers history, period analytics, rankings, routing, dashboard rendering and invalid periods.

`m31_org_history_test.go` covers non-aligned package timestamps, carry-forward organization totals and bounded-period baselines.

`m32_dashboard_test.go` covers dashboard period normalization and package-path validation.

`.github/workflows/m3-runtime.yml` builds the OCI image, starts a real container and qualifies the dashboard, package detail, period selection, rankings, package history and organization history through HTTP.

The normal CI additionally gates formatting, unit tests, live collector smoke, package-discovery behavior, `go vet`, Compose validation, container build, non-root/OCI metadata, runtime APIs and persisted restart state.
