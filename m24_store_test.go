package main

import (
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestCollectionFailureStateSurvivesRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "restart.db")
	now := time.Now().UTC()

	s1, err := OpenStore(path)
	if err != nil { t.Fatal(err) }
	if err := s1.RecordCollectionResult("soju", errors.New("parser changed"), now); err != nil { t.Fatal(err) }
	if err := s1.RecordCollectionResult("soju", errors.New("parser changed again"), now.Add(time.Minute)); err != nil { t.Fatal(err) }
	if err := s1.Close(); err != nil { t.Fatal(err) }

	s2, err := OpenStore(path)
	if err != nil { t.Fatal(err) }
	defer s2.Close()
	st, err := s2.CollectionFailureStats("soju")
	if err != nil { t.Fatal(err) }
	if st.Total != 2 || st.Consecutive != 2 || st.LastError != "parser changed again" {
		t.Fatalf("restored state = %#v", st)
	}

	if err := s2.RecordCollectionResult("soju", nil, now.Add(2*time.Minute)); err != nil { t.Fatal(err) }
	st, err = s2.CollectionFailureStats("soju")
	if err != nil { t.Fatal(err) }
	if st.Total != 2 || st.Consecutive != 0 || st.LastError != "" {
		t.Fatalf("post-success state = %#v", st)
	}
}

func TestCollectorHealthRestoresPersistedLastError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "health-restart.db")
	now := time.Now().UTC()

	s1, err := OpenStore(path)
	if err != nil { t.Fatal(err) }
	if err := s1.Save(PackageStat{Package: "soju", Downloads: 1, UpdatedAt: now}); err != nil { t.Fatal(err) }
	if err := s1.RecordCollectionResult("soju", errors.New("github markup changed"), now); err != nil { t.Fatal(err) }
	if err := s1.Close(); err != nil { t.Fatal(err) }

	s2, err := OpenStore(path)
	if err != nil { t.Fatal(err) }
	defer s2.Close()
	a := &App{cfg: Config{Owner: "Ploos-AS", Interval: time.Hour}, store: s2, collector: healthTestCollector{}, packages: []string{"soju"}, packageSource: "explicit", lastErr: map[string]string{}}
	h := a.collectorHealth("soju", now.Add(time.Minute))
	if h.Up || h.LastError != "github markup changed" {
		t.Fatalf("restored health = %#v", h)
	}
}
