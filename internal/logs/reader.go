package logs

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
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
		fmt.Println()
		fmt.Println("  No playbook runs recorded yet.")
		fmt.Println("  Run an ansible-playbook command and logs will appear here.")
		fmt.Println()
		return nil
	}

	greenF := color.New(color.FgGreen).SprintFunc()
	redF   := color.New(color.FgRed).SprintFunc()
	boldF  := color.New(color.Bold).SprintFunc()

	header := fmt.Sprintf("  %-19s  %-20s  %-5s  %-7s  %-6s  %s",
		"TIMESTAMP", "PLAYBOOK", "HOSTS", "SUCCESS", "FAILED", "FILE")
	sep := fmt.Sprintf("  %-19s  %-20s  %-5s  %-7s  %-6s  %s",
		"───────────────────", "────────────────────",
		"─────", "───────", "──────",
		"──────────────────────────────────────")
	fmt.Println()
	fmt.Println(boldF(header))
	fmt.Println(sep)

	for _, m := range metas {
		successStr := greenF(fmt.Sprintf("%-7d", m.Success))
		failedVal  := fmt.Sprintf("%-6d", m.Failed)
		failedStr  := failedVal
		if m.Failed > 0 {
			failedStr = redF(failedVal)
		}
		fmt.Printf("  %-19s  %-20s  %-5d  %s  %s  %s\n",
			m.Timestamp.Format("2006-01-02 15:04:05"),
			truncate(m.Playbook, 20),
			m.Total,
			successStr,
			failedStr,
			m.Filename,
		)
	}
	fmt.Println()
	return nil
}

// Show prints the detailed per-host result of a single playbook run.
// arg can be:
//   - exact filename:  sitecom.yml_2026-03-08_00-50-05.json
//   - playbook name:   sitecom.yml  → finds latest run for that playbook
func Show(logDir, arg string) error {
	path, err := resolveLog(logDir, arg)
	if err != nil {
		return err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("cannot open %s: %w", filepath.Base(path), err)
	}

	var run PlaybookRun
	if err := json.Unmarshal(data, &run); err != nil {
		return fmt.Errorf("parsing log: %w", err)
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

// resolveLog finds the log file from an exact filename or playbook name.
func resolveLog(logDir, arg string) (string, error) {
	// Exact filename
	if strings.HasSuffix(arg, ".json") {
		path := filepath.Join(logDir, arg)
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
		return "", fmt.Errorf("log file not found: %s", arg)
	}

	// Playbook name — find latest run
	pattern := filepath.Join(logDir, arg+"_*.json")
	matches, err := filepath.Glob(pattern)
	if err != nil || len(matches) == 0 {
		return "", fmt.Errorf("no runs found for playbook %q — use `neurader list` to see available runs", arg)
	}
	sort.Strings(matches)
	return matches[len(matches)-1], nil
}

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
		// Skip inventory snapshot
		if e.Name() == "inventory.json" {
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
		data, err := os.ReadFile(filepath.Join(logDir, e.Name()))
		if err == nil {
			var run PlaybookRun
			if json.Unmarshal(data, &run) == nil {
				m.Playbook = run.Playbook
				m.Total    = run.TotalHosts
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

	sort.Slice(metas, func(i, j int) bool {
		return metas[i].Timestamp.After(metas[j].Timestamp)
	})
	return metas, nil
}

func printRun(run PlaybookRun) {
	greenC := color.New(color.FgGreen, color.Bold)
	redC   := color.New(color.FgRed, color.Bold)
	boldC  := color.New(color.Bold)
	cyanC  := color.New(color.FgCyan)
	div    := strings.Repeat("─", 65)

	fmt.Println()
	boldC.Printf("  Playbook : %s\n", run.Playbook)
	fmt.Printf("  Started  : %s\n", run.StartTime)
	fmt.Printf("  Ended    : %s\n", run.EndTime)
	fmt.Printf("  Hosts    : %d total\n", run.TotalHosts)
	fmt.Println()
	fmt.Printf("  %s\n", div)

	// Sort — failed/unreachable first, then alphabetical
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

	failedCount  := 0
	successCount := 0

	for _, e := range hosts {
		h := e.h
		switch h.Status {
		case "failed", "unreachable":
			failedCount++
			icon := "✗"
			label := "FAILED"
			if h.Status == "unreachable" {
				label = "UNREACHABLE"
			}
			fmt.Println()
			redC.Printf("  %s  %-30s  [%s]\n", icon, e.name, label)
			fmt.Printf("     OK: %d  Changed: %d  Failed: %d  Skipped: %d\n",
				h.Summary.OK, h.Summary.Changed, h.Summary.Failures, h.Summary.Skipped)

			if len(h.FailedTasks) > 0 {
				fmt.Println()
				for i, t := range h.FailedTasks {
					cyanC.Printf("     Failed Task %d of %d\n", i+1, len(h.FailedTasks))
					fmt.Printf("     %s\n", strings.Repeat("·", 55))
					fmt.Printf("     Task    : %s\n", t.TaskName)
					if t.TaskPath != "" {
						fmt.Printf("     Path    : %s\n", t.TaskPath)
					}
					fmt.Printf("     Module  : %s\n", t.Module)
					if len(t.TaskArgs) > 0 {
						fmt.Printf("     Args    :\n")
						for k, v := range t.TaskArgs {
							fmt.Printf("               %s = %v\n", k, v)
						}
					}
					fmt.Printf("     RC      : %d\n", t.RC)
					if t.Msg != "" {
						fmt.Printf("     Message :\n")
						printIndented(t.Msg, "               ")
					}
					if t.Stdout != "" {
						fmt.Printf("     Stdout  :\n")
						printIndented(t.Stdout, "               ")
					}
					if t.Stderr != "" {
						fmt.Printf("     Stderr  :\n")
						printIndented(t.Stderr, "               ")
					}
					if t.Exception != "" {
						fmt.Printf("     Exception:\n")
						printIndented(t.Exception, "               ")
					}
					fmt.Println()
				}
			}
			fmt.Printf("  %s\n", div)

		case "success":
			successCount++
		}
	}

	// Success — count only, no detail
	if successCount > 0 {
		fmt.Println()
		greenC.Printf("  ✓  %d host(s) succeeded\n", successCount)
		fmt.Println()
	}
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

func printIndented(text, indent string) {
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	for _, line := range lines {
		fmt.Printf("%s%s\n", indent, line)
	}
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
