# M4.1 deployment hardening

M4.1 adds a supported rootless Podman Quadlet deployment alongside Docker Compose and documents the hardened runtime assumptions shared by both deployment paths.

## Runtime security baseline

The published image already runs as UID/GID 1000 and stores mutable state under `/data`. Deployment definitions keep the container root filesystem read-only, provide a small writable `/tmp`, drop all Linux capabilities, enable `no-new-privileges`, and persist only `/data`.

Docker Compose additionally sets an explicit UID/GID, a PID limit, and a bounded stop grace period. The Quadlet mirrors those controls with `ReadOnly=true`, `ReadOnlyTmpfs=true`, `DropCapability=all`, `NoNewPrivileges=true`, and `PidsLimit=128`.

## Docker Compose

```bash
cp .env.example .env
mkdir -p data
docker compose up -d
```

The bind-mounted `data` directory must be writable by container UID 1000. On the common case where the host user is UID 1000, `mkdir -p data` is sufficient. Otherwise adjust ownership or use a named volume.

The GitHub token can be supplied directly with `GHCR_STATS_GITHUB_TOKEN`, but a mounted secret file plus `GHCR_STATS_GITHUB_TOKEN_FILE` is preferred where practical.

## Rootless Podman Quadlet

Quadlet files for a rootless user live under `~/.config/containers/systemd/`.

```bash
mkdir -p ~/.config/containers/systemd
mkdir -p ~/.local/share/ghcr-stats
cp quadlet/ghcr-stats.container ~/.config/containers/systemd/
cp quadlet/ghcr-stats.env.example ~/.config/containers/systemd/ghcr-stats.env
systemctl --user daemon-reload
systemctl --user start ghcr-stats.service
```

Inspect the service with:

```bash
systemctl --user status ghcr-stats.service
journalctl --user -u ghcr-stats.service
podman ps
```

The Quadlet uses `AutoUpdate=registry`, so it is compatible with `podman auto-update`. Whether automatic update timers should be enabled is an operator policy decision; pin `Image=ghcr.io/ploos-as/ghcr-stats:0.4.0` or a digest instead of `latest` when deterministic upgrades are preferred.

To keep a rootless user service running across logout and boot, an administrator can enable systemd user lingering for that account:

```bash
sudo loginctl enable-linger "$USER"
```

## Token file with Quadlet

For organization-wide automatic discovery, leave `GHCR_STATS_PACKAGES` empty and provide a GitHub token. To avoid placing the token in the environment file, create a local secret file:

```bash
mkdir -p ~/.config/ghcr-stats
chmod 700 ~/.config/ghcr-stats
printf '%s\n' 'github_pat_...' > ~/.config/ghcr-stats/github-token
chmod 600 ~/.config/ghcr-stats/github-token
```

Then add this line to the local copy of `ghcr-stats.container`:

```ini
Volume=%h/.config/ghcr-stats/github-token:/run/secrets/github-token:ro,Z
```

and set this in `ghcr-stats.env`:

```env
GHCR_STATS_GITHUB_TOKEN_FILE=/run/secrets/github-token
```

Reload and restart after changing the local Quadlet:

```bash
systemctl --user daemon-reload
systemctl --user restart ghcr-stats.service
```

## Quadlet validation

When Podman is installed, the generated systemd unit can be inspected without starting the service:

```bash
QUADLET_UNIT_DIRS="$PWD/quadlet" \
  /usr/lib/systemd/system-generators/podman-system-generator --user --dryrun
```

This is useful when deploying to a Podman version older than the one used for development, because unsupported Quadlet keys cause generator errors rather than being silently ignored.

## Persistence and backup boundary

Everything required to preserve ghcr-stats history is under `/data`. Stop the service or otherwise obtain a consistent SQLite backup before copying the database. M4.2 will add explicit database maintenance and backup tooling; M4.1 only establishes the deployment boundary.
