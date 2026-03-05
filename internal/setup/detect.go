package setup

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
)

// SystemInfo holds everything detected about the host environment.
type SystemInfo struct {
	Distro               string
	Version              string
	PackageManager       string
	HasSystemd           bool
	HasCron              bool
	PythonBin            string
	AnsibleBin           string
	AnsibleVersion       string
	AnsibleInstallMethod string
	CallbackDir          string
	AnsibleCfgPath       string
	RealUser             string // actual human user — not root when using sudo
	RealHome             string // home directory of RealUser
}

// DetectSystem probes the host and returns a fully populated SystemInfo.
func DetectSystem() SystemInfo {
	info := SystemInfo{}

	// Step 1 — Who is the real user?
	// CRITICAL: must happen first. Everything else (pip paths, binaries,
	// home directories) depends on knowing WHO installed Ansible.
	// When `sudo neurader init` runs, SUDO_USER = the actual person.
	info.RealUser, info.RealHome = detectRealUser()

	info.Distro, info.Version = detectDistro()
	info.PackageManager = detectPackageManager()
	info.HasSystemd = detectSystemd()
	info.HasCron = detectCron()

	// Step 2 — Find ansible binary
	// Must search the real user's PATH, not root's.
	info.AnsibleBin = detectAnsibleBin(info.RealHome)

	// Step 3 — Ask ansible --version for everything
	// This is the most reliable source of truth — Ansible tells us exactly
	// where it is installed, which Python it uses, and its config file.
	ansibleVersionInfo := parseAnsibleVersion(info.AnsibleBin, info.RealHome)
	info.AnsibleVersion = ansibleVersionInfo.version
	info.PythonBin = ansibleVersionInfo.pythonBin
	info.AnsibleCfgPath = ansibleVersionInfo.cfgPath

	// If ansible --version gave us the module path, derive callback dir from it
	// e.g. /home/ec2-user/.local/lib/python3.12/site-packages/ansible
	//   →  /home/ec2-user/.local/lib/python3.12/site-packages/ansible/plugins/callback
	if ansibleVersionInfo.moduleLocation != "" {
		candidate := filepath.Join(ansibleVersionInfo.moduleLocation, "plugins", "callback")
		if dirExists(candidate) {
			info.CallbackDir = candidate
		}
	}

	// If we couldn't get it from ansible --version, fall back to other strategies
	if info.CallbackDir == "" {
		info.CallbackDir = detectCallbackDir(info.PythonBin, info.AnsibleBin, info.RealHome)
	}

	// If ansible --version didn't give us the cfg path, detect it separately
	if info.AnsibleCfgPath == "" {
		info.AnsibleCfgPath = detectAnsibleCfg(info.RealHome)
	}

	// Python fallback if ansible --version didn't give us it
	if info.PythonBin == "" {
		info.PythonBin = detectPythonBin(info.RealHome)
	}

	info.AnsibleInstallMethod = detectAnsibleInstallMethod(info.AnsibleBin, info.RealHome)

	return info
}

// PrintDetectionReport prints what was detected — shown during neurader init.
func PrintDetectionReport(info SystemInfo) {
	printOK("OS                : %s %s (%s/%s)", info.Distro, info.Version, runtime.GOOS, runtime.GOARCH)
	printOK("Real user         : %s (home: %s)", info.RealUser, info.RealHome)
	printOK("Package manager   : %s", info.PackageManager)
	printOK("Ansible binary    : %s", info.AnsibleBin)
	printOK("Ansible version   : %s", info.AnsibleVersion)
	printOK("Install method    : %s", info.AnsibleInstallMethod)
	printOK("Python binary     : %s", info.PythonBin)
	printOK("Callback dir      : %s", info.CallbackDir)
	printOK("ansible.cfg       : %s", info.AnsibleCfgPath)
	printOK("Init system       : %s", initSystemName(info))
}

// ── Real user detection ───────────────────────────────────────────────────────

func detectRealUser() (username, homeDir string) {
	// sudo sets SUDO_USER to the original calling user
	// e.g. ec2-user runs: sudo neurader init → SUDO_USER=ec2-user
	if sudoUser := os.Getenv("SUDO_USER"); sudoUser != "" && sudoUser != "root" {
		if u, err := user.Lookup(sudoUser); err == nil {
			return u.Username, u.HomeDir
		}
	}

	// No SUDO_USER — either running as normal user or logged in directly as root.
	// If current user is not root, use them directly.
	if u, err := user.Current(); err == nil {
		if u.Uid != "0" {
			return u.Username, u.HomeDir
		}
	}

	// Running directly as root (ssh root@server or su -).
	// SUDO_USER is not set so we don't know who the real user is.
	// Scan /home/* to find a user who has Ansible installed — that is
	// almost certainly the user who owns this Ansible controller.
	if found := findAnsibleUser(); found != nil {
		return found.Username, found.HomeDir
	}

	// Absolute fallback
	home, _ := os.UserHomeDir()
	return os.Getenv("USER"), home
}

// findAnsibleUser scans all home directories under /home/ and returns
// the first user who has Ansible installed via pip or pipx.
// Used when running directly as root with no SUDO_USER set.
func findAnsibleUser() *user.User {
	entries, err := os.ReadDir("/home")
	if err != nil {
		return nil
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		homeDir := "/home/" + e.Name()
		// Check common pip and pipx install locations
		ansiblePaths := []string{
			homeDir + "/.local/bin/ansible",
			homeDir + "/.local/pipx/venvs/ansible/bin/ansible",
		}
		for _, p := range ansiblePaths {
			if _, err := os.Stat(p); err == nil {
				// Found ansible — look up this user
				if u, err := user.Lookup(e.Name()); err == nil {
					return u
				}
			}
		}
	}
	return nil
}

// ── ansible --version parser ─────────────────────────────────────────────────
// This is the PRIMARY detection strategy.
//
// `ansible --version` outputs everything we need in one shot:
//
//   ansible [core 2.16.0]
//     config file = /etc/ansible/ansible.cfg
//     ansible python module location = /home/ec2-user/.local/lib/python3.12/site-packages/ansible
//     executable location = /home/ec2-user/.local/bin/ansible
//     python version = 3.12.0 (/home/ec2-user/.local/bin/python3.12)
//
// From this one command we get:
//   - Ansible version
//   - Exact module location → callback dir = module_location/plugins/callback
//   - Python binary
//   - Config file in use

type ansibleVersionData struct {
	version        string
	moduleLocation string
	pythonBin      string
	cfgPath        string
}

func parseAnsibleVersion(ansibleBin, realHome string) ansibleVersionData {
	result := ansibleVersionData{}

	bins := buildAnsibleBinCandidates(ansibleBin, realHome)
	var out []byte
	var err error

	for _, bin := range bins {
		out, err = exec.Command(bin, "--version").Output()
		if err == nil {
			break
		}
	}
	if err != nil || len(out) == 0 {
		return result
	}

	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		switch {
		// First line: "ansible [core 2.16.0]" or "ansible 2.9.x"
		case strings.HasPrefix(line, "ansible"):
			result.version = line

		// "config file = /etc/ansible/ansible.cfg"
		case strings.HasPrefix(line, "config file ="):
			p := strings.TrimSpace(strings.TrimPrefix(line, "config file ="))
			if p != "" && p != "None" && fileExists(p) {
				result.cfgPath = p
			}

		// "ansible python module location = /path/to/ansible"
		// This is the golden line — tells us exactly where ansible package lives
		case strings.HasPrefix(line, "ansible python module location ="):
			raw := strings.TrimSpace(strings.TrimPrefix(line, "ansible python module location ="))
			// Sometimes it's a list: ['/path1', '/path2']
			raw = strings.Trim(raw, "[]")
			parts := strings.Split(raw, ",")
			if len(parts) > 0 {
				p := strings.Trim(strings.TrimSpace(parts[0]), `'"`)
				if p != "" && dirExists(p) {
					result.moduleLocation = p
				}
			}

		// "python version = 3.12.0 (/usr/bin/python3.12)"
		case strings.HasPrefix(line, "python version ="):
			// Extract path from parentheses
			if start := strings.Index(line, "("); start != -1 {
				if end := strings.Index(line, ")"); end != -1 && end > start {
					bin := line[start+1 : end]
					if fileExists(bin) {
						result.pythonBin = bin
					}
				}
			}
		}
	}

	return result
}

// ── Ansible binary detection ──────────────────────────────────────────────────

func detectAnsibleBin(realHome string) string {
	candidates := buildAnsibleBinCandidates("", realHome)
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	if path, err := exec.LookPath("ansible"); err == nil {
		return path
	}
	// Last resort — scan all home directories
	// Handles: root running neurader init directly without sudo
	if entries, err := os.ReadDir("/home"); err == nil {
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			for _, suffix := range []string{
				"/.local/bin/ansible",
				"/.local/pipx/venvs/ansible/bin/ansible",
			} {
				p := "/home/" + e.Name() + suffix
				if _, err := os.Stat(p); err == nil {
					return p
				}
			}
		}
	}
	return ""
}

func buildAnsibleBinCandidates(known, realHome string) []string {
	var c []string
	if known != "" {
		c = append(c, known)
	}
	// Real user's local installs first — most common for pip/pipx
	if realHome != "" {
		c = append(c,
			realHome+"/.local/bin/ansible",
			realHome+"/.local/pipx/venvs/ansible/bin/ansible",
		)
	}
	// PATH
	if path, err := exec.LookPath("ansible"); err == nil {
		c = append(c, path)
	}
	// System-wide
	c = append(c,
		"/usr/bin/ansible",
		"/usr/local/bin/ansible",
	)
	return c
}

// ── Callback directory detection ─────────────────────────────────────────────
// Called only when ansible --version didn't give us the module location.
// Tries every possible strategy to find where Ansible loads callbacks from.

func detectCallbackDir(pythonBin, ansibleBin, realHome string) string {

	// Strategy 1: Ask Python where ansible package lives
	// Works even when ansible binary is not in root's PATH.
	if pythonBin != "" {
		if dir := callbackDirFromPython(pythonBin); dir != "" {
			return dir
		}
	}

	// Strategy 2: Ask ansible-config dump
	// Asks Ansible itself where DEFAULT_CALLBACK_PLUGIN_PATH points.
	if dir := callbackDirFromAnsibleConfig(ansibleBin); dir != "" {
		return dir
	}

	// Strategy 3: Walk real user's pip and pipx directories
	// Brute-force finds the callback dir — handles any Python version.
	if realHome != "" {
		for _, base := range []string{
			realHome + "/.local/lib",
			realHome + "/.local/pipx/venvs/ansible/lib",
		} {
			if dir := walkForCallbackDir(base); dir != "" {
				return dir
			}
		}
	}

	// Strategy 4: Walk system directories
	for _, base := range []string{"/usr/local/lib", "/usr/lib"} {
		if dir := walkForCallbackDir(base); dir != "" {
			return dir
		}
	}

	// Strategy 5: Exhaustive hardcoded path list
	// Covers every distro + Python version combination we know about.
	for _, p := range buildKnownCallbackPaths(realHome) {
		if dirExists(p) {
			return p
		}
	}

	// Strategy 6: Last resort — our own directory
	// We create it and tell ansible.cfg to look here via callback_plugins setting.
	fallback := "/etc/neurader/callback_plugins"
	os.MkdirAll(fallback, 0755) //nolint:errcheck
	return fallback
}

// callbackDirFromPython asks the Python interpreter that has Ansible installed.
func callbackDirFromPython(pythonBin string) string {
	script := `
import sys, os
try:
    import ansible
    cb = os.path.join(os.path.dirname(ansible.__file__), 'plugins', 'callback')
    if os.path.isdir(cb):
        print(cb)
    else:
        sys.exit(1)
except Exception:
    sys.exit(1)
`
	out, err := exec.Command(pythonBin, "-c", script).Output()
	if err != nil {
		return ""
	}
	p := strings.TrimSpace(string(out))
	if dirExists(p) {
		return p
	}
	return ""
}

// callbackDirFromAnsibleConfig asks Ansible via ansible-config dump.
func callbackDirFromAnsibleConfig(ansibleBin string) string {
	var cmds [][]string
	if ansibleBin != "" {
		cmds = append(cmds, []string{ansibleBin, "-c", "dump"})
	}
	if path, err := exec.LookPath("ansible-config"); err == nil {
		cmds = append(cmds, []string{path, "dump"})
	}

	for _, args := range cmds {
		out, err := exec.Command(args[0], args[1:]...).Output()
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(out), "\n") {
			if !strings.Contains(line, "DEFAULT_CALLBACK_PLUGIN_PATH") {
				continue
			}
			parts := strings.SplitN(line, "=", 2)
			if len(parts) < 2 {
				continue
			}
			val := strings.Trim(strings.TrimSpace(parts[1]), "[]")
			for _, raw := range strings.Split(val, ",") {
				p := strings.Trim(strings.TrimSpace(raw), `'"`)
				if p != "" && dirExists(p) {
					return p
				}
			}
		}
	}
	return ""
}

// walkForCallbackDir walks a base path to find ansible/plugins/callback.
// This is the most robust approach — works for any Python version since
// it doesn't hardcode Python version numbers in paths.
func walkForCallbackDir(base string) string {
	if !dirExists(base) {
		return ""
	}
	target := filepath.ToSlash(filepath.Join("ansible", "plugins", "callback"))
	found := ""

	filepath.Walk(base, func(path string, info os.FileInfo, err error) error { //nolint:errcheck
		if err != nil || found != "" {
			return filepath.SkipDir
		}
		if !info.IsDir() {
			return nil
		}
		if strings.HasSuffix(filepath.ToSlash(path), target) {
			found = path
			return filepath.SkipDir
		}
		// Skip too-deep paths to keep it fast
		rel := strings.TrimPrefix(path, base)
		if strings.Count(rel, string(os.PathSeparator)) > 8 {
			return filepath.SkipDir
		}
		return nil
	})
	return found
}

// buildKnownCallbackPaths builds an exhaustive list of possible callback
// directories covering every distro, package manager, and Python version.
func buildKnownCallbackPaths(realHome string) []string {
	var paths []string
	pyVersions := []string{"3.13", "3.12", "3.11", "3.10", "3.9", "3.8", "3.7"}

	// ── pip user install ──────────────────────────────────────────────────────
	// sudo pip3 install ansible  OR  pip3 install ansible (user)
	if realHome != "" {
		for _, v := range pyVersions {
			paths = append(paths,
				fmt.Sprintf("%s/.local/lib/python%s/site-packages/ansible/plugins/callback", realHome, v),
			)
		}
		// pipx install ansible
		for _, v := range pyVersions {
			paths = append(paths,
				fmt.Sprintf("%s/.local/pipx/venvs/ansible/lib/python%s/site-packages/ansible/plugins/callback", realHome, v),
			)
		}
		// conda install ansible
		for _, v := range pyVersions {
			paths = append(paths,
				fmt.Sprintf("%s/anaconda3/lib/python%s/site-packages/ansible/plugins/callback", realHome, v),
				fmt.Sprintf("%s/miniconda3/lib/python%s/site-packages/ansible/plugins/callback", realHome, v),
				fmt.Sprintf("%s/miniforge3/lib/python%s/site-packages/ansible/plugins/callback", realHome, v),
			)
		}
		// virtualenv/venv (common locations)
		for _, venvName := range []string{"venv", "ansible-venv", ".venv", "env"} {
			for _, v := range pyVersions {
				paths = append(paths,
					fmt.Sprintf("%s/%s/lib/python%s/site-packages/ansible/plugins/callback", realHome, venvName, v),
				)
			}
		}
	}

	// ── sudo pip install ansible (system-wide) ────────────────────────────────
	for _, v := range pyVersions {
		paths = append(paths,
			fmt.Sprintf("/usr/local/lib/python%s/site-packages/ansible/plugins/callback", v),
			fmt.Sprintf("/usr/local/lib/python%s/dist-packages/ansible/plugins/callback", v),
		)
	}

	// ── system package manager ────────────────────────────────────────────────
	// apt install ansible (Debian/Ubuntu)
	paths = append(paths, "/usr/lib/python3/dist-packages/ansible/plugins/callback")
	for _, v := range pyVersions {
		paths = append(paths,
			fmt.Sprintf("/usr/lib/python%s/dist-packages/ansible/plugins/callback", v),
			fmt.Sprintf("/usr/lib/python%s/site-packages/ansible/plugins/callback", v),
		)
	}

	// dnf/yum install ansible (RHEL/CentOS/Fedora/Amazon Linux)
	paths = append(paths, "/usr/share/ansible/plugins/callback")

	// ── system conda ──────────────────────────────────────────────────────────
	for _, v := range pyVersions {
		paths = append(paths,
			fmt.Sprintf("/opt/conda/lib/python%s/site-packages/ansible/plugins/callback", v),
			fmt.Sprintf("/opt/anaconda3/lib/python%s/site-packages/ansible/plugins/callback", v),
			fmt.Sprintf("/opt/miniconda3/lib/python%s/site-packages/ansible/plugins/callback", v),
		)
	}

	return paths
}

// ── Python binary detection ───────────────────────────────────────────────────
// Called as fallback when ansible --version didn't give us the Python path.

func detectPythonBin(realHome string) string {
	var candidates []string

	if realHome != "" {
		candidates = append(candidates,
			realHome+"/.local/bin/python3",
			realHome+"/.local/bin/python",
		)
		for _, v := range []string{"3.12", "3.11", "3.10", "3.9", "3.8"} {
			candidates = append(candidates,
				fmt.Sprintf("%s/.local/pipx/venvs/ansible/bin/python%s", realHome, v),
			)
		}
	}

	candidates = append(candidates,
		"python3", "python",
		"/usr/bin/python3",
		"/usr/local/bin/python3",
		"/usr/bin/python",
	)

	// Prefer Python that has Ansible installed
	for _, c := range candidates {
		path := resolveBin(c)
		if path == "" {
			continue
		}
		out, err := exec.Command(path, "-c", "import ansible; print('ok')").Output()
		if err == nil && strings.TrimSpace(string(out)) == "ok" {
			return path
		}
	}

	// Any working Python as last resort
	for _, c := range candidates {
		if path := resolveBin(c); path != "" {
			return path
		}
	}
	return ""
}

// ── ansible.cfg detection ─────────────────────────────────────────────────────

func detectAnsibleCfg(realHome string) string {
	// ANSIBLE_CONFIG env var takes highest priority (Ansible's own rule)
	if env := os.Getenv("ANSIBLE_CONFIG"); env != "" && fileExists(env) {
		return env
	}

	var candidates []string

	// ansible.cfg in current working directory
	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates, filepath.Join(cwd, "ansible.cfg"))
	}

	// ~/.ansible.cfg
	if realHome != "" {
		candidates = append(candidates, filepath.Join(realHome, ".ansible.cfg"))
	}

	// /etc/ansible/ansible.cfg
	candidates = append(candidates, "/etc/ansible/ansible.cfg")

	for _, c := range candidates {
		if fileExists(c) {
			return c
		}
	}

	// None found — create the system-wide default
	os.MkdirAll("/etc/ansible", 0755)                                      //nolint:errcheck
	os.WriteFile("/etc/ansible/ansible.cfg", []byte("[defaults]\n"), 0644) //nolint:errcheck
	return "/etc/ansible/ansible.cfg"
}

// ── Distro ────────────────────────────────────────────────────────────────────

func detectDistro() (distro, version string) {
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
	// Fallback for older distros without /etc/os-release
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
		if path, _ := exec.LookPath(m); path != "" {
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

// ── Install method ────────────────────────────────────────────────────────────

func detectAnsibleInstallMethod(ansibleBin, realHome string) string {
	switch {
	case ansibleBin == "":
		return "not found"
	case strings.Contains(ansibleBin, "pipx"):
		return "pipx"
	case strings.Contains(ansibleBin, "conda"):
		return "conda"
	case strings.Contains(ansibleBin, "venv") || strings.Contains(ansibleBin, ".venv"):
		return "virtualenv"
	case realHome != "" && strings.HasPrefix(ansibleBin, realHome+"/.local"):
		return "pip (user install)"
	case strings.HasPrefix(ansibleBin, "/usr/local"):
		return "pip (system install)"
	case strings.HasPrefix(ansibleBin, "/usr/bin") || strings.HasPrefix(ansibleBin, "/usr/share"):
		return "system package (apt/dnf/yum)"
	default:
		return "pip"
	}
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func dirExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}

func resolveBin(c string) string {
	if filepath.IsAbs(c) {
		if _, err := os.Stat(c); err == nil {
			return c
		}
		return ""
	}
	path, _ := exec.LookPath(c)
	return path
}
