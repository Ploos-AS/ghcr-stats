package main

import (
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func newM52TestApp(t *testing.T, dbPath string) (*App, *Store) {
	t.Helper()
	store, err := OpenStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	app := &App{
		cfg:           Config{Owner: "Ploos-AS", Interval: time.Hour},
		store:         store,
		packages:      []string{"soju"},
		packageSource: "explicit",
		lastErr:       map[string]string{},
	}
	return app, store
}

func eventTypes(t *testing.T, s *Store) []string {
	t.Helper()
	items, err := s.Events("", "", "", 100)
	if err != nil {
		t.Fatal(err)
	}
	out := make([]string, 0, len(items))
	for _, e := range items {
		out = append(out, e.Type)
	}
	return out
}

func countType(items []string, typ string) int {
	n := 0
	for _, item := range items {
		if item == typ {
			n++
		}
	}
	return n
}

func TestM52CollectorTransitionDeduplicatesAcrossRestart(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "m52.db")
	app, store := newM52TestApp(t, dbPath)

	app.recordCollectionResult("soju", errors.New("boom"))
	app.recordCollectionResult("soju", errors.New("boom again"))
	if got := countType(eventTypes(t, store), "collector_failed"); got != 1 {
		t.Fatalf("collector_failed count=%d want=1", got)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	app, store = newM52TestApp(t, dbPath)
	defer store.Close()
	app.recordCollectionResult("soju", errors.New("still failing"))
	if got := countType(eventTypes(t, store), "collector_failed"); got != 1 {
		t.Fatalf("collector_failed after restart=%d want=1", got)
	}

	app.recordCollectionResult("soju", nil)
	if got := countType(eventTypes(t, store), "collector_recovered"); got != 1 {
		t.Fatalf("collector_recovered count=%d want=1", got)
	}
	app.recordCollectionResult("soju", nil)
	if got := countType(eventTypes(t, store), "collector_recovered"); got != 1 {
		t.Fatalf("collector_recovered duplicate count=%d want=1", got)
	}
}

func TestM52DiscoveryFailureAndRecovery(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "m52-discovery.db")
	app, store := newM52TestApp(t, dbPath)
	defer store.Close()

	app.mu.Lock()
	app.lastDiscoveryErr = "github unavailable"
	app.mu.Unlock()
	app.recordCollectionResult("soju", nil)
	app.recordCollectionResult("soju", nil)
	if got := countType(eventTypes(t, store), "discovery_failed"); got != 1 {
		t.Fatalf("discovery_failed count=%d want=1", got)
	}

	app.mu.Lock()
	app.lastDiscoveryErr = ""
	app.mu.Unlock()
	app.recordCollectionResult("soju", nil)
	if got := countType(eventTypes(t, store), "discovery_recovered"); got != 1 {
		t.Fatalf("discovery_recovered count=%d want=1", got)
	}
}

func TestM52StaleAndFreshTransitions(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "m52-stale.db")
	app, store := newM52TestApp(t, dbPath)
	defer store.Close()

	old := time.Now().UTC().Add(-10 * time.Hour)
	if err := store.Save(PackageStat{Package: "soju", Downloads: 1, UpdatedAt: old}); err != nil {
		t.Fatal(err)
	}
	app.cfg.Interval = time.Hour
	app.recordCollectionResult("soju", errors.New("collector down"))
	if got := countType(eventTypes(t, store), "package_stale"); got != 1 {
		t.Fatalf("package_stale count=%d want=1", got)
	}

	app.recordCollectionResult("soju", nil)
	if got := countType(eventTypes(t, store), "package_fresh"); got != 1 {
		t.Fatalf("package_fresh count=%d want=1", got)
	}
}

func TestM52AlertStatePersists(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "m52-state.db")
	_, store := newM52TestApp(t, dbPath)
	at := time.Now().UTC()
	tr, err := store.SetAlertState("readiness", "ready", "ready", at)
	if err != nil {
		t.Fatal(err)
	}
	if tr.Changed {
		t.Fatal("seeded readiness must not be a transition")
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	_, store = newM52TestApp(t, dbPath)
	defer store.Close()
	state, ok, err := store.AlertState("readiness")
	if err != nil {
		t.Fatal(err)
	}
	if !ok || state != "ready" {
		t.Fatalf("state=%q ok=%t", state, ok)
	}
}
