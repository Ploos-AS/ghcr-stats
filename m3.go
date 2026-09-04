package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

type HistoryPoint struct {
	Timestamp time.Time `json:"timestamp"`
	Downloads int64     `json:"downloads"`
	Delta     int64     `json:"delta"`
}

type AnalyticsSummary struct {
	Package      string    `json:"package,omitempty"`
	Downloads    int64     `json:"downloads"`
	Downloads24h int64     `json:"downloads_24h"`
	Downloads7d  int64     `json:"downloads_7d"`
	Downloads30d int64     `json:"downloads_30d"`
	Downloads90d int64     `json:"downloads_90d"`
	UpdatedAt    time.Time `json:"updated_at,omitempty"`
}

type RankingEntry struct {
	Rank                int    `json:"rank"`
	Package             string `json:"package"`
	Downloads           int64  `json:"downloads"`
	Delta               int64  `json:"delta"`
	Stale               bool   `json:"stale"`
	CollectorUp         bool   `json:"collector_up"`
	ConsecutiveFailures uint64 `json:"consecutive_failures"`
}

func parsePeriod(v string) (time.Duration, string, error) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "", "30d":
		return 30 * 24 * time.Hour, "30d", nil
	case "24h", "1d":
		return 24 * time.Hour, "24h", nil
	case "7d":
		return 7 * 24 * time.Hour, "7d", nil
	case "90d":
		return 90 * 24 * time.Hour, "90d", nil
	case "all", "all-time":
		return 0, "all", nil
	default:
		return 0, "", fmt.Errorf("unsupported period %q", v)
	}
}

func (s *Store) History(pkg string, since time.Time) ([]HistoryPoint, error) {
	query := "SELECT downloads,collected_at FROM snapshots WHERE package=?"
	args := []any{pkg}
	if !since.IsZero() {
		query += " AND collected_at>=?"
		args = append(args, since.UTC().Format(time.RFC3339Nano))
	}
	query += " ORDER BY collected_at ASC"
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []HistoryPoint
	var prev int64
	var havePrev bool
	for rows.Next() {
		var downloads int64
		var raw string
		if err := rows.Scan(&downloads, &raw); err != nil {
			return nil, err
		}
		ts, err := time.Parse(time.RFC3339Nano, raw)
		if err != nil {
			return nil, err
		}
		delta := int64(0)
		if havePrev && downloads >= prev {
			delta = downloads - prev
		}
		out = append(out, HistoryPoint{Timestamp: ts, Downloads: downloads, Delta: delta})
		prev, havePrev = downloads, true
	}
	return out, rows.Err()
}

func (s *Store) First(pkg string) (PackageStat, error) {
	var st PackageStat
	var ts string
	err := s.db.QueryRow("SELECT package,downloads,collected_at FROM snapshots WHERE package=? ORDER BY collected_at ASC LIMIT 1", pkg).Scan(&st.Package, &st.Downloads, &ts)
	if err != nil {
		return st, err
	}
	st.UpdatedAt, err = time.Parse(time.RFC3339Nano, ts)
	return st, err
}

func (s *Store) DeltaAll(pkg string) (int64, error) {
	first, err := s.First(pkg)
	if err != nil {
		return 0, err
	}
	latest, err := s.Latest(pkg)
	if err != nil {
		return 0, err
	}
	if latest.Downloads < first.Downloads {
		return 0, nil
	}
	return latest.Downloads - first.Downloads, nil
}

func (a *App) analyticsSummary(pkg string, now time.Time) (AnalyticsSummary, error) {
	st, err := a.store.Latest(pkg)
	if err != nil {
		return AnalyticsSummary{}, err
	}
	d24, err := a.store.DeltaSince(pkg, now.Add(-24*time.Hour))
	if err != nil {
		return AnalyticsSummary{}, err
	}
	d7, err := a.store.DeltaSince(pkg, now.Add(-7*24*time.Hour))
	if err != nil {
		return AnalyticsSummary{}, err
	}
	d30, err := a.store.DeltaSince(pkg, now.Add(-30*24*time.Hour))
	if err != nil {
		return AnalyticsSummary{}, err
	}
	d90, err := a.store.DeltaSince(pkg, now.Add(-90*24*time.Hour))
	if err != nil {
		return AnalyticsSummary{}, err
	}
	return AnalyticsSummary{Package: pkg, Downloads: st.Downloads, Downloads24h: d24, Downloads7d: d7, Downloads30d: d30, Downloads90d: d90, UpdatedAt: st.UpdatedAt}, nil
}

func (a *App) orgAnalytics(now time.Time) AnalyticsSummary {
	var out AnalyticsSummary
	for _, pkg := range a.packageNames() {
		s, err := a.analyticsSummary(pkg, now)
		if err != nil {
			continue
		}
		out.Downloads += s.Downloads
		out.Downloads24h += s.Downloads24h
		out.Downloads7d += s.Downloads7d
		out.Downloads30d += s.Downloads30d
		out.Downloads90d += s.Downloads90d
		if s.UpdatedAt.After(out.UpdatedAt) {
			out.UpdatedAt = s.UpdatedAt
		}
	}
	return out
}

func periodDelta(s AnalyticsSummary, period string, allDelta int64) int64 {
	switch period {
	case "24h":
		return s.Downloads24h
	case "7d":
		return s.Downloads7d
	case "30d":
		return s.Downloads30d
	case "90d":
		return s.Downloads90d
	case "all":
		return allDelta
	default:
		return 0
	}
}

func (a *App) rankings(period string, now time.Time) ([]RankingEntry, error) {
	_, normalized, err := parsePeriod(period)
	if err != nil {
		return nil, err
	}
	entries := make([]RankingEntry, 0, len(a.packageNames()))
	for _, pkg := range a.packageNames() {
		s, err := a.analyticsSummary(pkg, now)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, err
		}
		allDelta, _ := a.store.DeltaAll(pkg)
		h := a.collectorHealth(pkg, now)
		entries = append(entries, RankingEntry{Package: pkg, Downloads: s.Downloads, Delta: periodDelta(s, normalized, allDelta), Stale: h.Stale, CollectorUp: h.Up, ConsecutiveFailures: h.ConsecutiveFailures})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Delta == entries[j].Delta {
			return entries[i].Package < entries[j].Package
		}
		return entries[i].Delta > entries[j].Delta
	})
	for i := range entries {
		entries[i].Rank = i + 1
	}
	return entries, nil
}

func (a *App) handlePackageAPI(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/v1/packages/")
	parts := strings.Split(strings.Trim(rest, "/"), "/")
	if len(parts) == 2 && parts[1] == "history" {
		a.handlePackageHistory(w, r, parts[0])
		return
	}
	if len(parts) == 2 && parts[1] == "export" {
		a.handlePackageExport(w, r, parts[0])
		return
	}
	if len(parts) == 1 && parts[0] != "" {
		a.handleJSON(w, r)
		return
	}
	http.NotFound(w, r)
}

func (a *App) handlePackageHistory(w http.ResponseWriter, r *http.Request, pkg string) {
	d, period, err := parsePeriod(r.URL.Query().Get("period"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	since := time.Time{}
	if d > 0 {
		since = time.Now().UTC().Add(-d)
	}
	points, err := a.store.History(pkg, since)
	if err != nil {
		http.Error(w, "history query failed", http.StatusInternalServerError)
		return
	}
	if len(points) == 0 {
		http.Error(w, "no data", http.StatusNotFound)
		return
	}
	h := a.collectorHealth(pkg, time.Now().UTC())
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"package": pkg, "period": period, "points": points, "collector_up": h.Up, "stale": h.Stale, "last_success": h.LastSuccess})
}

func (a *App) handleRankings(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/api/v1/rankings" {
		http.NotFound(w, r)
		return
	}
	period := r.URL.Query().Get("period")
	items, err := a.rankings(period, time.Now().UTC())
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	_, normalized, _ := parsePeriod(period)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"owner": a.cfg.Owner, "period": normalized, "rankings": items})
}

func (a *App) handleOrgHistory(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/api/v1/org/history" {
		http.NotFound(w, r)
		return
	}
	d, period, err := parsePeriod(r.URL.Query().Get("period"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	since := time.Time{}
	if d > 0 {
		since = time.Now().UTC().Add(-d)
	}
	points, err := a.store.OrgHistory(a.packageNames(), since)
	if err != nil {
		http.Error(w, "organization history query failed", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"owner": a.cfg.Owner, "period": period, "points": points})
}

func (a *App) handleM3Index(w http.ResponseWriter, r *http.Request) {
	period := normalizeDashboardPeriod(r.URL.Query().Get("period"))
	if r.URL.Path == "/" {
		rankings, _ := a.rankings(period, time.Now().UTC())
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = dashboardTemplateM32.Execute(w, map[string]any{
			"Owner":       a.cfg.Owner,
			"Org":         a.orgAnalytics(time.Now().UTC()),
			"Rankings":    rankings,
			"Period":      period,
			"Periods":     dashboardPeriods(period),
			"OrgDegraded": orgDashboardDegraded(a),
		})
		return
	}
	if strings.HasPrefix(r.URL.Path, "/package/") {
		pkg := strings.Trim(strings.TrimPrefix(r.URL.Path, "/package/"), "/")
		if !validPackagePath(pkg) {
			http.NotFound(w, r)
			return
		}
		s, err := a.analyticsSummary(pkg, time.Now().UTC())
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = packageTemplateM32.Execute(w, map[string]any{
			"Summary": s,
			"Health":  a.collectorHealth(pkg, time.Now().UTC()),
			"Period":  period,
			"Periods": dashboardPeriods(period),
		})
		return
	}
	http.NotFound(w, r)
}

func limitRankings(items []RankingEntry, raw string) []RankingEntry {
	if raw == "" {
		return items
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 || n >= len(items) {
		return items
	}
	return items[:n]
}
