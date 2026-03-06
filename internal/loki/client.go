package loki

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"neurader/internal/config"
)

// client sends log streams to Loki's push API.
// Works with any Loki endpoint — bare EC2, k8s service, Grafana Cloud.
type client struct {
	pushURL string       // e.g. http://loki:3100/loki/api/v1/push
	http    *http.Client
	// optional basic auth for Grafana Cloud Loki
	username string
	password string
}

func newClient(cfg config.Config) *client {
	endpoint := strings.TrimRight(cfg.LokiEndpoint, "/")
	return &client{
		pushURL:  endpoint + "/loki/api/v1/push",
		http:     &http.Client{Timeout: 15 * time.Second},
		username: cfg.LokiUsername,
		password: cfg.LokiPassword,
	}
}

// push sends a stream payload to Loki.
func (c *client) push(payload lokiPushPayload) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshalling loki payload: %w", err)
	}

	req, err := http.NewRequest("POST", c.pushURL, bytes.NewBuffer(data))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")

	// Basic auth — used by Grafana Cloud and some k8s setups
	if c.username != "" && c.password != "" {
		req.SetBasicAuth(c.username, c.password)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("POST %s: %w", c.pushURL, err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	// Loki returns 204 No Content on success
	if resp.StatusCode != 204 && resp.StatusCode != 200 {
		return fmt.Errorf("loki returned HTTP %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// Ping checks if Loki is reachable by hitting /ready endpoint.
func Ping(cfg config.Config) error {
	endpoint := strings.TrimRight(cfg.LokiEndpoint, "/")
	url := endpoint + "/ready"

	hc := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}

	if cfg.LokiUsername != "" && cfg.LokiPassword != "" {
		req.SetBasicAuth(cfg.LokiUsername, cfg.LokiPassword)
	}

	resp, err := hc.Do(req)
	if err != nil {
		return fmt.Errorf("cannot reach Loki at %s: %w", cfg.LokiEndpoint, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("Loki health check returned HTTP %d", resp.StatusCode)
	}
	return nil
}