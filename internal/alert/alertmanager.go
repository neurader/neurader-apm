package alert

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type amAlert struct {
	Labels      map[string]string `json:"labels"`
	Annotations map[string]string `json:"annotations"`
	StartsAt    string            `json:"startsAt"`
	EndsAt      string            `json:"endsAt,omitempty"`
	GeneratorURL string           `json:"generatorURL"`
}

func sendAlertmanager(amURL string, e Event) error {
	var alerts []amAlert

	for _, h := range e.FailedHosts {
		firstTask := ""
		firstModule := ""
		firstMsg := ""
		if len(h.FailedTasks) > 0 {
			firstTask   = h.FailedTasks[0].TaskName
			firstModule = h.FailedTasks[0].Module
			firstMsg    = h.FailedTasks[0].Msg
		}

		a := amAlert{
			Labels: map[string]string{
				"alertname": "AnsiblePlaybookFailure",
				"severity":  "critical",
				"source":    "neurader",
				"playbook":  e.Playbook,
				"host":      h.Name,
				"status":    h.Status,
				"run_id":    e.RunID,
			},
			Annotations: map[string]string{
				"summary": fmt.Sprintf("%s failed on %s", e.Playbook, h.Name),
				"description": fmt.Sprintf(
					"Playbook: %s\nHost: %s [%s]\nTask: %s\nModule: %s\nError: %s",
					e.Playbook, h.Name, strings.ToUpper(h.Status),
					firstTask, firstModule, firstMsg,
				),
				"failed_task_count": fmt.Sprintf("%d", len(h.FailedTasks)),
				"run_id":            e.RunID,
				"start_time":        e.StartTime,
			},
			StartsAt:     e.StartTime,
			GeneratorURL: "https://neurader.cloud",
		}
		alerts = append(alerts, a)
	}

	data, err := json.Marshal(alerts)
	if err != nil {
		return fmt.Errorf("alertmanager: marshal: %w", err)
	}

	url := strings.TrimRight(amURL, "/") + "/api/v2/alerts"

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("alertmanager: post: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("alertmanager: HTTP %d", resp.StatusCode)
	}
	return nil
}
