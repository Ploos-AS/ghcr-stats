package main

import (
	"path/filepath"
	"testing"
	"time"
)

func TestM53LifecycleTransitionsAndDebounce(t *testing.T) {
	store, err := OpenStore(filepath.Join(t.TempDir(), "m53.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	t0 := time.Date(2026, 9, 5, 0, 0, 0, 0, time.UTC)
	events, scanned, err := store.ObservePackageLifecycle([]string{"soju", "mineflayer"}, t0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !scanned || len(events) != 0 {
		t.Fatalf("baseline scanned=%v events=%v", scanned, events)
	}

	events, _, err = store.ObservePackageLifecycle([]string{"soju", "mineflayer", "newpkg"}, t0.Add(time.Hour), 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Type != "package_added" || events[0].Package != "newpkg" {
		t.Fatalf("added events=%+v", events)
	}

	events, _, err = store.ObservePackageLifecycle([]string{"soju", "newpkg"}, t0.Add(2*time.Hour), 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 0 {
		t.Fatalf("first missing cycle must be silent: %+v", events)
	}

	events, _, err = store.ObservePackageLifecycle([]string{"soju", "newpkg"}, t0.Add(3*time.Hour), 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Type != "package_missing" || events[0].Package != "mineflayer" {
		t.Fatalf("missing events=%+v", events)
	}

	events, _, err = store.ObservePackageLifecycle([]string{"soju", "newpkg"}, t0.Add(4*time.Hour), 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 0 {
		t.Fatalf("already missing must not duplicate: %+v", events)
	}

	events, _, err = store.ObservePackageLifecycle([]string{"soju", "mineflayer", "newpkg"}, t0.Add(5*time.Hour), 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Type != "package_returned" || events[0].Package != "mineflayer" {
		t.Fatalf("returned events=%+v", events)
	}
}

func TestM53LifecyclePersistsAcrossRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "m53-restart.db")
	t0 := time.Date(2026, 9, 5, 1, 0, 0, 0, time.UTC)

	store, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.ObservePackageLifecycle([]string{"soju", "mineflayer"}, t0, 0); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.ObservePackageLifecycle([]string{"soju"}, t0.Add(time.Hour), 0); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	store, err = OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	events, _, err := store.ObservePackageLifecycle([]string{"soju"}, t0.Add(2*time.Hour), 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Type != "package_missing" || events[0].Package != "mineflayer" {
		t.Fatalf("restart debounce events=%+v", events)
	}

	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	events, _, err = store.ObservePackageLifecycle([]string{"soju"}, t0.Add(3*time.Hour), 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 0 {
		t.Fatalf("restart must not duplicate missing event: %+v", events)
	}
}

func TestM53ScanGapPreventsPerPackageOvercount(t *testing.T) {
	store, err := OpenStore(filepath.Join(t.TempDir(), "m53-gap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	t0 := time.Date(2026, 9, 5, 2, 0, 0, 0, time.UTC)
	if _, scanned, err := store.ObservePackageLifecycle([]string{"soju", "mineflayer"}, t0, time.Minute); err != nil || !scanned {
		t.Fatalf("baseline scanned=%v err=%v", scanned, err)
	}
	if _, scanned, err := store.ObservePackageLifecycle([]string{"soju"}, t0.Add(2*time.Hour), time.Minute); err != nil || !scanned {
		t.Fatalf("first missing scan scanned=%v err=%v", scanned, err)
	}
	events, scanned, err := store.ObservePackageLifecycle([]string{"soju"}, t0.Add(2*time.Hour+10*time.Second), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if scanned || len(events) != 0 {
		t.Fatalf("same cycle should be suppressed scanned=%v events=%+v", scanned, events)
	}
	events, scanned, err = store.ObservePackageLifecycle([]string{"soju"}, t0.Add(3*time.Hour), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if !scanned || len(events) != 1 || events[0].Type != "package_missing" {
		t.Fatalf("next cycle scanned=%v events=%+v", scanned, events)
	}
}

func TestM53NormalizePackageSet(t *testing.T) {
	got := normalizePackageSet([]string{" soju ", "", "mineflayer", "soju"})
	if len(got) != 2 || got[0] != "mineflayer" || got[1] != "soju" {
		t.Fatalf("got=%v", got)
	}
}
