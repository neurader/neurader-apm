package setup

import (
	"os"
	"os/exec"
	"strings"
)

// SystemInfo holds everything detected about the host environment.
type SystemInfo struct {
	Distro               string // ubuntu, debian, rhel, centos, rocky, alma,
	//                             fedora, opensuse, arch, alpine, amzn, unknown
	Version              string
	PackageManager       string // apt-get, dnf, yum, zypper, apk, pacman
	HasSystemd           bool
	HasCron              bool
	PythonBin            string // absolute path to python3 / python
	AnsibleBin           string // absolute path to ansible binary
	AnsibleVersion       string
	AnsibleInstallMethod string // system, pip, pipx
	CallbackDir          string // where to install neurader_callback.py
	AnsibleCfgPath       string // ansible.cfg to patch
}

// DetectSystem probes the host and returns a fully populated SystemInfo.
func DetectSystem() SystemInfo {
	info := SystemInfo{}
	info.Distro, info.Version = detectDistro()
	info.PackageManager = detectPackageManager()
	info.HasSystemd = detectSystemd()
	info.HasCron = detectCron()
	info.PythonBin = detectPython()
	info.AnsibleBin = detectAnsibleBin()
	info.AnsibleVersion = detectAnsibleVersion(info.AnsibleBin)
	info.AnsibleInstallMethod = detectAnsibleInstallMethod(info.AnsibleBin)
	info.CallbackDir = detectCallbackDir(info.PythonBin, info.AnsibleBin)
	info.AnsibleCfgPath = detectAnsibleCfg()
	return info
}

// ── Distro ────────────────────────────────────────────────────────────────────

func detectDistro() (distro, version string) {
	// /etc/os-release is the modern standard present on virtually every distro
	data, err := os.ReadFile("/etc/os-release")
	if err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			switch {
			case strings.HasPrefix(line, "ID="):
				distro = strings.ToLower(strings.Trim(strings.TrimPrefix(line, "ID="), `"`))
			case strings.HasPrefix(line, "VERSION_ID="):
				version = strings.Trim(strings.TrimPrefix(line, "VERSION_ID="), `"`)
			}
		}
		if distro != "" {
			return
		}
	}

	// Legacy fallback files
	legacy := map[string]string{
		"/etc/debian_version": "debian",
		"/etc/redhat-release": "rhel",
		"/etc/alpine-release": "alpine",
		"/etc/arch-release":   "arch",
		"/etc/SuSE-release":   "opensuse",
	}
	for file, name := range legacy {
		if _, err := os.Stat(file); err == nil {
			return name, ""
		}
	}
	return "unknown", ""
}

// ── Package manager ───────────────────────────────────────────────────────────

func detectPackageManager() string {
	for _, m := range []string{"apt-get", "dnf", "yum", "zypper", "apk", "pacman", "emerge"} {
		if path, err := exec.LookPath(m); err == nil && path != "" {
			return m
		}
	}
	return "unknown"
}

// ── Init system ───────────────────────────────────────────────────────────────

func detectSystemd() bool {
	if target, err := os.Readlink("/proc/1/exe"); err == nil {
		if strings.Contains(target, "systemd") {
			return true
		}
	}
	return exec.Command("systemctl", "--version").Run() == nil
}

func detectCron() bool {
	_, err := exec.LookPath("crontab")
	return err == nil
}

// ── Python ────────────────────────────────────────────────────────────────────

func detectPython() string {
	candidates := []string{
		"python3", "python",
		"/usr/bin/python3", "/usr/local/bin/python3", "/usr/bin/python",
	}
	for _, c := range candidates {
		if path, err := exec.LookPath(c); err == nil {
			return path
		}
	}
	return ""
}

// ── Ansible binary ────────────────────────────────────────────────────────────

func detectAnsibleBin() string {
	if path, err := exec.LookPath("ansible"); err == nil {
		return path
	}
	sudoUser := os.Getenv("SUDO_USER")
	candidates := []string{
		"/usr/bin/ansible",
		"/usr/local/bin/ansible",
	}
	if sudoUser != "" {
		candidates = append(candidates,
			"/home/"+sudoUser+"/.local/bin/ansible",
			"/home/"+sudoUser+"/.local/pipx/venvs/ansible/bin/ansible",
		)
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return ""
}

func detectAnsibleVersion(ansibleBin string) string {
	if ansibleBin == "" {
		return ""
	}
	out, err := exec.Command(ansibleBin, "--version").Output()
	if err != nil {
		return ""
	}
	lines := strings.Split(string(out), "\n")
	if len(lines) > 0 {
		return strings.TrimSpace(lines[0])
	}
	return ""
}

func detectAnsibleInstallMethod(ansibleBin string) string {
	switch {
	case ansibleBin == "":
		return "not_found"
	case strings.Contains(ansibleBin, "pipx"):
		return "pipx"
	case strings.Contains(ansibleBin, ".local/bin"):
		return "pip"
	case strings.HasPrefix(ansibleBin, "/usr/bin"):
		return "system"
	default:
		return "pip"
	}
}

// ── Callback plugin directory ─────────────────────────────────────────────────
// Uses 4 strategies in order — works regardless of how Ansible was installed.

func detectCallbackDir(pythonBin, ansibleBin string) string {

	// Strategy 1: Ask Python directly where ansible installed itself
	if pythonBin != "" {
		script := `
import sys, os
try:
    import ansible
    cb = os.path.join(os.path.dirname(ansible.__file__), 'plugins', 'callback')
    print(cb)
except Exception:
    sys.exit(1)
`
		out, err := exec.Command(pythonBin, "-c", script).Output()
		if err == nil {
			p := strings.TrimSpace(string(out))
			if p != "" {
				os.MkdirAll(p, 0755) //nolint:errcheck
				return p
			}
		}
	}

	// Strategy 2: ansible-config dump (Ansible >= 2.8)
	if ansibleBin != "" {
		out, err := exec.Command("ansible-config", "dump").Output()
		if err == nil {
			for _, line := range strings.Split(string(out), "\n") {
				if strings.Contains(line, "DEFAULT_CALLBACK_PLUGIN_PATH") {
					parts := strings.SplitN(line, "=", 2)
					if len(parts) == 2 {
						p := strings.TrimSpace(parts[1])
						p = strings.Trim(p, `[]'"`)
						p = strings.Split(p, ",")[0]
						p = strings.Trim(p, `'" `)
						if p != "" {
							os.MkdirAll(p, 0755) //nolint:errcheck
							return p
						}
					}
				}
			}
		}
	}

	// Strategy 3: Known distro paths
	knownPaths := []string{
		"/usr/share/ansible/plugins/callback",
		"/usr/lib/python3/dist-packages/ansible/plugins/callback",
		"/usr/local/lib/python3.10/dist-packages/ansible/plugins/callback",
		"/usr/local/lib/python3.11/dist-packages/ansible/plugins/callback",
		"/usr/local/lib/python3.12/dist-packages/ansible/plugins/callback",
		"/usr/lib/python3.10/site-packages/ansible/plugins/callback",
		"/usr/lib/python3.11/site-packages/ansible/plugins/callback",
		"/usr/lib/python3.12/site-packages/ansible/plugins/callback",
	}
	for _, p := range knownPaths {
		if dirExists(p) {
			return p
		}
	}

	// Strategy 4: Fallback — neurader-owned dir, ansible.cfg will be told about it
	fallback := "/etc/neurader/callback_plugins"
	os.MkdirAll(fallback, 0755) //nolint:errcheck
	return fallback
}

// ── ansible.cfg path ──────────────────────────────────────────────────────────

func detectAnsibleCfg() string {
	if env := os.Getenv("ANSIBLE_CONFIG"); env != "" {
		return env
	}
	candidates := []string{
		"./ansible.cfg",
		os.ExpandEnv("$HOME/.ansible.cfg"),
		"/etc/ansible/ansible.cfg",
	}
	for _, c := range candidates {
		if fileExists(c) {
			return c
		}
	}
	// None found — create the system-wide default
	os.MkdirAll("/etc/ansible", 0755)           //nolint:errcheck
	os.WriteFile("/etc/ansible/ansible.cfg",     //nolint:errcheck
		[]byte("[defaults]\n"), 0644)
	return "/etc/ansible/ansible.cfg"
}

// ── helpers ───────────────────────────────────────────────────────────────────

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func dirExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}