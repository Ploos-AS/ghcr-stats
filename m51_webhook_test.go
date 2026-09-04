package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestM51WebhookDisabled(t *testing.T) {
	s, err := NewWebhookSender(WebhookConfig{})
	if err != nil || s.Enabled() {
		t.Fatalf("sender=%#v err=%v", s, err)
	}
	if err := s.Deliver(context.Background(), Event{Type: "test"}); err != nil {
		t.Fatal(err)
	}
}

func TestM51WebhookRejectsInvalidURL(t *testing.T) {
	if _, err := NewWebhookSender(WebhookConfig{URL: "file:///tmp/hook"}); err == nil {
		t.Fatal("expected invalid URL error")
	}
}

func TestM51WebhookDeliveryAndHMAC(t *testing.T) {
	const secret = "test-secret"
	var gotBody []byte
	var gotSig, gotEvent string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		gotSig = r.Header.Get("X-GHCR-Stats-Signature-256")
		gotEvent = r.Header.Get("X-GHCR-Stats-Event")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()
	s, err := NewWebhookSender(WebhookConfig{URL: ts.URL, Secret: secret})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Deliver(context.Background(), Event{Type: "collector_failed", Owner: "Ploos-AS", Package: "soju", Severity: "error"}); err != nil {
		t.Fatal(err)
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(gotBody)
	want := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(gotSig), []byte(want)) {
		t.Fatalf("signature=%q want=%q", gotSig, want)
	}
	if gotEvent != "collector_failed" {
		t.Fatalf("event header=%q", gotEvent)
	}
	ok, failed, lastOK := s.Metrics()
	if ok != 1 || failed != 0 || lastOK.IsZero() {
		t.Fatalf("metrics ok=%d failed=%d lastOK=%v", ok, failed, lastOK)
	}
}

func TestM51WebhookFailureIsReported(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "no", http.StatusInternalServerError)
	}))
	defer ts.Close()
	s, err := NewWebhookSender(WebhookConfig{URL: ts.URL})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Deliver(context.Background(), Event{Type: "test"}); err == nil {
		t.Fatal("expected delivery error")
	}
	ok, failed, _ := s.Metrics()
	if ok != 0 || failed != 1 {
		t.Fatalf("metrics ok=%d failed=%d", ok, failed)
	}
}
