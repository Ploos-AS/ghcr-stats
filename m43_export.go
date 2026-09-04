package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

func exportFormat(r *http.Request) (string, error) {
	v := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format")))
	if v == "" {
		v = "json"
	}
	if v != "json" && v != "csv" {
		return "", fmt.Errorf("unsupported format %q", v)
	}
	return v, nil
}

func exportPeriod(r *http.Request) (time.Time, string, error) {
	d, period, err := parsePeriod(r.URL.Query().Get("period"))
	if err != nil {
		return time.Time{}, "", err
	}
	if d == 0 {
		return time.Time{}, period, nil
	}
	return time.Now().UTC().Add(-d), period, nil
}

func writeHistoryExport(w http.ResponseWriter, format, owner, pkg, period string, points []HistoryPoint) error {
	if format == "json" {
		w.Header().Set("Content-Type", "application/json")
		return json.NewEncoder(w).Encode(map[string]any{"owner": owner, "package": pkg, "period": period, "points": points})
	}
	name := "org"
	if pkg != "" {
		name = pkg
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="ghcr-stats-%s-%s.csv"`, name, period))
	cw := csv.NewWriter(w)
	if err := cw.Write([]string{"owner", "package", "period", "timestamp", "downloads", "delta"}); err != nil {
		return err
	}
	for _, p := range points {
		if err := cw.Write([]string{owner, pkg, period, p.Timestamp.UTC().Format(time.RFC3339), fmt.Sprintf("%d", p.Downloads), fmt.Sprintf("%d", p.Delta)}); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

func (a *App) handlePackageExport(w http.ResponseWriter, r *http.Request, pkg string) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	format, err := exportFormat(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	since, period, err := exportPeriod(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
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
	_ = writeHistoryExport(w, format, a.cfg.Owner, pkg, period, points)
}

func (a *App) handleOrgExport(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/api/v1/org/export" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	format, err := exportFormat(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	since, period, err := exportPeriod(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	points, err := a.store.OrgHistory(a.packageNames(), since)
	if err != nil {
		http.Error(w, "organization history query failed", http.StatusInternalServerError)
		return
	}
	if len(points) == 0 {
		http.Error(w, "no data", http.StatusNotFound)
		return
	}
	_ = writeHistoryExport(w, format, a.cfg.Owner, "", period, points)
}
