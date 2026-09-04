package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
)

type PackageDiscoverer interface {
	Name() string
	Discover(context.Context, string) ([]string, error)
}

type GitHubPackagesDiscoverer struct {
	Client  *http.Client
	Token   string
	BaseURL string
}

func (GitHubPackagesDiscoverer) Name() string { return "github-api" }

type githubPackage struct {
	Name        string `json:"name"`
	PackageType string `json:"package_type"`
	Visibility  string `json:"visibility"`
}

type githubAPIError struct {
	Message string `json:"message"`
}

func (d GitHubPackagesDiscoverer) Discover(ctx context.Context, owner string) ([]string, error) {
	if strings.TrimSpace(d.Token) == "" {
		return nil, errors.New("GitHub token is required for package discovery")
	}
	if d.Client == nil {
		return nil, errors.New("HTTP client is required for package discovery")
	}

	base := strings.TrimRight(d.BaseURL, "/")
	if base == "" {
		base = "https://api.github.com"
	}

	seen := make(map[string]struct{})
	var packages []string
	for page := 1; page <= 100; page++ {
		u, err := url.Parse(fmt.Sprintf("%s/orgs/%s/packages", base, url.PathEscape(owner)))
		if err != nil {
			return nil, err
		}
		q := u.Query()
		q.Set("package_type", "container")
		q.Set("visibility", "public")
		q.Set("per_page", "100")
		q.Set("page", fmt.Sprintf("%d", page))
		u.RawQuery = q.Encode()

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(d.Token))
		req.Header.Set("X-GitHub-Api-Version", "2026-03-10")
		req.Header.Set("User-Agent", "Ploos-AS-ghcr-stats/0.2")

		resp, err := d.Client.Do(req)
		if err != nil {
			return nil, err
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
		resp.Body.Close()
		if readErr != nil {
			return nil, readErr
		}
		if resp.StatusCode != http.StatusOK {
			var apiErr githubAPIError
			_ = json.Unmarshal(body, &apiErr)
			if strings.TrimSpace(apiErr.Message) != "" {
				return nil, fmt.Errorf("GitHub packages API returned %s: %s", resp.Status, apiErr.Message)
			}
			return nil, fmt.Errorf("GitHub packages API returned %s", resp.Status)
		}

		var batch []githubPackage
		if err := json.Unmarshal(body, &batch); err != nil {
			return nil, err
		}
		for _, pkg := range batch {
			name := strings.TrimSpace(pkg.Name)
			if name == "" || pkg.PackageType != "container" {
				continue
			}
			if pkg.Visibility != "" && pkg.Visibility != "public" {
				continue
			}
			if _, ok := seen[name]; ok {
				continue
			}
			seen[name] = struct{}{}
			packages = append(packages, name)
		}
		if len(batch) < 100 {
			break
		}
	}
	if len(packages) == 0 {
		return nil, errors.New("GitHub packages API returned no public container packages")
	}
	sort.Strings(packages)
	return packages, nil
}
