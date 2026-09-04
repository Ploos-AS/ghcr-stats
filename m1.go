package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type MetricSummary struct {
	Downloads int64 `json:"downloads"`
	Downloads7d int64 `json:"downloads_7d"`
	Downloads30d int64 `json:"downloads_30d"`
}

type ShieldsBadge struct {
	SchemaVersion int `json:"schemaVersion"`
	Label string `json:"label"`
	Message string `json:"message"`
	Color string `json:"color,omitempty"`
}

func (a *App) packageSummary(pkg string) (MetricSummary, error) {
	st, err := a.store.Latest(pkg)
	if err != nil { return MetricSummary{}, err }
	d7, err := a.store.DeltaSince(pkg, time.Now().Add(-7*24*time.Hour))
	if err != nil { return MetricSummary{}, err }
	d30, err := a.store.DeltaSince(pkg, time.Now().Add(-30*24*time.Hour))
	if err != nil { return MetricSummary{}, err }
	return MetricSummary{Downloads: st.Downloads, Downloads7d: d7, Downloads30d: d30}, nil
}

func (a *App) orgSummary() MetricSummary {
	var out MetricSummary
	for _, pkg := range a.cfg.Packages {
		s, err := a.packageSummary(pkg)
		if err != nil { continue }
		out.Downloads += s.Downloads
		out.Downloads7d += s.Downloads7d
		out.Downloads30d += s.Downloads30d
	}
	return out
}

func metricValue(s MetricSummary, metric string) (int64, string, bool) {
	switch metric {
	case "pulls": return s.Downloads, "GHCR pulls", true
	case "pulls-7d": return s.Downloads7d, "GHCR pulls 7d", true
	case "pulls-30d": return s.Downloads30d, "GHCR pulls 30d", true
	default: return 0, "", false
	}
}

func (a *App) handleM1Badge(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/badge/"), "/")
	if len(parts) != 2 || !strings.HasSuffix(parts[1], ".svg") { http.NotFound(w, r); return }
	metric := strings.TrimSuffix(parts[1], ".svg")
	var summary MetricSummary
	var err error
	if parts[0] == "org" { summary = a.orgSummary() } else { summary, err = a.packageSummary(parts[0]) }
	if err != nil { http.NotFound(w, r); return }
	value, label, ok := metricValue(summary, metric)
	if !ok { http.NotFound(w, r); return }
	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Cache-Control", "public, max-age=300")
	_, _ = w.Write([]byte(badgeSVG(label, compact(value))))
}

func (a *App) handleShields(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v1/badge/"), "/")
	if len(parts) != 2 { http.NotFound(w, r); return }
	var summary MetricSummary
	var err error
	if parts[0] == "org" { summary = a.orgSummary() } else { summary, err = a.packageSummary(parts[0]) }
	if err != nil { http.NotFound(w, r); return }
	value, label, ok := metricValue(summary, parts[1])
	if !ok { http.NotFound(w, r); return }
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(ShieldsBadge{SchemaVersion:1, Label:label, Message:compact(value), Color:"2ea44f"})
}

func (a *App) handleOrgJSON(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/api/v1/org" { http.NotFound(w, r); return }
	s := a.orgSummary()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"owner":a.cfg.Owner,"downloads":s.Downloads,"downloads_7d":s.Downloads7d,"downloads_30d":s.Downloads30d})
}

func (a *App) writeM1Metrics(w http.ResponseWriter) {
	var org MetricSummary
	for _, pkg := range a.cfg.Packages {
		s, err := a.packageSummary(pkg)
		if err != nil && !errors.Is(err, sql.ErrNoRows) { continue }
		if err != nil { continue }
		fmt.Fprintf(w, "ghcr_downloads_7d{owner=%q,package=%q} %d\n", a.cfg.Owner, pkg, s.Downloads7d)
		fmt.Fprintf(w, "ghcr_downloads_30d{owner=%q,package=%q} %d\n", a.cfg.Owner, pkg, s.Downloads30d)
		org.Downloads += s.Downloads
		org.Downloads7d += s.Downloads7d
		org.Downloads30d += s.Downloads30d
	}
	fmt.Fprintf(w, "ghcr_org_downloads_total{owner=%q} %d\n", a.cfg.Owner, org.Downloads)
	fmt.Fprintf(w, "ghcr_org_downloads_7d{owner=%q} %d\n", a.cfg.Owner, org.Downloads7d)
	fmt.Fprintf(w, "ghcr_org_downloads_30d{owner=%q} %d\n", a.cfg.Owner, org.Downloads30d)
}
