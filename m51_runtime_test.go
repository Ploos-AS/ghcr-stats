package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
)

func TestM51RuntimeCollectorTransitionsPersistAndDeliver(t *testing.T) {
	var mu sync.Mutex
	var events []string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		mu.Lock()
		events = append(events, r.Header.Get("X-GHCR-Stats-Event"))
		mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()

	t.Setenv("GHCR_STATS_WEBHOOK_URL", ts.URL)
	t.Setenv("GHCR_STATS_WEBHOOK_URL_FILE", "")
	t.Setenv("GHCR_STATS_WEBHOOK_SECRET", "runtime-secret")
	t.Setenv("GHCR_STATS_WEBHOOK_SECRET_FILE", "")

	store, err := OpenStore(filepath.Join(t.TempDir(), "runtime.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	app := &App{cfg: Config{Owner: "Ploos-AS"}, store: store, lastErr: map[string]string{}}

	app.recordCollectionResult("soju", errRuntimeTest)
	app.recordCollectionResult("soju", errRuntimeTest)
	app.recordCollectionResult("soju", nil)

	items, err := store.Events("soju", "", "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("events=%d want=2: %#v", len(items), items)
	}
	if items[0].Type != "collector_recovered" || items[1].Type != "collector_failed" {
		t.Fatalf("unexpected persisted events: %#v", items)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(events) != 2 || events[0] != "collector_failed" || events[1] != "collector_recovered" {
		t.Fatalf("webhook events=%v", events)
	}
}

var errRuntimeTest = runtimeTestError("collector unavailable")

type runtimeTestError string

func (e runtimeTestError) Error() string { return string(e) }
