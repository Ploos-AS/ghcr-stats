# M4.2 data lifecycle and SQLite maintenance

M4.2 adds explicit retention, integrity checking, consistent SQLite backup, restore, and maintenance commands to the `ghcr-stats` binary.

## Retention

Automatic snapshot pruning is disabled by default.

Set `GHCR_STATS_RETENTION` to a Go duration of at least 24 hours to enable it, for example:

```env
GHCR_STATS_RETENTION=2160h
```

`2160h` is 90 days. Retention runs after each normal collection cycle.

The retention algorithm deliberately keeps the newest snapshot at or before the cutoff for each package. That boundary snapshot remains available as a baseline for bounded-period delta calculations, while older redundant snapshots are removed.

Invalid values and values shorter than 24 hours are treated as disabled.

## Integrity check

Run a SQLite `quick_check` without starting the HTTP service:

```bash
docker compose run --rm ghcr-stats integrity
```

The command exits non-zero if SQLite reports corruption or another integrity problem.

## Consistent backup

Create a standalone consistent SQLite backup with SQLite `VACUUM INTO`:

```bash
mkdir -p data/backups
docker compose run --rm ghcr-stats backup /data/backups/ghcr-stats-$(date +%F).db
```

The destination must not already exist. Backup first runs an integrity check and then asks SQLite itself to create the backup, so it does not depend on copying a live database file while writes may be in progress.

Backups created below `/data` are persisted by the normal data volume/bind mount. Copy them to independent storage as part of the operator's backup policy.

## Restore

Restore is intentionally an offline operation. Stop the service first:

```bash
docker compose stop ghcr-stats
docker compose run --rm ghcr-stats restore /data/backups/ghcr-stats-2026-09-04.db
docker compose start ghcr-stats
```

The source database is opened and integrity-checked before it replaces the configured `GHCR_STATS_DB`. The replacement is written to a temporary file and atomically renamed into place.

Do not run restore concurrently with the normal service process.

## Manual maintenance

The maintenance command performs:

1. SQLite integrity check.
2. Retention pruning when `GHCR_STATS_RETENTION` is enabled.
3. `PRAGMA optimize`.
4. `VACUUM`.

```bash
docker compose stop ghcr-stats
docker compose run --rm ghcr-stats maintain
docker compose start ghcr-stats
```

`VACUUM` is intentionally not run after every collection cycle because it is a heavier database rewrite. Normal service-mode retention only prunes old snapshots; operators can schedule `maintain` during an appropriate maintenance window.

## Podman Quadlet

The same commands can be run against the image with the persistent rootless data directory mounted at `/data`. Stop the user service before offline maintenance or restore:

```bash
systemctl --user stop ghcr-stats.service
podman run --rm \
  --user 1000:1000 \
  -v "$HOME/.local/share/ghcr-stats:/data:Z" \
  -e GHCR_STATS_DB=/data/ghcr-stats.db \
  -e GHCR_STATS_RETENTION=2160h \
  ghcr.io/ploos-as/ghcr-stats:latest maintain
systemctl --user start ghcr-stats.service
```

For backups, replace `maintain` with `backup /data/backups/<name>.db`. For integrity-only checks, use `integrity`.
