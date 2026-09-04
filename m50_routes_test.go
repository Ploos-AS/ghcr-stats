package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

func TestM50ProductionRoutesExposeEvents(t *testing.T) {
	store, err := OpenStore(filepath.Join(t.TempDir(), "events-route.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	app := &App{
		cfg:           Config{Owner: "Ploos-AS", Interval: 6 * time.Hour},
		store:         store,
		packages:      []string{"soju"},
		packageSource: "explicit",
		lastErr:       map[string]string{},
	}
	if _, err := store.SaveEvent(Event{Type: "collector_failed", Severity: "error", Package: "soju", CreatedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/events?package=soju&type=collector_failed&severity=error&limit=10", nil)
	w := httptest.NewRecorder()
	app.routes().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var body struct {
		Owner  string  `json:"owner"`
		Events []Event `json:"events"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Owner != "Ploos-AS" || len(body.Events) != 1 || body.Events[0].Package != "soju" {
		t.Fatalf("unexpected body: %#v", body)
	}
}

func TestM50ProductionRoutesEventsMethodGuard(t *testing.T) {
	store, err := OpenStore(filepath.Join(t.TempDir(), "events-route-method.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	app := &App{cfg: Config{Owner: "Ploos-AS"}, store: store, lastErr: map[string]string{}}

	w := httptest.NewRecorder()
	app.routes().ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/events", nil))
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if got := w.Header().Get("Allow"); got != http.MethodGet {
		t.Fatalf("Allow=%q", got)
	}
}
