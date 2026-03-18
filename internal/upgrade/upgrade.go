package upgrade

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

const (
	baseURL    = "https://neurader.cloud/neurader/releases"
	versionURL = "https://neurader.cloud/neurader/releases/version.json"
	installDir = "/usr/bin"
	binaryName = "neurader"
)

// versionInfo is the structure of version.json on S3.
type versionInfo struct {
	Version    string `json:"version"`
	ReleasedAt string `json:"released_at"`
}

// Run checks for a newer version and upgrades if one is available.
// Equivalent to: detect arch → check version → download → replace binary.
func Run(currentVersion string) error {

	// ── Step 1: Re-exec with sudo if not root ─────────────────────────────
	// neurader upgrade replaces /usr/bin/neurader which needs root.
	// Instead of asking users to remember sudo, we ask for it ourselves.
	if os.Geteuid() != 0 {
		fmt.Println("[neurader] upgrade requires root — re-running with sudo...")
		cmd := exec.Command("sudo", append([]string{os.Args[0]}, os.Args[1:]...)...)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("sudo failed: %w", err)
		}
		return nil
	}

	// ── Step 2: Detect architecture ───────────────────────────────────────
	arch, err := detectArch()
	if err != nil {
		return err
	}
	fmt.Printf("[neurader] architecture : %s\n", arch)
	fmt.Printf("[neurader] current      : %s\n", currentVersion)

	// ── Step 3: Check latest version from S3 ─────────────────────────────
	latest, err := fetchLatestVersion()
	if err != nil {
		return fmt.Errorf("failed to check latest version: %w", err)
	}
	fmt.Printf("[neurader] latest        : %s\n", latest.Version)

	// ── Step 4: Compare versions ──────────────────────────────────────────
	if currentVersion == latest.Version {
		fmt.Printf("[neurader] already up to date (%s)\n", currentVersion)
		return nil
	}
	if currentVersion != "dev" && !isNewer(latest.Version, currentVersion) {
		fmt.Printf("[neurader] already up to date (%s)\n", currentVersion)
		return nil
	}

	// ── Step 5: Build download URL ────────────────────────────────────────
	// e.g. https://neurader.cloud/neurader/releases/latest/neurader-linux-amd64
	binaryFile := fmt.Sprintf("neurader-linux-%s", arch)
	downloadURL := fmt.Sprintf("%s/latest/%s", baseURL, binaryFile)
	fmt.Printf("[neurader] downloading   : %s\n", downloadURL)

	// ── Step 6: Download to /tmp ──────────────────────────────────────────
	tmpFile := filepath.Join(os.TempDir(), "neurader-new")
	if err := downloadFile(downloadURL, tmpFile); err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer os.Remove(tmpFile) // clean up tmp if anything goes wrong

	// ── Step 7: Make executable ───────────────────────────────────────────
	if err := os.Chmod(tmpFile, 0755); err != nil {
		return fmt.Errorf("chmod failed: %w", err)
	}

	// ── Step 8: Verify the downloaded binary works ────────────────────────
	out, err := exec.Command(tmpFile, "version").Output()
	if err != nil {
		return fmt.Errorf("downloaded binary failed verification: %w", err)
	}
	downloadedVersion := strings.TrimSpace(string(out))
	fmt.Printf("[neurader] verified      : %s\n", downloadedVersion)

	// ── Step 9: Replace the current binary ───────────────────────────────
	installPath := filepath.Join(installDir, binaryName)

	// Find where the current binary actually lives
	if current, err := exec.LookPath(binaryName); err == nil {
		installPath = current
	}

	// Write to a temp file in the same directory as the binary first,
	// then rename atomically. This avoids "text file busy" which happens
	// when trying to overwrite a running binary directly.
	stagingFile := filepath.Join(filepath.Dir(installPath), ".neurader-upgrade-tmp")

	if err := copyFile(tmpFile, stagingFile); err != nil {
		return fmt.Errorf("failed to stage binary to %s: %w", stagingFile, err)
	}
	if err := os.Chmod(stagingFile, 0755); err != nil {
		os.Remove(stagingFile)
		return fmt.Errorf("chmod on staged binary failed: %w", err)
	}
	// Atomic rename — replaces the running binary safely
	if err := os.Rename(stagingFile, installPath); err != nil {
		os.Remove(stagingFile)
		return fmt.Errorf("failed to install binary to %s: %w", installPath, err)
	}

	fmt.Printf("\n[neurader] ✓ upgraded %s → %s\n", currentVersion, latest.Version)
	fmt.Printf("[neurader] ✓ installed at %s\n", installPath)
	return nil
}

// detectArch maps runtime.GOARCH to the binary suffix used in S3.
//
//	amd64 → amd64   (neurader-linux-amd64)
//	arm64 → arm64   (neurader-linux-arm64)
//	arm   → arm     (neurader-linux-arm)
func detectArch() (string, error) {
	switch runtime.GOARCH {
	case "amd64":
		return "amd64", nil
	case "arm64":
		return "arm64", nil
	case "arm":
		return "arm", nil
	default:
		return "", fmt.Errorf("unsupported architecture: %s — please download manually from %s/latest/", runtime.GOARCH, baseURL)
	}
}

// fetchLatestVersion reads version.json from S3 via CloudFront.
func fetchLatestVersion() (*versionInfo, error) {
	resp, err := http.Get(versionURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("version check returned HTTP %d", resp.StatusCode)
	}

	var info versionInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, fmt.Errorf("failed to parse version.json: %w", err)
	}
	if info.Version == "" {
		return nil, fmt.Errorf("version.json has no version field")
	}
	return &info, nil
}

// downloadFile downloads a URL to a local file path with progress output.
func downloadFile(url, dest string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download returned HTTP %d from %s", resp.StatusCode, url)
	}

	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()

	total := resp.ContentLength
	var downloaded int64

	buf := make([]byte, 32*1024) // 32KB chunks
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := f.Write(buf[:n]); werr != nil {
				return werr
			}
			downloaded += int64(n)
			if total > 0 {
				pct := float64(downloaded) / float64(total) * 100
				fmt.Printf("\r[neurader] progress      : %.1f%% (%s / %s)",
					pct, formatBytes(downloaded), formatBytes(total))
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
	}
	fmt.Println() // newline after progress
	return nil
}

// copyFile copies src to dst — used when os.Rename fails across filesystems.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}

// isNewer returns true if candidate is a newer semver than current.
// Compares MAJOR.MINOR.PATCH numerically so v0.10.0 > v0.9.0 correctly.
func isNewer(candidate, current string) bool {
	return parseSemver(candidate) > parseSemver(current)
}

// parseSemver converts "v1.2.3" into a single integer 001002003
// for easy numeric comparison. Supports up to 999 per component.
func parseSemver(v string) int {
	v = strings.TrimPrefix(v, "v")
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return 0
	}
	major, _ := strconv.Atoi(parts[0])
	minor, _ := strconv.Atoi(parts[1])
	patch, _ := strconv.Atoi(parts[2])
	return major*1_000_000 + minor*1_000 + patch
}

// formatBytes returns a human-readable byte size string.
func formatBytes(b int64) string {
	const mb = 1024 * 1024
	if b >= mb {
		return fmt.Sprintf("%.1f MB", float64(b)/float64(mb))
	}
	return fmt.Sprintf("%d KB", b/1024)
}
