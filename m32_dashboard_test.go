package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestM32DashboardPeriods(t *testing.T) {
	if got := normalizeDashboardPeriod("7d"); got != "7d" {
		t.Fatalf("period=%q", got)
	}
	if got := normalizeDashboardPeriod("bogus"); got != "30d" {
		t.Fatalf("fallback=%q", got)
	}
	items := dashboardPeriods("90d")
	if len(items) != 5 || !items[3].Active {
		t.Fatalf("periods=%#v", items)
	}
}

func TestM32DashboardPeriodSelectionAndOrgChart(t *testing.T) {
	a := m3TestApp(t)
	now := time.Now().UTC().Truncate(time.Second)
	seedM3(t, a, now)
	rr := httptest.NewRecorder()
	a.routes().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/?period=7d", nil))
	body := rr.Body.String()
	if rr.Code != http.StatusOK || !strings.Contains(body, "Organization history · 7d") || !strings.Contains(body, "/api/v1/org/history?period=") || !strings.Contains(body, "class=\"active\">7d</a>") {
		t.Fatalf("dashboard status=%d body=%s", rr.Code, body)
	}
}

func TestM32PackagePeriodSelection(t *testing.T) {
	a := m3TestApp(t)
	now := time.Now().UTC().Truncate(time.Second)
	seedM3(t, a, now)
	rr := httptest.NewRecorder()
	a.routes().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/package/alpha?period=24h", nil))
	body := rr.Body.String()
	if rr.Code != http.StatusOK || !strings.Contains(body, "History · 24h") || !strings.Contains(body, "period=24h") || !strings.Contains(body, "Not enough history yet") {
		t.Fatalf("package status=%d body=%s", rr.Code, body)
	}
}
