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
}

// Defaults returns a Config with sensible defaults.
func Defaults() Config {
	return Config{
		RetentionDays: 3,
		LogDir:        LogDir,
	}
}

// Load reads and parses /etc/neurader/neurader.conf.
// Supports INI-style (key = value, # comments) and legacy JSON format.
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
		// legacy JSON format — backward compatible with v0.2.x installs
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

// Save writes cfg to /etc/neurader/neurader.conf in INI format with
// sections and comments so users can easily understand and edit it.
func Save(cfg Config) error {
	if err := os.MkdirAll(ConfigDir, 0755); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}

	var b strings.Builder

	b.WriteString("# ── neurader configuration ────────────────────────────────────────────────\n")
	b.WriteString("# /etc/neurader/neurader.conf\n")
	b.WriteString("# Edit directly or run: sudo neurader grafana-config\n")
	b.WriteString("# Lines starting with # are comments — uncomment a line to activate it.\n")
	b.WriteString("\n")

	b.WriteString("# ── Core ──────────────────────────────────────────────────────────────────\n")
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("retention_days   = %d\n", cfg.RetentionDays))
	b.WriteString(fmt.Sprintf("log_dir          = %s\n", cfg.LogDir))
	b.WriteString(fmt.Sprintf("callback_dir     = %s\n", cfg.CallbackDir))
	b.WriteString(fmt.Sprintf("ansible_cfg_path = %s\n", cfg.AnsibleCfgPath))
	b.WriteString("\n")

	b.WriteString("# ── Loki (optional) ────────────────────────────────────────────────────────\n")
	b.WriteString("# Push playbook results to Loki for Grafana dashboard visibility.\n")
	b.WriteString("# Scenario 1 — bare EC2/VM : http://<grafana-ip>:3100\n")
	b.WriteString("# Scenario 2 — k8s ingress : https://loki.company.com\n")
	b.WriteString("# Scenario 3 — Grafana Cloud: https://logs-prod-xxx.grafana.net\n")
	b.WriteString("# After filling in, run: neurader loki-setup\n")
	b.WriteString("#\n")
	writeOptional(&b, "loki_endpoint", cfg.LokiEndpoint, "http://<grafana-ip>:3100")
	writeOptional(&b, "loki_username", cfg.LokiUsername, "")
	writeOptional(&b, "loki_password", cfg.LokiPassword, "")
	b.WriteString("\n")

	b.WriteString("# ── Grafana (optional) ─────────────────────────────────────────────────────\n")
	b.WriteString("# Required for: neurader loki-setup (auto dashboard import).\n")
	b.WriteString("# grafana_api_key must be a service account token with Admin role.\n")
	b.WriteString("#\n")
	writeOptional(&b, "grafana_endpoint", cfg.GrafanaEndpoint, "http://<grafana-ip>:3000")
	writeOptional(&b, "grafana_api_key",  cfg.GrafanaAPIKey,   "glsa_xxxxxxxxxxxx")
	b.WriteString("\n")

	b.WriteString("# ── SSH auto-install (optional) ────────────────────────────────────────────\n")
	b.WriteString("# neurader can install Loki on your Grafana EC2/VM automatically via SSH.\n")
	b.WriteString("# Leave commented if Loki is already running (k8s, Grafana Cloud, etc).\n")
	b.WriteString("#\n")
	writeOptional(&b, "loki_ssh_host", cfg.LokiSSHHost, "<grafana-ip>")
	writeOptional(&b, "loki_ssh_user", cfg.LokiSSHUser, "ec2-user")
	writeOptional(&b, "loki_ssh_key",  cfg.LokiSSHKey,  "~/.ssh/id_rsa")
	writeOptional(&b, "loki_ssh_port", cfg.LokiSSHPort, "22")
	b.WriteString("\n")

	return os.WriteFile(ConfigFile, []byte(b.String()), 0644)
}

// writeOptional writes an active line if val is set,
// or a commented-out placeholder if val is empty.
func writeOptional(b *strings.Builder, key, val, placeholder string) {
	if val != "" {
		b.WriteString(fmt.Sprintf("%-17s= %s\n", key, val))
	} else if placeholder != "" {
		b.WriteString(fmt.Sprintf("# %-16s= %s\n", key, placeholder))
	} else {
		b.WriteString(fmt.Sprintf("# %-16s=\n", key))
	}
}
