package grafana

import (
	"encoding/json"
	"fmt"

	"neurader/internal/config"
)

const datasourceName = "Neurader"

// Setup creates the Neurader datasource and imports the dashboard.
// Safe to call multiple times — will not duplicate existing resources.
func Setup(cfg config.Config) error {
	c := newClient(cfg)

	// Verify Grafana is reachable
	_, status, err := c.get("/api/health")
	if err != nil {
		return fmt.Errorf("cannot reach Grafana at %s: %w", cfg.GrafanaEndpoint, err)
	}
	if status != 200 {
		return fmt.Errorf("Grafana health check returned HTTP %d", status)
	}

	uid, err := ensureDatasource(c)
	if err != nil {
		return fmt.Errorf("datasource: %w", err)
	}

	if err := importDashboard(c, uid); err != nil {
		return fmt.Errorf("dashboard: %w", err)
	}

	return nil
}

// ensureDatasource creates the Neurader datasource if it does not already
// exist and returns the datasource UID.
func ensureDatasource(c *client) (string, error) {
	// Check if already exists
	body, status, err := c.get("/api/datasources/name/" + datasourceName)
	if err != nil {
		return "", err
	}
	if status == 200 {
		var ds struct {
			UID string `json:"uid"`
		}
		if json.Unmarshal(body, &ds) == nil && ds.UID != "" {
			return ds.UID, nil
		}
	}

	// Create it using the SimpleJSON datasource type
	payload := map[string]interface{}{
		"name":      datasourceName,
		"type":      "simplejson",
		"access":    "proxy",
		"isDefault": false,
		"jsonData":  map[string]interface{}{},
	}

	respBody, status, err := c.post("/api/datasources", payload)
	if err != nil {
		return "", err
	}
	if status != 200 && status != 201 {
		return "", fmt.Errorf("creating datasource: HTTP %d — %s", status, string(respBody))
	}

	var result struct {
		Datasource struct {
			UID string `json:"uid"`
		} `json:"datasource"`
		UID string `json:"uid"` // older Grafana versions return at top level
	}
	json.Unmarshal(respBody, &result) //nolint:errcheck

	uid := result.Datasource.UID
	if uid == "" {
		uid = result.UID
	}
	return uid, nil
}