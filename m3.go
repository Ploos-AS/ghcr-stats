package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
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
	byTime := map[string]int64{}
	for _, pkg := range a.packageNames() {
		points, err := a.store.History(pkg, since)
		if err != nil {
			continue
		}
		for _, p := range points {
			byTime[p.Timestamp.UTC().Format(time.RFC3339Nano)] += p.Downloads
		}
	}
	keys := make([]string, 0, len(byTime))
	for k := range byTime {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	points := make([]HistoryPoint, 0, len(keys))
	var prev int64
	for i, k := range keys {
		ts, _ := time.Parse(time.RFC3339Nano, k)
		delta := int64(0)
		if i > 0 && byTime[k] >= prev {
			delta = byTime[k] - prev
		}
		points = append(points, HistoryPoint{Timestamp: ts, Downloads: byTime[k], Delta: delta})
		prev = byTime[k]
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"owner": a.cfg.Owner, "period": period, "points": points})
}

var dashboardTemplate = template.Must(template.New("dashboard").Funcs(template.FuncMap{"compact": compact}).Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>ghcr-stats</title>
<style>body{font-family:system-ui,sans-serif;margin:0;background:#0d1117;color:#e6edf3}main{max-width:1100px;margin:auto;padding:28px}.cards{display:grid;grid-template-columns:repeat(auto-fit,minmax(150px,1fr));gap:12px}.card,table{background:#161b22;border:1px solid #30363d;border-radius:8px}.card{padding:16px}.big{font-size:1.7rem;font-weight:700}table{width:100%;border-collapse:collapse;margin-top:20px;overflow:hidden}th,td{text-align:left;padding:11px;border-bottom:1px solid #30363d}a{color:#58a6ff;text-decoration:none}.ok{color:#3fb950}.bad{color:#f85149}.muted{color:#8b949e}</style></head><body><main>
<h1>{{.Owner}} GHCR stats</h1><p class="muted">Historical analytics from periodic GHCR snapshots.</p>
<div class="cards"><div class="card"><div class="muted">Total</div><div class="big">{{compact .Org.Downloads}}</div></div><div class="card"><div class="muted">24h</div><div class="big">+{{compact .Org.Downloads24h}}</div></div><div class="card"><div class="muted">7d</div><div class="big">+{{compact .Org.Downloads7d}}</div></div><div class="card"><div class="muted">30d</div><div class="big">+{{compact .Org.Downloads30d}}</div></div><div class="card"><div class="muted">90d</div><div class="big">+{{compact .Org.Downloads90d}}</div></div></div>
<table><thead><tr><th>#</th><th>Package</th><th>30d</th><th>Total</th><th>Health</th></tr></thead><tbody>{{range .Rankings}}<tr><td>{{.Rank}}</td><td><a href="/package/{{.Package}}">{{.Package}}</a></td><td>+{{compact .Delta}}</td><td>{{compact .Downloads}}</td><td>{{if .CollectorUp}}{{if .Stale}}<span class="bad">stale</span>{{else}}<span class="ok">healthy</span>{{end}}{{else}}<span class="bad">collector error</span>{{end}}</td></tr>{{end}}</tbody></table>
</main></body></html>`))

var packageTemplate = template.Must(template.New("package").Funcs(template.FuncMap{"compact": compact}).Parse(`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>{{.Summary.Package}} · ghcr-stats</title><style>body{font-family:system-ui,sans-serif;max-width:960px;margin:auto;padding:28px;background:#0d1117;color:#e6edf3}a{color:#58a6ff}.cards{display:flex;gap:12px;flex-wrap:wrap}.card{padding:14px;border:1px solid #30363d;background:#161b22;border-radius:8px;min-width:120px}.big{font-size:1.5rem;font-weight:700}canvas{width:100%;height:280px;background:#161b22;border:1px solid #30363d;border-radius:8px;margin-top:20px}</style></head><body><a href="/">← overview</a><h1>{{.Summary.Package}}</h1><div class="cards"><div class="card"><div>Total</div><div class="big">{{compact .Summary.Downloads}}</div></div><div class="card"><div>24h</div><div class="big">+{{compact .Summary.Downloads24h}}</div></div><div class="card"><div>7d</div><div class="big">+{{compact .Summary.Downloads7d}}</div></div><div class="card"><div>30d</div><div class="big">+{{compact .Summary.Downloads30d}}</div></div><div class="card"><div>90d</div><div class="big">+{{compact .Summary.Downloads90d}}</div></div></div><p>Collector: {{if .Health.Up}}healthy{{else}}error{{end}} · stale: {{.Health.Stale}} · last success: {{.Health.LastSuccess}}</p><canvas id="chart" width="900" height="280"></canvas><script>fetch('/api/v1/packages/{{.Summary.Package}}/history?period=90d').then(r=>r.json()).then(d=>{const c=document.getElementById('chart'),x=c.getContext('2d'),p=d.points||[];if(p.length<2)return;const vals=p.map(v=>v.downloads),min=Math.min(...vals),max=Math.max(...vals),span=Math.max(1,max-min);x.strokeStyle='#58a6ff';x.lineWidth=2;x.beginPath();p.forEach((v,i)=>{const px=20+i*(c.width-40)/(p.length-1),py=c.height-20-(v.downloads-min)*(c.height-40)/span;i?x.lineTo(px,py):x.moveTo(px,py)});x.stroke()})</script></body></html>`))

func (a *App) handleM3Index(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/" {
		rankings, _ := a.rankings("30d", time.Now().UTC())
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = dashboardTemplate.Execute(w, map[string]any{"Owner": a.cfg.Owner, "Org": a.orgAnalytics(time.Now().UTC()), "Rankings": rankings})
		return
	}
	if strings.HasPrefix(r.URL.Path, "/package/") {
		pkg := strings.Trim(strings.TrimPrefix(r.URL.Path, "/package/"), "/")
		if pkg == "" || strings.Contains(pkg, "/") {
			http.NotFound(w, r)
			return
		}
		s, err := a.analyticsSummary(pkg, time.Now().UTC())
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = packageTemplate.Execute(w, map[string]any{"Summary": s, "Health": a.collectorHealth(pkg, time.Now().UTC())})
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
