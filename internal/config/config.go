package config

import (
	"encoding/json"
	"fmt"
	"os"
)

const (
	ConfigDir  = "/etc/neurader"
	ConfigFile = "/etc/neurader/neurader.conf"
	LogDir     = "/var/log/neurader"
)

// Config holds all neurader settings persisted to /etc/neurader/neurader.conf
type Config struct {
	RetentionDays  int    `json:"retention_days"`   // how many days to keep log files
	LogDir         string `json:"log_dir"`           // default /var/log/neurader
	CallbackDir    string `json:"callback_dir"`      // detected ansible callback plugin dir
	AnsibleCfgPath string `json:"ansible_cfg_path"`  // path to ansible.cfg that was patched

	// Loki integration (optional)
	// Scenario 1: bare EC2/VM  → http://<loki-server-ip>:3100
	// Scenario 2: k8s ingress  → https://loki.company.com
	// Scenario 3: Grafana Cloud → https://logs-prod-xxx.grafana.net
	LokiEndpoint string `json:"loki_endpoint"` // Loki push endpoint base URL
	LokiUsername string `json:"loki_username"` // basic auth username (Grafana Cloud / k8s)
	LokiPassword string `json:"loki_password"` // basic auth password or API key

	// Grafana integration (for dashboard import via loki-setup)
	GrafanaEndpoint string `json:"grafana_endpoint"` // e.g. http://grafana-ip:3000
	GrafanaAPIKey   string `json:"grafana_api_key"`  // service account token (Admin role)

	// SSH config for auto-installing Loki on EC2/VM via loki-setup
	// Leave blank if Loki is already running (k8s, Grafana Cloud, etc.)
	LokiSSHHost string `json:"loki_ssh_host"` // IP or hostname of Grafana/Loki node
	LokiSSHUser string `json:"loki_ssh_user"` // SSH user, default: ec2-user
	LokiSSHKey  string `json:"loki_ssh_key"`  // path to private key, default: ~/.ssh/id_rsa
	LokiSSHPort string `json:"loki_ssh_port"` // SSH port, default: 22
}

// Defaults returns a Config with sensible defaults.
func Defaults() Config {
	return Config{
		RetentionDays: 3,
		LogDir:        LogDir,
	}
}

// Load reads /etc/neurader/neurader.conf.
func Load() (Config, error) {
	cfg := Defaults()

	data, err := os.ReadFile(ConfigFile)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, fmt.Errorf("neurader is not initialised — run: sudo neurader init")
		}
		return cfg, fmt.Errorf("reading config: %w", err)
	}

	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parsing config %s: %w", ConfigFile, err)
	}

	// Validate critical fields
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

// Save writes cfg to /etc/neurader/neurader.conf
func Save(cfg Config) error {
	if err := os.MkdirAll(ConfigDir, 0755); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(ConfigFile, data, 0644)
}
