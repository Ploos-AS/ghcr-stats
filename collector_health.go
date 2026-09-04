package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type CollectorHealth struct {
	Package     string    `json:"package"`
	Up          bool      `json:"up"`
	Stale       bool      `json:"stale"`
	LastSuccess time.Time `json:"last_success,omitempty"`
	LastError   string    `json:"last_error,omitempty"`
}

func (a *App) staleAfter() time.Duration {
	// A snapshot is considered stale after three missed normal collection cycles.
	// Keep a floor so test/dev configurations do not become noisy.
	d := 3 * a.cfg.Interval
	if d < 3*time.Hour {
		return 3 * time.Hour
	}
	return d
}

func (a *App) collectorHealth(pkg string, now time.Time) CollectorHealth {
	h := CollectorHealth{Package: pkg, Up: true}
	if st, err := a.store.Latest(pkg); err == nil {
		h.LastSuccess = st.UpdatedAt
		h.Stale = now.Sub(st.UpdatedAt) > a.staleAfter()
	} else {
		h.Up = false
		h.Stale = true
	}

	fs := a.failureStats(pkg)
	h.LastError = fs.LastError
	if h.LastError != "" || fs.Consecutive > 0 {
		h.Up = false
	}
	return h
}

func (a *App) handleHealthJSON(w http.ResponseWriter, r *http.Request) {
	pkg := r.URL.Query().Get("package")
	w.Header().Set("Content-Type", "application/json")
	if pkg != "" {
		_ = json.NewEncoder(w).Encode(a.collectorHealth(pkg, time.Now().UTC()))
		return
	}
	items := make([]CollectorHealth, 0, len(a.packageNames()))
	for _, name := range a.packageNames() {
		items = append(items, a.collectorHealth(name, time.Now().UTC()))
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"collector": a.collector.Name(), "stale_after_seconds": int64(a.staleAfter().Seconds()), "packages": items, "org": a.orgHealth(time.Now().UTC())})
}

func (a *App) writeCollectorHealthMetrics(w http.ResponseWriter) {
	now := time.Now().UTC()
	for _, pkg := range a.packageNames() {
		h := a.collectorHealth(pkg, now)
		up := 0
		if h.Up {
			up = 1
		}
		stale := 0
		if h.Stale {
			stale = 1
		}
		fmt.Fprintf(w, "ghcr_stats_collector_up{owner=%q,package=%q} %d\n", a.cfg.Owner, pkg, up)
		fmt.Fprintf(w, "ghcr_stats_snapshot_stale{owner=%q,package=%q} %d\n", a.cfg.Owner, pkg, stale)
		if !h.LastSuccess.IsZero() {
			fmt.Fprintf(w, "ghcr_stats_last_success_timestamp_seconds{owner=%q,package=%q} %d\n", a.cfg.Owner, pkg, h.LastSuccess.Unix())
			fmt.Fprintf(w, "ghcr_stats_snapshot_age_seconds{owner=%q,package=%q} %.0f\n", a.cfg.Owner, pkg, now.Sub(h.LastSuccess).Seconds())
		}
	}
}
