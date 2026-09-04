package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRetentionKeepsBoundaryBaseline(t *testing.T) {
	s, err := OpenStore(filepath.Join(t.TempDir(), "retention.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	base := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	points := []struct {
		pkg string
		dl  int64
		at  time.Time
	}{
		{"soju", 10, base.Add(-72 * time.Hour)},
		{"soju", 20, base.Add(-48 * time.Hour)},
		{"soju", 30, base.Add(-24 * time.Hour)},
		{"soju", 40, base},
		{"mineflayer", 5, base.Add(-60 * time.Hour)},
		{"mineflayer", 15, base.Add(-12 * time.Hour)},
	}
	for _, p := range points {
		if err := s.Save(PackageStat{Package: p.pkg, Downloads: p.dl, UpdatedAt: p.at}); err != nil {
			t.Fatal(err)
		}
	}

	deleted, err := s.ApplyRetention(base.Add(-18 * time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if deleted != 2 {
		t.Fatalf("deleted=%d want 2", deleted)
	}

	var sojuCount, mineflayerCount int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM snapshots WHERE package='soju'").Scan(&sojuCount); err != nil {
		t.Fatal(err)
	}
	if err := s.db.QueryRow("SELECT COUNT(*) FROM snapshots WHERE package='mineflayer'").Scan(&mineflayerCount); err != nil {
		t.Fatal(err)
	}
	if sojuCount != 2 || mineflayerCount != 2 {
		t.Fatalf("counts soju=%d mineflayer=%d", sojuCount, mineflayerCount)
	}

	var baseline int64
	if err := s.db.QueryRow("SELECT downloads FROM snapshots WHERE package='soju' ORDER BY collected_at ASC LIMIT 1").Scan(&baseline); err != nil {
		t.Fatal(err)
	}
	if baseline != 30 {
		t.Fatalf("baseline=%d want 30", baseline)
	}
}

func TestBackupAndRestoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "live.db")
	backupPath := filepath.Join(dir, "backup.db")
	restoredPath := filepath.Join(dir, "restored.db")

	s, err := OpenStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Save(PackageStat{Package: "soju", Downloads: 123, UpdatedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	if err := s.IntegrityCheck(); err != nil {
		t.Fatal(err)
	}
	if err := s.Backup(backupPath); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	if err := restoreDatabase(backupPath, restoredPath); err != nil {
		t.Fatal(err)
	}
	r, err := OpenStore(restoredPath)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	st, err := r.Latest("soju")
	if err != nil {
		t.Fatal(err)
	}
	if st.Downloads != 123 {
		t.Fatalf("downloads=%d want 123", st.Downloads)
	}
	if err := r.IntegrityCheck(); err != nil {
		t.Fatal(err)
	}
}

func TestRetentionDurationDefaultsDisabled(t *testing.T) {
	old, had := os.LookupEnv("GHCR_STATS_RETENTION")
	defer func() {
		if had {
			_ = os.Setenv("GHCR_STATS_RETENTION", old)
		} else {
			_ = os.Unsetenv("GHCR_STATS_RETENTION")
		}
	}()

	_ = os.Unsetenv("GHCR_STATS_RETENTION")
	if got := retentionDuration(); got != 0 {
		t.Fatalf("default retention=%s want disabled", got)
	}
	_ = os.Setenv("GHCR_STATS_RETENTION", "720h")
	if got := retentionDuration(); got != 30*24*time.Hour {
		t.Fatalf("retention=%s want 720h", got)
	}
	_ = os.Setenv("GHCR_STATS_RETENTION", "12h")
	if got := retentionDuration(); got != 0 {
		t.Fatalf("short retention=%s want disabled", got)
	}
}
