package grafana

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

// client wraps authenticated HTTP calls to the Grafana REST API.
type client struct {
	baseURL string
	apiKey  string
	orgID   string
	http    *http.Client
}

func newClient(cfg config.Config) *client {
	return &client{
		baseURL: strings.TrimRight(cfg.GrafanaEndpoint, "/"),
		apiKey:  cfg.GrafanaAPIKey,
		orgID:   cfg.GrafanaOrgID,
		http:    &http.Client{Timeout: 15 * time.Second},
	}
}

func (c *client) post(path string, body interface{}) ([]byte, int, error) {
	return c.do("POST", path, body)
}

func (c *client) get(path string) ([]byte, int, error) {
	return c.do("GET", path, nil)
}

func (c *client) do(method, path string, body interface{}) ([]byte, int, error) {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, 0, fmt.Errorf("marshalling request body: %w", err)
		}
		bodyReader = bytes.NewBuffer(data)
	}

	req, err := http.NewRequest(method, c.baseURL+path, bodyReader)
	if err != nil {
		return nil, 0, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	if c.orgID != "" {
		req.Header.Set("X-Grafana-Org-Id", c.orgID)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("HTTP %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	return respBody, resp.StatusCode, nil
}