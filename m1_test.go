package main

import (
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testM1App(t *testing.T) *App {
	t.Helper()
	s, err := OpenStore(filepath.Join(t.TempDir(), "m1.db"))
	if err != nil { t.Fatal(err) }
	t.Cleanup(func(){ _ = s.Close() })
	now := time.Now().UTC()
	for _, st := range []PackageStat{
		{Package:"soju", Downloads:100, UpdatedAt:now.Add(-31*24*time.Hour)},
		{Package:"soju", Downloads:120, UpdatedAt:now.Add(-8*24*time.Hour)},
		{Package:"soju", Downloads:150, UpdatedAt:now},
		{Package:"mineflayer", Downloads:200, UpdatedAt:now.Add(-31*24*time.Hour)},
		{Package:"mineflayer", Downloads:210, UpdatedAt:now.Add(-8*24*time.Hour)},
		{Package:"mineflayer", Downloads:230, UpdatedAt:now},
	} {
		if err := s.Save(st); err != nil { t.Fatal(err) }
	}
	return &App{cfg:Config{Owner:"Ploos-AS", Packages:[]string{"soju","mineflayer"}}, store:s, collector:GitHubHTMLCollector{}, lastErr:map[string]string{}}
}

func TestPackageSummary(t *testing.T) {
	a := testM1App(t)
	s, err := a.packageSummary("soju")
	if err != nil { t.Fatal(err) }
	if s.Downloads != 150 || s.Downloads7d != 30 || s.Downloads30d != 50 { t.Fatalf("%+v", s) }
}

func TestOrgSummary(t *testing.T) {
	a := testM1App(t)
	s := a.orgSummary()
	if s.Downloads != 380 || s.Downloads7d != 50 || s.Downloads30d != 80 { t.Fatalf("%+v", s) }
}

func TestBadgeAndShieldsEndpoints(t *testing.T) {
	a := testM1App(t)
	r := httptest.NewRequest("GET", "/badge/org/pulls-30d.svg", nil)
	w := httptest.NewRecorder(); a.handleM1Badge(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "80") { t.Fatalf("%d %s", w.Code, w.Body.String()) }

	r = httptest.NewRequest("GET", "/api/v1/badge/soju/pulls", nil)
	w = httptest.NewRecorder(); a.handleShields(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"schemaVersion":1`) || !strings.Contains(w.Body.String(), `"message":"150"`) { t.Fatalf("%d %s", w.Code, w.Body.String()) }
}

func TestM1Metrics(t *testing.T) {
	a := testM1App(t)
	w := httptest.NewRecorder(); r := httptest.NewRequest("GET", "/metrics", nil)
	a.handleMetrics(w, r)
	body := w.Body.String()
	for _, want := range []string{"ghcr_downloads_7d", "ghcr_downloads_30d", "ghcr_org_downloads_total", "ghcr_org_downloads_30d"} {
		if !strings.Contains(body, want) { t.Fatalf("missing %s in %s", want, body) }
	}
}
