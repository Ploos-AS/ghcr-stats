package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

const webhookPayloadVersion = "1"

type WebhookConfig struct {
	URL    string
	Secret string
}

type WebhookDelivery struct {
	Version string `json:"version"`
	Event   Event  `json:"event"`
}

type WebhookSender struct {
	cfg    WebhookConfig
	client *http.Client
	mu     sync.RWMutex
	ok     uint64
	failed uint64
	lastOK time.Time
}

func readSecretValue(valueEnv, fileEnv string) string {
	if v := strings.TrimSpace(os.Getenv(valueEnv)); v != "" {
		return v
	}
	if path := strings.TrimSpace(os.Getenv(fileEnv)); path != "" {
		if b, err := os.ReadFile(path); err == nil {
			return strings.TrimSpace(string(b))
		}
	}
	return ""
}

func loadWebhookConfig() WebhookConfig {
	return WebhookConfig{
		URL:    readSecretValue("GHCR_STATS_WEBHOOK_URL", "GHCR_STATS_WEBHOOK_URL_FILE"),
		Secret: readSecretValue("GHCR_STATS_WEBHOOK_SECRET", "GHCR_STATS_WEBHOOK_SECRET_FILE"),
	}
}

func NewWebhookSender(cfg WebhookConfig) (*WebhookSender, error) {
	if cfg.URL == "" {
		return &WebhookSender{cfg: cfg, client: &http.Client{Timeout: 10 * time.Second}}, nil
	}
	u, err := url.Parse(cfg.URL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, errors.New("webhook URL must be an absolute http or https URL")
	}
	return &WebhookSender{cfg: cfg, client: &http.Client{Timeout: 10 * time.Second}}, nil
}

func (s *WebhookSender) Enabled() bool { return s != nil && s.cfg.URL != "" }

func (s *WebhookSender) Deliver(ctx context.Context, event Event) error {
	if !s.Enabled() {
		return nil
	}
	body, err := json.Marshal(WebhookDelivery{Version: webhookPayloadVersion, Event: event})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.cfg.URL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Ploos-AS-ghcr-stats/0.5")
	req.Header.Set("X-GHCR-Stats-Event", event.Type)
	if s.cfg.Secret != "" {
		mac := hmac.New(sha256.New, []byte(s.cfg.Secret))
		_, _ = mac.Write(body)
		req.Header.Set("X-GHCR-Stats-Signature-256", "sha256="+hex.EncodeToString(mac.Sum(nil)))
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
		return fmt.Errorf("webhook returned %s", resp.Status)
	}
	s.record(true)
	return nil
}

func (s *WebhookSender) record(success bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if success {
		s.ok++
		s.lastOK = time.Now().UTC()
	} else {
		s.failed++
	}
}

func (s *WebhookSender) Metrics() (ok, failed uint64, lastOK time.Time) {
	if s == nil {
		return 0, 0, time.Time{}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.ok, s.failed, s.lastOK
}
