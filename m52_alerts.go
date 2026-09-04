package main

import (
	"database/sql"
	"fmt"
	"log"
	"time"
)

const alertStateSchema = `
CREATE TABLE IF NOT EXISTS alert_state (
  alert_key TEXT PRIMARY KEY,
  state TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
`

type alertTransition struct {
	Previous string
	Current  string
	Changed  bool
	Existed  bool
}

func (s *Store) initAlertStateSchema() error {
	_, err := s.db.Exec(alertStateSchema)
	return err
}

func (s *Store) AlertState(key string) (string, bool, error) {
	if err := s.initAlertStateSchema(); err != nil {
		return "", false, err
	}
	var state string
	err := s.db.QueryRow(`SELECT state FROM alert_state WHERE alert_key=?`, key).Scan(&state)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	return state, err == nil, err
}

func (s *Store) SetAlertState(key, state, seed string, at time.Time) (alertTransition, error) {
	if err := s.initAlertStateSchema(); err != nil {
		return alertTransition{}, err
	}
	previous, existed, err := s.AlertState(key)
	if err != nil {
		return alertTransition{}, err
	}
	if !existed {
		previous = seed
	}
	changed := previous != state
	_, err = s.db.Exec(`INSERT INTO alert_state(alert_key,state,updated_at) VALUES(?,?,?)
		ON CONFLICT(alert_key) DO UPDATE SET state=excluded.state, updated_at=excluded.updated_at`,
		key, state, at.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return alertTransition{}, err
	}
	return alertTransition{Previous: previous, Current: state, Changed: changed, Existed: existed}, nil
}

func (a *App) m52Transition(key, state, seed string, at time.Time, eventFor func(alertTransition) *Event) {
	tr, err := a.store.SetAlertState(key, state, seed, at)
	if err != nil {
		log.Printf("persist alert state %s: %v", key, err)
		return
	}
	if !tr.Changed {
		return
	}
	e := eventFor(tr)
	if e == nil {
		return
	}
	if e.Metadata == nil {
		e.Metadata = map[string]any{}
	}
	e.Metadata["previous_state"] = tr.Previous
	e.Metadata["current_state"] = tr.Current
	e.CreatedAt = at
	a.emitEvent(*e)
}

func (a *App) reconcileM52(pkg string, collectErr error, before failureCounter, at time.Time) {
	collectorState := "healthy"
	if collectErr != nil {
		collectorState = "failing"
	}
	collectorSeed := "healthy"
	if before.Consecutive > 0 {
		collectorSeed = "failing"
	}
	a.m52Transition("collector:"+pkg, collectorState, collectorSeed, at, func(tr alertTransition) *Event {
		if tr.Current == "failing" {
			msg := "collector failed"
			if collectErr != nil {
				msg = collectErr.Error()
			}
			return &Event{Type: "collector_failed", Severity: "error", Package: pkg, Message: msg}
		}
		return &Event{Type: "collector_recovered", Severity: "info", Package: pkg, Message: "collector recovered"}
	})

	// A successful collection is treated as fresh prospectively: collectAll persists
	// the new snapshot immediately after this hook. Other packages use stored age.
	for _, name := range a.packageNames() {
		state := "fresh"
		if name != pkg || collectErr != nil {
			if a.collectorHealth(name, at).Stale {
				state = "stale"
			}
		}
		name := name
		a.m52Transition("stale:"+name, state, "fresh", at, func(tr alertTransition) *Event {
			if tr.Current == "stale" {
				return &Event{Type: "package_stale", Severity: "warning", Package: name, Message: "package snapshot is stale"}
			}
			return &Event{Type: "package_fresh", Severity: "info", Package: name, Message: "package snapshot is fresh again"}
		})
	}

	_, _, discoveryErr := a.packageState()
	discoveryState := "healthy"
	if discoveryErr != "" {
		discoveryState = "failing"
	}
	a.m52Transition("discovery", discoveryState, "healthy", at, func(tr alertTransition) *Event {
		if tr.Current == "failing" {
			return &Event{Type: "discovery_failed", Severity: "warning", Message: discoveryErr}
		}
		return &Event{Type: "discovery_recovered", Severity: "info", Message: "package discovery recovered"}
	})

	ready := a.readiness(at).Ready
	if collectErr == nil && a.store != nil && a.store.db != nil && a.store.db.Ping() == nil && len(a.packageNames()) > 0 {
		ready = true
	}
	readinessState := "not_ready"
	if ready {
		readinessState = "ready"
	}
	// Seed readiness from the first observed state so startup itself does not emit
	// an alert. Subsequent changes are persisted and survive process restarts.
	seed := readinessState
	if old, ok, err := a.store.AlertState("readiness"); err == nil && ok {
		seed = old
	}
	a.m52Transition("readiness", readinessState, seed, at, func(tr alertTransition) *Event {
		if tr.Current == "not_ready" {
			return &Event{Type: "readiness_not_ready", Severity: "warning", Message: "service is not ready"}
		}
		return &Event{Type: "readiness_recovered", Severity: "info", Message: "service readiness recovered"}
	})
}

func (a *App) writeM52Metrics(w interface{ Write([]byte) (int, error) }) {
	states := []struct {
		key  string
		kind string
	}{
		{key: "discovery", kind: "discovery"},
		{key: "readiness", kind: "readiness"},
	}
	for _, item := range states {
		state, ok, err := a.store.AlertState(item.key)
		if err != nil || !ok {
			continue
		}
		_, _ = fmt.Fprintf(w, "ghcr_stats_alert_state{owner=%q,kind=%q,state=%q} 1\n", a.cfg.Owner, item.kind, state)
	}
}
