package loki

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"time"

	"neurader/internal/config"
	"neurader/internal/logs"
)

// lokiPushPayload is the JSON body for POST /loki/api/v1/push
// https://grafana.com/docs/loki/latest/reference/loki-http-api/#push-log-entries-to-loki
type lokiPushPayload struct {
	Streams []lokiStream `json:"streams"`
}

// lokiStream is one labelled log stream containing one or more log lines.
type lokiStream struct {
	Stream map[string]string `json:"stream"` // labels — indexed by Loki
	Values [][]string        `json:"values"` // [[timestamp_ns, log_line], ...]
}

// PushLatest reads the most recently written log and pushes it to Loki.
// Called automatically by `neurader post-run` after each playbook finishes.
func PushLatest(cfg config.Config) error {
	path, err := logs.LatestPath(cfg.LogDir)
	if err != nil || path == "" {
		return err
	}
	return pushFile(cfg, path)
}

// PushAll pushes every log file in logDir to Loki.
// Called by `neurader push`.
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

// pushFile reads one neurader JSON log and sends one Loki stream
// entry per host. Each entry carries the host result as a JSON line
// with labels for fast filtering.
//
// Label design:
//   job      = "neurader"            — always set, used to find all neurader logs
//   playbook = "site.yml"            — playbook name
//   host     = "node3"               — managed host
//   status   = "success"|"failed"|"unreachable"
//
// Log line: full JSON of the host result for detailed inspection in Grafana.
func pushFile(cfg config.Config, filePath string) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("reading %s: %w", filePath, err)
	}

	var run logs.PlaybookRun
	if err := json.Unmarshal(data, &run); err != nil {
		return fmt.Errorf("parsing %s: %w", filePath, err)
	}

	// Use the run's start time as the log timestamp.
	// Fall back to now if parsing fails.
	ts := parseTimeNS(run.StartTime)

	c := newClient(cfg)

	// Build one stream per host so each host is independently queryable.
	var streams []lokiStream

	for hostName, result := range run.Hosts {

		// Build a rich log line with all host details as JSON
		logLine := buildLogLine(run, hostName, result)

		stream := lokiStream{
			Stream: map[string]string{
				"job":      "neurader",
				"playbook": run.Playbook,
				"host":     hostName,
				"status":   result.Status,
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

// buildLogLine serialises the host result into a JSON log line.
// This is what appears in Grafana's log explorer and table panels.
func buildLogLine(run logs.PlaybookRun, host string, result logs.HostResult) string {
	entry := map[string]interface{}{
		"playbook":   run.Playbook,
		"start_time": run.StartTime,
		"end_time":   run.EndTime,
		"host":       host,
		"status":     result.Status,
		"ok":         result.Summary.OK,
		"changed":    result.Summary.Changed,
		"failed":     result.Summary.Failures,
		"unreachable": result.Summary.Unreachable,
		"skipped":    result.Summary.Skipped,
	}

	if result.ErrorOutput != nil {
		entry["error_msg"]    = result.ErrorOutput.Msg
		entry["error_stderr"] = result.ErrorOutput.Stderr
		entry["error_stdout"] = result.ErrorOutput.Stdout
		entry["error_rc"]     = result.ErrorOutput.RC
		entry["error_module"] = result.ErrorOutput.Module
	}

	line, err := json.Marshal(entry)
	if err != nil {
		return fmt.Sprintf(`{"host":%q,"status":%q}`, host, result.Status)
	}
	return string(line)
}

// parseTimeNS converts a time string to Unix nanoseconds for Loki.
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