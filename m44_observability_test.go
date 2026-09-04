package main

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newM44TestApp(t *testing.T) *App {
	t.Helper()
	store, err := OpenStore(filepath.Join(t.TempDir(), "m44.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return &App{
		cfg: Config{Owner: "Ploos-AS", Packages: []string{"pkg"}, Interval: time.Hour},
		store: store,
		collector: GitHubHTMLCollector{Client: http.DefaultClient},
		packages: []string{"pkg"},
		packageSource: "explicit",
		lastErr: map[string]string{},
	}
}

func TestReadinessRequiresFreshData(t *testing.T) {
	a := newM44TestApp(t)
	now := time.Now().UTC()
	if got := a.readiness(now); got.Ready {
		t.Fatalf("expected not ready without data: %+v", got)
	}
	if err := a.store.Save(PackageStat{Package: "pkg", Downloads: 10, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if got := a.readiness(now); !got.Ready || got.Fresh != 1 || got.WithData != 1 {
		t.Fatalf("expected ready with fresh data: %+v", got)
	}
}

func TestReadinessRejectsStaleOnlyData(t *testing.T) {
	a := newM44TestApp(t)
	now := time.Now().UTC()
	if err := a.store.Save(PackageStat{Package: "pkg", Downloads: 10, UpdatedAt: now.Add(-4 * time.Hour)}); err != nil {
		t.Fatal(err)
	}
	got := a.readiness(now)
	if got.Ready || got.Stale != 1 || got.Fresh != 0 {
		t.Fatalf("expected stale-only data to be not ready: %+v", got)
	}
}

func TestReadyzMethodsAndStatus(t *testing.T) {
	a := newM44TestApp(t)
	rr := httptest.NewRecorder()
	a.handleReadyz(rr, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rr.Code)
	}
	if err := a.store.Save(PackageStat{Package: "pkg", Downloads: 10, UpdatedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	rr = httptest.NewRecorder()
	a.handleReadyz(rr, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if rr.Code != http.StatusOK || !strings.HasPrefix(rr.Body.String(), "ready\n") {
		t.Fatalf("expected ready 200, got %d %q", rr.Code, rr.Body.String())
	}
	rr = httptest.NewRecorder()
	a.handleReadyz(rr, httptest.NewRequest(http.MethodPost, "/readyz", nil))
	if rr.Code != http.StatusMethodNotAllowed || rr.Header().Get("Allow") != "GET, HEAD" {
		t.Fatalf("expected 405 and Allow header, got %d %q", rr.Code, rr.Header().Get("Allow"))
	}
}

func TestM44Metrics(t *testing.T) {
	a := newM44TestApp(t)
	if err := a.store.Save(PackageStat{Package: "pkg", Downloads: 10, UpdatedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	rr := httptest.NewRecorder()
	a.writeM44Metrics(rr)
	body := rr.Body.String()
	for _, want := range []string{
		"ghcr_stats_info{version=",
		"ghcr_stats_ready{owner=\"Ploos-AS\"} 1",
		"ghcr_stats_database_up{owner=\"Ploos-AS\"} 1",
		"ghcr_stats_packages_with_data{owner=\"Ploos-AS\"} 1",
		"ghcr_stats_fresh_packages{owner=\"Ploos-AS\"} 1",
		"ghcr_stats_stale_packages{owner=\"Ploos-AS\"} 0",
		"ghcr_stats_stale_after_seconds{owner=\"Ploos-AS\"}",
		"ghcr_stats_process_uptime_seconds",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("metrics missing %q:\n%s", want, body)
		}
	}
}
