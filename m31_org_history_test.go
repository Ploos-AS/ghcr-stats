package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestOrgHistoryCarriesForwardStaggeredSnapshots(t *testing.T) {
	a := m3TestApp(t)
	base := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	for _, st := range []PackageStat{
		{Package: "alpha", Downloads: 100, UpdatedAt: base},
		{Package: "beta", Downloads: 50, UpdatedAt: base.Add(30 * time.Minute)},
		{Package: "alpha", Downloads: 120, UpdatedAt: base.Add(2 * time.Hour)},
		{Package: "beta", Downloads: 70, UpdatedAt: base.Add(3 * time.Hour)},
	} {
		if err := a.store.Save(st); err != nil {
			t.Fatal(err)
		}
	}

	points, err := a.store.OrgHistory([]string{"alpha", "beta"}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	want := []struct {
		downloads int64
		delta     int64
	}{
		{100, 0},
		{150, 50},
		{170, 20},
		{190, 20},
	}
	if len(points) != len(want) {
		t.Fatalf("got %d points: %#v", len(points), points)
	}
	for i := range want {
		if points[i].Downloads != want[i].downloads || points[i].Delta != want[i].delta {
			t.Fatalf("point %d = %#v, want downloads=%d delta=%d", i, points[i], want[i].downloads, want[i].delta)
		}
	}
}

func TestOrgHistoryEmitsPeriodBaseline(t *testing.T) {
	a := m3TestApp(t)
	base := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	since := base.Add(90 * time.Minute)
	for _, st := range []PackageStat{
		{Package: "alpha", Downloads: 100, UpdatedAt: base},
		{Package: "beta", Downloads: 50, UpdatedAt: base.Add(30 * time.Minute)},
		{Package: "alpha", Downloads: 120, UpdatedAt: base.Add(2 * time.Hour)},
	} {
		if err := a.store.Save(st); err != nil {
			t.Fatal(err)
		}
	}

	points, err := a.store.OrgHistory([]string{"alpha", "beta"}, since)
	if err != nil {
		t.Fatal(err)
	}
	if len(points) != 2 {
		t.Fatalf("points=%#v", points)
	}
	if !points[0].Timestamp.Equal(since) || points[0].Downloads != 150 || points[0].Delta != 0 {
		t.Fatalf("baseline=%#v", points[0])
	}
	if points[1].Downloads != 170 || points[1].Delta != 20 {
		t.Fatalf("event=%#v", points[1])
	}
}

func TestOrgHistoryAPIUsesCarryForwardAggregation(t *testing.T) {
	a := m3TestApp(t)
	now := time.Now().UTC().Truncate(time.Second)
	for _, st := range []PackageStat{
		{Package: "alpha", Downloads: 100, UpdatedAt: now.Add(-3 * time.Hour)},
		{Package: "beta", Downloads: 50, UpdatedAt: now.Add(-2 * time.Hour)},
		{Package: "alpha", Downloads: 125, UpdatedAt: now.Add(-time.Hour)},
	} {
		if err := a.store.Save(st); err != nil {
			t.Fatal(err)
		}
	}

	rr := httptest.NewRecorder()
	a.routes().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/org/history?period=24h", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var payload struct {
		Points []HistoryPoint `json:"points"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Points) != 3 {
		t.Fatalf("points=%#v", payload.Points)
	}
	last := payload.Points[len(payload.Points)-1]
	if last.Downloads != 175 || last.Delta != 25 {
		t.Fatalf("last=%#v", last)
	}
}
