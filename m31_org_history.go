package main

import (
	"database/sql"
	"errors"
	"sort"
	"time"
)

type orgHistoryEvent struct {
	Timestamp time.Time
	Package   string
	Downloads int64
}

// OrgHistory builds an organization-wide history using last-known-value carry-forward.
// Package snapshots do not need to share timestamps: every event updates one package,
// while the organization total at that instant includes the latest known value for all
// other packages. For bounded periods, a synthetic baseline is emitted at the period
// boundary from the newest snapshot at or before that boundary for each package.
func (s *Store) OrgHistory(packages []string, since time.Time) ([]HistoryPoint, error) {
	current := make(map[string]int64, len(packages))
	events := make([]orgHistoryEvent, 0)

	for _, pkg := range packages {
		if !since.IsZero() {
			var baseline int64
			err := s.db.QueryRow(
				"SELECT downloads FROM snapshots WHERE package=? AND collected_at<=? ORDER BY collected_at DESC LIMIT 1",
				pkg,
				since.UTC().Format(time.RFC3339Nano),
			).Scan(&baseline)
			if err == nil {
				current[pkg] = baseline
			} else if !errors.Is(err, sql.ErrNoRows) {
				return nil, err
			}
		}

		query := "SELECT downloads,collected_at FROM snapshots WHERE package=?"
		args := []any{pkg}
		if !since.IsZero() {
			query += " AND collected_at>?"
			args = append(args, since.UTC().Format(time.RFC3339Nano))
		}
		query += " ORDER BY collected_at ASC"

		rows, err := s.db.Query(query, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var downloads int64
			var raw string
			if err := rows.Scan(&downloads, &raw); err != nil {
				rows.Close()
				return nil, err
			}
			ts, err := time.Parse(time.RFC3339Nano, raw)
			if err != nil {
				rows.Close()
				return nil, err
			}
			events = append(events, orgHistoryEvent{Timestamp: ts, Package: pkg, Downloads: downloads})
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		rows.Close()
	}

	sort.Slice(events, func(i, j int) bool {
		if events[i].Timestamp.Equal(events[j].Timestamp) {
			return events[i].Package < events[j].Package
		}
		return events[i].Timestamp.Before(events[j].Timestamp)
	})

	total := func() int64 {
		var n int64
		for _, value := range current {
			n += value
		}
		return n
	}

	points := make([]HistoryPoint, 0, len(events)+1)
	var previous int64
	if !since.IsZero() && len(current) > 0 {
		previous = total()
		points = append(points, HistoryPoint{Timestamp: since.UTC(), Downloads: previous})
	}

	for i := 0; i < len(events); {
		ts := events[i].Timestamp
		for i < len(events) && events[i].Timestamp.Equal(ts) {
			current[events[i].Package] = events[i].Downloads
			i++
		}
		n := total()
		delta := int64(0)
		if len(points) > 0 && n >= previous {
			delta = n - previous
		}
		points = append(points, HistoryPoint{Timestamp: ts, Downloads: n, Delta: delta})
		previous = n
	}

	return points, nil
}
