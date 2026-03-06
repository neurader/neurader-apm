package grafana

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"neurader/assets"
	"neurader/internal/config"
	"neurader/internal/loki"
)

// SetupResult holds what happened during loki-setup
type SetupResult struct {
	LokiInstalled     bool
	LokiReachable     bool
	DashboardImported bool
}

// Setup is the full loki-setup flow:
//
//	Scenario A — EC2 / Linux VM:
//	  1. SSH into Grafana node
//	  2. Detect OS → run correct install commands
//	  3. Start Loki via systemd
//	  4. Ping Loki endpoint
//	  5. Import dashboard into Grafana
//
//	Scenario B — Kubernetes (Loki already running):
//	  1. Ping Loki endpoint (skip SSH install)
//	  2. Import dashboard into Grafana
func Setup(cfg config.Config) error {
	fmt.Println()
	fmt.Println("  ┌─────────────────────────────────────┐")
	fmt.Println("  │     neurader  loki-setup             │")
	fmt.Println("  └─────────────────────────────────────┘")
	fmt.Println()

	if cfg.LokiEndpoint == "" {
		return fmt.Errorf("loki_endpoint not set — run: sudo neurader init")
	}
	if cfg.GrafanaEndpoint == "" {
		return fmt.Errorf("grafana_endpoint not set — run: sudo neurader init")
	}

	// ── Step 1: Try reaching Loki ─────────────────────────────────────────
	printStep(1, "Checking Loki reachability")
	lokiReachable := loki.Ping(cfg) == nil

	if !lokiReachable {
		// Loki not reachable — try SSH install if node info is configured
		if cfg.LokiSSHHost == "" {
			printWarn("Loki is not reachable at %s", cfg.LokiEndpoint)
			printWarn("To auto-install Loki, set loki_ssh_host in config")
			printWarn("Or run the standalone script on your Grafana node:")
			fmt.Println()
			fmt.Printf("    curl -L https://neurader.operman.in/neurader/releases/loki-node-setup.sh | sudo bash\n")
			fmt.Println()
			return fmt.Errorf("loki not reachable and no ssh host configured")
		}

		// ── Step 2: SSH install Loki ──────────────────────────────────────
		printStep(2, "Installing Loki via SSH → %s", cfg.LokiSSHHost)
		if err := sshInstallLoki(cfg); err != nil {
			printWarn("SSH install failed: %v", err)
			printWarn("Try running the standalone script manually on your Grafana node:")
			fmt.Println()
			fmt.Printf("    curl -L https://neurader.operman.in/neurader/releases/loki-node-setup.sh | sudo bash\n")
			fmt.Println()
			return fmt.Errorf("loki install failed: %w", err)
		}
		printOK("Loki installed and started")

		// Wait for Loki to come up
		printStep(3, "Waiting for Loki to be ready")
		if err := waitForLoki(cfg, 30); err != nil {
			return fmt.Errorf("loki did not become ready in time: %w", err)
		}
		printOK("Loki is ready at %s", cfg.LokiEndpoint)

	} else {
		printOK("Loki reachable at %s", cfg.LokiEndpoint)
	}

	// ── Step 4: Import dashboard into Grafana ─────────────────────────────
	printStep(4, "Importing Neurader dashboard into Grafana")
	if err := importDashboard(cfg); err != nil {
		printWarn("Dashboard import failed: %v", err)
		printWarn("Import manually: Dashboards → Import → upload assets/loki/dashboard.json")
	} else {
		printOK("Dashboard imported into Grafana")
	}

	// ── Done ──────────────────────────────────────────────────────────────
	fmt.Println()
	fmt.Println("  ✅  Loki setup complete!")
	fmt.Println()
	fmt.Println("  Next steps:")
	fmt.Printf("  • Run a playbook — logs push automatically\n")
	fmt.Printf("  • Backfill existing logs: neurader push\n")
	fmt.Printf("  • Open Grafana: %s\n", cfg.GrafanaEndpoint)
	fmt.Println()
	return nil
}

// ── SSH Install ───────────────────────────────────────────────────────────────

// sshInstallLoki SSHes into the Grafana/Loki node and runs the install script.
// It detects the remote OS and runs the appropriate package manager commands.
func sshInstallLoki(cfg config.Config) error {
	sshArgs := buildSSHArgs(cfg)

	// Detect remote OS first
	osID, err := sshRun(cfg, sshArgs, "cat /etc/os-release | grep '^ID=' | cut -d= -f2 | tr -d '\"'")
	if err != nil {
		return fmt.Errorf("could not detect remote OS: %w", err)
	}
	osID = strings.TrimSpace(osID)
	fmt.Printf("  → Remote OS: %s\n", osID)

	// Build install script based on OS
	script := buildInstallScript(osID)

	// Run the install script over SSH
	_, err = sshRun(cfg, sshArgs, script)
	return err
}

// buildInstallScript returns a shell script that installs Loki for the given OS.
func buildInstallScript(osID string) string {
	lokiVersion := "3.0.0"

	base := fmt.Sprintf(`
set -euo pipefail

LOKI_VERSION="%s"
ARCH=$(uname -m)
case "$ARCH" in
  x86_64)  LOKI_ARCH="amd64" ;;
  aarch64) LOKI_ARCH="arm64" ;;
  armv7l)  LOKI_ARCH="arm"   ;;
  *)       echo "Unsupported arch: $ARCH"; exit 1 ;;
esac

# Download Loki if not already installed
if ! command -v loki &>/dev/null; then
  echo "→ Downloading Loki ${LOKI_VERSION} (${LOKI_ARCH})"
  curl -LO "https://github.com/grafana/loki/releases/download/v${LOKI_VERSION}/loki-linux-${LOKI_ARCH}.zip"
  unzip -o "loki-linux-${LOKI_ARCH}.zip"
  mv "loki-linux-${LOKI_ARCH}" /usr/bin/loki
  chmod +x /usr/bin/loki
  rm -f "loki-linux-${LOKI_ARCH}.zip"
  echo "✓ Loki installed"
else
  echo "✓ Loki already installed: $(loki --version 2>&1 | head -1)"
fi

# Write config
mkdir -p /etc/loki /var/loki/chunks /var/loki/rules
cat > /etc/loki/loki.yaml << 'LOKICFG'
auth_enabled: false

server:
  http_listen_port: 3100

common:
  path_prefix: /var/loki
  storage:
    filesystem:
      chunks_directory: /var/loki/chunks
      rules_directory:  /var/loki/rules
  replication_factor: 1
  ring:
    instance_addr: 127.0.0.1
    kvstore:
      store: inmemory

schema_config:
  configs:
    - from: 2024-01-01
      store: tsdb
      object_store: filesystem
      schema: v13
      index:
        prefix: index_
        period: 24h

limits_config:
  reject_old_samples: false
LOKICFG

# Write systemd service
cat > /etc/systemd/system/loki.service << 'SYSTEMD'
[Unit]
Description=Loki log aggregation
After=network.target

[Service]
ExecStart=/usr/bin/loki -config.file=/etc/loki/loki.yaml
Restart=on-failure
User=root

[Install]
WantedBy=multi-user.target
SYSTEMD

systemctl daemon-reload
systemctl enable loki
systemctl restart loki
echo "✓ Loki service started"
`, lokiVersion)

	return base
}

// buildSSHArgs returns the base ssh arguments for the configured host.
func buildSSHArgs(cfg config.Config) []string {
	args := []string{
		"-o", "StrictHostKeyChecking=no",
		"-o", "ConnectTimeout=15",
	}
	if cfg.LokiSSHKey != "" {
		args = append(args, "-i", cfg.LokiSSHKey)
	}
	if cfg.LokiSSHPort != "" && cfg.LokiSSHPort != "22" {
		args = append(args, "-p", cfg.LokiSSHPort)
	}

	user := cfg.LokiSSHUser
	if user == "" {
		user = "ec2-user"
	}
	args = append(args, fmt.Sprintf("%s@%s", user, cfg.LokiSSHHost))
	return args
}

// sshRun runs a command on the remote host and returns stdout.
func sshRun(cfg config.Config, sshArgs []string, command string) (string, error) {
	args := append(sshArgs, command)
	cmd := exec.Command("ssh", args...)
	cmd.Stdin = os.Stdin

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("ssh command failed: %w\nstderr: %s", err, stderr.String())
	}
	return stdout.String(), nil
}

// waitForLoki polls Loki /ready until it responds or timeout is reached.
func waitForLoki(cfg config.Config, timeoutSecs int) error {
	endpoint := strings.TrimRight(cfg.LokiEndpoint, "/")
	url := endpoint + "/ready"
	hc := &http.Client{Timeout: 3 * time.Second}

	for i := 0; i < timeoutSecs; i++ {
		resp, err := hc.Get(url)
		if err == nil && resp.StatusCode == 200 {
			resp.Body.Close()
			return nil
		}
		if resp != nil {
			resp.Body.Close()
		}
		fmt.Printf("  → waiting... (%ds)\n", i+1)
		time.Sleep(1 * time.Second)
	}
	return fmt.Errorf("timed out after %ds", timeoutSecs)
}

// ── Dashboard Import ──────────────────────────────────────────────────────────

// importDashboard loads the embedded dashboard JSON and imports it into Grafana.
// It first ensures the Loki datasource exists, then imports the dashboard.
func importDashboard(cfg config.Config) error {
	// Load embedded dashboard JSON
	dashJSON, err := assets.LokiDashboard()
	if err != nil {
		return fmt.Errorf("loading embedded dashboard: %w", err)
	}

	var dash map[string]interface{}
	if err := json.Unmarshal(dashJSON, &dash); err != nil {
		return fmt.Errorf("parsing dashboard JSON: %w", err)
	}

	// Ensure Loki datasource exists in Grafana and get its UID
	datasourceUID, err := ensureLokiDatasource(cfg)
	if err != nil {
		return fmt.Errorf("creating Loki datasource: %w", err)
	}
	fmt.Printf("  → Loki datasource UID: %s\n", datasourceUID)

	// Patch dashboard — replace DS_LOKI placeholder with real UID
	delete(dash, "id")
	delete(dash, "__inputs")
	delete(dash, "__requires")
	patchDatasource(dash, datasourceUID)

	payload := map[string]interface{}{
		"dashboard": dash,
		"overwrite": true,
		"folderId":  0,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	grafanaBase := strings.TrimRight(cfg.GrafanaEndpoint, "/")
	req, err := http.NewRequest("POST", grafanaBase+"/api/dashboards/import", bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.GrafanaAPIKey)

	hc := &http.Client{Timeout: 15 * time.Second}
	resp, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return fmt.Errorf("Grafana returned HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	// Extract dashboard URL from response
	var result map[string]interface{}
	if json.Unmarshal(respBody, &result) == nil {
		if url, ok := result["importedUrl"].(string); ok {
			fmt.Printf("  → Dashboard URL: %s%s\n", grafanaBase, url)
		}
	}

	return nil
}

// ensureLokiDatasource creates the Loki datasource in Grafana if it doesn't
// exist and returns its UID.
func ensureLokiDatasource(cfg config.Config) (string, error) {
	grafanaBase := strings.TrimRight(cfg.GrafanaEndpoint, "/")
	hc := &http.Client{Timeout: 15 * time.Second}

	// Check if already exists
	req, _ := http.NewRequest("GET", grafanaBase+"/api/datasources/name/Loki", nil)
	req.Header.Set("Authorization", "Bearer "+cfg.GrafanaAPIKey)
	resp, err := hc.Do(req)
	if err == nil && resp.StatusCode == 200 {
		defer resp.Body.Close()
		var ds struct {
			UID string `json:"uid"`
		}
		body, _ := io.ReadAll(resp.Body)
		if json.Unmarshal(body, &ds) == nil && ds.UID != "" {
			return ds.UID, nil
		}
	}
	if resp != nil {
		resp.Body.Close()
	}

	// Create it
	lokiEndpoint := strings.TrimRight(cfg.LokiEndpoint, "/")
	payload := map[string]interface{}{
		"name":      "Loki",
		"type":      "loki",
		"access":    "proxy",
		"url":       lokiEndpoint,
		"isDefault": false,
		"jsonData":  map[string]interface{}{},
	}

	body, _ := json.Marshal(payload)
	req, err = http.NewRequest("POST", grafanaBase+"/api/datasources", bytes.NewBuffer(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.GrafanaAPIKey)

	resp, err = hc.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 && resp.StatusCode != 201 {
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Datasource struct {
			UID string `json:"uid"`
		} `json:"datasource"`
		UID string `json:"uid"`
	}
	json.Unmarshal(respBody, &result)
	uid := result.Datasource.UID
	if uid == "" {
		uid = result.UID
	}
	return uid, nil
}

// patchDatasource walks the dashboard JSON and replaces the DS_LOKI
// placeholder UID with the real datasource UID from Grafana.
func patchDatasource(dash map[string]interface{}, uid string) {
	panels, ok := dash["panels"].([]interface{})
	if !ok {
		return
	}
	for _, p := range panels {
		panel, ok := p.(map[string]interface{})
		if !ok {
			continue
		}
		if ds, ok := panel["datasource"].(map[string]interface{}); ok {
			ds["uid"] = uid
		}
		// patch targets too
		if targets, ok := panel["targets"].([]interface{}); ok {
			for _, t := range targets {
				if target, ok := t.(map[string]interface{}); ok {
					if ds, ok := target["datasource"].(map[string]interface{}); ok {
						ds["uid"] = uid
					}
				}
			}
		}
	}
	// patch templating variables
	if tmpl, ok := dash["templating"].(map[string]interface{}); ok {
		if list, ok := tmpl["list"].([]interface{}); ok {
			for _, v := range list {
				if variable, ok := v.(map[string]interface{}); ok {
					if ds, ok := variable["datasource"].(map[string]interface{}); ok {
						ds["uid"] = uid
					}
				}
			}
		}
	}
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func printStep(n int, format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	fmt.Printf("\n  [%d] %s\n", n, msg)
}

func printOK(format string, args ...interface{}) {
	fmt.Printf("  ✓ %s\n", fmt.Sprintf(format, args...))
}

func printWarn(format string, args ...interface{}) {
	fmt.Printf("  ⚠ %s\n", fmt.Sprintf(format, args...))
}