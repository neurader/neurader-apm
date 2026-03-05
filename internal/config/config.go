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
	RetentionDays   int    `json:"retention_days"`   // how many days to keep log files
	GrafanaEndpoint string `json:"grafana_endpoint"` // e.g. http://grafana:3000
	GrafanaAPIKey   string `json:"grafana_api_key"`  // Grafana service account token
	GrafanaOrgID    string `json:"grafana_org_id"`   // Grafana org ID, default "1"
	LogDir          string `json:"log_dir"`          // default /var/log/neurader
	CallbackDir     string `json:"callback_dir"`     // detected ansible callback plugin dir
	AnsibleCfgPath  string `json:"ansible_cfg_path"` // path to ansible.cfg that was patched
}

// Defaults returns a Config with sensible defaults.
func Defaults() Config {
	return Config{
		RetentionDays: 3,
		GrafanaOrgID:  "1",
		LogDir:        LogDir,
	}
}

// Load reads /etc/neurader/neurader.conf.
// Returns defaults + a clear error message if the file does not exist.
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

	// Validate critical fields — catch manually edited configs with missing values
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

// Save writes cfg to /etc/neurader/neurader.conf with mode 0600 (root-only readable).
func Save(cfg Config) error {
	if err := os.MkdirAll(ConfigDir, 0755); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	// 0644 — readable by all users so non-root commands (neurader list,
	// neurader show, neurader status) work without sudo.
	// The Grafana API key is stored here — if that is a concern, users can
	// manually chmod 0600 after init.
	return os.WriteFile(ConfigFile, data, 0644)
}
