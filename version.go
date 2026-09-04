package main

import (
	"encoding/json"
	"net/http"
)

var (
	version  = "dev"
	revision = "unknown"
)

type VersionInfo struct {
	Version  string `json:"version"`
	Revision string `json:"revision"`
}

func handleVersion(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(VersionInfo{Version: version, Revision: revision})
}
