package alert

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type teamsPayload struct {
	Type       string        `json:"@type"`
	Context    string        `json:"@context"`
	ThemeColor string        `json:"themeColor"`
	Summary    string        `json:"summary"`
	Sections   []teamsSection `json:"sections"`
}

type teamsSection struct {
	ActivityTitle    string       `json:"activityTitle"`
	ActivitySubtitle string       `json:"activitySubtitle"`
	Facts            []teamsFact  `json:"facts"`
	Markdown         bool         `json:"markdown"`
}

type teamsFact struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

func sendTeams(webhookURL, message string) error {
	// Parse message to build Teams card
	isSuccess := strings.HasPrefix(message, "✅")

	color := "FF0000" // red for failure
	if isSuccess {
		color = "00AA00" // green for success
	}

	title := "❌ NeuRader — Playbook Failed"
	if isSuccess {
		title = "✅ NeuRader — Playbook Succeeded"
	}

	payload := teamsPayload{
		Type:       "MessageCard",
		Context:    "http://schema.org/extensions",
		ThemeColor: color,
		Summary:    title,
		Sections: []teamsSection{
			{
				ActivityTitle:    title,
				ActivitySubtitle: "Ansible Execution Monitor",
				Facts:            []teamsFact{{Name: "Details", Value: message}},
				Markdown:         true,
			},
		},
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("teams: marshal: %w", err)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post(webhookURL, "application/json", bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("teams: post: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("teams: HTTP %d", resp.StatusCode)
	}
	return nil
}
