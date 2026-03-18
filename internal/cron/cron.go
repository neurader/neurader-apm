package cron

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fatih/color"
)

// Job represents a single cron job entry
type Job struct {
	Schedule string
	Command  string
	User     string // only present in /etc/crontab and /etc/cron.d/
	Source   string // file it was read from
}

// cronSources returns all cron file paths to check across all supported distros
func cronSources() []string {
	sources := []string{}

	// /etc/crontab — present on all distros
	if _, err := os.Stat("/etc/crontab"); err == nil {
		sources = append(sources, "/etc/crontab")
	}

	// /etc/cron.d/* — present on all distros
	entries, err := os.ReadDir("/etc/cron.d")
	if err == nil {
		for _, e := range entries {
			if !e.IsDir() {
				sources = append(sources, filepath.Join("/etc/cron.d", e.Name()))
			}
		}
	}

	// User crontabs — path differs by distro
	// Debian/Ubuntu: /var/spool/cron/crontabs/
	// Amazon Linux / Rocky / RHEL: /var/spool/cron/
	spoolDirs := []string{
		"/var/spool/cron/crontabs",
		"/var/spool/cron",
	}
	for _, dir := range spoolDirs {
		info, err := os.Stat(dir)
		if err != nil || !info.IsDir() {
			continue
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() {
				sources = append(sources, filepath.Join(dir, e.Name()))
			}
		}
	}

	return sources
}

// parseFile reads a cron file and returns all active jobs
func parseFile(path string) []Job {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	// determine if this is a system crontab (has user field)
	isSystem := path == "/etc/crontab" || strings.HasPrefix(path, "/etc/cron.d/")

	var jobs []Job
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// skip blank lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// skip environment variable lines like SHELL=, PATH=, MAILTO=
		if strings.Contains(line, "=") && !strings.Contains(line, " ") {
			continue
		}

		fields := strings.Fields(line)

		if isSystem {
			// system crontab format: min hour dom month dow user command
			if len(fields) < 7 {
				continue
			}
			schedule := strings.Join(fields[0:5], " ")
			user := fields[5]
			command := strings.Join(fields[6:], " ")
			jobs = append(jobs, Job{
				Schedule: schedule,
				Command:  command,
				User:     user,
				Source:   path,
			})
		} else {
			// user crontab format: min hour dom month dow command
			if len(fields) < 6 {
				continue
			}
			schedule := strings.Join(fields[0:5], " ")
			command := strings.Join(fields[5:], " ")
			jobs = append(jobs, Job{
				Schedule: schedule,
				Command:  command,
				Source:   path,
			})
		}
	}

	return jobs
}

// List reads all cron sources and prints jobs in a clean table
func List() error {
	sources := cronSources()

	var allJobs []Job
	for _, src := range sources {
		jobs := parseFile(src)
		allJobs = append(allJobs, jobs...)
	}

	bold := color.New(color.Bold)
	dim := color.New(color.FgHiBlack)
	cyan := color.New(color.FgCyan, color.Bold)

	fmt.Println()
	cyan.Println("  Scheduled Jobs")
	fmt.Println("  " + strings.Repeat("─", 80))

	if len(allJobs) == 0 {
		dim.Println("  No cron jobs found.")
		fmt.Println()
		return nil
	}

	// print header
	bold.Printf("  %-25s  %-10s  %-35s  %s\n", "SCHEDULE", "USER", "COMMAND", "SOURCE")
	fmt.Println("  " + strings.Repeat("─", 80))

	for _, job := range allJobs {
		// truncate long commands for display
		cmd := job.Command
		if len(cmd) > 50 {
			cmd = cmd[:47] + "..."
		}

		// truncate source path for display
		src := job.Source
		src = strings.TrimPrefix(src, "/var/spool/cron/crontabs/")
		src = strings.TrimPrefix(src, "/var/spool/cron/")
		src = strings.TrimPrefix(src, "/etc/cron.d/")
		if job.Source == "/etc/crontab" {
			src = "/etc/crontab"
		} else if strings.HasPrefix(job.Source, "/etc/cron.d/") {
			src = "cron.d/" + src
		} else {
			src = "crontabs/" + src
		}

		user := job.User
		if user == "" {
			// for user crontabs, derive username from filename
			user = filepath.Base(job.Source)
		}

		fmt.Printf("  %-25s  %-10s  %-50s\n", job.Schedule, user, cmd)
		dim.Printf("  %-25s  %-10s  source: %s\n", "", "", src)
		fmt.Println()
	}

	fmt.Println("  " + strings.Repeat("─", 80))
	if len(allJobs) == 1 {
		bold.Printf("  %d job found\n", len(allJobs))
	} else {
		bold.Printf("  %d jobs found\n", len(allJobs))
	}
	fmt.Println()

	return nil
}
