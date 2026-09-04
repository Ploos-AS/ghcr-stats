package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"testing"
	"time"
)

func TestGitHubPackagesDiscoverer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Fatalf("authorization = %q", got)
		}
		if got := r.URL.Query().Get("package_type"); got != "container" {
			t.Fatalf("package_type = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{"name":"soju","package_type":"container","visibility":"public"},
			{"name":"ghcr-stats","package_type":"container","visibility":"public"},
			{"name":"private-one","package_type":"container","visibility":"private"},
			{"name":"not-container","package_type":"npm","visibility":"public"}
		]`))
	}))
	defer server.Close()

	d := GitHubPackagesDiscoverer{
		Client:  &http.Client{Timeout: 5 * time.Second},
		Token:   "test-token",
		BaseURL: server.URL,
	}
	got, err := d.Discover(context.Background(), "Ploos-AS")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"ghcr-stats", "soju"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("packages = %#v, want %#v", got, want)
	}
}

func TestGitHubPackagesDiscovererRequiresToken(t *testing.T) {
	d := GitHubPackagesDiscoverer{Client: http.DefaultClient}
	if _, err := d.Discover(context.Background(), "Ploos-AS"); err == nil {
		t.Fatal("expected missing-token error")
	}
}

func TestGitHubPackagesDiscovererLive(t *testing.T) {
	if os.Getenv("GHCR_LIVE_DISCOVERY_TEST") != "1" {
		t.Skip("set GHCR_LIVE_DISCOVERY_TEST=1 to exercise GitHub Packages API")
	}
	token := os.Getenv("GHCR_STATS_GITHUB_TOKEN")
	if token == "" {
		t.Fatal("GHCR_STATS_GITHUB_TOKEN is required for live discovery test")
	}
	d := GitHubPackagesDiscoverer{
		Client: &http.Client{Timeout: 25 * time.Second},
		Token:  token,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pkgs, err := d.Discover(ctx, "Ploos-AS")
	if err != nil {
		t.Fatal(err)
	}
	if len(pkgs) == 0 {
		t.Fatal("no packages discovered")
	}
	t.Logf("discovered %d packages: %v", len(pkgs), pkgs)
}
