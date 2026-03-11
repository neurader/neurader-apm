package config

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

const (
	ConfigDir  = "/etc/neurader"
	ConfigFile = "/etc/neurader/neurader.conf"
	LogDir     = "/var/log/neurader"
)

// Config holds all neurader settings persisted to /etc/neurader/neurader.conf
type Config struct {
	RetentionDays  int
	LogDir         string
	CallbackDir    string
	AnsibleCfgPath string

	// Loki integration (optional)
	LokiEndpoint string
	LokiUsername string
	LokiPassword string

	// Grafana integration (optional)
	GrafanaEndpoint string
	GrafanaAPIKey   string

	// SSH config for loki-setup auto-install (optional)
	LokiSSHHost string
	LokiSSHUser string
	LokiSSHKey  string
	LokiSSHPort string

	// Alert settings
	AlertOnFailure     bool
	AlertOnUnreachable bool

	// Slack
	SlackWebhook string

	// PagerDuty
	PagerDutyRoutingKey string

	// Microsoft Teams
	TeamsWebhook string

	// Jira
	JiraURL     string
	JiraUser    string
	JiraToken   string
	JiraProject string

	// Email
	EmailSMTPHost string
	EmailSMTPPort string
	EmailFrom     string
	EmailTo       string
	EmailPassword string

	// Telegram
	TelegramBotToken string
	TelegramChatID   string

	// Generic webhook
	WebhookURL    string
	WebhookMethod string

	// Prometheus Alertmanager
	AlertmanagerURL string
}

// Defaults returns a Config with sensible defaults.
func Defaults() Config {
	return Config{
		RetentionDays:      3,
		LogDir:             LogDir,
		AlertOnFailure:     true,
		AlertOnUnreachable: true,
		WebhookMethod:      "POST",
	}
}

// Load reads and parses /etc/neurader/neurader.conf.
func Load() (Config, error) {
	cfg := Defaults()

	data, err := os.ReadFile(ConfigFile)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, fmt.Errorf("neurader is not initialised — run: sudo neurader init")
		}
		return cfg, fmt.Errorf("reading config: %w", err)
	}

	trimmed := strings.TrimSpace(string(data))
	if strings.HasPrefix(trimmed, "{") {
		if err := loadJSON(trimmed, &cfg); err != nil {
			return cfg, fmt.Errorf("parsing config %s: %w", ConfigFile, err)
		}
	} else {
		if err := loadINI(trimmed, &cfg); err != nil {
			return cfg, fmt.Errorf("parsing config %s: %w", ConfigFile, err)
		}
	}

	if cfg.LogDir == "" {
		cfg.LogDir = LogDir
	}
	if cfg.RetentionDays < 1 {
		cfg.RetentionDays = 3
	}
	if cfg.CallbackDir == "" {
		return cfg, fmt.Errorf("config is missing callback_dir — run: sudo neurader init")
	}
	if cfg.AnsibleCfgPath == "" {
		return cfg, fmt.Errorf("config is missing ansible_cfg_path — run: sudo neurader init")
	}

	return cfg, nil
}

// loadINI parses key = value lines, skipping # comments and blank lines.
func loadINI(content string, cfg *Config) error {
	scanner := bufio.NewScanner(strings.NewReader(content))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])

		switch key {
		case "retention_days":
			if n, err := strconv.Atoi(val); err == nil {
				cfg.RetentionDays = n
			}
		case "log_dir":          cfg.LogDir = val
		case "callback_dir":     cfg.CallbackDir = val
		case "ansible_cfg_path": cfg.AnsibleCfgPath = val
		case "loki_endpoint":    cfg.LokiEndpoint = val
		case "loki_username":    cfg.LokiUsername = val
		case "loki_password":    cfg.LokiPassword = val
		case "grafana_endpoint": cfg.GrafanaEndpoint = val
		case "grafana_api_key":  cfg.GrafanaAPIKey = val
		case "loki_ssh_host":    cfg.LokiSSHHost = val
		case "loki_ssh_user":    cfg.LokiSSHUser = val
		case "loki_ssh_key":     cfg.LokiSSHKey = val
		case "loki_ssh_port":    cfg.LokiSSHPort = val

		// Alert settings
		case "alert_on_failure":
			cfg.AlertOnFailure = val == "true" || val == "1" || val == "yes"
		case "alert_on_unreachable":
			cfg.AlertOnUnreachable = val == "true" || val == "1" || val == "yes"

		// Slack
		case "slack_webhook": cfg.SlackWebhook = val

		// PagerDuty
		case "pagerduty_routing_key": cfg.PagerDutyRoutingKey = val

		// Teams
		case "teams_webhook": cfg.TeamsWebhook = val

		// Jira
		case "jira_url":     cfg.JiraURL = val
		case "jira_user":    cfg.JiraUser = val
		case "jira_token":   cfg.JiraToken = val
		case "jira_project": cfg.JiraProject = val

		// Email
		case "email_smtp_host": cfg.EmailSMTPHost = val
		case "email_smtp_port": cfg.EmailSMTPPort = val
		case "email_from":      cfg.EmailFrom = val
		case "email_to":        cfg.EmailTo = val
		case "email_password":  cfg.EmailPassword = val

		// Telegram
		case "telegram_bot_token": cfg.TelegramBotToken = val
		case "telegram_chat_id":   cfg.TelegramChatID = val

		// Generic webhook
		case "webhook_url":    cfg.WebhookURL = val
		case "webhook_method": cfg.WebhookMethod = val

		// Alertmanager
		case "alertmanager_url": cfg.AlertmanagerURL = val
		}
	}
	return scanner.Err()
}

// loadJSON handles legacy JSON config files (v0.2.x) for backward compatibility.
func loadJSON(content string, cfg *Config) error {
	get := func(key string) string {
		needle := `"` + key + `"`
		idx := strings.Index(content, needle)
		if idx == -1 {
			return ""
		}
		rest := content[idx+len(needle):]
		colon := strings.Index(rest, ":")
		if colon == -1 {
			return ""
		}
		rest = strings.TrimSpace(rest[colon+1:])
		if strings.HasPrefix(rest, `"`) {
			end := strings.Index(rest[1:], `"`)
			if end == -1 {
				return ""
			}
			return rest[1 : end+1]
		}
		end := strings.IndexAny(rest, ",}\n")
		if end == -1 {
			return strings.TrimSpace(rest)
		}
		return strings.TrimSpace(rest[:end])
	}

	if v := get("retention_days"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.RetentionDays = n
		}
	}
	cfg.LogDir          = get("log_dir")
	cfg.CallbackDir     = get("callback_dir")
	cfg.AnsibleCfgPath  = get("ansible_cfg_path")
	cfg.LokiEndpoint    = get("loki_endpoint")
	cfg.LokiUsername    = get("loki_username")
	cfg.LokiPassword    = get("loki_password")
	cfg.GrafanaEndpoint = get("grafana_endpoint")
	cfg.GrafanaAPIKey   = get("grafana_api_key")
	cfg.LokiSSHHost     = get("loki_ssh_host")
	cfg.LokiSSHUser     = get("loki_ssh_user")
	cfg.LokiSSHKey      = get("loki_ssh_key")
	cfg.LokiSSHPort     = get("loki_ssh_port")
	return nil
}

// Save writes cfg to /etc/neurader/neurader.conf in INI format.
func Save(cfg Config) error {
	if err := os.MkdirAll(ConfigDir, 0755); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}

	var b strings.Builder

	b.WriteString("# ── neurader configuration ────────────────────────────────────────────────\n")
	b.WriteString("# /etc/neurader/neurader.conf\n")
	b.WriteString("# Edit directly or run: neurader alert-setup\n")
	b.WriteString("# Lines starting with # are comments.\n\n")

	b.WriteString("# ── Core ──────────────────────────────────────────────────────────────────\n\n")
	b.WriteString(fmt.Sprintf("retention_days   = %d\n", cfg.RetentionDays))
	b.WriteString(fmt.Sprintf("log_dir          = %s\n", cfg.LogDir))
	b.WriteString(fmt.Sprintf("callback_dir     = %s\n", cfg.CallbackDir))
	b.WriteString(fmt.Sprintf("ansible_cfg_path = %s\n", cfg.AnsibleCfgPath))
	b.WriteString("\n")

	b.WriteString("# ── Loki (optional) ────────────────────────────────────────────────────────\n\n")
	writeOptional(&b, "loki_endpoint", cfg.LokiEndpoint, "http://<grafana-ip>:3100")
	writeOptional(&b, "loki_username", cfg.LokiUsername, "")
	writeOptional(&b, "loki_password", cfg.LokiPassword, "")
	b.WriteString("\n")

	b.WriteString("# ── Grafana (optional) ─────────────────────────────────────────────────────\n\n")
	writeOptional(&b, "grafana_endpoint", cfg.GrafanaEndpoint, "http://<grafana-ip>:3000")
	writeOptional(&b, "grafana_api_key",  cfg.GrafanaAPIKey,   "glsa_xxxxxxxxxxxx")
	b.WriteString("\n")

	b.WriteString("# ── SSH auto-install (optional) ────────────────────────────────────────────\n\n")
	writeOptional(&b, "loki_ssh_host", cfg.LokiSSHHost, "<grafana-ip>")
	writeOptional(&b, "loki_ssh_user", cfg.LokiSSHUser, "ec2-user")
	writeOptional(&b, "loki_ssh_key",  cfg.LokiSSHKey,  "~/.ssh/id_rsa")
	writeOptional(&b, "loki_ssh_port", cfg.LokiSSHPort, "22")
	b.WriteString("\n")

	b.WriteString("# ── Alerting ───────────────────────────────────────────────────────────────\n")
	b.WriteString("# Alerts fire automatically after every playbook run.\n")
	b.WriteString("# Run: neurader alert-setup  to configure interactively.\n")
	b.WriteString("# Run: neurader alert-test   to test all configured channels.\n\n")
	writeBool(&b, "alert_on_failure",     cfg.AlertOnFailure)
	writeBool(&b, "alert_on_unreachable", cfg.AlertOnUnreachable)
	b.WriteString("\n")

	b.WriteString("# ── Slack ──────────────────────────────────────────────────────────────────\n\n")
	writeOptional(&b, "slack_webhook", cfg.SlackWebhook, "https://hooks.slack.com/services/xxx")
	b.WriteString("\n")

	b.WriteString("# ── PagerDuty ──────────────────────────────────────────────────────────────\n\n")
	writeOptional(&b, "pagerduty_routing_key", cfg.PagerDutyRoutingKey, "")
	b.WriteString("\n")

	b.WriteString("# ── Microsoft Teams ────────────────────────────────────────────────────────\n\n")
	writeOptional(&b, "teams_webhook", cfg.TeamsWebhook, "https://outlook.office.com/webhook/xxx")
	b.WriteString("\n")

	b.WriteString("# ── Jira ───────────────────────────────────────────────────────────────────\n\n")
	writeOptional(&b, "jira_url",     cfg.JiraURL,     "https://yourorg.atlassian.net")
	writeOptional(&b, "jira_user",    cfg.JiraUser,    "user@yourorg.com")
	writeOptional(&b, "jira_token",   cfg.JiraToken,   "")
	writeOptional(&b, "jira_project", cfg.JiraProject, "OPS")
	b.WriteString("\n")

	b.WriteString("# ── Email ──────────────────────────────────────────────────────────────────\n\n")
	writeOptional(&b, "email_smtp_host", cfg.EmailSMTPHost, "smtp.gmail.com")
	writeOptional(&b, "email_smtp_port", cfg.EmailSMTPPort, "587")
	writeOptional(&b, "email_from",      cfg.EmailFrom,     "neurader@yourorg.com")
	writeOptional(&b, "email_to",        cfg.EmailTo,       "team@yourorg.com")
	writeOptional(&b, "email_password",  cfg.EmailPassword, "")
	b.WriteString("\n")

	b.WriteString("# ── Telegram ───────────────────────────────────────────────────────────────\n\n")
	writeOptional(&b, "telegram_bot_token", cfg.TelegramBotToken, "")
	writeOptional(&b, "telegram_chat_id",   cfg.TelegramChatID,   "")
	b.WriteString("\n")

	b.WriteString("# ── Generic Webhook ────────────────────────────────────────────────────────\n\n")
	writeOptional(&b, "webhook_url",    cfg.WebhookURL,    "https://your-endpoint.com/webhook")
	writeOptional(&b, "webhook_method", cfg.WebhookMethod, "POST")
	b.WriteString("\n")

	b.WriteString("# ── Prometheus Alertmanager ─────────────────────────────────────────────────\n\n")
	writeOptional(&b, "alertmanager_url", cfg.AlertmanagerURL, "http://alertmanager:9093")
	b.WriteString("\n")

	return os.WriteFile(ConfigFile, []byte(b.String()), 0644)
}

func writeOptional(b *strings.Builder, key, val, placeholder string) {
	if val != "" {
		b.WriteString(fmt.Sprintf("%-22s= %s\n", key, val))
	} else if placeholder != "" {
		b.WriteString(fmt.Sprintf("# %-21s= %s\n", key, placeholder))
	} else {
		b.WriteString(fmt.Sprintf("# %-21s=\n", key))
	}
}

func writeBool(b *strings.Builder, key string, val bool) {
	v := "false"
	if val {
		v = "true"
	}
	b.WriteString(fmt.Sprintf("%-22s= %s\n", key, v))
}
