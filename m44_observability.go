package main

import (
	"fmt"
	"net/http"
	"time"
)

var processStartedAt = time.Now().UTC()

type readinessStatus struct {
	Ready        bool
	DatabaseUp   bool
	Packages     int
	WithData     int
	Fresh        int
	Stale        int
}

func (a *App) readiness(now time.Time) readinessStatus {
	status := readinessStatus{Packages: len(a.packageNames())}
	if a.store != nil && a.store.db != nil {
		status.DatabaseUp = a.store.db.Ping() == nil
	}
	for _, pkg := range a.packageNames() {
		st, err := a.store.Latest(pkg)
		if err != nil {
			continue
		}
		status.WithData++
		if now.Sub(st.UpdatedAt) > a.staleAfter() {
			status.Stale++
		} else {
			status.Fresh++
		}
	}
	status.Ready = status.DatabaseUp && status.Packages > 0 && status.Fresh > 0
	return status
}

func (a *App) handleReadyz(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	status := a.readiness(time.Now().UTC())
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if !status.Ready {
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	if r.Method == http.MethodGet {
		state := "not ready"
		if status.Ready {
			state = "ready"
		}
		_, _ = fmt.Fprintf(w, "%s\ndatabase_up=%t packages=%d with_data=%d fresh=%d stale=%d\n", state, status.DatabaseUp, status.Packages, status.WithData, status.Fresh, status.Stale)
	}
}

func (a *App) writeM44Metrics(w http.ResponseWriter) {
	now := time.Now().UTC()
	status := a.readiness(now)
	ready := 0
	if status.Ready {
		ready = 1
	}
	dbUp := 0
	if status.DatabaseUp {
		dbUp = 1
	}
	fmt.Fprintf(w, "ghcr_stats_info{version=%q,revision=%q} 1\n", version, revision)
	fmt.Fprintf(w, "ghcr_stats_ready{owner=%q} %d\n", a.cfg.Owner, ready)
	fmt.Fprintf(w, "ghcr_stats_database_up{owner=%q} %d\n", a.cfg.Owner, dbUp)
	fmt.Fprintf(w, "ghcr_stats_packages_with_data{owner=%q} %d\n", a.cfg.Owner, status.WithData)
	fmt.Fprintf(w, "ghcr_stats_fresh_packages{owner=%q} %d\n", a.cfg.Owner, status.Fresh)
	fmt.Fprintf(w, "ghcr_stats_stale_packages{owner=%q} %d\n", a.cfg.Owner, status.Stale)
	fmt.Fprintf(w, "ghcr_stats_stale_after_seconds{owner=%q} %.0f\n", a.cfg.Owner, a.staleAfter().Seconds())
	fmt.Fprintf(w, "ghcr_stats_process_uptime_seconds %.0f\n", now.Sub(processStartedAt).Seconds())
}
