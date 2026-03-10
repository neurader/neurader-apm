package loki

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"neurader/internal/config"
	"neurader/internal/logs"
)

type lokiPushPayload struct {
	Streams []lokiStream `json:"streams"`
}

type lokiStream struct {
	Stream map[string]string `json:"stream"`
	Values [][]string        `json:"values"`
}

// PushLatest reads the most recently written log and pushes it to Loki.
func PushLatest(cfg config.Config) error {
	path, err := logs.LatestPath(cfg.LogDir)
	if err != nil || path == "" {
		return err
	}
	return pushFile(cfg, path)
}

// PushAll pushes every log file in logDir to Loki.
func PushAll(cfg config.Config) error {
	paths, err := logs.AllPaths(cfg.LogDir)
	if err != nil {
		return err
	}
	if len(paths) == 0 {
		fmt.Println("  No log files found.")
		return nil
	}
	for _, p := range paths {
		if err := pushFile(cfg, p); err != nil {
			fmt.Printf("  ⚠ %s: %v\n", p, err)
		} else {
			fmt.Printf("  ✓ pushed %s\n", p)
		}
	}
	return nil
}

func pushFile(cfg config.Config, filePath string) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("reading %s: %w", filePath, err)
	}

	var run logs.PlaybookRun
	if err := json.Unmarshal(data, &run); err != nil {
		return fmt.Errorf("parsing %s: %w", filePath, err)
	}

	ts := parseTimeNS(run.StartTime)
	c  := newClient(cfg)

	var streams []lokiStream
	for hostName, result := range run.Hosts {
		logLine := buildLogLine(run, hostName, result, filePath)
		stream  := lokiStream{
			Stream: map[string]string{
				"job":      "neurader",
				"playbook": run.Playbook,
				"host":     hostName,
				"status":   result.Status,
				"run_id":   runID(filePath),
			},
			Values: [][]string{
				{strconv.FormatInt(ts, 10), logLine},
			},
		}
		streams = append(streams, stream)
	}

	if len(streams) == 0 {
		return nil
	}
	return c.push(lokiPushPayload{Streams: streams})
}

// buildLogLine serialises the host result into a JSON log line for Grafana.
// For failed hosts includes first failed task details for dashboard panels.
func buildLogLine(run logs.PlaybookRun, host string, result logs.HostResult, filePath string) string {
	entry := map[string]interface{}{
		"playbook":    run.Playbook,
		"start_time":  run.StartTime,
		"end_time":    run.EndTime,
		"host":        host,
		"run_id":      runID(filePath),
		"status":      result.Status,
		"ok":          result.Summary.OK,
		"changed":     result.Summary.Changed,
		"failed":      result.Summary.Failures,
		"unreachable": result.Summary.Unreachable,
		"skipped":     result.Summary.Skipped,
	}

	// For Grafana dashboard panels — include first failed task details
	// Full task list is in the local log file, accessible via neurader show
	if len(result.FailedTasks) > 0 {
		first := result.FailedTasks[0]
		entry["error_module"]      = first.Module
		entry["error_msg"]         = first.Msg
		entry["error_rc"]          = first.RC
		entry["error_stderr"]      = first.Stderr
		entry["failed_task_count"] = len(result.FailedTasks)
		entry["first_failed_task"] = first.TaskName
	}

	line, err := json.Marshal(entry)
	if err != nil {
		return fmt.Sprintf(`{"host":%q,"status":%q}`, host, result.Status)
	}
	return string(line)
}

func runID(filePath string) string {
	base := filepath.Base(filePath)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

func parseTimeNS(s string) int64 {
	formats := []string{
		time.RFC3339,
		"2006-01-02T15:04:05.999999999",
		"2006-01-02T15:04:05",
	}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t.UnixNano()
		}
	}
	return time.Now().UnixNano()
}
