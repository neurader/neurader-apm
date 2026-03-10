package logs

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

// PlaybookRun is the top-level structure written to each JSON log file.
type PlaybookRun struct {
	Playbook   string                `json:"playbook"`
	StartTime  string                `json:"start_time"`
	EndTime    string                `json:"end_time"`
	TotalHosts int                   `json:"total_hosts"`
	Hosts      map[string]HostResult `json:"hosts"`
}

// HostResult captures the outcome for a single managed node.
type HostResult struct {
	Status      string       `json:"status"`       // "success" | "failed" | "unreachable"
	FailedTasks []FailedTask `json:"failed_tasks"` // empty when status == success
	Summary     HostSummary  `json:"summary"`
}

// FailedTask captures full details of a single failed Ansible task.
type FailedTask struct {
	TaskName  string                 `json:"task_name"`
	TaskPath  string                 `json:"task_path"`
	Module    string                 `json:"module"`
	Msg       string                 `json:"msg"`
	Stdout    string                 `json:"stdout"`
	Stderr    string                 `json:"stderr"`
	RC        int                    `json:"rc"`
	Exception string                 `json:"exception"`
	TaskArgs  map[string]interface{} `json:"task_args"`
}

// HostSummary mirrors Ansible's stats.summarize() output.
type HostSummary struct {
	OK          int `json:"ok"`
	Failures    int `json:"failures"`
	Unreachable int `json:"unreachable"`
	Changed     int `json:"changed"`
	Skipped     int `json:"skipped"`
}

var safeFilename = regexp.MustCompile(`[^\w\-.]`)

// Write persists a PlaybookRun as a JSON file in logDir.
func Write(logDir string, run PlaybookRun) (string, error) {
	if err := os.MkdirAll(logDir, 0777); err != nil {
		return "", fmt.Errorf("creating log dir: %w", err)
	}
	os.Chmod(logDir, 0777) //nolint:errcheck

	safe      := safeFilename.ReplaceAllString(run.Playbook, "_")
	timestamp := time.Now().UTC().Format("2006-01-02_15-04-05")
	filename  := fmt.Sprintf("%s_%s.json", safe, timestamp)
	fullPath  := filepath.Join(logDir, filename)

	data, err := json.MarshalIndent(run, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshalling run: %w", err)
	}
	if err := os.WriteFile(fullPath, data, 0644); err != nil {
		return "", fmt.Errorf("writing log file: %w", err)
	}
	return fullPath, nil
}
