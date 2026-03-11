package alert

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type webhookPayload struct {
	Source      string        `json:"source"`
	Playbook    string        `json:"playbook"`
	RunID       string        `json:"run_id"`
	StartTime   string        `json:"start_time"`
	EndTime     string        `json:"end_time"`
	TotalHosts  int           `json:"total_hosts"`
	Success     bool          `json:"success"`
	FailedCount int           `json:"failed_count"`
	FailedHosts []webhookHost `json:"failed_hosts,omitempty"`
}

type webhookHost struct {
	Name        string         `json:"name"`
	Status      string         `json:"status"`
	FailedTasks []webhookTask  `json:"failed_tasks,omitempty"`
}

type webhookTask struct {
	TaskName string `json:"task_name"`
	Module   string `json:"module"`
	Msg      string `json:"msg"`
	RC       int    `json:"rc"`
	Stderr   string `json:"stderr,omitempty"`
}

func sendWebhook(url, method string, e Event) error {
	if method == "" {
		method = "POST"
	}

	payload := webhookPayload{
		Source:      "neurader",
		Playbook:    e.Playbook,
		RunID:       e.RunID,
		StartTime:   e.StartTime,
		EndTime:     e.EndTime,
		TotalHosts:  e.TotalHosts,
		Success:     e.Success,
		FailedCount: len(e.FailedHosts),
	}

	for _, h := range e.FailedHosts {
		wh := webhookHost{
			Name:   h.Name,
			Status: h.Status,
		}
		for _, t := range h.FailedTasks {
			wh.FailedTasks = append(wh.FailedTasks, webhookTask{
				TaskName: t.TaskName,
				Module:   t.Module,
				Msg:      t.Msg,
				RC:       t.RC,
				Stderr:   t.Stderr,
			})
		}
		payload.FailedHosts = append(payload.FailedHosts, wh)
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("webhook: marshal: %w", err)
	}

	req, err := http.NewRequest(method, url, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("webhook: request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "NeuRader/1.0")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("webhook: %s: %w", method, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("webhook: HTTP %d", resp.StatusCode)
	}
	return nil
}
