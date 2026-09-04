package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNotificationSenderDisabled(t *testing.T) {
	s, err := NewNotificationSender(NotificationConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if s.Enabled() {
		t.Fatal("expected disabled sender")
	}
	if err := s.Deliver(context.Background(), Event{Type: "collector_failed"}); err != nil {
		t.Fatalf("disabled sender returned error: %v", err)
	}
}

func TestNotificationSenderRejectsInvalidURL(t *testing.T) {
	if _, err := NewNotificationSender(NotificationConfig{AppriseURL: "ftp://example.invalid/notify/key"}); err == nil {
		t.Fatal("expected invalid URL error")
	}
}

func TestNotificationSenderApprisePayload(t *testing.T) {
	var got apprisePayload
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("content type = %q", r.Header.Get("Content-Type"))
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decode payload: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	s, err := NewNotificationSender(NotificationConfig{
		AppriseURL:  srv.URL + "/notify/ghcr-stats",
		AppriseTag:  "ops",
		BearerToken: "secret-token",
	})
	if err != nil {
		t.Fatal(err)
	}
	e := Event{Type: "collector_failed", Severity: "error", Owner: "Ploos-AS", Package: "soju", Message: "collector failed"}
	if err := s.Deliver(context.Background(), e); err != nil {
		t.Fatal(err)
	}
	if auth != "Bearer secret-token" {
		t.Fatalf("authorization = %q", auth)
	}
	if got.Type != "failure" {
		t.Fatalf("type = %q", got.Type)
	}
	if got.Tag != "ops" {
		t.Fatalf("tag = %q", got.Tag)
	}
	if !strings.Contains(got.Title, "collector_failed") || !strings.Contains(got.Title, "soju") {
		t.Fatalf("title = %q", got.Title)
	}
	if !strings.Contains(got.Body, "owner=Ploos-AS") || !strings.Contains(got.Body, "package=soju") {
		t.Fatalf("body = %q", got.Body)
	}
	ok, failed, lastOK := s.Metrics()
	if ok != 1 || failed != 0 || lastOK.IsZero() {
		t.Fatalf("metrics = ok:%d failed:%d last:%v", ok, failed, lastOK)
	}
}

func TestNotificationSenderSeverityMapping(t *testing.T) {
	cases := map[string]string{"info": "info", "warning": "warning", "error": "failure", "": "info"}
	for in, want := range cases {
		if got := appriseType(in); got != want {
			t.Fatalf("appriseType(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNotificationSenderFailureIsCounted(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusInternalServerError)
	}))
	defer srv.Close()
	s, err := NewNotificationSender(NotificationConfig{AppriseURL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := s.Deliver(ctx, Event{Type: "package_missing", Severity: "warning", Owner: "Ploos-AS"}); err == nil {
		t.Fatal("expected delivery failure")
	}
	ok, failed, _ := s.Metrics()
	if ok != 0 || failed != 1 {
		t.Fatalf("metrics = ok:%d failed:%d", ok, failed)
	}
}
