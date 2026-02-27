package main

import (
	"fmt"
	"os"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"neurader/internal/config"
	"neurader/internal/grafana"
	"neurader/internal/logs"
	"neurader/internal/setup"
)

// version is injected at build time via -ldflags "-X main.version=v1.0.0"
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

	// ── neurader post-run (hidden — called by Python callback after each run) ──
	root.AddCommand(&cobra.Command{
		Use:    "post-run",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			logs.Clean(cfg.LogDir, cfg.RetentionDays)
			if cfg.GrafanaEndpoint != "" {
				return grafana.PushLatest(cfg)
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

	// ── neurader show <filename> ───────────────────────────────────────────
	root.AddCommand(&cobra.Command{
		Use:   "show [filename]",
		Short: "Show the detailed result of a specific playbook run",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			return logs.Show(cfg.LogDir, args[0])
		},
	})

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
		Short: "Push all logs to Grafana",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			return grafana.PushAll(cfg)
		},
	})

	// ── neurader grafana-setup ─────────────────────────────────────────────
	root.AddCommand(&cobra.Command{
		Use:   "grafana-setup",
		Short: "Create datasource and import dashboard in Grafana",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			return grafana.Setup(cfg)
		},
	})

	// ── neurader status ────────────────────────────────────────────────────
	root.AddCommand(&cobra.Command{
		Use:   "status",
		Short: "Show current configuration and health",
		RunE: func(cmd *cobra.Command, args []string) error {
			return setup.Status()
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