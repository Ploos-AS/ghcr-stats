package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

type failureCounter struct {
	Total       uint64 `json:"total_failures"`
	Consecutive uint64 `json:"consecutive_failures"`
	LastError   string `json:"last_error,omitempty"`
}

type OrgHealth struct {
	Status              string `json:"status"`
	Healthy             bool   `json:"healthy"`
	Packages            int    `json:"packages"`
	HealthyPackages     int    `json:"healthy_packages"`
	UnhealthyPackages   int    `json:"unhealthy_packages"`
	StalePackages       int    `json:"stale_packages"`
	FailingPackages     int    `json:"failing_packages"`
	TotalFailures       uint64 `json:"total_failures"`
	ConsecutiveFailures uint64 `json:"consecutive_failures"`
}

func (a *App) recordCollectionResult(pkg string, collectErr error) {
	before := a.failureStats(pkg)
	at := time.Now().UTC()
	if dbErr := a.store.RecordCollectionResult(pkg, collectErr, at); dbErr != nil {
		log.Printf("persist collection state %s: %v", pkg, dbErr)
		return
	}
	a.reconcileM52(pkg, collectErr, before, at)
	a.observePackageLifecycle(at)
}

func (a *App) failureStats(pkg string) failureCounter {
	st, err := a.store.CollectionFailureStats(pkg)
	if err != nil {
		log.Printf("read collection state %s: %v", pkg, err)
		return failureCounter{}
	}
	return st
}

func (a *App) orgHealth(now time.Time) OrgHealth {
	org := OrgHealth{Status: "healthy", Healthy: true}
	for _, pkg := range a.packageNames() {
		org.Packages++
		h := a.collectorHealth(pkg, now)
		fs := a.failureStats(pkg)
		org.TotalFailures += fs.Total
		org.ConsecutiveFailures += fs.Consecutive
		if h.Stale {
			org.StalePackages++
		}
		if !h.Up || fs.Consecutive > 0 {
			org.FailingPackages++
		}
		if h.Up && !h.Stale && fs.Consecutive == 0 {
			org.HealthyPackages++
		} else {
			org.UnhealthyPackages++
		}
	}
	if org.UnhealthyPackages > 0 {
		org.Status = "degraded"
		org.Healthy = false
	}
	return org
}

func (a *App) writeM23Metrics(w http.ResponseWriter) {
	now := time.Now().UTC()
	for _, pkg := range a.packageNames() {
		fs := a.failureStats(pkg)
		fmt.Fprintf(w, "ghcr_stats_collection_errors_total{owner=%q,package=%q} %d\n", a.cfg.Owner, pkg, fs.Total)
		fmt.Fprintf(w, "ghcr_stats_consecutive_failures{owner=%q,package=%q} %d\n", a.cfg.Owner, pkg, fs.Consecutive)
	}
	org := a.orgHealth(now)
	healthy := 0
	if org.Healthy {
		healthy = 1
	}
	fmt.Fprintf(w, "ghcr_stats_org_healthy{owner=%q} %d\n", a.cfg.Owner, healthy)
	fmt.Fprintf(w, "ghcr_stats_org_unhealthy_packages{owner=%q} %d\n", a.cfg.Owner, org.UnhealthyPackages)
	fmt.Fprintf(w, "ghcr_stats_org_stale_packages{owner=%q} %d\n", a.cfg.Owner, org.StalePackages)
	fmt.Fprintf(w, "ghcr_stats_org_failing_packages{owner=%q} %d\n", a.cfg.Owner, org.FailingPackages)
	a.writeM50Metrics(w)
	a.writeM51Metrics(w)
	a.writeM52Metrics(w)
	a.writeM53Metrics(w)
	a.writeM54Metrics(w)
}

func (a *App) handleOrgHealthJSON(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(a.orgHealth(time.Now().UTC()))
}
