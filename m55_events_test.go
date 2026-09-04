package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestM55EventCursorPagination(t *testing.T) {
	store, err := OpenStore(filepath.Join(t.TempDir(), "events.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	now := time.Now().UTC()
	for i, typ := range []string{"oldest", "middle", "newest"} {
		if _, err := store.SaveEvent(Event{Type: typ, Severity: "info", Package: "soju", CreatedAt: now.Add(time.Duration(i) * time.Second)}); err != nil {
			t.Fatal(err)
		}
	}
	app := &App{cfg: Config{Owner: "Ploos-AS"}, store: store}

	first := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/events?limit=2", nil)
	app.handleEvents(first, req)
	if first.Code != http.StatusOK {
		t.Fatalf("first page status=%d body=%s", first.Code, first.Body.String())
	}
	var p1 struct {
		Events     []Event `json:"events"`
		NextCursor string  `json:"next_cursor"`
	}
	if err := json.Unmarshal(first.Body.Bytes(), &p1); err != nil {
		t.Fatal(err)
	}
	if len(p1.Events) != 2 || p1.Events[0].Type != "newest" || p1.Events[1].Type != "middle" {
		t.Fatalf("unexpected first page: %+v", p1.Events)
	}
	if p1.NextCursor == "" {
		t.Fatal("expected next_cursor")
	}
	cursor, err := strconv.ParseInt(p1.NextCursor, 10, 64)
	if err != nil || cursor != p1.Events[1].ID {
		t.Fatalf("unexpected cursor %q", p1.NextCursor)
	}

	second := httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/v1/events?limit=2&cursor="+p1.NextCursor, nil)
	app.handleEvents(second, req)
	if second.Code != http.StatusOK {
		t.Fatalf("second page status=%d body=%s", second.Code, second.Body.String())
	}
	var p2 struct {
		Events     []Event `json:"events"`
		NextCursor string  `json:"next_cursor"`
	}
	if err := json.Unmarshal(second.Body.Bytes(), &p2); err != nil {
		t.Fatal(err)
	}
	if len(p2.Events) != 1 || p2.Events[0].Type != "oldest" {
		t.Fatalf("unexpected second page: %+v", p2.Events)
	}
	if p2.NextCursor != "" {
		t.Fatalf("unexpected terminal cursor %q", p2.NextCursor)
	}
}

func TestM55EventFiltersAndInvalidCursor(t *testing.T) {
	store, err := OpenStore(filepath.Join(t.TempDir(), "events.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	for _, e := range []Event{
		{Type: "collector_failed", Severity: "error", Package: "soju"},
		{Type: "collector_recovered", Severity: "info", Package: "soju"},
		{Type: "collector_failed", Severity: "error", Package: "mineflayer"},
	} {
		if _, err := store.SaveEvent(e); err != nil {
			t.Fatal(err)
		}
	}
	app := &App{cfg: Config{Owner: "Ploos-AS"}, store: store}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/events?package=soju&type=collector_failed&severity=error&limit=10", nil)
	app.handleEvents(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("filter status=%d body=%s", rr.Code, rr.Body.String())
	}
	var got struct{ Events []Event `json:"events"` }
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Events) != 1 || got.Events[0].Package != "soju" || got.Events[0].Type != "collector_failed" {
		t.Fatalf("unexpected filtered events: %+v", got.Events)
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/v1/events?cursor=wat", nil)
	app.handleEvents(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("invalid cursor status=%d", rr.Code)
	}
}

func TestM55DashboardTemplatesExposeEvents(t *testing.T) {
	if !containsAll(dashboardTemplateM32.Tree.Root.String(), "Recent events", "/api/v1/events?limit=10") {
		t.Fatal("organization dashboard does not expose recent events")
	}
	if !containsAll(packageTemplateM32.Tree.Root.String(), "Recent events", "/api/v1/events?package={{.Summary.Package}}&limit=10") {
		t.Fatal("package dashboard does not expose package events")
	}
}

func containsAll(s string, wants ...string) bool {
	for _, want := range wants {
		if !strings.Contains(s, want) {
			return false
		}
	}
	return true
}
