package main

import (
	"context"
	"errors"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type healthTestCollector struct{}

func (healthTestCollector) Name() string { return "test" }
func (healthTestCollector) Collect(_ context.Context, _, _ string) (int64, error) {
	return 0, errors.New("unused")
}

func newHealthTestApp(t *testing.T) *App {
	t.Helper()
	s, err := OpenStore(filepath.Join(t.TempDir(), "health.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return &App{
		cfg:           Config{Owner: "Ploos-AS", Interval: time.Hour},
		store:         s,
		collector:     healthTestCollector{},
		packages:      []string{"soju"},
		packageSource: "explicit",
		lastErr:       map[string]string{},
	}
}

func TestCollectorHealthFreshAndStale(t *testing.T) {
	a := newHealthTestApp(t)
	now := time.Now().UTC()
	if err := a.store.Save(PackageStat{Package: "soju", Downloads: 1, UpdatedAt: now.Add(-2 * time.Hour)}); err != nil {
		t.Fatal(err)
	}
	h := a.collectorHealth("soju", now)
	if !h.Up || h.Stale {
		t.Fatalf("fresh health = %#v", h)
	}
	if err := a.store.Save(PackageStat{Package: "old", Downloads: 1, UpdatedAt: now.Add(-4 * time.Hour)}); err != nil {
		t.Fatal(err)
	}
	a.packages = append(a.packages, "old")
	h = a.collectorHealth("old", now)
	if !h.Up || !h.Stale {
		t.Fatalf("stale health = %#v", h)
	}
}

func TestCollectorHealthError(t *testing.T) {
	a := newHealthTestApp(t)
	now := time.Now().UTC()
	if err := a.store.Save(PackageStat{Package: "soju", Downloads: 1, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	a.lastErr["soju"] = "parser changed"
	h := a.collectorHealth("soju", now)
	if h.Up || h.LastError != "parser changed" {
		t.Fatalf("health = %#v", h)
	}
}

func TestCollectorHealthMetricsAndAPI(t *testing.T) {
	a := newHealthTestApp(t)
	now := time.Now().UTC()
	if err := a.store.Save(PackageStat{Package: "soju", Downloads: 1, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/health?package=soju", nil)
	a.handleHealthJSON(rr, req)
	if rr.Code != 200 || !strings.Contains(rr.Body.String(), `"package":"soju"`) || !strings.Contains(rr.Body.String(), `"up":true`) {
		t.Fatalf("health API: %d %s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/metrics", nil)
	a.handleMetrics(rr, req)
	body := rr.Body.String()
	for _, metric := range []string{"ghcr_stats_collector_up", "ghcr_stats_snapshot_stale", "ghcr_stats_last_success_timestamp_seconds", "ghcr_stats_snapshot_age_seconds"} {
		if !strings.Contains(body, metric) {
			t.Fatalf("missing %s in metrics", metric)
		}
	}
}
