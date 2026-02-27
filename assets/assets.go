// Package assets embeds all static files into the neurader binary so it is
// fully self-contained. No external files need to be distributed alongside it.
package assets

import _ "embed"

//go:embed callback/neurader_callback.py
var callbackPlugin []byte

//go:embed systemd/neurader-cleanup.service
var systemdService []byte

//go:embed systemd/neurader-cleanup.timer
var systemdTimer []byte

//go:embed grafana/dashboard.json
var grafanaDashboard []byte

// CallbackPlugin returns the Ansible callback plugin Python source.
func CallbackPlugin() ([]byte, error) { return callbackPlugin, nil }

// SystemdService returns the systemd one-shot service unit file.
func SystemdService() ([]byte, error) { return systemdService, nil }

// SystemdTimer returns the systemd timer unit file.
func SystemdTimer() ([]byte, error) { return systemdTimer, nil }

// GrafanaDashboard returns the pre-built Grafana dashboard JSON.
func GrafanaDashboard() ([]byte, error) { return grafanaDashboard, nil }