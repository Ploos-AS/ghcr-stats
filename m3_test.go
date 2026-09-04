package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func m3TestApp(t *testing.T) *App {
	t.Helper()
	s, err := OpenStore(filepath.Join(t.TempDir(), "m3.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return &App{cfg: Config{Owner: "Ploos-AS", Interval: time.Hour}, store: s, collector: fakeCollector{name: "test"}, packages: []string{"alpha", "beta"}, packageSource: "explicit", lastErr: map[string]string{}}
}

type fakeCollector struct{ name string }

func (f fakeCollector) Name() string                                          { return f.name }
func (f fakeCollector) Collect(_ context.Context, _, _ string) (int64, error) { return 0, nil }

func seedM3(t *testing.T, a *App, now time.Time) {
	t.Helper()
	for _, st := range []PackageStat{
		{Package: "alpha", Downloads: 100, UpdatedAt: now.Add(-40 * 24 * time.Hour)},
		{Package: "alpha", Downloads: 125, UpdatedAt: now.Add(-8 * 24 * time.Hour)},
		{Package: "alpha", Downloads: 150, UpdatedAt: now},
		{Package: "beta", Downloads: 50, UpdatedAt: now.Add(-40 * 24 * time.Hour)},
		{Package: "beta", Downloads: 55, UpdatedAt: now.Add(-8 * 24 * time.Hour)},
		{Package: "beta", Downloads: 60, UpdatedAt: now},
	} {
		if err := a.store.Save(st); err != nil {
			t.Fatal(err)
		}
	}
}

func TestM3HistoryAndAnalytics(t *testing.T) {
	a := m3TestApp(t)
	now := time.Now().UTC().Truncate(time.Second)
	seedM3(t, a, now)
	points, err := a.store.History("alpha", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(points) != 3 || points[1].Delta != 25 || points[2].Delta != 25 {
		t.Fatalf("unexpected history: %#v", points)
	}
	s, err := a.analyticsSummary("alpha", now)
	if err != nil {
		t.Fatal(err)
	}
	if s.Downloads != 150 || s.Downloads24h != 25 || s.Downloads7d != 25 || s.Downloads30d != 50 || s.Downloads90d != 0 {
		t.Fatalf("unexpected summary: %#v", s)
	}
}

func TestM3Rankings(t *testing.T) {
	a := m3TestApp(t)
	now := time.Now().UTC().Truncate(time.Second)
	seedM3(t, a, now)
	items, err := a.rankings("30d", now)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].Package != "alpha" || items[0].Delta != 50 || items[0].Rank != 1 {
		t.Fatalf("unexpected ranking: %#v", items)
	}
}

func TestM3HistoryAPIAndDashboard(t *testing.T) {
	a := m3TestApp(t)
	now := time.Now().UTC().Truncate(time.Second)
	seedM3(t, a, now)
	h := a.routes()

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/packages/alpha/history?period=90d", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("history status=%d body=%s", rr.Code, rr.Body.String())
	}
	var payload struct {
		Package string         `json:"package"`
		Period  string         `json:"period"`
		Points  []HistoryPoint `json:"points"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Package != "alpha" || payload.Period != "90d" || len(payload.Points) != 3 {
		t.Fatalf("payload=%#v", payload)
	}

	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/rankings?period=30d", nil))
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"package":"alpha"`) {
		t.Fatalf("ranking: %d %s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "Ploos-AS GHCR stats") || !strings.Contains(rr.Body.String(), "/package/alpha") {
		t.Fatalf("dashboard: %d %s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/package/alpha", nil))
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "alpha") || !strings.Contains(rr.Body.String(), "canvas") {
		t.Fatalf("package page: %d %s", rr.Code, rr.Body.String())
	}
}

func TestM3RejectsUnknownPeriod(t *testing.T) {
	a := m3TestApp(t)
	rr := httptest.NewRecorder()
	a.routes().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/rankings?period=banana", nil))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d", rr.Code)
	}
}
