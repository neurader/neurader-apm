package grafana

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"neurader/internal/config"
	"neurader/internal/logs"
)

// annotationPayload is sent to Grafana's POST /api/annotations endpoint.
type annotationPayload struct {
	Time    int64    `json:"time"`    // Unix milliseconds
	TimeEnd int64    `json:"timeEnd"`
	Tags    []string `json:"tags"`
	Text    string   `json:"text"`
}

// PushLatest reads the most recently written log and pushes it to Grafana.
// Called automatically by `neurader post-run` after each playbook finishes.
func PushLatest(cfg config.Config) error {
	path, err := logs.LatestPath(cfg.LogDir)
	if err != nil || path == "" {
		return err
	}
	return pushFile(cfg, path)
}

// PushAll pushes every log file to Grafana.
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
			fmt.Printf("  ⚠ Failed to push %s: %v\n", p, err)
		} else {
			fmt.Printf("  ✓ Pushed %s\n", p)
		}
	}
	return nil
}

// pushFile reads one log file and creates a Grafana annotation for it.
func pushFile(cfg config.Config, filePath string) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("reading %s: %w", filePath, err)
	}

	var run logs.PlaybookRun
	if err := json.Unmarshal(data, &run); err != nil {
		return fmt.Errorf("parsing %s: %w", filePath, err)
	}

	c := newClient(cfg)

	// Count failures
	failCount := 0
	for _, h := range run.Hosts {
		if h.Status != "success" {
			failCount++
		}
	}

	statusStr := "SUCCESS"
	if failCount > 0 {
		statusStr = fmt.Sprintf("FAILED (%d/%d hosts)", failCount, run.TotalHosts)
	}

	text := fmt.Sprintf(
		"<b>%s</b><br/>Status: %s<br/>Total hosts: %d — Failed: %d",
		run.Playbook, statusStr, run.TotalHosts, failCount,
	)

	tags := []string{"neurader", "ansible", run.Playbook}
	if failCount > 0 {
		tags = append(tags, "failed")
	}

	startMs := parseTimeMS(run.StartTime)
	endMs := parseTimeMS(run.EndTime)
	if endMs == 0 {
		endMs = startMs
	}

	payload := annotationPayload{
		Time:    startMs,
		TimeEnd: endMs,
		Tags:    tags,
		Text:    text,
	}

	respBody, status, err := c.post("/api/annotations", payload)
	if err != nil {
		return err
	}
	if status != 200 {
		return fmt.Errorf("Grafana annotations API HTTP %d: %s", status, string(respBody))
	}

	return nil
}

func parseTimeMS(s string) int64 {
	formats := []string{
		time.RFC3339,
		"2006-01-02T15:04:05.999999999",
		"2006-01-02T15:04:05",
	}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t.UnixMilli()
		}
	}
	return time.Now().UnixMilli()
}