package main

import (
	"fmt"
	"net/http"
)

func (a *App) writeM53Metrics(w http.ResponseWriter) {
	present, missing, err := a.store.PackageLifecycleCounts()
	if err != nil {
		return
	}
	fmt.Fprintf(w, "ghcr_stats_lifecycle_present_packages{owner=%q} %d\n", a.cfg.Owner, present)
	fmt.Fprintf(w, "ghcr_stats_lifecycle_missing_packages{owner=%q} %d\n", a.cfg.Owner, missing)
}
