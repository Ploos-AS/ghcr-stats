package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestEmitEventDeliversToApprise(t *testing.T) {
	var calls int
	var got apprisePayload
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decode: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	t.Setenv("GHCR_STATS_WEBHOOK_URL", "")
	t.Setenv("GHCR_STATS_WEBHOOK_URL_FILE", "")
	t.Setenv("GHCR_STATS_APPRISE_URL", srv.URL+"/notify/ghcr-stats")
	t.Setenv("GHCR_STATS_APPRISE_URL_FILE", "")
	t.Setenv("GHCR_STATS_APPRISE_TAG", "ops")
	t.Setenv("GHCR_STATS_APPRISE_TAG_FILE", "")
	t.Setenv("GHCR_STATS_APPRISE_BEARER_TOKEN", "")
	t.Setenv("GHCR_STATS_APPRISE_BEARER_TOKEN_FILE", "")

	store, err := OpenStore(t.TempDir() + "/state.db")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	app := &App{cfg: Config{Owner: "Ploos-AS"}, store: store}
	app.emitEvent(Event{Type: "package_missing", Severity: "warning", Package: "soju", Message: "package missing from discovery", CreatedAt: time.Now().UTC()})

	if calls != 1 {
		t.Fatalf("Apprise calls = %d, want 1", calls)
	}
	if got.Type != "warning" || got.Tag != "ops" {
		t.Fatalf("payload type/tag = %q/%q", got.Type, got.Tag)
	}
	events, err := store.Events("soju", "package_missing", "warning", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("persisted events = %d, want 1", len(events))
	}
}
