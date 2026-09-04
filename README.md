# ghcr-stats

Small self-hosted GHCR statistics service for Ploos-AS.

## Why

GitHub's supported Packages API does not expose GHCR pull/download counts.
For public packages, ghcr-stats therefore uses GitHub's public package page as
a collector and stores periodic snapshots. The rest of the service is provider
independent so the collector can be replaced later.

## Features

- periodic collection for configured GHCR packages
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

## Endpoints

- `/healthz`
- `/metrics`
- `/api/v1/packages/<package>`
- `/api/v1/org`
- `/api/v1/badge/<package|org>/<pulls|pulls-7d|pulls-30d>`
- `/badge/<package|org>/pulls.svg`
- `/badge/<package|org>/pulls-7d.svg`
- `/badge/<package|org>/pulls-30d.svg`

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

Per package:

- `ghcr_downloads_total`
- `ghcr_downloads_7d`
- `ghcr_downloads_30d`
- `ghcr_snapshot_timestamp_seconds`

Organization totals:

- `ghcr_org_downloads_total`
- `ghcr_org_downloads_7d`
- `ghcr_org_downloads_30d`

## Run

```bash
cp .env.example .env
mkdir -p data
docker compose up -d
```

The first usable statistic appears after a successful collection. Historical
7/30-day deltas become meaningful only after enough snapshots have accumulated.
For each period, the baseline is the newest snapshot at or before the period
boundary. If no such baseline exists yet, that package contributes zero to the
period delta rather than an invented increase.

## Caveat

The `github-html` collector parses GitHub's public package HTML because GitHub
does not provide this metric through its supported Packages API. HTML can change
without notice. Collection failures do not delete existing history.
