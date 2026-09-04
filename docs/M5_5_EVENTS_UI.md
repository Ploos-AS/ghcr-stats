# M5.5 Events UI and API

M5.5 makes the persistent M5 event log directly useful for operators through a paginated JSON API and the built-in dashboard.

## Events API

`GET /api/v1/events`

Supported query parameters:

- `limit`: 1-1000, default 100.
- `cursor`: positive event ID returned as `next_cursor`; returns events older than that ID.
- `package`: exact package filter.
- `type`: exact event type filter.
- `severity`: `info`, `warning`, or `error`.

Results are ordered newest first. Cursor pagination is based on the immutable SQLite event ID, avoiding duplicate or skipped rows if new events arrive while a client is paging through older history.

Example first page:

```text
/api/v1/events?package=soju&severity=error&limit=50
```

When more rows exist, the response contains `next_cursor`. Pass that value unchanged as `cursor` for the next page.

Invalid limits, severities, or cursors return HTTP 400. Non-GET methods return HTTP 405.

## Dashboard

The organization overview shows the ten most recent events and links to the JSON API. Package detail pages show the ten most recent events for that package.

The dashboard is still rendered and served by the existing Go binary. The event panels use the same `/api/v1/events` endpoint and require no frontend framework or additional service.

## Qualification

M5.5 qualification verifies:

- stable cursor pagination with no duplicate event between pages;
- package/type/severity filtering;
- invalid cursor rejection;
- organization and package dashboard templates expose recent-event panels;
- repository-wide `gofmt`, tests, and `go vet` remain green.
