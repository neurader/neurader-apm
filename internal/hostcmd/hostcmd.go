package hostcmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"

	"neurader/internal/config"
)

// PlaybookRun mirrors the JSON log structure written by the callback
type PlaybookRun struct {
	Playbook   string                `json:"playbook"`
	StartTime  string                `json:"start_time"`
	EndTime    string                `json:"end_time"`
	TotalHosts int                   `json:"total_hosts"`
	Hosts      map[string]HostResult `json:"hosts"`
}

type HostResult struct {
	Status      string      `json:"status"`
	ErrorOutput *ErrorOut   `json:"error_output"`
	Summary     Summary     `json:"summary"`
}

type ErrorOut struct {
	Msg    string `json:"msg"`
	Module string `json:"module"`
	RC     int    `json:"rc"`
	Stderr string `json:"stderr"`
}

type Summary struct {
	OK          int `json:"ok"`
	Failures    int `json:"failures"`
	Unreachable int `json:"unreachable"`
	Changed     int `json:"changed"`
	Skipped     int `json:"skipped"`
}

// hostStats tracks per-host stats across all runs
type hostStats struct {
	name        string
	totalRuns   int
	failed      int
	unreachable int
	success     int
}

// Last shows the most recent playbook run. If onlyFailed is true, shows only failed hosts.
func Last(cfg config.Config, onlyFailed bool) error {
	path, err := latestLog(cfg.LogDir)
	if err != nil {
		return err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading log: %w", err)
	}

	var run PlaybookRun
	if err := json.Unmarshal(data, &run); err != nil {
		return fmt.Errorf("parsing log: %w", err)
	}

	fmt.Println()
	fmt.Printf("  Last Run: %s\n", run.Playbook)
	fmt.Printf("  Started:  %s\n", run.StartTime)
	fmt.Printf("  Ended:    %s\n", run.EndTime)
	fmt.Println()

	// Count totals
	success, failed, unreachable := 0, 0, 0
	for _, r := range run.Hosts {
		switch r.Status {
		case "success":
			success++
		case "failed":
			failed++
		case "unreachable":
			unreachable++
		}
	}

	fmt.Printf("  ✓ %d success   ✕ %d failed   ⚠ %d unreachable\n",
		success, failed, unreachable)
	fmt.Println()
	fmt.Printf("  %s\n", strings.Repeat("─", 65))

	// Sort hosts
	hosts := make([]string, 0, len(run.Hosts))
	for h := range run.Hosts {
		hosts = append(hosts, h)
	}
	sort.Strings(hosts)

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintf(w, "  %-25s\t%-12s\t%4s\t%7s\t%6s\n",
		"HOST", "STATUS", "OK", "CHANGED", "FAILED")
	fmt.Fprintf(w, "  %-25s\t%-12s\t%4s\t%7s\t%6s\n",
		strings.Repeat("─", 25), strings.Repeat("─", 12),
		strings.Repeat("─", 4), strings.Repeat("─", 7), strings.Repeat("─", 6))

	for _, h := range hosts {
		r := run.Hosts[h]
		if onlyFailed && r.Status == "success" {
			continue
		}
		status := statusIcon(r.Status)
		fmt.Fprintf(w, "  %-25s\t%-12s\t%4d\t%7d\t%6d\n",
			h, status, r.Summary.OK, r.Summary.Changed, r.Summary.Failures)

		// Show error detail if failed/unreachable
		if r.ErrorOutput != nil && r.ErrorOutput.Msg != "" {
			msg := r.ErrorOutput.Msg
			if len(msg) > 55 {
				msg = msg[:52] + "..."
			}
			fmt.Fprintf(w, "  %-25s\t%s\n", "", "  → "+r.ErrorOutput.Module+": "+msg)
		}
	}
	w.Flush()
	fmt.Println()

	return nil
}

// Hosts shows per-host success/failure stats across all recorded runs.
func Hosts(cfg config.Config) error {
	paths, err := allLogs(cfg.LogDir)
	if err != nil {
		return err
	}
	if len(paths) == 0 {
		fmt.Println("  No run logs found.")
		return nil
	}

	stats := map[string]*hostStats{}

	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		var run PlaybookRun
		if err := json.Unmarshal(data, &run); err != nil {
			continue
		}
		for h, r := range run.Hosts {
			if _, ok := stats[h]; !ok {
				stats[h] = &hostStats{name: h}
			}
			stats[h].totalRuns++
			switch r.Status {
			case "success":
				stats[h].success++
			case "failed":
				stats[h].failed++
			case "unreachable":
				stats[h].unreachable++
			}
		}
	}

	// Sort by success rate ascending (worst hosts first)
	list := make([]*hostStats, 0, len(stats))
	for _, s := range stats {
		list = append(list, s)
	}
	sort.Slice(list, func(i, j int) bool {
		ri := float64(list[i].success) / float64(list[i].totalRuns)
		rj := float64(list[j].success) / float64(list[j].totalRuns)
		if ri != rj {
			return ri < rj // worst first
		}
		return list[i].name < list[j].name
	})

	fmt.Println()
	fmt.Printf("  Host Health — based on %d run logs\n", len(paths))
	fmt.Println()

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintf(w, "  %-25s\t%5s\t%7s\t%11s\t%12s\n",
		"HOST", "RUNS", "SUCCESS", "FAILED", "SUCCESS RATE")
	fmt.Fprintf(w, "  %-25s\t%5s\t%7s\t%11s\t%12s\n",
		strings.Repeat("─", 25), strings.Repeat("─", 5),
		strings.Repeat("─", 7), strings.Repeat("─", 11), strings.Repeat("─", 12))

	for _, s := range list {
		rate := float64(s.success) / float64(s.totalRuns) * 100
		rateStr := fmt.Sprintf("%.0f%%", rate)
		health := healthIcon(rate)
		fmt.Fprintf(w, "  %-25s\t%5d\t%7d\t%11d\t%s %s\n",
			s.name, s.totalRuns, s.success,
			s.failed+s.unreachable, health, rateStr)
	}
	w.Flush()
	fmt.Println()
	fmt.Printf("  Legend: ✓ healthy (≥95%%)  ~ degraded (≥80%%)  ✕ critical (<80%%)\n")
	fmt.Println()

	return nil
}

// ── helpers ───────────────────────────────────────────────────────────────────

func statusIcon(status string) string {
	switch status {
	case "success":
		return "✓  success"
	case "failed":
		return "✕  failed"
	case "unreachable":
		return "⚠  unreachable"
	}
	return status
}

func healthIcon(rate float64) string {
	if rate >= 95 {
		return "✓"
	}
	if rate >= 80 {
		return "~"
	}
	return "✕"
}

func latestLog(logDir string) (string, error) {
	paths, err := allLogs(logDir)
	if err != nil {
		return "", err
	}
	if len(paths) == 0 {
		return "", fmt.Errorf("no run logs found in %s — run a playbook first", logDir)
	}
	return paths[len(paths)-1], nil
}

func allLogs(logDir string) ([]string, error) {
	entries, err := filepath.Glob(filepath.Join(logDir, "*.json"))
	if err != nil {
		return nil, fmt.Errorf("reading log dir: %w", err)
	}
	// Filter out inventory.json
	var paths []string
	for _, e := range entries {
		if filepath.Base(e) != "inventory.json" {
			paths = append(paths, e)
		}
	}
	sort.Strings(paths) // alphabetical = chronological due to timestamp in name
	return paths, nil
}
