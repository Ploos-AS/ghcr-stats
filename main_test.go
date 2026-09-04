package main

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCompact(t *testing.T) {
	if compact(1234) != "1.2k" { t.Fatal("compact") }
	if compact(42) != "42" { t.Fatal("compact") }
}

func TestRegex(t *testing.T) {
	m := downloadsPatterns[0].FindStringSubmatch("12,345 downloads")
	if len(m) != 2 || m[1] != "12,345" { t.Fatalf("%#v", m) }
}

func TestStore(t *testing.T) {
	s, err := OpenStore(filepath.Join(t.TempDir(), "test.db"))
	if err != nil { t.Fatal(err) }
	defer s.Close()
	now := time.Now().UTC()
	if err := s.Save(PackageStat{Package:"soju", Downloads:100, UpdatedAt:now.Add(-8*24*time.Hour)}); err != nil { t.Fatal(err) }
	if err := s.Save(PackageStat{Package:"soju", Downloads:125, UpdatedAt:now}); err != nil { t.Fatal(err) }
	st, err := s.Latest("soju")
	if err != nil || st.Downloads != 125 { t.Fatalf("%v %#v", err, st) }
}

func TestLoadConfigDefaults(t *testing.T) {
	for _, k := range []string{"GHCR_STATS_LISTEN","GHCR_STATS_OWNER","GHCR_STATS_DB","GHCR_STATS_PACKAGES","GHCR_STATS_INTERVAL"} { os.Unsetenv(k) }
	cfg := loadConfig()
	if cfg.Owner != "Ploos-AS" || len(cfg.Packages) == 0 { t.Fatalf("%#v", cfg) }
}

func TestBadgeSVG(t *testing.T) {
	s := badgeSVG("GHCR pulls", "1.2k")
	if !strings.Contains(s, "<svg") || !strings.Contains(s, "1.2k") { t.Fatal("bad svg") }
}

func TestGitHubHTMLCollectorLive(t *testing.T) {
	if os.Getenv("GHCR_LIVE_TEST") != "1" {
		t.Skip("set GHCR_LIVE_TEST=1 to exercise GitHub's public package pages")
	}
	collector := GitHubHTMLCollector{Client: &http.Client{Timeout: 25 * time.Second}}
	for _, pkg := range []string{"soju", "mineflayer"} {
		t.Run(pkg, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			count, err := collector.Collect(ctx, "Ploos-AS", pkg)
			if err != nil { t.Fatalf("collect %s: %v", pkg, err) }
			if count < 0 { t.Fatalf("negative download count for %s: %d", pkg, count) }
			t.Logf("%s downloads=%d", pkg, count)
		})
	}
}
