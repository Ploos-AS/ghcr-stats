package main

import (
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (s *Store) IntegrityCheck() error {
	rows, err := s.db.Query("PRAGMA quick_check")
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var result string
		if err := rows.Scan(&result); err != nil {
			return err
		}
		if result != "ok" {
			return fmt.Errorf("sqlite quick_check: %s", result)
		}
	}
	return rows.Err()
}

// ApplyRetention removes old snapshots while retaining the newest snapshot at
// or before the cutoff for each package. That retained point remains available
// as a baseline for bounded-period delta calculations.
func (s *Store) ApplyRetention(cutoff time.Time) (int64, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	rows, err := tx.Query("SELECT DISTINCT package FROM snapshots")
	if err != nil {
		return 0, err
	}
	var packages []string
	for rows.Next() {
		var pkg string
		if err := rows.Scan(&pkg); err != nil {
			rows.Close()
			return 0, err
		}
		packages = append(packages, pkg)
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}

	cutoffText := cutoff.UTC().Format(time.RFC3339Nano)
	var deleted int64
	for _, pkg := range packages {
		var baselineRowID int64
		err := tx.QueryRow("SELECT rowid FROM snapshots WHERE package=? AND collected_at<=? ORDER BY collected_at DESC LIMIT 1", pkg, cutoffText).Scan(&baselineRowID)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return 0, err
		}
		res, err := tx.Exec("DELETE FROM snapshots WHERE package=? AND collected_at<? AND rowid<>?", pkg, cutoffText, baselineRowID)
		if err != nil {
			return 0, err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return 0, err
		}
		deleted += n
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return deleted, nil
}

func (s *Store) Optimize() error {
	_, err := s.db.Exec("PRAGMA optimize")
	return err
}

func (s *Store) Vacuum() error {
	_, err := s.db.Exec("VACUUM")
	return err
}

func sqliteQuoteString(v string) string {
	return "'" + strings.ReplaceAll(v, "'", "''") + "'"
}

// Backup creates a transactionally consistent standalone SQLite database.
// SQLite's VACUUM INTO refuses to overwrite an existing destination.
func (s *Store) Backup(dest string) error {
	if strings.TrimSpace(dest) == "" {
		return errors.New("backup destination is empty")
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	_, err := s.db.Exec("VACUUM INTO " + sqliteQuoteString(dest))
	return err
}

func copyFileAtomic(src, dest string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	tmp := dest + ".restore-tmp"
	_ = os.Remove(tmp)
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	ok := false
	defer func() {
		_ = out.Close()
		if !ok {
			_ = os.Remove(tmp)
		}
	}()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	if err := out.Sync(); err != nil {
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, dest); err != nil {
		return err
	}
	ok = true
	return nil
}

func restoreDatabase(src, dest string) error {
	if filepath.Clean(src) == filepath.Clean(dest) {
		return errors.New("restore source and destination must differ")
	}
	check, err := OpenStore(src)
	if err != nil {
		return fmt.Errorf("open restore source: %w", err)
	}
	if err := check.IntegrityCheck(); err != nil {
		check.Close()
		return fmt.Errorf("restore source integrity: %w", err)
	}
	if err := check.Close(); err != nil {
		return err
	}
	return copyFileAtomic(src, dest)
}

func runMaintenanceCommand(cfg Config, args []string) (bool, error) {
	if len(args) == 0 {
		return false, nil
	}
	switch args[0] {
	case "integrity":
		if len(args) != 1 {
			return true, errors.New("usage: ghcr-stats integrity")
		}
		s, err := OpenStore(cfg.DBPath)
		if err != nil {
			return true, err
		}
		defer s.Close()
		return true, s.IntegrityCheck()
	case "backup":
		if len(args) != 2 {
			return true, errors.New("usage: ghcr-stats backup DESTINATION")
		}
		s, err := OpenStore(cfg.DBPath)
		if err != nil {
			return true, err
		}
		defer s.Close()
		if err := s.IntegrityCheck(); err != nil {
			return true, err
		}
		return true, s.Backup(args[1])
	case "restore":
		if len(args) != 2 {
			return true, errors.New("usage: ghcr-stats restore SOURCE")
		}
		return true, restoreDatabase(args[1], cfg.DBPath)
	case "maintain":
		if len(args) != 1 {
			return true, errors.New("usage: ghcr-stats maintain")
		}
		s, err := OpenStore(cfg.DBPath)
		if err != nil {
			return true, err
		}
		defer s.Close()
		if err := s.IntegrityCheck(); err != nil {
			return true, err
		}
		if cfg.Retention > 0 {
			if _, err := s.ApplyRetention(time.Now().UTC().Add(-cfg.Retention)); err != nil {
				return true, err
			}
		}
		if err := s.Optimize(); err != nil {
			return true, err
		}
		return true, s.Vacuum()
	default:
		return false, nil
	}
}
