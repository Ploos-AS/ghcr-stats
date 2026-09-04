package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type NotificationConfig struct {
	AppriseURL   string
	AppriseTag   string
	BearerToken  string
}

type apprisePayload struct {
	Body  string `json:"body"`
	Title string `json:"title,omitempty"`
	Type  string `json:"type,omitempty"`
	Tag   string `json:"tag,omitempty"`
}

type NotificationSender struct {
	cfg    NotificationConfig
	client *http.Client
	mu     sync.RWMutex
	ok     uint64
	failed uint64
	lastOK time.Time
}

var runtimeNotifications struct {
	sync.Mutex
	sender *NotificationSender
	key    string
	err    error
}

func loadNotificationConfig() NotificationConfig {
	return NotificationConfig{
		AppriseURL:  readSecretValue("GHCR_STATS_APPRISE_URL", "GHCR_STATS_APPRISE_URL_FILE"),
		AppriseTag:  strings.TrimSpace(readSecretValue("GHCR_STATS_APPRISE_TAG", "GHCR_STATS_APPRISE_TAG_FILE")),
		BearerToken: readSecretValue("GHCR_STATS_APPRISE_BEARER_TOKEN", "GHCR_STATS_APPRISE_BEARER_TOKEN_FILE"),
	}
}

func NewNotificationSender(cfg NotificationConfig) (*NotificationSender, error) {
	if cfg.AppriseURL == "" {
		return &NotificationSender{cfg: cfg, client: &http.Client{Timeout: 10 * time.Second}}, nil
	}
	u, err := url.Parse(cfg.AppriseURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, errors.New("Apprise URL must be an absolute http or https URL")
	}
	return &NotificationSender{cfg: cfg, client: &http.Client{Timeout: 10 * time.Second}}, nil
}

func runtimeNotificationSender() (*NotificationSender, error) {
	cfg := loadNotificationConfig()
	key := cfg.AppriseURL + "\x00" + cfg.AppriseTag + "\x00" + cfg.BearerToken
	runtimeNotifications.Lock()
	defer runtimeNotifications.Unlock()
	if runtimeNotifications.sender != nil && runtimeNotifications.key == key {
		return runtimeNotifications.sender, runtimeNotifications.err
	}
	runtimeNotifications.sender, runtimeNotifications.err = NewNotificationSender(cfg)
	runtimeNotifications.key = key
	return runtimeNotifications.sender, runtimeNotifications.err
}

func (s *NotificationSender) Enabled() bool {
	return s != nil && s.cfg.AppriseURL != ""
}

func appriseType(severity string) string {
	switch severity {
	case "error":
		return "failure"
	case "warning":
		return "warning"
	default:
		return "info"
	}
}

func notificationTitle(e Event) string {
	if e.Package != "" {
		return fmt.Sprintf("ghcr-stats: %s (%s)", e.Type, e.Package)
	}
	return "ghcr-stats: " + e.Type
}

func notificationBody(e Event) string {
	parts := []string{"owner=" + e.Owner, "event=" + e.Type}
	if e.Package != "" {
		parts = append(parts, "package="+e.Package)
	}
	if e.Message != "" {
		parts = append(parts, e.Message)
	}
	return strings.Join(parts, " | ")
}

func (s *NotificationSender) Deliver(ctx context.Context, e Event) error {
	if !s.Enabled() {
		return nil
	}
	body, err := json.Marshal(apprisePayload{
		Body:  notificationBody(e),
		Title: notificationTitle(e),
		Type:  appriseType(e.Severity),
		Tag:   s.cfg.AppriseTag,
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.cfg.AppriseURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Ploos-AS-ghcr-stats/0.5")
	if s.cfg.BearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+s.cfg.BearerToken)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		s.record(false)
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		s.record(false)
		return fmt.Errorf("Apprise returned %s", resp.Status)
	}
	s.record(true)
	return nil
}

func (s *NotificationSender) record(success bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if success {
		s.ok++
		s.lastOK = time.Now().UTC()
	} else {
		s.failed++
	}
}

func (s *NotificationSender) Metrics() (ok, failed uint64, lastOK time.Time) {
	if s == nil {
		return 0, 0, time.Time{}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.ok, s.failed, s.lastOK
}

func (a *App) writeM54Metrics(w http.ResponseWriter) {
	sender, err := runtimeNotificationSender()
	if err != nil {
		fmt.Fprintf(w, "ghcr_stats_notifications_config_valid{owner=%q,provider=%q} 0\n", a.cfg.Owner, "apprise")
		return
	}
	fmt.Fprintf(w, "ghcr_stats_notifications_config_valid{owner=%q,provider=%q} 1\n", a.cfg.Owner, "apprise")
	if !sender.Enabled() {
		fmt.Fprintf(w, "ghcr_stats_notifications_enabled{owner=%q,provider=%q} 0\n", a.cfg.Owner, "apprise")
		return
	}
	fmt.Fprintf(w, "ghcr_stats_notifications_enabled{owner=%q,provider=%q} 1\n", a.cfg.Owner, "apprise")
	ok, failed, lastOK := sender.Metrics()
	fmt.Fprintf(w, "ghcr_stats_notification_deliveries_total{owner=%q,provider=%q,result=%q} %d\n", a.cfg.Owner, "apprise", "success", ok)
	fmt.Fprintf(w, "ghcr_stats_notification_deliveries_total{owner=%q,provider=%q,result=%q} %d\n", a.cfg.Owner, "apprise", "failure", failed)
	if !lastOK.IsZero() {
		fmt.Fprintf(w, "ghcr_stats_notification_last_success_timestamp_seconds{owner=%q,provider=%q} %d\n", a.cfg.Owner, "apprise", lastOK.Unix())
	}
}
