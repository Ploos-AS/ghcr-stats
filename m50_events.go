package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const eventSchema = `
CREATE TABLE IF NOT EXISTS events (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  event_type TEXT NOT NULL,
  severity TEXT NOT NULL,
  package TEXT NOT NULL DEFAULT '',
  message TEXT NOT NULL DEFAULT '',
  metadata_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_events_created_at ON events(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_events_package_created_at ON events(package, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_events_type_created_at ON events(event_type, created_at DESC);
`

type Event struct {
	ID        int64          `json:"id"`
	Type      string         `json:"type"`
	Severity  string         `json:"severity"`
	Owner     string         `json:"owner"`
	Package   string         `json:"package,omitempty"`
	Message   string         `json:"message,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
}

func (s *Store) initEventSchema() error {
	_, err := s.db.Exec(eventSchema)
	return err
}

func (s *Store) SaveEvent(e Event) (int64, error) {
	if e.CreatedAt.IsZero() {
		e.CreatedAt = time.Now().UTC()
	}
	if e.Severity == "" {
		e.Severity = "info"
	}
	meta, err := json.Marshal(e.Metadata)
	if err != nil {
		return 0, err
	}
	res, err := s.db.Exec(`INSERT INTO events(event_type,severity,package,message,metadata_json,created_at) VALUES(?,?,?,?,?,?)`, e.Type, e.Severity, e.Package, e.Message, string(meta), e.CreatedAt.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) Events(pkg, typ, severity string, limit int) ([]Event, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}
	q := `SELECT id,event_type,severity,package,message,metadata_json,created_at FROM events WHERE 1=1`
	args := []any{}
	if pkg != "" {
		q += ` AND package=?`
		args = append(args, pkg)
	}
	if typ != "" {
		q += ` AND event_type=?`
		args = append(args, typ)
	}
	if severity != "" {
		q += ` AND severity=?`
		args = append(args, severity)
	}
	q += ` ORDER BY id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Event{}
	for rows.Next() {
		var e Event
		var meta, ts string
		if err := rows.Scan(&e.ID, &e.Type, &e.Severity, &e.Package, &e.Message, &meta, &ts); err != nil {
			return nil, err
		}
		if meta != "" && meta != "{}" {
			_ = json.Unmarshal([]byte(meta), &e.Metadata)
		}
		e.CreatedAt, err = time.Parse(time.RFC3339Nano, ts)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *Store) EventCount() (int64, error) {
	var n int64
	err := s.db.QueryRow(`SELECT COUNT(*) FROM events`).Scan(&n)
	return n, err
}

func (a *App) emitEvent(e Event) {
	e.Owner = a.cfg.Owner
	if _, err := a.store.SaveEvent(e); err != nil {
		fmt.Printf("save event %s: %v\n", e.Type, err)
	}
}

func (a *App) handleEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	limit := 100
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 1000 {
			http.Error(w, "invalid limit", http.StatusBadRequest)
			return
		}
		limit = n
	}
	severity := strings.TrimSpace(r.URL.Query().Get("severity"))
	if severity != "" && severity != "info" && severity != "warning" && severity != "error" {
		http.Error(w, "invalid severity", http.StatusBadRequest)
		return
	}
	items, err := a.store.Events(strings.TrimSpace(r.URL.Query().Get("package")), strings.TrimSpace(r.URL.Query().Get("type")), severity, limit)
	if err != nil && err != sql.ErrNoRows {
		http.Error(w, "event query failed", http.StatusInternalServerError)
		return
	}
	for i := range items {
		items[i].Owner = a.cfg.Owner
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"owner": a.cfg.Owner, "events": items})
}

func (a *App) writeM50Metrics(w http.ResponseWriter) {
	if n, err := a.store.EventCount(); err == nil {
		fmt.Fprintf(w, "ghcr_stats_events_total{owner=%q} %d\n", a.cfg.Owner, n)
	}
}
