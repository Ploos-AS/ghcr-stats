# M5.4 Notification integrations

M5.4 adds an optional Apprise API notification adapter on top of the persistent M5 event model.

The existing M5.1 generic webhook remains independent. Both outputs may be enabled at the same time.

## Why Apprise

Apprise provides one HTTP API in front of many downstream notification services. ghcr-stats therefore does not need native provider-specific code for Discord, Matrix, Slack, Telegram, email and similar services.

Use a persistent Apprise API notification endpoint such as:

`http://apprise:8000/notify/ghcr-stats`

The endpoint must be an absolute `http` or `https` URL.

## Configuration

- `GHCR_STATS_APPRISE_URL`
- `GHCR_STATS_APPRISE_URL_FILE`
- `GHCR_STATS_APPRISE_TAG`
- `GHCR_STATS_APPRISE_TAG_FILE`
- `GHCR_STATS_APPRISE_BEARER_TOKEN`
- `GHCR_STATS_APPRISE_BEARER_TOKEN_FILE`

Direct values take precedence over matching `_FILE` values.

The Bearer token is optional and is intended for deployments where a reverse proxy or API gateway protects the Apprise endpoint.

M5.4 is disabled by default when no Apprise URL is configured.

## Payload

The adapter sends JSON with:

- `body`
- `title`
- `type`
- optional `tag`

Severity mapping:

- `info` -> `info`
- `warning` -> `warning`
- `error` -> `failure`

The body contains the owner, event type, optional package and event message.

## Failure semantics

Notification delivery is best-effort. A failure:

- does not roll back the persisted event
- does not fail collection
- does not change `/readyz`
- is recorded in Prometheus metrics

Each delivery has a 10 second timeout.

## Metrics

- `ghcr_stats_notifications_config_valid{owner,provider="apprise"}`
- `ghcr_stats_notifications_enabled{owner,provider="apprise"}`
- `ghcr_stats_notification_deliveries_total{owner,provider="apprise",result="success|failure"}`
- `ghcr_stats_notification_last_success_timestamp_seconds{owner,provider="apprise"}`

## Security

Prefer HTTPS when Apprise is not on the same trusted private network. Keep credentials out of the URL when possible and use `_FILE` variables for secrets. ghcr-stats never includes configured credentials in event payloads or metrics.
