from pathlib import Path

main = Path("main.go")
s = main.read_text()
route_anchor = 'mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok\\n")) })\n'
route_line = '\tmux.HandleFunc("/readyz", a.handleReadyz)\n'
if 'mux.HandleFunc("/readyz", a.handleReadyz)' not in s:
    if route_anchor not in s:
        raise SystemExit("healthz route anchor not found")
    s = s.replace(route_anchor, route_anchor + route_line, 1)
metrics_anchor = '\ta.writeCollectorHealthMetrics(w)\n'
metrics_line = '\ta.writeM44Metrics(w)\n'
if 'a.writeM44Metrics(w)' not in s:
    if metrics_anchor not in s:
        raise SystemExit("metrics anchor not found")
    s = s.replace(metrics_anchor, metrics_anchor + metrics_line, 1)
main.write_text(s)

readme = Path("README.md")
s = readme.read_text()
if "### Readiness and observability (M4.4)" not in s:
    s += '''\n\n### Readiness and observability (M4.4)\n\n`/healthz` remains a pure liveness endpoint. `/readyz` is a stricter readiness endpoint: the SQLite database must be reachable, at least one package must be configured, and at least one package must have a non-stale snapshot. A collector failure does not make the service unready while usable fresh cached data still exists. `/readyz` returns `200` when ready and `503` otherwise, and supports `GET` and `HEAD`.\n\nAdditional Prometheus gauges are exported from `/metrics`:\n\n- `ghcr_stats_info{version,revision}`\n- `ghcr_stats_ready{owner}`\n- `ghcr_stats_database_up{owner}`\n- `ghcr_stats_packages_with_data{owner}`\n- `ghcr_stats_fresh_packages{owner}`\n- `ghcr_stats_stale_packages{owner}`\n- `ghcr_stats_stale_after_seconds{owner}`\n- `ghcr_stats_process_uptime_seconds`\n'''
readme.write_text(s)
