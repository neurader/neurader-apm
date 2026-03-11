package alert

import (
	"fmt"
	"strings"
	"time"

	"neurader/internal/config"
	"neurader/internal/logs"
)

// Event carries all the information about a playbook run for alerting.
type Event struct {
	Playbook    string
	RunID       string
	StartTime   string
	EndTime     string
	TotalHosts  int
	FailedHosts []FailedHost
	Success     bool
}

// FailedHost carries per-host failure details.
type FailedHost struct {
	Name        string
	Status      string
	FailedTasks []logs.FailedTask
}

// Result is the outcome of firing one alert channel.
type Result struct {
	Channel string
	OK      bool
	Err     error
}

// Fire fires all configured alert channels for a given playbook run.
// Called from neurader post-run after every playbook execution.
func Fire(cfg config.Config, run logs.PlaybookRun, runID string) []Result {
	event := buildEvent(cfg, run, runID)
	var results []Result

	// Success — only Slack/Teams/Telegram get a success message
	if event.Success {
		if cfg.SlackWebhook != "" {
			err := sendSlack(cfg.SlackWebhook, successMessage(event))
			results = append(results, Result{"Slack", err == nil, err})
		}
		if cfg.TeamsWebhook != "" {
			err := sendTeams(cfg.TeamsWebhook, successMessage(event))
			results = append(results, Result{"Teams", err == nil, err})
		}
		if cfg.TelegramBotToken != "" && cfg.TelegramChatID != "" {
			err := sendTelegram(cfg.TelegramBotToken, cfg.TelegramChatID, successMessage(event))
			results = append(results, Result{"Telegram", err == nil, err})
		}
		return results
	}

	// Failure — fire all configured channels simultaneously
	if cfg.SlackWebhook != "" {
		err := sendSlack(cfg.SlackWebhook, failureSlackPayload(event))
		results = append(results, Result{"Slack", err == nil, err})
	}
	if cfg.PagerDutyRoutingKey != "" {
		err := sendPagerDuty(cfg.PagerDutyRoutingKey, event)
		results = append(results, Result{"PagerDuty", err == nil, err})
	}
	if cfg.TeamsWebhook != "" {
		err := sendTeams(cfg.TeamsWebhook, failureMessage(event))
		results = append(results, Result{"Teams", err == nil, err})
	}
	if cfg.JiraURL != "" && cfg.JiraToken != "" {
		err := sendJira(cfg, event)
		results = append(results, Result{"Jira", err == nil, err})
	}
	if cfg.EmailSMTPHost != "" && cfg.EmailTo != "" {
		err := sendEmail(cfg, event)
		results = append(results, Result{"Email", err == nil, err})
	}
	if cfg.TelegramBotToken != "" && cfg.TelegramChatID != "" {
		err := sendTelegram(cfg.TelegramBotToken, cfg.TelegramChatID, failureMessage(event))
		results = append(results, Result{"Telegram", err == nil, err})
	}
	if cfg.WebhookURL != "" {
		err := sendWebhook(cfg.WebhookURL, cfg.WebhookMethod, event)
		results = append(results, Result{"Webhook", err == nil, err})
	}
	if cfg.AlertmanagerURL != "" {
		err := sendAlertmanager(cfg.AlertmanagerURL, event)
		results = append(results, Result{"Alertmanager", err == nil, err})
	}

	return results
}

// Test sends a test message to all configured channels.
func Test(cfg config.Config) []Result {
	event := Event{
		Playbook:   "test-playbook.yml",
		RunID:      "test_" + time.Now().Format("2006-01-02_15-04-05"),
		StartTime:  time.Now().Format(time.RFC3339),
		EndTime:    time.Now().Format(time.RFC3339),
		TotalHosts: 3,
		Success:    false,
		FailedHosts: []FailedHost{
			{
				Name:   "test-node1",
				Status: "failed",
				FailedTasks: []logs.FailedTask{
					{
						TaskName: "Install test package",
						Module:   "yum",
						Msg:      "This is a test alert from NeuRader",
						RC:       1,
					},
				},
			},
		},
	}
	return Fire(cfg, logs.PlaybookRun{
		Playbook:   event.Playbook,
		StartTime:  event.StartTime,
		EndTime:    event.EndTime,
		TotalHosts: event.TotalHosts,
		Hosts: map[string]logs.HostResult{
			"test-node1": {
				Status: "failed",
				FailedTasks: []logs.FailedTask{
					{TaskName: "Install test package", Module: "yum", Msg: "This is a test alert from NeuRader", RC: 1},
				},
			},
		},
	}, event.RunID)
}

// ── Event builder ─────────────────────────────────────────────────────────────

func buildEvent(cfg config.Config, run logs.PlaybookRun, runID string) Event {
	event := Event{
		Playbook:   run.Playbook,
		RunID:      runID,
		StartTime:  run.StartTime,
		EndTime:    run.EndTime,
		TotalHosts: run.TotalHosts,
		Success:    true,
	}

	for name, result := range run.Hosts {
		isFailed      := result.Status == "failed" && cfg.AlertOnFailure
		isUnreachable := result.Status == "unreachable" && cfg.AlertOnUnreachable
		if isFailed || isUnreachable {
			event.Success = false
			event.FailedHosts = append(event.FailedHosts, FailedHost{
				Name:        name,
				Status:      result.Status,
				FailedTasks: result.FailedTasks,
			})
		}
	}

	return event
}

// ── Message formatters ────────────────────────────────────────────────────────

func successMessage(e Event) string {
	return fmt.Sprintf("✅ *NeuRader* — Playbook succeeded\n*Playbook:* %s\n*Hosts:* %d\n*Time:* %s",
		e.Playbook, e.TotalHosts, e.StartTime)
}

func failureMessage(e Event) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("❌ *NeuRader* — Playbook failed\n"))
	sb.WriteString(fmt.Sprintf("*Playbook:* %s\n", e.Playbook))
	sb.WriteString(fmt.Sprintf("*Time:* %s\n", e.StartTime))
	sb.WriteString(fmt.Sprintf("*Failed Hosts:* %d of %d\n\n", len(e.FailedHosts), e.TotalHosts))
	for _, h := range e.FailedHosts {
		sb.WriteString(fmt.Sprintf("• *%s* [%s]\n", h.Name, strings.ToUpper(h.Status)))
		for _, t := range h.FailedTasks {
			sb.WriteString(fmt.Sprintf("  Task: %s\n", t.TaskName))
			sb.WriteString(fmt.Sprintf("  Module: %s  RC: %d\n", t.Module, t.RC))
			if t.Msg != "" {
				sb.WriteString(fmt.Sprintf("  Error: %s\n", t.Msg))
			}
		}
	}
	return sb.String()
}

// failureSlackPayload returns a rich Slack Block Kit payload for failures.
func failureSlackPayload(e Event) string {
	return failureMessage(e) // slack.go builds the full Block Kit payload
}

// FailedHostsSummary returns a short comma-separated list of failed host names.
func FailedHostsSummary(hosts []FailedHost) string {
	names := make([]string, len(hosts))
	for i, h := range hosts {
		names[i] = h.Name
	}
	return strings.Join(names, ", ")
}
