# M4.3 history export

M4.3 adds read-only JSON and CSV export endpoints for package and organization history. Export uses the same stored snapshots, period parsing, and organization carry-forward aggregation as the existing M3 analytics APIs.

## Endpoints

Package history:

```text
GET /api/v1/packages/{package}/export?format=json&period=30d
GET /api/v1/packages/{package}/export?format=csv&period=30d
```

Organization history:

```text
GET /api/v1/org/export?format=json&period=30d
GET /api/v1/org/export?format=csv&period=30d
```

Supported periods are `24h`, `7d`, `30d`, `90d`, and `all`. The default period is `30d`. The default format is `json`.

## JSON

Package JSON contains:

- `owner`
- `package`
- `period`
- `points`

Organization JSON contains the same fields except that `package` is empty. Each point contains `timestamp`, `downloads`, and `delta`.

Package exports use the package snapshot history. Organization exports use the M3.1 carry-forward aggregation, so staggered package collection timestamps are not naïvely summed only when timestamps happen to match.

## CSV

CSV responses are returned with `Content-Type: text/csv` and a downloadable `Content-Disposition` filename. The stable column order is:

```text
owner,package,period,timestamp,downloads,delta
```

Example package row:

```text
Ploos-AS,soju,30d,2026-09-04T18:00:00Z,1234,17
```

Organization rows leave the `package` column empty because each row represents the aggregate organization series.

## Errors and methods

Export endpoints accept `GET` only. Other methods return `405 Method Not Allowed` with `Allow: GET`.

Unsupported `format` or `period` values return `400 Bad Request`. Package export returns `404 Not Found` when the requested package has no stored history. Organization export returns `404 Not Found` when no organization history is available.

## Retention interaction

Exports reflect the history currently retained in SQLite. If `GHCR_STATS_RETENTION` is enabled, older redundant snapshots may have been pruned. M4.2 retains one boundary baseline per package before the retention cutoff; that baseline remains available to analytics where required, but export is not an archival substitute for backups.
