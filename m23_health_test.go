package main

import (
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestFailureCountersResetConsecutiveOnSuccess(t *testing.T) {
	a := newHealthTestApp(t)
	a.recordCollectionResult("soju", errors.New("boom 1"))
	a.recordCollectionResult("soju", errors.New("boom 2"))
	fs := a.failureStats("soju")
	if fs.Total != 2 || fs.Consecutive != 2 {
		t.Fatalf("after failures = %#v", fs)
	}
	a.recordCollectionResult("soju", nil)
	fs = a.failureStats("soju")
	if fs.Total != 2 || fs.Consecutive != 0 {
		t.Fatalf("after recovery = %#v", fs)
	}
}

func TestOrgHealthDegradedAndRecovered(t *testing.T) {
	a := newHealthTestApp(t)
	now := time.Now().UTC()
	if err := a.store.Save(PackageStat{Package: "soju", Downloads: 1, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}

	a.recordCollectionResult("soju", errors.New("parser failed"))
	a.lastErr["soju"] = "parser failed"
	org := a.orgHealth(now)
	if org.Healthy || org.Status != "degraded" || org.FailingPackages != 1 || org.ConsecutiveFailures != 1 {
		t.Fatalf("degraded org = %#v", org)
	}

	a.recordCollectionResult("soju", nil)
	delete(a.lastErr, "soju")
	org = a.orgHealth(now)
	if !org.Healthy || org.Status != "healthy" || org.UnhealthyPackages != 0 || org.TotalFailures != 1 {
		t.Fatalf("recovered org = %#v", org)
	}
}

func TestM23Metrics(t *testing.T) {
	a := newHealthTestApp(t)
	now := time.Now().UTC()
	if err := a.store.Save(PackageStat{Package: "soju", Downloads: 1, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	a.recordCollectionResult("soju", errors.New("boom"))
	a.lastErr["soju"] = "boom"

	rr := httptest.NewRecorder()
	a.writeM23Metrics(rr)
	body := rr.Body.String()
	for _, metric := range []string{
		"ghcr_stats_collection_errors_total",
		"ghcr_stats_consecutive_failures",
		"ghcr_stats_org_healthy",
		"ghcr_stats_org_unhealthy_packages",
		"ghcr_stats_org_stale_packages",
		"ghcr_stats_org_failing_packages",
	} {
		if !strings.Contains(body, metric) {
			t.Fatalf("missing %s in metrics: %s", metric, body)
		}
	}
}

func TestHealthAPIIncludesOrgAndFailureCounts(t *testing.T) {
	a := newHealthTestApp(t)
	now := time.Now().UTC()
	if err := a.store.Save(PackageStat{Package: "soju", Downloads: 1, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	a.recordCollectionResult("soju", errors.New("boom"))
	a.lastErr["soju"] = "boom"

	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/health", nil)
	a.handleHealthJSON(rr, req)
	body := rr.Body.String()
	for _, want := range []string{`"org":`, `"status":"degraded"`, `"total_failures":1`, `"consecutive_failures":1`} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %s in health API: %s", want, body)
		}
	}
}
