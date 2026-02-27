package logs

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Clean deletes JSON log files in logDir that are older than retentionDays.
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
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			path := filepath.Join(logDir, e.Name())
			if err := os.Remove(path); err == nil {
				fmt.Printf("[neurader] Deleted old log: %s\n", e.Name())
			}
		}
	}
}