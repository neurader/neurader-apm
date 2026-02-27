package grafana

import (
	"encoding/json"
	"fmt"

	"neurader/assets"
)

// importDashboard imports the bundled Neurader dashboard into Grafana.
// Sets overwrite: true so re-running grafana-setup updates the dashboard safely.
func importDashboard(c *client, datasourceUID string) error {
	rawDash, err := assets.GrafanaDashboard()
	if err != nil {
		return fmt.Errorf("loading embedded dashboard: %w", err)
	}

	var dashObj map[string]interface{}
	if err := json.Unmarshal(rawDash, &dashObj); err != nil {
		return fmt.Errorf("parsing dashboard JSON: %w", err)
	}

	// Remove id so Grafana assigns a new one on first import
	delete(dashObj, "id")

	// Patch every panel's datasource uid to point at our datasource
	patchDatasourceUID(dashObj, datasourceUID)

	payload := map[string]interface{}{
		"dashboard": dashObj,
		"overwrite": true,
		"folderId":  0,
		"inputs": []map[string]interface{}{
			{
				"name":     "DS_NEURADER",
				"type":     "datasource",
				"pluginId": "simplejson",
				"value":    datasourceUID,
			},
		},
	}

	respBody, status, err := c.post("/api/dashboards/import", payload)
	if err != nil {
		return err
	}
	if status != 200 {
		return fmt.Errorf("importing dashboard: HTTP %d — %s", status, string(respBody))
	}

	return nil
}

// patchDatasourceUID walks the dashboard JSON and replaces any datasource
// uid placeholders with the real UID returned from Grafana.
func patchDatasourceUID(obj map[string]interface{}, uid string) {
	panels, ok := obj["panels"].([]interface{})
	if !ok {
		return
	}
	for _, p := range panels {
		panel, ok := p.(map[string]interface{})
		if !ok {
			continue
		}
		if ds, ok := panel["datasource"].(map[string]interface{}); ok {
			ds["uid"] = uid
		}
	}
}