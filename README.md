# ghcr-stats

Small self-hosted GHCR statistics service for Ploos-AS.

## Why

GitHub's supported Packages API exposes package metadata, but not GHCR pull/download counts. ghcr-stats therefore separates two concerns:

- package discovery uses GitHub's supported Packages API
- pull/download collection uses GitHub's public package HTML and stores periodic snapshots

This keeps the package inventory on a supported API while isolating the best-effort HTML collector behind a provider interface.

## Features

- automatic discovery of public GHCR container packages
- explicit package-list override when desired
- periodic pull/download collection
- collector health and stale-data detection
- collection error counters and consecutive-failure tracking
- organization-level health aggregation
- SQLite history in `/data`
- Prometheus `/metrics`
- package and organization JSON APIs
- built-in SVG badges
- Shields.io-compatible JSON badge API
- 7-day and 30-day deltas
- non-root OCI image
- Docker Compose
- amd64 + arm64 release workflow
- SBOM and provenance

## Package discovery

Automatic discovery is enabled when a GitHub token is provided and `GHCR_STATS_PACKAGES` is empty.

```env
GHCR_STATS_OWNER=Ploos-AS
GHCR_STATS_GITHUB_TOKEN=github_pat_...
GHCR_STATS_PACKAGES=
```

For reliable organization-wide package discovery, use a token that can read package metadata. A classic personal access token with `read:packages` is the conservative choice. Do not assume a repository workflow's built-in `GITHUB_TOKEN` can enumerate all packages belonging to the organization.

For secret-file deployments, use `GHCR_STATS_GITHUB_TOKEN_FILE` instead of putting the token directly in the environment.

```env
GHCR_STATS_GITHUB_TOKEN_FILE=/run/secrets/github_token
```

The discovery request asks GitHub's organization Packages API for `container` packages and retains public packages from the response. Discovery runs before each normal collection cycle, so newly published containers enter the monitored package set automatically.

If discovery fails, the service retains its last known/fallback package set and reports the error through the package/org APIs and `ghcr_stats_discovery_up`. A non-empty `GHCR_STATS_PACKAGES` value is an explicit override and disables automatic discovery.

## Collector health

A package is considered stale when its newest successful snapshot is older than three configured collection intervals. With the default six-hour interval, the stale threshold is 18 hours. A minimum threshold of three hours is used for shorter test/development intervals.

Collection failures never delete the last successful snapshot. Instead, the previous statistics remain available while health surfaces report the failure.

`/api/v1/health` reports all packages plus an organization summary. To inspect one package:

```text
/api/v1/health?package=soju
```

Per-package health includes `up`, `stale`, `last_success`, `last_error`, `total_failures`, and `consecutive_failures`. A successful collection resets `consecutive_failures` to zero while preserving the lifetime in-process failure counter.

The organization summary reports `healthy`/`degraded`, healthy and unhealthy package counts, stale packages, failing packages, total failures, and current consecutive failures. `/healthz` remains a process/liveness endpoint and is intentionally not failed by transient collector problems.

Package JSON also contains `collector_up`, `stale`, `last_success`, and `last_error`.

## Endpoints

- `/healthz`
- `/metrics`
- `/api/v1/health`
- `/api/v1/packages`
- `/api/v1/packages/<package>`
- `/api/v1/org`
- `/api/v1/badge/<package|org>/<pulls|pulls-7d|pulls-30d>`
- `/badge/<package|org>/pulls.svg`
- `/badge/<package|org>/pulls-7d.svg`
- `/badge/<package|org>/pulls-30d.svg`

`/api/v1/packages` reports the active package set and whether it came from `github-api`, `explicit`, or `fallback` configuration.

Examples:

```markdown
![GHCR pulls](https://stats.example.org/badge/soju/pulls.svg)
![GHCR pulls 30d](https://stats.example.org/badge/soju/pulls-30d.svg)
![Ploos-AS GHCR pulls](https://stats.example.org/badge/org/pulls.svg)
```

The JSON badge endpoints implement the Shields custom endpoint schema:

```json
{
  "schemaVersion": 1,
  "label": "GHCR pulls",
  "message": "1.2k",
  "color": "2ea44f"
}
```

## Prometheus metrics

Per package statistics:

- `ghcr_downloads_total`
- `ghcr_downloads_7d`
- `ghcr_downloads_30d`
- `ghcr_snapshot_timestamp_seconds`

Per package collector health:

- `ghcr_stats_collector_up`
- `ghcr_stats_snapshot_stale`
- `ghcr_stats_last_success_timestamp_seconds`
- `ghcr_stats_snapshot_age_seconds`
- `ghcr_stats_collection_errors_total`
- `ghcr_stats_consecutive_failures`

Organization health:

- `ghcr_stats_org_healthy`
- `ghcr_stats_org_unhealthy_packages`
- `ghcr_stats_org_stale_packages`
- `ghcr_stats_org_failing_packages`

Organization totals:

- `ghcr_org_downloads_total`
- `ghcr_org_downloads_7d`
- `ghcr_org_downloads_30d`

Discovery/service state:

- `ghcr_stats_packages`
- `ghcr_stats_discovery_up`

Example alerting can use `ghcr_stats_consecutive_failures >= 3`, `ghcr_stats_snapshot_stale == 1`, or `ghcr_stats_org_healthy == 0` without coupling external dependency health to the container's liveness probe.

## Run

```bash
cp .env.example .env
mkdir -p data
docker compose up -d
```

For automatic discovery, add a suitable GitHub token to `.env`. Without one, ghcr-stats keeps a built-in fallback list so the service remains usable.

The first usable statistic appears after a successful collection. Historical 7/30-day deltas become meaningful only after enough snapshots have accumulated. For each period, the baseline is the newest snapshot at or before the period boundary. If no such baseline exists yet, that package contributes zero to the period delta rather than an invented increase.

## Caveat

The `github-html` collector parses GitHub's public package HTML because GitHub does not provide pull/download counts through its supported Packages API. HTML can change without notice. Collection failures do not delete existing history; collector health and stale-data metrics are intended to make such breakage visible.
