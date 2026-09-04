package main

import (
	"fmt"
	"net/http"
)

// initM50 initializes persistent M5 event storage.
func (a *App) initM50() error {
	return a.store.initEventSchema()
}

// m50Handler exposes the persistent event log without changing the existing
// package API routing contract.
func (a *App) m50Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/events" {
			a.handleEvents(w, r)
			return
		}
		if r.URL.Path == "/metrics" {
			rw := &m50MetricsWriter{ResponseWriter: w, app: a}
			next.ServeHTTP(rw, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

type m50MetricsWriter struct {
	http.ResponseWriter
	app *App
}

func (w *m50MetricsWriter) Write(p []byte) (int, error) {
	n, err := w.ResponseWriter.Write(p)
	if err == nil {
		w.app.writeM50Metrics(w.ResponseWriter)
	}
	return n, err
}

func (a *App) m50StartupEvent() {
	a.emitEvent(Event{Type: "service_started", Severity: "info", Message: fmt.Sprintf("ghcr-stats %s started", version)})
}
