package main

import (
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

const packageLifecycleSchema = `
CREATE TABLE IF NOT EXISTS package_lifecycle_state (
  package TEXT PRIMARY KEY,
  present INTEGER NOT NULL DEFAULT 1,
  missing_cycles INTEGER NOT NULL DEFAULT 0,
  ever_present INTEGER NOT NULL DEFAULT 1,
  updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS package_lifecycle_meta (
  singleton INTEGER PRIMARY KEY CHECK(singleton = 1),
  initialized INTEGER NOT NULL DEFAULT 0,
  last_scan_at TEXT NOT NULL DEFAULT ''
);
INSERT OR IGNORE INTO package_lifecycle_meta(singleton,initialized,last_scan_at) VALUES(1,0,'');
`

const lifecycleMissingDebounce = 2
const lifecycleProductionScanGap = time.Minute

type packageLifecycleState struct {
	Package       string
	Present       bool
	MissingCycles int
	EverPresent   bool
	UpdatedAt     time.Time
}

func (s *Store) initPackageLifecycleSchema() error {
	_, err := s.db.Exec(packageLifecycleSchema)
	return err
}

func normalizePackageSet(pkgs []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(pkgs))
	for _, pkg := range pkgs {
		pkg = strings.TrimSpace(pkg)
		if pkg == "" {
			continue
		}
		if _, ok := seen[pkg]; ok {
			continue
		}
		seen[pkg] = struct{}{}
		out = append(out, pkg)
	}
	sort.Strings(out)
	return out
}

func (s *Store) packageLifecycleInitialized(tx *sql.Tx) (bool, time.Time, error) {
	var initialized int
	var raw string
	if err := tx.QueryRow(`SELECT initialized,last_scan_at FROM package_lifecycle_meta WHERE singleton=1`).Scan(&initialized, &raw); err != nil {
		return false, time.Time{}, err
	}
	var last time.Time
	if raw != "" {
		parsed, err := time.Parse(time.RFC3339Nano, raw)
		if err != nil {
			return false, time.Time{}, err
		}
		last = parsed
	}
	return initialized != 0, last, nil
}

func (s *Store) PackageLifecycleStates() ([]packageLifecycleState, error) {
	if err := s.initPackageLifecycleSchema(); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`SELECT package,present,missing_cycles,ever_present,updated_at FROM package_lifecycle_state ORDER BY package`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []packageLifecycleState
	for rows.Next() {
		var st packageLifecycleState
		var present, ever int
		var raw string
		if err := rows.Scan(&st.Package, &present, &st.MissingCycles, &ever, &raw); err != nil {
			return nil, err
		}
		st.Present = present != 0
		st.EverPresent = ever != 0
		st.UpdatedAt, err = time.Parse(time.RFC3339Nano, raw)
		if err != nil {
			return nil, err
		}
		out = append(out, st)
	}
	return out, rows.Err()
}

// ObservePackageLifecycle records one successful discovery observation. The first
// observation establishes a baseline without emitting package_added events.
// Later observations emit transition-only events. A package must be absent from
// lifecycleMissingDebounce distinct observations before package_missing fires.
func (s *Store) ObservePackageLifecycle(pkgs []string, at time.Time, minGap time.Duration) ([]Event, bool, error) {
	if err := s.initPackageLifecycleSchema(); err != nil {
		return nil, false, err
	}
	pkgs = normalizePackageSet(pkgs)
	at = at.UTC()
	tx, err := s.db.Begin()
	if err != nil {
		return nil, false, err
	}
	defer tx.Rollback()

	initialized, lastScan, err := s.packageLifecycleInitialized(tx)
	if err != nil {
		return nil, false, err
	}
	if initialized && minGap > 0 && !lastScan.IsZero() && at.Sub(lastScan) < minGap {
		return nil, false, nil
	}

	rows, err := tx.Query(`SELECT package,present,missing_cycles,ever_present FROM package_lifecycle_state`)
	if err != nil {
		return nil, false, err
	}
	existing := map[string]packageLifecycleState{}
	for rows.Next() {
		var st packageLifecycleState
		var present, ever int
		if err := rows.Scan(&st.Package, &present, &st.MissingCycles, &ever); err != nil {
			rows.Close()
			return nil, false, err
		}
		st.Present = present != 0
		st.EverPresent = ever != 0
		existing[st.Package] = st
	}
	if err := rows.Close(); err != nil {
		return nil, false, err
	}

	current := map[string]struct{}{}
	for _, pkg := range pkgs {
		current[pkg] = struct{}{}
	}

	var events []Event
	stamp := at.Format(time.RFC3339Nano)
	for _, pkg := range pkgs {
		st, exists := existing[pkg]
		switch {
		case !exists:
			if _, err := tx.Exec(`INSERT INTO package_lifecycle_state(package,present,missing_cycles,ever_present,updated_at) VALUES(?,1,0,1,?)`, pkg, stamp); err != nil {
				return nil, false, err
			}
			if initialized {
				events = append(events, Event{Type: "package_added", Severity: "info", Package: pkg, Message: "package discovered", CreatedAt: at})
			}
		case !st.Present:
			if _, err := tx.Exec(`UPDATE package_lifecycle_state SET present=1,missing_cycles=0,ever_present=1,updated_at=? WHERE package=?`, stamp, pkg); err != nil {
				return nil, false, err
			}
			events = append(events, Event{Type: "package_returned", Severity: "info", Package: pkg, Message: "package returned", CreatedAt: at})
		default:
			if _, err := tx.Exec(`UPDATE package_lifecycle_state SET missing_cycles=0,updated_at=? WHERE package=?`, stamp, pkg); err != nil {
				return nil, false, err
			}
		}
	}

	for pkg, st := range existing {
		if _, ok := current[pkg]; ok {
			continue
		}
		if !st.Present {
			continue
		}
		missing := st.MissingCycles + 1
		if missing >= lifecycleMissingDebounce {
			if _, err := tx.Exec(`UPDATE package_lifecycle_state SET present=0,missing_cycles=?,updated_at=? WHERE package=?`, missing, stamp, pkg); err != nil {
				return nil, false, err
			}
			events = append(events, Event{Type: "package_missing", Severity: "warning", Package: pkg, Message: "package missing from discovery", CreatedAt: at, Metadata: map[string]any{"missing_cycles": missing}})
		} else {
			if _, err := tx.Exec(`UPDATE package_lifecycle_state SET missing_cycles=?,updated_at=? WHERE package=?`, missing, stamp, pkg); err != nil {
				return nil, false, err
			}
		}
	}

	if _, err := tx.Exec(`UPDATE package_lifecycle_meta SET initialized=1,last_scan_at=? WHERE singleton=1`, stamp); err != nil {
		return nil, false, err
	}
	if err := tx.Commit(); err != nil {
		return nil, false, err
	}
	return events, true, nil
}

func (a *App) observePackageLifecycle(at time.Time) {
	// Explicit package configuration is not discovery lifecycle and therefore
	// must not generate added/missing events.
	if a.cfg.PackagesExplicit || a.discoverer == nil {
		return
	}
	_, _, discoveryErr := a.packageState()
	if discoveryErr != "" {
		return
	}
	events, scanned, err := a.store.ObservePackageLifecycle(a.packageNames(), at, lifecycleProductionScanGap)
	if err != nil {
		fmt.Printf("package lifecycle observation: %v\n", err)
		return
	}
	if !scanned {
		return
	}
	for _, event := range events {
		a.emitEvent(event)
	}
}

func (s *Store) PackageLifecycleCounts() (present, missing int64, err error) {
	if err = s.initPackageLifecycleSchema(); err != nil {
		return 0, 0, err
	}
	if err = s.db.QueryRow(`SELECT COUNT(*) FROM package_lifecycle_state WHERE present=1`).Scan(&present); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, 0, err
	}
	if err = s.db.QueryRow(`SELECT COUNT(*) FROM package_lifecycle_state WHERE present=0`).Scan(&missing); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, 0, err
	}
	return present, missing, nil
}
