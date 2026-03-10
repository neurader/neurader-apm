package ping

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"

	"neurader/internal/config"
)

// hostResult is the per-host result from ansible -m ping
type hostResult struct {
	name        string
	reachable   bool
	msg         string
}

// Run executes `ansible all -m ping` and prints a clean result table.
// If group is set, runs against that group only.
func Run(cfg config.Config, group string) error {
	target := "all"
	if group != "" {
		target = group
	}

	fmt.Println()
	fmt.Printf("  Pinging %s hosts...\n\n", target)

	// Build ansible command
	args := []string{target, "-m", "ping", "-o", "--output-format=json"}
	cmd  := exec.Command("ansible", args...)

	env := os.Environ()
	if cfg.AnsibleCfgPath != "" {
		env = append(env, "ANSIBLE_CONFIG="+cfg.AnsibleCfgPath)
	}
	cmd.Env = env

	out, _ := cmd.Output() // ignore error — ansible exits non-zero if any host fails

	// Parse JSON output
	// ansible -o --output-format=json gives:
	// { "node1": { "ping": "pong" }, "node2": { "unreachable": true, "msg": "..." } }
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(out, &raw); err != nil {
		// Fall back to line-by-line parsing if JSON fails
		return runFallback(cfg, target)
	}

	var results []hostResult
	reachable := 0
	unreachable := 0

	for host, val := range raw {
		var r struct {
			Ping        string `json:"ping"`
			Unreachable bool   `json:"unreachable"`
			Msg         string `json:"msg"`
			Changed     bool   `json:"changed"`
		}
		json.Unmarshal(val, &r)

		if r.Unreachable || r.Ping == "" {
			results = append(results, hostResult{name: host, reachable: false, msg: r.Msg})
			unreachable++
		} else {
			results = append(results, hostResult{name: host, reachable: true})
			reachable++
		}
	}

	// Sort by name
	sort.Slice(results, func(i, j int) bool {
		return results[i].name < results[j].name
	})

	// Print results
	for _, r := range results {
		if r.reachable {
			fmt.Printf("  ✓  %-25s reachable\n", r.name)
		} else {
			msg := r.msg
			if len(msg) > 50 {
				msg = msg[:47] + "..."
			}
			fmt.Printf("  ✗  %-25s unreachable  %s\n", r.name, msg)
		}
	}

	fmt.Println()
	fmt.Printf("  %s\n", strings.Repeat("─", 50))
	fmt.Printf("  Total: %d hosts — %d reachable, %d unreachable\n",
		len(results), reachable, unreachable)
	fmt.Println()

	return nil
}

// runFallback uses plain text output if JSON parsing fails
func runFallback(cfg config.Config, target string) error {
	args := []string{target, "-m", "ping", "-o"}
	cmd  := exec.Command("ansible", args...)

	env := os.Environ()
	if cfg.AnsibleCfgPath != "" {
		env = append(env, "ANSIBLE_CONFIG="+cfg.AnsibleCfgPath)
	}
	cmd.Env    = env
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	cmd.Run()
	return nil
}
