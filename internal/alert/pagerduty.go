package alert

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type pdPayload struct {
	RoutingKey  string    `json:"routing_key"`
	EventAction string    `json:"event_action"`
	DedupKey    string    `json:"dedup_key"`
	Payload     pdDetails `json:"payload"`
}

type pdDetails struct {
	Summary   string            `json:"summary"`
	Severity  string            `json:"severity"`
	Source    string            `json:"source"`
	Timestamp string            `json:"timestamp"`
	CustomDetails map[string]interface{} `json:"custom_details"`
}

func sendPagerDuty(routingKey string, e Event) error {
	// Build summary
	summary := fmt.Sprintf("[NeuRader] %s failed — %s",
		e.Playbook, FailedHostsSummary(e.FailedHosts))

	// Severity based on number of failed hosts
	severity := "warning"
	if len(e.FailedHosts) > 2 {
		severity = "critical"
	}

	// Custom details with full failure info
	details := map[string]interface{}{
		"playbook":     e.Playbook,
		"run_id":       e.RunID,
		"start_time":   e.StartTime,
		"total_hosts":  e.TotalHosts,
		"failed_hosts": len(e.FailedHosts),
	}

	// Add failed host details
	for i, h := range e.FailedHosts {
		key := fmt.Sprintf("host_%d", i+1)
		hostDetail := map[string]interface{}{
			"name":   h.Name,
			"status": h.Status,
		}
		if len(h.FailedTasks) > 0 {
			t := h.FailedTasks[0]
			hostDetail["task"]   = t.TaskName
			hostDetail["module"] = t.Module
			hostDetail["error"]  = t.Msg
			hostDetail["rc"]     = t.RC
		}
		details[key] = hostDetail
	}

	payload := pdPayload{
		RoutingKey:  routingKey,
		EventAction: "trigger",
		DedupKey:    e.RunID, // one incident per run
		Payload: pdDetails{
			Summary:       summary,
			Severity:      severity,
			Source:        "neurader",
			Timestamp:     e.StartTime,
			CustomDetails: details,
		},
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("pagerduty: marshal: %w", err)
	}

	// Build failed hosts summary for description
	var hostLines []string
	for _, h := range e.FailedHosts {
		line := fmt.Sprintf("• %s [%s]", h.Name, strings.ToUpper(h.Status))
		if len(h.FailedTasks) > 0 {
			t := h.FailedTasks[0]
			line += fmt.Sprintf(" — %s: %s", t.Module, t.Msg)
		}
		hostLines = append(hostLines, line)
	}

	_ = hostLines // used in custom_details above

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post(
		"https://events.pagerduty.com/v2/enqueue",
		"application/json",
		bytes.NewReader(data),
	)
	if err != nil {
		return fmt.Errorf("pagerduty: post: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("pagerduty: HTTP %d", resp.StatusCode)
	}
	return nil
}
