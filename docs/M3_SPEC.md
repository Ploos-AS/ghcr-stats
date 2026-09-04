# M3 — Historical analytics and dashboard

M3 turns ghcr-stats from a collector/API into a read-only analytics application while keeping the same single Go binary, SQLite database and OCI image.

## Scope

- package history API
- organization history API
- 24h, 7d, 30d and 90d analytics
- all-time ranking support
- package rankings by period
- built-in organization dashboard
- built-in package detail page with a dependency-free canvas history graph
- collector health/staleness integrated into rankings and UI
- HTTP runtime qualification in CI

## API

- `GET /api/v1/packages/{package}/history?period=24h|7d|30d|90d|all`
- `GET /api/v1/org/history?period=24h|7d|30d|90d|all`
- `GET /api/v1/rankings?period=24h|7d|30d|90d|all`

Existing package JSON remains backwards compatible and additionally exposes `downloads_24h` and `downloads_90d` while retaining the existing week/month fields.

## Period semantics

Period deltas use the same contract established before M3: the baseline is the newest snapshot at or before the period boundary. If no baseline exists yet, the delta is zero rather than an invented increase. Counter regressions also produce a zero delta.

## UI

`/` is the organization overview. It shows total pulls, period deltas, a 30-day ranking and package health.

`/package/{package}` is the package detail page. It shows totals/deltas, collector health and a 90-day history graph. The graph uses the browser Canvas API; no Node.js, npm, React or external chart library is required.

## Qualification

`m3_test.go` covers history, period analytics, rankings, routing, dashboard rendering and invalid periods.

`.github/workflows/m3-runtime.yml` builds the OCI image, starts a real container and qualifies the dashboard, package detail, rankings, package history and organization history through HTTP.
