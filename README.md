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
- JSON API
- built-in SVG badges
- non-root OCI image
- Docker Compose
- amd64 + arm64 release workflow
- SBOM and provenance

## Endpoints

- `/healthz`
- `/metrics`
- `/api/v1/packages/<package>`
- `/badge/<package>/pulls.svg`

Example:

```markdown
![GHCR pulls](https://stats.example.org/badge/soju/pulls.svg)
```

## Run

```bash
cp .env.example .env
mkdir -p data
docker compose up -d
```

The first usable statistic appears after a successful collection. Historical
7/30 day deltas become meaningful only after enough snapshots have accumulated.

## Caveat

The `github-html` collector parses GitHub's public package HTML because GitHub
does not provide this metric through its supported Packages API. HTML can change
without notice. Collection failures do not delete existing history.
