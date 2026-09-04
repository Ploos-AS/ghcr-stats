package main

import (
	"context"
	"net/http"
	"net/http/httptest"
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
		if got := r.URL.Query().Get("visibility"); got != "public" {
			t.Fatalf("visibility = %q", got)
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
