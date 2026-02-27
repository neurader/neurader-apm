package logs

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/fatih/color"
)

// LogMeta is a lightweight summary used by the list command.
type LogMeta struct {
	Filename  string
	Playbook  string
	Timestamp time.Time
	SizeBytes int64
	Total     int
	Success   int
	Failed    int
}

// List prints a table of all recorded playbook runs, newest first.
func List(logDir string) error {
	metas, err := readMetas(logDir)
	if err != nil {
		return err
	}

	if len(metas) == 0 {
		fmt.Println("\n  No playbook runs recorded yet.")
		fmt.Println("  Run an ansible-playbook command and logs will appear here.\n")
		return nil
	}

	greenF := color.New(color.FgGreen).SprintFunc()
	redF := color.New(color.FgRed).SprintFunc()
	boldF := color.New(color.Bold).SprintFunc()

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintf(w, "\n  %s\t%s\t%s\t%s\t%s\t%s\n",
		boldF("TIMESTAMP"), boldF("PLAYBOOK"),
		boldF("HOSTS"), boldF("SUCCESS"), boldF("FAILED"), boldF("FILE"))
	fmt.Fprintln(w, "  ─────────────────────\t─────────────────────\t─────\t───────\t──────\t──────────────────────────────")

	for _, m := range metas {
		failedStr := fmt.Sprintf("%d", m.Failed)
		if m.Failed > 0 {
			failedStr = redF(failedStr)
		}
		fmt.Fprintf(w, "  %s\t%s\t%d\t%s\t%s\t%s\n",
			m.Timestamp.Format("2006-01-02 15:04:05"),
			truncate(m.Playbook, 20),
			m.Total,
			greenF(fmt.Sprintf("%d", m.Success)),
			failedStr,
			m.Filename,
		)
	}
	w.Flush()
	fmt.Println()
	return nil
}

// Show prints the detailed per-host result of a single playbook run.
func Show(logDir, filename string) error {
	fullPath := filename
	if !filepath.IsAbs(filename) {
		fullPath = filepath.Join(logDir, filename)
	}

	data, err := os.ReadFile(fullPath)
	if err != nil {
		return fmt.Errorf("cannot open %s: %w", filename, err)
	}

	var run PlaybookRun
	if err := json.Unmarshal(data, &run); err != nil {
		return fmt.Errorf("parsing %s: %w", filename, err)
	}

	printRun(run)
	return nil
}

// LatestPath returns the full path to the most recently written log file.
func LatestPath(logDir string) (string, error) {
	metas, err := readMetas(logDir)
	if err != nil || len(metas) == 0 {
		return "", err
	}
	return filepath.Join(logDir, metas[0].Filename), nil
}

// AllPaths returns full paths to every log file, newest first.
func AllPaths(logDir string) ([]string, error) {
	metas, err := readMetas(logDir)
	if err != nil {
		return nil, err
	}
	paths := make([]string, len(metas))
	for i, m := range metas {
		paths[i] = filepath.Join(logDir, m.Filename)
	}
	return paths, nil
}

// ── Internal ──────────────────────────────────────────────────────────────────

func readMetas(logDir string) ([]LogMeta, error) {
	entries, err := os.ReadDir(logDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading log dir: %w", err)
	}

	var metas []LogMeta
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		m := LogMeta{
			Filename:  e.Name(),
			Timestamp: info.ModTime(),
			SizeBytes: info.Size(),
		}
		// Parse the file to get playbook name and host counts
		data, err := os.ReadFile(filepath.Join(logDir, e.Name()))
		if err == nil {
			var run PlaybookRun
			if json.Unmarshal(data, &run) == nil {
				m.Playbook = run.Playbook
				m.Total = run.TotalHosts
				for _, h := range run.Hosts {
					if h.Status == "success" {
						m.Success++
					} else {
						m.Failed++
					}
				}
			}
		}
		metas = append(metas, m)
	}

	// Sort newest first
	sort.Slice(metas, func(i, j int) bool {
		return metas[i].Timestamp.After(metas[j].Timestamp)
	})
	return metas, nil
}

func printRun(run PlaybookRun) {
	greenC := color.New(color.FgGreen, color.Bold)
	redC := color.New(color.FgRed, color.Bold)
	boldC := color.New(color.Bold)
	cyanC := color.New(color.FgCyan)

	fmt.Println()
	boldC.Printf("  Playbook : %s\n", run.Playbook)
	fmt.Printf("  Started  : %s\n", run.StartTime)
	fmt.Printf("  Ended    : %s\n", run.EndTime)
	fmt.Printf("  Hosts    : %d total\n", run.TotalHosts)
	fmt.Println()

	// Sort: failed/unreachable first, then alphabetical
	type entry struct {
		name string
		h    HostResult
	}
	var hosts []entry
	for name, h := range run.Hosts {
		hosts = append(hosts, entry{name, h})
	}
	sort.Slice(hosts, func(i, j int) bool {
		ri, rj := statusRank(hosts[i].h.Status), statusRank(hosts[j].h.Status)
		if ri != rj {
			return ri < rj
		}
		return hosts[i].name < hosts[j].name
	})

	for _, e := range hosts {
		h := e.h
		switch h.Status {
		case "success":
			greenC.Printf("  ✓ %-35s SUCCESS", e.name)
			fmt.Printf("  (ok=%d changed=%d skipped=%d)\n",
				h.Summary.OK, h.Summary.Changed, h.Summary.Skipped)

		case "failed":
			redC.Printf("  ✗ %-35s FAILED\n", e.name)
			if h.ErrorOutput != nil {
				cyanC.Printf("    Module : %s\n", h.ErrorOutput.Module)
				if h.ErrorOutput.Msg != "" {
					fmt.Printf("    Msg    : %s\n", h.ErrorOutput.Msg)
				}
				if h.ErrorOutput.Stderr != "" {
					fmt.Printf("    Stderr : %s\n", indentLines(h.ErrorOutput.Stderr, "             "))
				}
				if h.ErrorOutput.Stdout != "" {
					fmt.Printf("    Stdout : %s\n", indentLines(h.ErrorOutput.Stdout, "             "))
				}
				fmt.Printf("    RC     : %d\n", h.ErrorOutput.RC)
			}

		case "unreachable":
			redC.Printf("  ✗ %-35s UNREACHABLE\n", e.name)
			if h.ErrorOutput != nil && h.ErrorOutput.Msg != "" {
				fmt.Printf("    Msg    : %s\n", h.ErrorOutput.Msg)
			}
		}
	}
	fmt.Println()
}

func statusRank(s string) int {
	if s == "failed" || s == "unreachable" {
		return 0
	}
	return 1
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

func indentLines(s, prefix string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) == 1 {
		return lines[0]
	}
	result := lines[0]
	for _, l := range lines[1:] {
		result += "\n" + prefix + l
	}
	return result
}