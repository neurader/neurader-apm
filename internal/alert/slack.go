package alert

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type slackPayload struct {
	Username string       `json:"username"`
	IconURL  string       `json:"icon_url"`
	Blocks []slackBlock `json:"blocks"`
}

type slackBlock struct {
	Type     string      `json:"type"`
	Text     *slackText  `json:"text,omitempty"`
	Fields   []slackText `json:"fields,omitempty"`
	Elements []slackText `json:"elements,omitempty"`
}

type slackText struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func sendSlack(webhookURL, message string) error {
	payload := buildSlackBlocks(message)
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("slack: marshal: %w", err)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post(webhookURL, "application/json", bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("slack: post: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("slack: HTTP %d", resp.StatusCode)
	}
	return nil
}

func buildSlackBlocks(message string) slackPayload {
	lines := strings.Split(message, "\n")
	var blocks []slackBlock

	// Header block
	if len(lines) > 0 {
		blocks = append(blocks, slackBlock{
			Type: "section",
			Text: &slackText{Type: "mrkdwn", Text: lines[0]},
		})
	}

	// Divider
	blocks = append(blocks, slackBlock{Type: "divider"})

	// Body
	body := strings.Join(lines[1:], "\n")
	if body != "" {
		blocks = append(blocks, slackBlock{
			Type: "section",
			Text: &slackText{Type: "mrkdwn", Text: body},
		})
	}

	// Footer — context block uses "elements" not "fields"
	blocks = append(blocks, slackBlock{
		Type: "context",
		Elements: []slackText{
			{Type: "mrkdwn", Text: "Sent by *NeuRader* — Ansible Execution Monitor"},
		},
	})

	return slackPayload{
		Username: "NeuRader",
		IconURL:  "https://neurader.cloud/neurader/neurader.png",
		Blocks:   blocks,
	}
}
