package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

type failureCounter struct {
	Total       uint64 `json:"total_failures"`
	Consecutive uint64 `json:"consecutive_failures"`
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

var collectionFailureState = struct {
	sync.RWMutex
	byApp map[*App]map[string]failureCounter
}{byApp: make(map[*App]map[string]failureCounter)}

func (a *App) recordCollectionResult(pkg string, err error) {
	collectionFailureState.Lock()
	defer collectionFailureState.Unlock()
	m := collectionFailureState.byApp[a]
	if m == nil {
		m = make(map[string]failureCounter)
		collectionFailureState.byApp[a] = m
	}
	st := m[pkg]
	if err != nil {
		st.Total++
		st.Consecutive++
	} else {
		st.Consecutive = 0
	}
	m[pkg] = st
}

func (a *App) failureStats(pkg string) failureCounter {
	collectionFailureState.RLock()
	defer collectionFailureState.RUnlock()
	return collectionFailureState.byApp[a][pkg]
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
}

func (a *App) handleOrgHealthJSON(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(a.orgHealth(time.Now().UTC()))
}
