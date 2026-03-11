package alert

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"neurader/internal/config"
)

type jiraIssue struct {
	Fields jiraFields `json:"fields"`
}

type jiraFields struct {
	Project     jiraProject  `json:"project"`
	Summary     string       `json:"summary"`
	Description jiraDesc     `json:"description"`
	IssueType   jiraType     `json:"issuetype"`
	Priority    jiraPriority `json:"priority"`
	Labels      []string     `json:"labels"`
}

type jiraProject  struct { Key  string `json:"key"`  }
type jiraType     struct { Name string `json:"name"` }
type jiraPriority struct { Name string `json:"name"` }

// jiraDesc uses Atlassian Document Format (ADF) for Jira Cloud
type jiraDesc struct {
	Version int         `json:"version"`
	Type    string      `json:"type"`
	Content []jiraNode  `json:"content"`
}

type jiraNode struct {
	Type    string      `json:"type"`
	Content []jiraNode  `json:"content,omitempty"`
	Text    string      `json:"text,omitempty"`
	Attrs   interface{} `json:"attrs,omitempty"`
}

func sendJira(cfg config.Config, e Event) error {
	summary := fmt.Sprintf("[NeuRader] %s — %d host(s) failed: %s",
		e.Playbook, len(e.FailedHosts), FailedHostsSummary(e.FailedHosts))

	// Priority based on failed host count
	priority := "High"
	if len(e.FailedHosts) > 3 {
		priority = "Highest"
	}

	issue := jiraIssue{
		Fields: jiraFields{
			Project:     jiraProject{Key: cfg.JiraProject},
			Summary:     summary,
			IssueType:   jiraType{Name: "Bug"},
			Priority:    jiraPriority{Name: priority},
			Labels:      []string{"neurader", "ansible", "infrastructure"},
			Description: buildJiraDescription(e),
		},
	}

	data, err := json.Marshal(issue)
	if err != nil {
		return fmt.Errorf("jira: marshal: %w", err)
	}

	url := strings.TrimRight(cfg.JiraURL, "/") + "/rest/api/3/issue"

	req, err := http.NewRequest("POST", url, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("jira: request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(cfg.JiraUser, cfg.JiraToken)

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("jira: post: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("jira: HTTP %d", resp.StatusCode)
	}
	return nil
}

// buildJiraDescription builds an ADF document with full failure details.
func buildJiraDescription(e Event) jiraDesc {
	var content []jiraNode

	// Header
	content = append(content, textNode(fmt.Sprintf(
		"NeuRader detected failures in playbook: %s", e.Playbook)))
	content = append(content, textNode(fmt.Sprintf("Run ID: %s", e.RunID)))
	content = append(content, textNode(fmt.Sprintf("Time: %s", e.StartTime)))
	content = append(content, textNode(fmt.Sprintf(
		"Failed Hosts: %d of %d", len(e.FailedHosts), e.TotalHosts)))
	content = append(content, textNode(""))

	// Per-host details
	for _, h := range e.FailedHosts {
		content = append(content, textNode(fmt.Sprintf(
			"Host: %s [%s]", h.Name, strings.ToUpper(h.Status))))
		for i, t := range h.FailedTasks {
			content = append(content, textNode(fmt.Sprintf(
				"  Failed Task %d: %s", i+1, t.TaskName)))
			content = append(content, textNode(fmt.Sprintf(
				"  Module: %s  RC: %d", t.Module, t.RC)))
			if t.Msg != "" {
				content = append(content, textNode(fmt.Sprintf(
					"  Error: %s", t.Msg)))
			}
			if t.Stderr != "" {
				content = append(content, textNode(fmt.Sprintf(
					"  Stderr: %s", t.Stderr)))
			}
		}
		content = append(content, textNode(""))
	}

	content = append(content, textNode("—"))
	content = append(content, textNode("Created automatically by NeuRader — Ansible Execution Monitor"))
	content = append(content, textNode("Run `neurader show "+e.Playbook+"` for full details."))

	return jiraDesc{
		Version: 1,
		Type:    "doc",
		Content: content,
	}
}

func textNode(text string) jiraNode {
	return jiraNode{
		Type: "paragraph",
		Content: []jiraNode{
			{Type: "text", Text: text},
		},
	}
}
