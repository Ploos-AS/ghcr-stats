package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

func newM50TestApp(t *testing.T) *App {
	t.Helper()
	s, err := OpenStore(filepath.Join(t.TempDir(), "events.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	a := &App{cfg: Config{Owner: "Ploos-AS"}, store: s}
	if err := a.initM50(); err != nil {
		t.Fatal(err)
	}
	return a
}

func TestM50EventRoundTripAndFilters(t *testing.T) {
	a := newM50TestApp(t)
	when := time.Date(2026, 9, 4, 20, 0, 0, 0, time.UTC)
	id, err := a.store.SaveEvent(Event{Type: "collector_failed", Severity: "error", Package: "soju", Message: "boom", Metadata: map[string]any{"attempt": float64(2)}, CreatedAt: when})
	if err != nil || id == 0 {
		t.Fatalf("save id=%d err=%v", id, err)
	}
	items, err := a.store.Events("soju", "collector_failed", "error", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Message != "boom" || !items[0].CreatedAt.Equal(when) {
		t.Fatalf("unexpected events: %#v", items)
	}
}

func TestM50EventsAPI(t *testing.T) {
	a := newM50TestApp(t)
	_, _ = a.store.SaveEvent(Event{Type: "package_added", Severity: "info", Package: "soju", CreatedAt: time.Now().UTC()})
	r := httptest.NewRequest(http.MethodGet, "/api/v1/events?package=soju&limit=10", nil)
	w := httptest.NewRecorder()
	a.handleEvents(w, r)
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
	if body.Owner != "Ploos-AS" || len(body.Events) != 1 || body.Events[0].Owner != "Ploos-AS" {
		t.Fatalf("unexpected body: %#v", body)
	}
}

func TestM50EventsAPIValidation(t *testing.T) {
	a := newM50TestApp(t)
	for _, u := range []string{"/api/v1/events?limit=0", "/api/v1/events?severity=critical"} {
		w := httptest.NewRecorder()
		a.handleEvents(w, httptest.NewRequest(http.MethodGet, u, nil))
		if w.Code != http.StatusBadRequest {
			t.Fatalf("%s status=%d", u, w.Code)
		}
	}
	w := httptest.NewRecorder()
	a.handleEvents(w, httptest.NewRequest(http.MethodPost, "/api/v1/events", nil))
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("post status=%d", w.Code)
	}
}
