package setup

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"neurader/assets"
)

// InstallCleanupSchedule installs the best available log cleanup scheduler.
// Priority: systemd timer > cron > nothing (cleanup falls back to post-run).
func InstallCleanupSchedule(info SystemInfo) error {
	if info.HasSystemd {
		return installSystemdTimer()
	}
	if info.HasCron {
		return installCronJob()
	}
	return nil
}

// ── Systemd ───────────────────────────────────────────────────────────────────

func installSystemdTimer() error {
	units := map[string]func() ([]byte, error){
		"/etc/systemd/system/neurader-cleanup.service": assets.SystemdService,
		"/etc/systemd/system/neurader-cleanup.timer":   assets.SystemdTimer,
	}
	for dest, loader := range units {
		data, err := loader()
		if err != nil {
			return fmt.Errorf("loading embedded unit %s: %w", dest, err)
		}
		if err := os.WriteFile(dest, data, 0644); err != nil {
			return fmt.Errorf("writing %s: %w", dest, err)
		}
	}
	for _, args := range [][]string{
		{"systemctl", "daemon-reload"},
		{"systemctl", "enable", "--now", "neurader-cleanup.timer"},
	} {
		if out, err := exec.Command(args[0], args[1:]...).CombinedOutput(); err != nil {
			return fmt.Errorf("%s: %w — %s", strings.Join(args, " "), err, string(out))
		}
	}
	return nil
}

func removeSystemdUnits() {
	exec.Command("systemctl", "disable", "--now", "neurader-cleanup.timer").Run() //nolint:errcheck
	os.Remove("/etc/systemd/system/neurader-cleanup.service")
	os.Remove("/etc/systemd/system/neurader-cleanup.timer")
	exec.Command("systemctl", "daemon-reload").Run() //nolint:errcheck
}

// ── Cron ──────────────────────────────────────────────────────────────────────

const (
	cronMark = "# neurader log cleanup"
	cronLine = "0 */12 * * * /usr/bin/neurader clean > /dev/null 2>&1"
)

func installCronJob() error {
	out, _ := exec.Command("crontab", "-l").Output()
	existing := string(out)
	if strings.Contains(existing, cronMark) {
		return nil
	}
	newCron := existing + "\n" + cronMark + "\n" + cronLine + "\n"
	cmd := exec.Command("crontab", "-")
	cmd.Stdin = strings.NewReader(newCron)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("installing cron job: %w — %s", err, string(out))
	}
	return nil
}

func removeCronJob() {
	out, err := exec.Command("crontab", "-l").Output()
	if err != nil {
		return
	}
	lines := strings.Split(string(out), "\n")
	var kept []string
	for _, l := range lines {
		if strings.Contains(l, "neurader") {
			continue
		}
		kept = append(kept, l)
	}
	cmd := exec.Command("crontab", "-")
	cmd.Stdin = strings.NewReader(strings.Join(kept, "\n"))
	cmd.Run() //nolint:errcheck
}
