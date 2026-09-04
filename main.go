package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

type Config struct {
	Listen           string
	Owner            string
	Packages         []string
	PackagesExplicit bool
	DBPath           string
	Interval         time.Duration
	GitHubToken      string
}

type PackageStat struct {
	Package   string    `json:"package"`
	Downloads int64     `json:"downloads"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Collector interface {
	Name() string
	Collect(context.Context, string, string) (int64, error)
}

type GitHubHTMLCollector struct{ Client *http.Client }

func (GitHubHTMLCollector) Name() string { return "github-html" }

var downloadsPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)([0-9][0-9,._ ]*)\s+downloads?`),
	regexp.MustCompile(`(?i)downloads?[^0-9]{0,40}([0-9][0-9,._ ]*)`),
}

func (c GitHubHTMLCollector) Collect(ctx context.Context, owner, pkg string) (int64, error) {
	u := fmt.Sprintf("https://github.com/orgs/%s/packages/container/package/%s", owner, pkg)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("User-Agent", "Ploos-AS-ghcr-stats/0.2")
	resp, err := c.Client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("github package page returned %s", resp.Status)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return 0, err
	}
	text := html.UnescapeString(string(b))
	for _, re := range downloadsPatterns {
		m := re.FindStringSubmatch(text)
		if len(m) == 2 {
			n := strings.NewReplacer(",", "", ".", "", "_", "", " ", "").Replace(m[1])
			if v, err := strconv.ParseInt(n, 10, 64); err == nil {
				return v, nil
			}
		}
	}
	return 0, errors.New("download count not found on GitHub package page")
}

type Store struct{ db *sql.DB }

func OpenStore(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	if _, err = db.Exec(`CREATE TABLE IF NOT EXISTS snapshots (package TEXT NOT NULL, downloads INTEGER NOT NULL, collected_at TEXT NOT NULL); CREATE INDEX IF NOT EXISTS idx_snapshots_pkg_time ON snapshots(package, collected_at);`); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) Save(st PackageStat) error {
	_, err := s.db.Exec("INSERT INTO snapshots(package,downloads,collected_at) VALUES(?,?,?)", st.Package, st.Downloads, st.UpdatedAt.UTC().Format(time.RFC3339Nano))
	return err
}

func (s *Store) Latest(pkg string) (PackageStat, error) {
	var st PackageStat
	var ts string
	err := s.db.QueryRow("SELECT package,downloads,collected_at FROM snapshots WHERE package=? ORDER BY collected_at DESC LIMIT 1", pkg).Scan(&st.Package, &st.Downloads, &ts)
	if err != nil {
		return st, err
	}
	st.UpdatedAt, err = time.Parse(time.RFC3339Nano, ts)
	return st, err
}

func (s *Store) DeltaSince(pkg string, since time.Time) (int64, error) {
	latest, err := s.Latest(pkg)
	if err != nil {
		return 0, err
	}
	var baseline int64
	err = s.db.QueryRow("SELECT downloads FROM snapshots WHERE package=? AND collected_at<=? ORDER BY collected_at DESC LIMIT 1", pkg, since.UTC().Format(time.RFC3339Nano)).Scan(&baseline)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	d := latest.Downloads - baseline
	if d < 0 {
		return 0, nil
	}
	return d, nil
}

type App struct {
	cfg              Config
	store            *Store
	collector        Collector
	discoverer       PackageDiscoverer
	mu               sync.RWMutex
	packages         []string
	packageSource    string
	lastDiscoveryErr string
	lastErr          map[string]string
}

func (a *App) packageNames() []string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	out := append([]string(nil), a.packages...)
	sort.Strings(out)
	return out
}

func (a *App) packageState() ([]string, string, string) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	out := append([]string(nil), a.packages...)
	sort.Strings(out)
	return out, a.packageSource, a.lastDiscoveryErr
}

func (a *App) refreshPackages(ctx context.Context) {
	if a.cfg.PackagesExplicit || a.discoverer == nil {
		return
	}
	cctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	pkgs, err := a.discoverer.Discover(cctx, a.cfg.Owner)
	cancel()
	if err != nil {
		a.mu.Lock()
		a.lastDiscoveryErr = err.Error()
		a.mu.Unlock()
		log.Printf("discover packages: %v; retaining %d configured packages", err, len(a.packageNames()))
		return
	}
	a.mu.Lock()
	a.packages = append([]string(nil), pkgs...)
	a.packageSource = a.discoverer.Name()
	a.lastDiscoveryErr = ""
	a.mu.Unlock()
	log.Printf("discovered %d public container packages for %s", len(pkgs), a.cfg.Owner)
}

func (a *App) collectAll(ctx context.Context) {
	a.refreshPackages(ctx)
	for _, pkg := range a.packageNames() {
		cctx, cancel := context.WithTimeout(ctx, 20*time.Second)
		count, err := a.collector.Collect(cctx, a.cfg.Owner, pkg)
		cancel()
		a.recordCollectionResult(pkg, err)
		a.mu.Lock()
		if err != nil {
			a.lastErr[pkg] = err.Error()
			a.mu.Unlock()
			log.Printf("collect %s: %v", pkg, err)
			continue
		}
		delete(a.lastErr, pkg)
		a.mu.Unlock()
		if err := a.store.Save(PackageStat{Package: pkg, Downloads: count, UpdatedAt: time.Now().UTC()}); err != nil {
			log.Printf("save %s: %v", pkg, err)
		}
	}
}

func (a *App) loop(ctx context.Context) {
	a.collectAll(ctx)
	t := time.NewTicker(a.cfg.Interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			a.collectAll(ctx)
		}
	}
}

func (a *App) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok\n")) })
	mux.HandleFunc("/api/v1/health", a.handleHealthJSON)
	mux.HandleFunc("/api/v1/packages", a.handlePackageList)
	mux.HandleFunc("/api/v1/packages/", a.handleJSON)
	mux.HandleFunc("/api/v1/org", a.handleOrgJSON)
	mux.HandleFunc("/api/v1/badge/", a.handleShields)
	mux.HandleFunc("/badge/", a.handleM1Badge)
	mux.HandleFunc("/metrics", a.handleMetrics)
	mux.HandleFunc("/", a.handleIndex)
	return mux
}

func (a *App) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	io.WriteString(w, "<!doctype html><meta charset=utf-8><title>ghcr-stats</title><h1>Ploos-AS GHCR stats</h1><ul>")
	for _, pkg := range a.packageNames() {
		fmt.Fprintf(w, "<li><a href=\"/api/v1/packages/%s\">%s</a> — <img alt=\"pulls\" src=\"/badge/%s/pulls.svg\"></li>", pkg, pkg, pkg)
	}
	io.WriteString(w, "</ul>")
}

func (a *App) handlePackageList(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/api/v1/packages" {
		http.NotFound(w, r)
		return
	}
	pkgs, source, discoveryErr := a.packageState()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"owner":           a.cfg.Owner,
		"source":          source,
		"count":           len(pkgs),
		"packages":        pkgs,
		"discovery_error": discoveryErr,
	})
}

func (a *App) handleJSON(w http.ResponseWriter, r *http.Request) {
	pkg := strings.TrimPrefix(r.URL.Path, "/api/v1/packages/")
	if pkg == "" || strings.Contains(pkg, "/") {
		http.NotFound(w, r)
		return
	}
	st, err := a.store.Latest(pkg)
	if err != nil {
		http.Error(w, "no data", http.StatusNotFound)
		return
	}
	d7, _ := a.store.DeltaSince(pkg, time.Now().Add(-7*24*time.Hour))
	d30, _ := a.store.DeltaSince(pkg, time.Now().Add(-30*24*time.Hour))
	health := a.collectorHealth(pkg, time.Now().UTC())
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"package": st.Package, "downloads": st.Downloads, "downloads_week": d7, "downloads_month": d30, "updated_at": st.UpdatedAt, "collector": a.collector.Name(), "collector_up": health.Up, "stale": health.Stale, "last_success": health.LastSuccess, "last_error": health.LastError})
}

func compact(n int64) string {
	switch {
	case n >= 1_000_000_000:
		return fmt.Sprintf("%.1fB", float64(n)/1_000_000_000)
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 1_000:
		return fmt.Sprintf("%.1fk", float64(n)/1_000)
	default:
		return strconv.FormatInt(n, 10)
	}
}

func badgeSVG(label, value string) string {
	lw := 8*len(label) + 18
	vw := 8*len(value) + 18
	body := fmt.Sprintf(`<rect width="%d" height="20" fill="#555"/><rect x="%d" width="%d" height="20" fill="#2ea44f"/><g fill="#fff" text-anchor="middle" font-family="Verdana,DejaVu Sans,sans-serif" font-size="11"><text x="%d" y="14">%s</text><text x="%d" y="14">%s</text></g>`, lw+vw, lw, vw, lw/2, html.EscapeString(label), lw+vw/2, html.EscapeString(value))
	return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="20" role="img">%s</svg>`, lw+vw, body)
}

func (a *App) handleMetrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	for _, pkg := range a.packageNames() {
		if st, err := a.store.Latest(pkg); err == nil {
			fmt.Fprintf(w, "ghcr_downloads_total{owner=%q,package=%q} %d\n", a.cfg.Owner, pkg, st.Downloads)
			fmt.Fprintf(w, "ghcr_snapshot_timestamp_seconds{owner=%q,package=%q} %d\n", a.cfg.Owner, pkg, st.UpdatedAt.Unix())
		}
	}
	pkgs, _, discoveryErr := a.packageState()
	fmt.Fprintf(w, "ghcr_stats_packages{owner=%q} %d\n", a.cfg.Owner, len(pkgs))
	if a.discoverer != nil && discoveryErr != "" {
		fmt.Fprintf(w, "ghcr_stats_discovery_up{owner=%q} 0\n", a.cfg.Owner)
	} else {
		fmt.Fprintf(w, "ghcr_stats_discovery_up{owner=%q} 1\n", a.cfg.Owner)
	}
	a.writeCollectorHealthMetrics(w)
	a.writeM23Metrics(w)
	a.writeM1Metrics(w)
}

func readToken() string {
	if v := strings.TrimSpace(os.Getenv("GHCR_STATS_GITHUB_TOKEN")); v != "" {
		return v
	}
	if path := strings.TrimSpace(os.Getenv("GHCR_STATS_GITHUB_TOKEN_FILE")); path != "" {
		b, err := os.ReadFile(path)
		if err != nil {
			log.Printf("read GitHub token file: %v", err)
			return ""
		}
		return strings.TrimSpace(string(b))
	}
	return ""
}

func loadConfig() Config {
	var pkgs []string
	for _, p := range strings.Split(os.Getenv("GHCR_STATS_PACKAGES"), ",") {
		if p = strings.TrimSpace(p); p != "" {
			pkgs = append(pkgs, p)
		}
	}
	explicit := len(pkgs) > 0
	if !explicit {
		pkgs = []string{"nmap-zenmap", "red-discordbot", "limnoria", "sopel", "cloudbot", "yagpdb", "mineflayer", "minecraft-console-client", "minecraft-console-client-web", "soju", "soju-web", "psotnic", "ghcr-stats"}
	}
	interval := 6 * time.Hour
	if v := os.Getenv("GHCR_STATS_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d >= time.Hour {
			interval = d
		}
	}
	cfg := Config{
		Listen:           os.Getenv("GHCR_STATS_LISTEN"),
		Owner:            os.Getenv("GHCR_STATS_OWNER"),
		DBPath:           os.Getenv("GHCR_STATS_DB"),
		Packages:         pkgs,
		PackagesExplicit: explicit,
		Interval:         interval,
		GitHubToken:      readToken(),
	}
	if cfg.Listen == "" {
		cfg.Listen = ":8080"
	}
	if cfg.Owner == "" {
		cfg.Owner = "Ploos-AS"
	}
	if cfg.DBPath == "" {
		cfg.DBPath = "/data/ghcr-stats.db"
	}
	return cfg
}

func main() {
	cfg := loadConfig()
	store, err := OpenStore(cfg.DBPath)
	if err != nil {
		log.Fatal(err)
	}
	defer store.Close()

	client := &http.Client{Timeout: 25 * time.Second}
	app := &App{
		cfg:           cfg,
		store:         store,
		collector:     GitHubHTMLCollector{Client: client},
		packages:      append([]string(nil), cfg.Packages...),
		packageSource: "fallback",
		lastErr:       map[string]string{},
	}
	if cfg.PackagesExplicit {
		app.packageSource = "explicit"
	} else if cfg.GitHubToken != "" {
		app.discoverer = GitHubPackagesDiscoverer{Client: client, Token: cfg.GitHubToken}
	}

	go app.loop(context.Background())
	srv := &http.Server{Addr: cfg.Listen, Handler: app.routes(), ReadHeaderTimeout: 5 * time.Second}
	log.Printf("ghcr-stats listening on %s for %s (%d initial packages, source=%s)", cfg.Listen, cfg.Owner, len(cfg.Packages), app.packageSource)
	log.Fatal(srv.ListenAndServe())
}