package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"neurader/internal/alert"
	"neurader/internal/alertui"
	"neurader/internal/config"
	"neurader/internal/grafana"
	"neurader/internal/hostcmd"
	"neurader/internal/inventory"
	"neurader/internal/logs"
	"neurader/internal/loki"
	"neurader/internal/ping"
	"neurader/internal/setup"
	"neurader/internal/upgrade"
)

var version = "dev"

func main() {
	root := &cobra.Command{
		Use:   "neurader",
		Short: "Ansible Execution Monitor",
		Long:  banner(),
	}

	// ── neurader init ──────────────────────────────────────────────────────
	root.AddCommand(&cobra.Command{
		Use:   "init",
		Short: "Setup neurader on this Ansible controller (run once as root)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return setup.Run()
		},
	})

	// ── neurader post-run (hidden) ─────────────────────────────────────────
	root.AddCommand(&cobra.Command{
		Use:    "post-run",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}

			// 1. Clean old logs
			logs.Clean(cfg.LogDir, cfg.RetentionDays)

			// 2. Push to Loki
			if cfg.LokiEndpoint != "" {
				if err := loki.PushLatest(cfg); err != nil {
					fmt.Fprintf(os.Stderr, "[neurader] loki push failed: %v\n", err)
				}
			}

			// 3. Fire alerts
			latestPath, err := logs.LatestPath(cfg.LogDir)
			if err != nil || latestPath == "" {
				return nil
			}
			data, err := os.ReadFile(latestPath)
			if err != nil {
				return nil
			}
			var run logs.PlaybookRun
			if err := json.Unmarshal(data, &run); err != nil {
				return nil
			}
			runID := filepath.Base(latestPath)
			runID = runID[:len(runID)-len(filepath.Ext(runID))]

			results := alert.Fire(cfg, run, runID)
			for _, r := range results {
				if !r.OK {
					fmt.Fprintf(os.Stderr, "[neurader] alert %s failed: %v\n", r.Channel, r.Err)
				}
			}

			return nil
		},
	})

	// ── neurader list ──────────────────────────────────────────────────────
	root.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List all recorded playbook runs (newest first)",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			return logs.List(cfg.LogDir)
		},
	})

	// ── neurader show <filename|playbook> ──────────────────────────────────
	root.AddCommand(&cobra.Command{
		Use:   "show [playbook or filename]",
		Short: "Show the detailed result of a playbook run",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			return logs.Show(cfg.LogDir, args[0])
		},
	})

	// ── neurader last ──────────────────────────────────────────────────────
	root.AddCommand(func() *cobra.Command {
		var onlyFailed bool
		cmd := &cobra.Command{
			Use:   "last",
			Short: "Show the most recent playbook run",
			RunE: func(cmd *cobra.Command, args []string) error {
				cfg, err := config.Load()
				if err != nil {
					return err
				}
				return hostcmd.Last(cfg, onlyFailed)
			},
		}
		cmd.Flags().BoolVarP(&onlyFailed, "failed", "f", false, "Show only failed/unreachable hosts")
		return cmd
	}())

	// ── neurader hosts ─────────────────────────────────────────────────────
	root.AddCommand(&cobra.Command{
		Use:   "hosts",
		Short: "Show per-host success/failure stats across all runs",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			return hostcmd.Hosts(cfg)
		},
	})

	// ── neurader ping ──────────────────────────────────────────────────────
	root.AddCommand(func() *cobra.Command {
		var group string
		cmd := &cobra.Command{
			Use:   "ping",
			Short: "Check if all inventory hosts are reachable right now",
			RunE: func(cmd *cobra.Command, args []string) error {
				cfg, err := config.Load()
				if err != nil {
					return err
				}
				return ping.Run(cfg, group)
			},
		}
		cmd.Flags().StringVarP(&group, "group", "g", "", "Ping hosts in a specific group only")
		return cmd
	}())

	// ── neurader clean ─────────────────────────────────────────────────────
	root.AddCommand(&cobra.Command{
		Use:   "clean",
		Short: "Delete logs older than the configured retention period",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			logs.Clean(cfg.LogDir, cfg.RetentionDays)
			return nil
		},
	})

	// ── neurader push ──────────────────────────────────────────────────────
	root.AddCommand(&cobra.Command{
		Use:   "push",
		Short: "Push all logs to Loki",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			return loki.PushAll(cfg)
		},
	})

	// ── neurader inventory ─────────────────────────────────────────────────
	root.AddCommand(func() *cobra.Command {
		var group string
		cmd := &cobra.Command{
			Use:   "inventory",
			Short: "Show all hosts and groups from Ansible inventory",
			RunE: func(cmd *cobra.Command, args []string) error {
				cfg, err := config.Load()
				if err != nil {
					return err
				}
				return inventory.Show(cfg, group)
			},
		}
		cmd.Flags().StringVarP(&group, "group", "g", "", "Filter by group name")
		return cmd
	}())

	// ── neurader loki-setup ────────────────────────────────────────────────
	root.AddCommand(&cobra.Command{
		Use:   "loki-setup",
		Short: "Install Loki, create datasource and import dashboard into Grafana",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			return grafana.Setup(cfg)
		},
	})


	// ── neurader alert-setup ──────────────────────────────────────────────
	root.AddCommand(&cobra.Command{
		Use:   "alert-setup",
		Short: "Configure alert channels interactively (Slack, PagerDuty, Jira, Email...)",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			_, saved, err := alertui.Run(cfg)
			if err != nil {
				return err
			}
			if saved {
				fmt.Println()
				fmt.Println("  ✓  Alert configuration saved to /etc/neurader/neurader.conf")
				fmt.Println("  Run: neurader alert-test  to verify all channels")
				fmt.Println()
			}
			return nil
		},
	})
	// ── neurader alert-test ────────────────────────────────────────────────
	root.AddCommand(func() *cobra.Command {
		var channel string
		cmd := &cobra.Command{
			Use:   "alert-test",
			Short: "Send a test alert to all configured channels",
			RunE: func(cmd *cobra.Command, args []string) error {
				cfg, err := config.Load()
				if err != nil {
					return err
				}

				greenC := color.New(color.FgGreen, color.Bold)
				redC   := color.New(color.FgRed, color.Bold)
				boldC  := color.New(color.Bold)

				fmt.Println()
				boldC.Println("  Sending test alerts...")
				fmt.Println()

				results := alert.Test(cfg)

				if len(results) == 0 {
					fmt.Println("  No alert channels configured.")
					fmt.Println("  Run: neurader alert-setup  to configure alerts.")
					fmt.Println()
					return nil
				}

				_ = channel
				for _, r := range results {
					if r.OK {
						greenC.Printf("  ✓  %-20s  sent\n", r.Channel)
					} else {
						redC.Printf("  ✗  %-20s  failed: %v\n", r.Channel, r.Err)
					}
				}
				fmt.Println()
				return nil
			},
		}
		cmd.Flags().StringVarP(&channel, "channel", "c", "", "Test a specific channel (slack, pagerduty, jira, email, teams, telegram, webhook, alertmanager)")
		return cmd
	}())

	// ── neurader status ────────────────────────────────────────────────────
	root.AddCommand(&cobra.Command{
		Use:   "status",
		Short: "Show current configuration and health",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := setup.Status(); err != nil {
				return err
			}
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			green  := color.New(color.FgGreen, color.Bold)
			dim    := color.New(color.FgHiBlack)
			yellow := color.New(color.FgYellow)

			fmt.Println()
			fmt.Println("  Alert Channels")
			fmt.Println("  ──────────────────────────────────────────")
			channels := alert.Channels(cfg)
			activeCount := 0
			for _, ch := range channels {
				if ch.Enabled {
					activeCount++
					green.Printf("  ✓  %-20s", ch.Name)
					dim.Printf("  %s\n", ch.Detail)
				} else {
					dim.Printf("  -  %-20s", ch.Name)
					dim.Printf("  %s\n", ch.Detail)
				}
			}
			fmt.Println("  ──────────────────────────────────────────")
			if activeCount == 0 {
				yellow.Println("  No alert channels configured.")
				dim.Println("  Run: neurader alert-setup")
			} else {
				green.Printf("  %d", activeCount)
				fmt.Printf(" of %d channel(s) active\n", len(channels))
			}
			fmt.Println()
			return nil
		},
	})

	// ── neurader reset-callback ────────────────────────────────────────────
	root.AddCommand(&cobra.Command{
		Use:   "reset-callback",
		Short: "Restore the default callback plugin (overwrites any custom changes)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return setup.ResetCallback()
		},
	})

	// ── neurader uninstall ─────────────────────────────────────────────────
	root.AddCommand(&cobra.Command{
		Use:   "uninstall",
		Short: "Remove neurader completely from this system",
		RunE: func(cmd *cobra.Command, args []string) error {
			return setup.Uninstall()
		},
	})

	// ── neurader upgrade ───────────────────────────────────────────────────
	root.AddCommand(&cobra.Command{
		Use:   "upgrade",
		Short: "Upgrade neurader to the latest version",
		RunE: func(cmd *cobra.Command, args []string) error {
			return upgrade.Run(version)
		},
	})

	// ── neurader version ───────────────────────────────────────────────────
	root.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Print version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("neurader %s\n", version)
		},
	})

	if err := root.Execute(); err != nil {
		color.Red("\nError: %v\n", err)
		os.Exit(1)
	}
}

func banner() string {
	return `
███╗   ██╗███████╗██╗   ██╗██████╗  █████╗ ██████╗ ███████╗██████╗
████╗  ██║██╔════╝██║   ██║██╔══██╗██╔══██╗██╔══██╗██╔════╝██╔══██╗
██╔██╗ ██║█████╗  ██║   ██║██████╔╝███████║██║  ██║█████╗  ██████╔╝
██║╚██╗██║██╔══╝  ██║   ██║██╔══██╗██╔══██║██║  ██║██╔══╝  ██╔══██╗
██║ ╚████║███████╗╚██████╔╝██║  ██║██║  ██║██████╔╝███████╗██║  ██║
╚═╝  ╚═══╝╚══════╝ ╚═════╝ ╚═╝  ╚═╝╚═╝  ╚═╝╚═════╝ ╚══════╝╚═╝  ╚═╝

Ansible Execution Monitor`
}
