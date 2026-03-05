package setup

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/fatih/color"

	"neurader/assets"
	"neurader/internal/config"
	"neurader/internal/grafana"
)

var (
	boldC  = color.New(color.Bold)
	greenC = color.New(color.FgGreen, color.Bold)
	warnC  = color.New(color.FgYellow)
	cyanC  = color.New(color.FgCyan)
)

// Run is the full setup wizard — triggered by `neurader init`.
func Run() error {
	printBanner()

	if runtime.GOOS == "linux" && os.Getuid() != 0 {
		return fmt.Errorf("neurader init must be run as root — try: sudo neurader init")
	}

	reader := bufio.NewReader(os.Stdin)

	// ── Step 1: Detect environment ────────────────────────────────────────
	stepHeader(1, "Detecting environment")
	info := DetectSystem()

	if info.AnsibleBin == "" {
		return fmt.Errorf("Ansible not found in PATH — please install Ansible first")
	}

	printOK("Distro            : %s %s", info.Distro, info.Version)
	printOK("Architecture      : %s", runtime.GOARCH)
	printOK("Ansible binary    : %s", info.AnsibleBin)
	printOK("Ansible version   : %s", info.AnsibleVersion)
	printOK("Install method    : %s", info.AnsibleInstallMethod)
	printOK("Callback dir      : %s", info.CallbackDir)
	printOK("ansible.cfg       : %s", info.AnsibleCfgPath)
	printOK("Init system       : %s", initSystemName(info))

	// ── Step 2: Create directories ────────────────────────────────────────
	stepHeader(2, "Creating directories")

	// Config dir — root only write
	if err := os.MkdirAll(config.ConfigDir, 0755); err != nil {
		return fmt.Errorf("creating %s: %w", config.ConfigDir, err)
	}
	printOK("Created %s", config.ConfigDir)

	// Log dir — world writable so pip/pipx Ansible running as non-root
	// can write log files directly from the callback plugin
	if err := os.MkdirAll(config.LogDir, 0777); err != nil {
		return fmt.Errorf("creating %s: %w", config.LogDir, err)
	}
	os.Chmod(config.LogDir, 0777) //nolint:errcheck
	printOK("Created %s", config.LogDir)

	// ── Step 3: Configuration prompts ─────────────────────────────────────
	stepHeader(3, "Configuration")

	retentionStr := prompt(reader, "  Log retention (days)", "3")
	retentionDays, err := strconv.Atoi(retentionStr)
	if err != nil || retentionDays < 1 {
		retentionDays = 3
	}

	grafanaEndpoint := prompt(reader, "  Grafana endpoint (leave blank to skip)", "")
	var grafanaAPIKey, grafanaOrgID string
	if grafanaEndpoint != "" {
		grafanaAPIKey = prompt(reader, "  Grafana API key", "")
		grafanaOrgID = prompt(reader, "  Grafana Org ID", "1")
	}

	// ── Step 4: Write config ──────────────────────────────────────────────
	stepHeader(4, "Writing config → "+config.ConfigFile)
	cfg := config.Config{
		RetentionDays:   retentionDays,
		GrafanaEndpoint: grafanaEndpoint,
		GrafanaAPIKey:   grafanaAPIKey,
		GrafanaOrgID:    grafanaOrgID,
		LogDir:          config.LogDir,
		CallbackDir:     info.CallbackDir,
		AnsibleCfgPath:  info.AnsibleCfgPath,
	}
	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}
	printOK("Config written")

	// ── Step 5: Install Python callback plugin ────────────────────────────
	stepHeader(5, "Installing Ansible callback plugin")
	installed, err := installCallbackPlugin(info.CallbackDir, reader)
	if err != nil {
		return fmt.Errorf("installing callback plugin: %w", err)
	}
	if installed {
		printOK("neurader_callback.py → %s", info.CallbackDir)
	} else {
		printWarn("Keeping existing (user-modified) callback plugin")
	}

	// ── Step 6: Patch ansible.cfg ─────────────────────────────────────────
	stepHeader(6, "Patching ansible.cfg")
	if err := patchAnsibleCfg(info.AnsibleCfgPath, info.CallbackDir); err != nil {
		return fmt.Errorf("patching ansible.cfg: %w", err)
	}
	printOK("callbacks_enabled = neurader added to %s", info.AnsibleCfgPath)

	// ── Step 7: Install cleanup scheduler ────────────────────────────────
	stepHeader(7, "Installing log cleanup scheduler")
	if err := InstallCleanupSchedule(info); err != nil {
		printWarn("Could not install scheduler: %v", err)
		printWarn("Log cleanup will run after each playbook execution instead")
	} else {
		printOK("Cleanup scheduler installed (runs every 12 hours)")
	}

	// ── Step 8: Grafana (optional) ────────────────────────────────────────
	if grafanaEndpoint != "" {
		stepHeader(8, "Configuring Grafana")
		if err := grafana.Setup(cfg); err != nil {
			printWarn("Grafana setup failed: %v", err)
			printWarn("Retry anytime with: neurader grafana-setup")
		} else {
			printOK("Datasource created")
			printOK("Dashboard imported")
		}
	}

	// ── Done ──────────────────────────────────────────────────────────────
	fmt.Println()
	greenC.Println("  ✅  Neurader setup complete!")
	fmt.Println()
	cyanC.Println("  What happens next:")
	fmt.Println("  • Run any ansible-playbook command as normal")
	fmt.Println("  • Logs appear in /var/log/neurader/ automatically")
	fmt.Println("  • View runs  :  neurader list")
	fmt.Println("  • Inspect run:  neurader show <filename>")
	fmt.Println("  • Health     :  neurader status")
	if grafanaEndpoint != "" {
		fmt.Println("  • Push manual:  neurader push")
	}
	fmt.Println()
	return nil
}

// Status prints the current config and a basic health summary.
func Status() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	boldC.Println("\n  Neurader Status")
	fmt.Println("  ───────────────────────────────────────────")
	fmt.Printf("  Log dir          : %s\n", cfg.LogDir)
	fmt.Printf("  Retention        : %d days\n", cfg.RetentionDays)
	fmt.Printf("  Callback dir     : %s\n", cfg.CallbackDir)
	fmt.Printf("  ansible.cfg      : %s\n", cfg.AnsibleCfgPath)

	if cfg.GrafanaEndpoint != "" {
		fmt.Printf("  Grafana endpoint : %s\n", cfg.GrafanaEndpoint)
		fmt.Printf("  Grafana org      : %s\n", cfg.GrafanaOrgID)
	} else {
		fmt.Println("  Grafana          : not configured")
	}

	// Count stored log files
	entries, _ := os.ReadDir(cfg.LogDir)
	count := 0
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".json" {
			count++
		}
	}
	fmt.Printf("  Stored runs      : %d\n", count)

	// Check callback plugin exists
	pluginPath := filepath.Join(cfg.CallbackDir, "neurader_callback.py")
	if fileExists(pluginPath) {
		fmt.Printf("  Callback plugin  : installed ✓\n")
	} else {
		fmt.Printf("  Callback plugin  : MISSING — run: sudo neurader init\n")
	}

	fmt.Println()
	return nil
}

// ResetCallback restores the default embedded callback plugin,
// always overwriting whatever is currently on disk.
func ResetCallback() error {
	if os.Getuid() != 0 {
		return fmt.Errorf("reset-callback must be run as root — try: sudo neurader reset-callback")
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	data, err := assets.CallbackPlugin()
	if err != nil {
		return fmt.Errorf("loading embedded plugin: %w", err)
	}

	dest := filepath.Join(cfg.CallbackDir, "neurader_callback.py")
	if err := os.WriteFile(dest, data, 0644); err != nil {
		return fmt.Errorf("writing callback plugin: %w", err)
	}

	greenC.Printf("\n  ✓ Callback plugin restored to default → %s\n\n", dest)
	return nil
}

// Uninstall removes everything neurader installed from the system.
func Uninstall() error {
	if os.Getuid() != 0 {
		return fmt.Errorf("uninstall must be run as root — try: sudo neurader uninstall")
	}

	cfg, _ := config.Load()

	reader := bufio.NewReader(os.Stdin)
	fmt.Println()
	warnC.Println("  This will remove neurader, all its config, and all logs.")
	fmt.Print("  Continue? [y/N]: ")
	answer, _ := reader.ReadString('\n')
	if strings.ToLower(strings.TrimSpace(answer)) != "y" {
		fmt.Println("  Aborted.")
		return nil
	}

	steps := []struct {
		label string
		fn    func() error
	}{
		{"Remove callback plugin", func() error {
			dest := filepath.Join(cfg.CallbackDir, "neurader_callback.py")
			err := os.Remove(dest)
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}},
		{"Revert ansible.cfg", func() error {
			return unpatchAnsibleCfg(cfg.AnsibleCfgPath)
		}},
		{"Remove systemd units", func() error {
			removeSystemdUnits()
			return nil
		}},
		{"Remove cron job", func() error {
			removeCronJob()
			return nil
		}},
		{"Remove config dir", func() error {
			return os.RemoveAll(config.ConfigDir)
		}},
		{"Remove log dir", func() error {
			return os.RemoveAll(config.LogDir)
		}},
	}

	fmt.Println()
	for _, s := range steps {
		if err := s.fn(); err != nil {
			printWarn("%s: %v", s.label, err)
		} else {
			printOK(s.label)
		}
	}

	fmt.Println()
	greenC.Println("  ✅  Neurader uninstalled.")
	fmt.Println("  Remove the binary manually: sudo rm /usr/bin/neurader")
	fmt.Println()
	return nil
}

// ── Internal helpers ──────────────────────────────────────────────────────────

// installCallbackPlugin writes the embedded callback plugin to callbackDir.
// If the file already exists and has been modified by the user it prompts
// before overwriting — preserving custom log format changes.
func installCallbackPlugin(callbackDir string, reader *bufio.Reader) (installed bool, err error) {
	dest := filepath.Join(callbackDir, "neurader_callback.py")

	embedded, err := assets.CallbackPlugin()
	if err != nil {
		return false, fmt.Errorf("loading embedded plugin: %w", err)
	}

	// If file already exists, check whether the user has modified it
	if fileExists(dest) {
		existing, readErr := os.ReadFile(dest)
		if readErr == nil && !bytes.Equal(existing, embedded) {
			// Content differs — the user has customised the plugin
			fmt.Println()
			warnC.Println("      ⚠  Existing callback plugin has been modified")
			fmt.Printf("         Location: %s\n", dest)
			fmt.Print("         Overwrite with default? [y/N]: ")
			answer, _ := reader.ReadString('\n')
			if strings.ToLower(strings.TrimSpace(answer)) != "y" {
				return false, nil // keep user's version
			}
		}
	}

	if err := os.WriteFile(dest, embedded, 0644); err != nil {
		return false, fmt.Errorf("writing plugin to %s: %w", dest, err)
	}
	return true, nil
}

func patchAnsibleCfg(cfgPath, callbackDir string) error {
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		return err
	}
	content := string(data)

	// Already patched — nothing to do
	if strings.Contains(content, "neurader") {
		return nil
	}

	var insert string

	// If callbackDir is the fallback dir we created, tell ansible.cfg about it
	if callbackDir == "/etc/neurader/callback_plugins" {
		insert = "\ncallbacks_enabled = neurader\nstdout_callback = default\ncallback_plugins = " + callbackDir + "\n"
	} else {
		insert = "\ncallbacks_enabled = neurader\nstdout_callback = default\n"
	}

	if strings.Contains(content, "[defaults]") {
		content = strings.Replace(content, "[defaults]", "[defaults]"+insert, 1)
	} else {
		content += "\n[defaults]" + insert
	}

	return os.WriteFile(cfgPath, []byte(content), 0644)
}

func unpatchAnsibleCfg(cfgPath string) error {
	if cfgPath == "" {
		return nil
	}
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		return nil
	}
	lines := strings.Split(string(data), "\n")
	var out []string
	for _, l := range lines {
		if strings.Contains(l, "neurader") {
			continue
		}
		out = append(out, l)
	}
	return os.WriteFile(cfgPath, []byte(strings.Join(out, "\n")), 0644)
}

func initSystemName(info SystemInfo) string {
	switch {
	case info.HasSystemd:
		return "systemd"
	case info.HasCron:
		return "cron"
	default:
		return "none (cleanup runs per-playbook)"
	}
}

// ── Output helpers ────────────────────────────────────────────────────────────

func printBanner() {
	cyanC.Println(`
███╗   ██╗███████╗██╗   ██╗██████╗  █████╗ ██████╗ ███████╗██████╗
████╗  ██║██╔════╝██║   ██║██╔══██╗██╔══██╗██╔══██╗██╔════╝██╔══██╗
██╔██╗ ██║█████╗  ██║   ██║██████╔╝███████║██║  ██║█████╗  ██████╔╝
██║╚██╗██║██╔══╝  ██║   ██║██╔══██╗██╔══██║██║  ██║██╔══╝  ██╔══██╗
██║ ╚████║███████╗╚██████╔╝██║  ██║██║  ██║██████╔╝███████╗██║  ██║
╚═╝  ╚═══╝╚══════╝ ╚═════╝ ╚═╝  ╚═╝╚═╝  ╚═╝╚═════╝ ╚══════╝╚═╝  ╚═╝`)
	boldC.Println("\n  Ansible Execution Monitor — Setup Wizard")
	fmt.Println("  ─────────────────────────────────────────")
}

func stepHeader(n int, msg string) {
	fmt.Println()
	boldC.Printf("  [%d] %s\n", n, msg)
}

func printOK(format string, args ...interface{}) {
	fmt.Print("      ")
	greenC.Print("✓ ")
	fmt.Printf(format+"\n", args...)
}

func printWarn(format string, args ...interface{}) {
	fmt.Print("      ")
	warnC.Print("⚠ ")
	fmt.Printf(format+"\n", args...)
}

func prompt(r *bufio.Reader, label, defaultVal string) string {
	if defaultVal != "" {
		fmt.Printf("  %s [%s]: ", label, defaultVal)
	} else {
		fmt.Printf("  %s: ", label)
	}
	val, _ := r.ReadString('\n')
	val = strings.TrimSpace(val)
	if val == "" {
		return defaultVal
	}
	return val
}
