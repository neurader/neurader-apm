package inventory

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"text/tabwriter"

	"neurader/internal/config"
)

// ansibleInventory is the parsed output of `ansible-inventory --list`
type ansibleInventory struct {
	Meta struct {
		HostVars map[string]interface{} `json:"hostvars"`
	} `json:"_meta"`
	// remaining keys are group names
}

// Show runs `ansible-inventory --list` and prints a clean inventory table.
func Show(cfg config.Config, group string) error {
	fmt.Println()
	fmt.Println("  Fetching inventory...")

	// Build command — use ansible_cfg_path if configured
	args := []string{"--list", "--output=/dev/stdout"}
	cmd  := exec.Command("ansible-inventory", args...)

	// Point to the right ansible.cfg so all inventory sources are picked up
	env := os.Environ()
	if cfg.AnsibleCfgPath != "" {
		env = append(env, "ANSIBLE_CONFIG="+cfg.AnsibleCfgPath)
	}
	cmd.Env = env

	out, err := cmd.Output()
	if err != nil {
		// Try to show stderr for better error messages
		if exitErr, ok := err.(*exec.ExitError); ok {
			return fmt.Errorf("ansible-inventory failed: %s", string(exitErr.Stderr))
		}
		return fmt.Errorf("running ansible-inventory: %w", err)
	}

	// Parse the raw JSON
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(out, &raw); err != nil {
		return fmt.Errorf("parsing ansible-inventory output: %w", err)
	}

	// Extract all hosts from _meta.hostvars
	var meta struct {
		HostVars map[string]interface{} `json:"hostvars"`
	}
	if metaRaw, ok := raw["_meta"]; ok {
		if err := json.Unmarshal(metaRaw, &meta); err == nil {
			// meta parsed ok
		}
	}

	// Extract groups (skip _meta, all, ungrouped)
	skipKeys := map[string]bool{"_meta": true, "all": true, "ungrouped": true}
	type groupEntry struct {
		name  string
		hosts []string
	}
	var groups []groupEntry
	allHosts := map[string]string{} // host → group

	for key, val := range raw {
		if skipKeys[key] {
			continue
		}
		var g struct {
			Hosts    []string               `json:"hosts"`
			Children []string               `json:"children"`
			Vars     map[string]interface{} `json:"vars"`
		}
		if err := json.Unmarshal(val, &g); err != nil {
			continue
		}
		if len(g.Hosts) == 0 {
			continue
		}
		hosts := make([]string, len(g.Hosts))
		copy(hosts, g.Hosts)
		sort.Strings(hosts)
		groups = append(groups, groupEntry{name: key, hosts: hosts})
		for _, h := range hosts {
			if _, exists := allHosts[h]; !exists {
				allHosts[h] = key
			}
		}
	}

	// If no custom groups, fall back to ungrouped
	if len(groups) == 0 {
		if ungroupedRaw, ok := raw["ungrouped"]; ok {
			var g struct {
				Hosts []string `json:"hosts"`
			}
			if err := json.Unmarshal(ungroupedRaw, &g); err == nil && len(g.Hosts) > 0 {
				sort.Strings(g.Hosts)
				groups = append(groups, groupEntry{name: "ungrouped", hosts: g.Hosts})
				for _, h := range g.Hosts {
					allHosts[h] = "ungrouped"
				}
			}
		}
	}

	// Sort groups alphabetically
	sort.Slice(groups, func(i, j int) bool {
		return groups[i].name < groups[j].name
	})

	// ── Filter by group if requested ──────────────────────────────────────
	if group != "" {
		var found *groupEntry
		for i := range groups {
			if groups[i].name == group {
				found = &groups[i]
				break
			}
		}
		if found == nil {
			// List available groups to help user
			names := make([]string, len(groups))
			for i, g := range groups {
				names[i] = g.name
			}
			return fmt.Errorf("group %q not found\n  Available groups: %s",
				group, strings.Join(names, ", "))
		}
		printGroupDetail(found.name, found.hosts)
		return nil
	}

	// ── Full inventory display ────────────────────────────────────────────
	fmt.Print("\r") // clear "Fetching..." line
	fmt.Println()

	// Header
	fmt.Printf("  ┌─────────────────────────────────────────────────────────────┐\n")
	fmt.Printf("  │  Ansible Inventory — %d hosts, %d groups%s│\n",
		len(allHosts), len(groups),
		strings.Repeat(" ", max(0, 38-countDigits(len(allHosts))-countDigits(len(groups)))))
	fmt.Printf("  └─────────────────────────────────────────────────────────────┘\n")
	fmt.Println()

	// Groups table
	fmt.Printf("  GROUPS\n")
	fmt.Printf("  %s\n", strings.Repeat("─", 63))
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintf(w, "  %-20s\t%6s\t%s\n", "GROUP", "HOSTS", "HOST LIST")
	fmt.Fprintf(w, "  %-20s\t%6s\t%s\n",
		strings.Repeat("─", 20), strings.Repeat("─", 5), strings.Repeat("─", 35))
	for _, g := range groups {
		hostList := strings.Join(g.hosts, ", ")
		if len(hostList) > 40 {
			hostList = hostList[:37] + "..."
		}
		fmt.Fprintf(w, "  %-20s\t%6d\t%s\n", g.name, len(g.hosts), hostList)
	}
	w.Flush()

	fmt.Println()

	// All hosts table
	fmt.Printf("  ALL HOSTS (%d)\n", len(allHosts))
	fmt.Printf("  %s\n", strings.Repeat("─", 63))
	w2 := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintf(w2, "  %-25s\t%s\n", "HOST", "GROUP")
	fmt.Fprintf(w2, "  %-25s\t%s\n", strings.Repeat("─", 25), strings.Repeat("─", 20))

	// Sort hosts for consistent display
	sortedHosts := make([]string, 0, len(allHosts))
	for h := range allHosts {
		sortedHosts = append(sortedHosts, h)
	}
	sort.Strings(sortedHosts)

	for _, h := range sortedHosts {
		fmt.Fprintf(w2, "  %-25s\t%s\n", h, allHosts[h])
	}
	w2.Flush()
	fmt.Println()

	// Tip
	fmt.Printf("  Tip: neurader inventory --group <name>  to filter by group\n")
	fmt.Println()

	return nil
}

func printGroupDetail(name string, hosts []string) {
	fmt.Print("\r")
	fmt.Println()
	fmt.Printf("  Group: %s (%d hosts)\n", name, len(hosts))
	fmt.Printf("  %s\n", strings.Repeat("─", 40))
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintf(w, "  HOST\n")
	fmt.Fprintf(w, "  %s\n", strings.Repeat("─", 25))
	for _, h := range hosts {
		fmt.Fprintf(w, "  %s\n", h)
	}
	w.Flush()
	fmt.Println()
}

func countDigits(n int) int {
	if n == 0 {
		return 1
	}
	count := 0
	for n > 0 {
		count++
		n /= 10
	}
	return count
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
