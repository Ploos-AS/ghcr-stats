package main

import (
	"encoding/csv"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func m43TestApp(t *testing.T) *App {
	t.Helper()
	s, err := OpenStore(filepath.Join(t.TempDir(), "m43.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	base := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	for _, st := range []PackageStat{
		{Package: "soju", Downloads: 100, UpdatedAt: base},
		{Package: "mineflayer", Downloads: 50, UpdatedAt: base.Add(6 * time.Hour)},
		{Package: "soju", Downloads: 130, UpdatedAt: base.Add(24 * time.Hour)},
		{Package: "mineflayer", Downloads: 70, UpdatedAt: base.Add(30 * time.Hour)},
	} {
		if err := s.Save(st); err != nil {
			t.Fatal(err)
		}
	}
	return &App{cfg: Config{Owner: "Ploos-AS"}, store: s, packages: []string{"soju", "mineflayer"}, lastErr: map[string]string{}}
}

func TestM43PackageJSONExport(t *testing.T) {
	a := m43TestApp(t)
	r := httptest.NewRequest(http.MethodGet, "/api/v1/packages/soju/export?format=json&period=all", nil)
	w := httptest.NewRecorder()
	a.routes().ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var got struct {
		Owner   string         `json:"owner"`
		Package string         `json:"package"`
		Period  string         `json:"period"`
		Points  []HistoryPoint `json:"points"`
	}
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.Owner != "Ploos-AS" || got.Package != "soju" || got.Period != "all" || len(got.Points) != 2 {
		t.Fatalf("unexpected export: %+v", got)
	}
	if got.Points[1].Downloads != 130 || got.Points[1].Delta != 30 {
		t.Fatalf("unexpected final point: %+v", got.Points[1])
	}
}

func TestM43PackageCSVExport(t *testing.T) {
	a := m43TestApp(t)
	r := httptest.NewRequest(http.MethodGet, "/api/v1/packages/soju/export?format=csv&period=all", nil)
	w := httptest.NewRecorder()
	a.routes().ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if got := w.Header().Get("Content-Disposition"); !strings.Contains(got, `filename="ghcr-stats-soju-all.csv"`) {
		t.Fatalf("Content-Disposition=%q", got)
	}
	rows, err := csv.NewReader(strings.NewReader(w.Body.String())).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("rows=%d want 3", len(rows))
	}
	wantHeader := []string{"owner", "package", "period", "timestamp", "downloads", "delta"}
	for i := range wantHeader {
		if rows[0][i] != wantHeader[i] {
			t.Fatalf("header[%d]=%q want %q", i, rows[0][i], wantHeader[i])
		}
	}
	if rows[2][0] != "Ploos-AS" || rows[2][1] != "soju" || rows[2][2] != "all" || rows[2][4] != "130" || rows[2][5] != "30" {
		t.Fatalf("unexpected row: %#v", rows[2])
	}
}

func TestM43OrgExportUsesCarryForwardHistory(t *testing.T) {
	a := m43TestApp(t)
	r := httptest.NewRequest(http.MethodGet, "/api/v1/org/export?format=json&period=all", nil)
	w := httptest.NewRecorder()
	a.routes().ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var got struct {
		Owner  string         `json:"owner"`
		Period string         `json:"period"`
		Points []HistoryPoint `json:"points"`
	}
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.Owner != "Ploos-AS" || got.Period != "all" || len(got.Points) != 4 {
		t.Fatalf("unexpected export: %+v", got)
	}
	if got.Points[len(got.Points)-1].Downloads != 200 {
		t.Fatalf("final org downloads=%d want 200", got.Points[len(got.Points)-1].Downloads)
	}
}

func TestM43ExportValidation(t *testing.T) {
	a := m43TestApp(t)
	for _, tc := range []struct {
		method string
		url    string
		want   int
	}{
		{http.MethodGet, "/api/v1/packages/soju/export?format=xml&period=all", http.StatusBadRequest},
		{http.MethodGet, "/api/v1/org/export?format=json&period=banana", http.StatusBadRequest},
		{http.MethodPost, "/api/v1/packages/soju/export?format=json&period=all", http.StatusMethodNotAllowed},
		{http.MethodPost, "/api/v1/org/export?format=json&period=all", http.StatusMethodNotAllowed},
		{http.MethodGet, "/api/v1/packages/unknown/export?format=json&period=all", http.StatusNotFound},
	} {
		r := httptest.NewRequest(tc.method, tc.url, nil)
		w := httptest.NewRecorder()
		a.routes().ServeHTTP(w, r)
		if w.Code != tc.want {
			t.Fatalf("%s %s status=%d want %d body=%s", tc.method, tc.url, w.Code, tc.want, w.Body.String())
		}
	}
}
