package alert

import (
	"crypto/tls"
	"fmt"
	"net/smtp"
	"strconv"
	"strings"

	"neurader/internal/config"
)

func sendEmail(cfg config.Config, e Event) error {
	port, err := strconv.Atoi(cfg.EmailSMTPPort)
	if err != nil || port == 0 {
		port = 587
	}

	subject := fmt.Sprintf("[NeuRader] %s — %d host(s) failed", e.Playbook, len(e.FailedHosts))
	if e.Success {
		subject = fmt.Sprintf("[NeuRader] %s — succeeded", e.Playbook)
	}

	body := buildEmailBody(e)

	msg := "From: " + cfg.EmailFrom + "\r\n" +
		"To: " + cfg.EmailTo + "\r\n" +
		"Subject: " + subject + "\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/plain; charset=UTF-8\r\n" +
		"\r\n" +
		body

	addr := fmt.Sprintf("%s:%d", cfg.EmailSMTPHost, port)
	auth := smtp.PlainAuth("", cfg.EmailFrom, cfg.EmailPassword, cfg.EmailSMTPHost)

	// Try STARTTLS first (port 587), fall back to plain TLS (port 465)
	if port == 465 {
		tlsCfg := &tls.Config{ServerName: cfg.EmailSMTPHost}
		conn, err := tls.Dial("tcp", addr, tlsCfg)
		if err != nil {
			return fmt.Errorf("email: tls dial: %w", err)
		}
		client, err := smtp.NewClient(conn, cfg.EmailSMTPHost)
		if err != nil {
			return fmt.Errorf("email: smtp client: %w", err)
		}
		defer client.Close()
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("email: auth: %w", err)
		}
		if err := client.Mail(cfg.EmailFrom); err != nil {
			return fmt.Errorf("email: mail from: %w", err)
		}
		if err := client.Rcpt(cfg.EmailTo); err != nil {
			return fmt.Errorf("email: rcpt to: %w", err)
		}
		w, err := client.Data()
		if err != nil {
			return fmt.Errorf("email: data: %w", err)
		}
		if _, err = fmt.Fprint(w, msg); err != nil {
			return fmt.Errorf("email: write: %w", err)
		}
		return w.Close()
	}

	// STARTTLS (port 587)
	if err := smtp.SendMail(addr, auth, cfg.EmailFrom, []string{cfg.EmailTo}, []byte(msg)); err != nil {
		return fmt.Errorf("email: send: %w", err)
	}
	return nil
}

func buildEmailBody(e Event) string {
	var sb strings.Builder

	if e.Success {
		sb.WriteString(fmt.Sprintf("NeuRader — Playbook Succeeded\n"))
		sb.WriteString(strings.Repeat("─", 50) + "\n\n")
		sb.WriteString(fmt.Sprintf("Playbook : %s\n", e.Playbook))
		sb.WriteString(fmt.Sprintf("Time     : %s\n", e.StartTime))
		sb.WriteString(fmt.Sprintf("Hosts    : %d (all succeeded)\n", e.TotalHosts))
		return sb.String()
	}

	sb.WriteString("NeuRader — Playbook Failed\n")
	sb.WriteString(strings.Repeat("─", 50) + "\n\n")
	sb.WriteString(fmt.Sprintf("Playbook     : %s\n", e.Playbook))
	sb.WriteString(fmt.Sprintf("Time         : %s\n", e.StartTime))
	sb.WriteString(fmt.Sprintf("Total Hosts  : %d\n", e.TotalHosts))
	sb.WriteString(fmt.Sprintf("Failed Hosts : %d\n\n", len(e.FailedHosts)))

	for _, h := range e.FailedHosts {
		sb.WriteString(fmt.Sprintf("Host: %s [%s]\n", h.Name, strings.ToUpper(h.Status)))
		sb.WriteString(strings.Repeat("·", 40) + "\n")
		for i, t := range h.FailedTasks {
			sb.WriteString(fmt.Sprintf("  Failed Task %d of %d\n", i+1, len(h.FailedTasks)))
			sb.WriteString(fmt.Sprintf("  Task   : %s\n", t.TaskName))
			sb.WriteString(fmt.Sprintf("  Module : %s\n", t.Module))
			sb.WriteString(fmt.Sprintf("  RC     : %d\n", t.RC))
			if t.Msg != "" {
				sb.WriteString(fmt.Sprintf("  Error  : %s\n", t.Msg))
			}
			if t.Stderr != "" {
				sb.WriteString(fmt.Sprintf("  Stderr : %s\n", t.Stderr))
			}
			sb.WriteString("\n")
		}
	}

	sb.WriteString(strings.Repeat("─", 50) + "\n")
	sb.WriteString("Run `neurader show " + e.Playbook + "` for full details.\n")
	sb.WriteString("Sent by NeuRader — Ansible Execution Monitor\n")
	return sb.String()
}
