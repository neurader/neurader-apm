package logs

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Clean deletes JSON log files in logDir that are older than retentionDays.
// It parses the date from the filename (format: playbook_YYYY-MM-DD_HH-MM-SS.json)
// rather than relying on ModTime, which can be unreliable on copied or synced files.
// Silently does nothing if the log dir does not yet exist.
func Clean(logDir string, retentionDays int) {
	cutoff := time.Now().AddDate(0, 0, -retentionDays)

	entries, err := os.ReadDir(logDir)
	if err != nil {
		return // log dir may not exist on first run — that is fine
	}

	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}

		fileTime, err := parseFileDate(e.Name())
		if err != nil {
			// filename date unparseable — fall back to ModTime
			info, err := e.Info()
			if err != nil {
				continue
			}
			fileTime = info.ModTime()
		}

		if fileTime.Before(cutoff) {
			path := filepath.Join(logDir, e.Name())
			if err := os.Remove(path); err == nil {
				fmt.Fprintf(os.Stderr, "[neurader] deleted old log: %s\n", e.Name())
			} else {
				fmt.Fprintf(os.Stderr, "[neurader] failed to delete %s: %v\n", e.Name(), err)
			}
		}
	}
}

// parseFileDate extracts the timestamp from a neurader log filename.
// Expected format: <playbook>_YYYY-MM-DD_HH-MM-SS.json
// e.g. site_2024-01-15_14-30-00.json
func parseFileDate(name string) (time.Time, error) {
	// Strip extension
	base := name[:len(name)-len(filepath.Ext(name))]

	// The last two underscore-separated segments are date and time
	// Walk backwards to find YYYY-MM-DD_HH-MM-SS
	if len(base) < 19 {
		return time.Time{}, fmt.Errorf("too short")
	}

	// Last 19 chars should be YYYY-MM-DD_HH-MM-SS
	datePart := base[len(base)-19:]
	t, err := time.ParseInLocation("2006-01-02_15-04-05", datePart, time.Local)
	if err != nil {
		return time.Time{}, err
	}
	return t, nil
}
