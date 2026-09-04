package main

import (
	"database/sql"
	"time"
)

const collectionStateSchema = `CREATE TABLE IF NOT EXISTS collection_state (
	package TEXT PRIMARY KEY,
	total_failures INTEGER NOT NULL DEFAULT 0,
	consecutive_failures INTEGER NOT NULL DEFAULT 0,
	last_error TEXT NOT NULL DEFAULT '',
	last_attempt_at TEXT NOT NULL DEFAULT ''
);`

func (s *Store) ensureCollectionState() error {
	_, err := s.db.Exec(collectionStateSchema)
	return err
}

func (s *Store) RecordCollectionResult(pkg string, collectErr error, at time.Time) error {
	if err := s.ensureCollectionState(); err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var total, consecutive uint64
	var lastError string
	err = tx.QueryRow(`SELECT total_failures, consecutive_failures, last_error FROM collection_state WHERE package=?`, pkg).Scan(&total, &consecutive, &lastError)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	if collectErr != nil {
		total++
		consecutive++
		lastError = collectErr.Error()
	} else {
		consecutive = 0
		lastError = ""
	}
	_, err = tx.Exec(`INSERT INTO collection_state(package,total_failures,consecutive_failures,last_error,last_attempt_at)
		VALUES(?,?,?,?,?)
		ON CONFLICT(package) DO UPDATE SET
			total_failures=excluded.total_failures,
			consecutive_failures=excluded.consecutive_failures,
			last_error=excluded.last_error,
			last_attempt_at=excluded.last_attempt_at`,
		pkg, total, consecutive, lastError, at.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) CollectionFailureStats(pkg string) (failureCounter, error) {
	if err := s.ensureCollectionState(); err != nil {
		return failureCounter{}, err
	}
	var st failureCounter
	err := s.db.QueryRow(`SELECT total_failures, consecutive_failures, last_error FROM collection_state WHERE package=?`, pkg).Scan(&st.Total, &st.Consecutive, &st.LastError)
	if err == sql.ErrNoRows {
		return failureCounter{}, nil
	}
	return st, err
}
