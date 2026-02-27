# neurader-integration

# Neurader

**Ansible Execution Monitor** — automatically logs every playbook run as structured JSON with per-host success/failure status, full error output capture, and optional Grafana dashboard integration.

## How it works

Every time `ansible-playbook` runs on your controller:

1. The callback plugin captures every host result in real time
2. A JSON log is written to `/var/log/neurader/<playbook>_<date>_<time>.json`
3. Each managed node is recorded as **success**, **failed**, or **unreachable**
4. Failed nodes include full error output — msg, stdout, stderr, rc, module name
5. `neurader post-run` fires silently in the background to clean old logs and push to Grafana

Zero ports opened. Zero daemons. No network exposure on the controller.

---

## Installation

```bash
curl -fsSL https://neurader.operman.in/releases/install.sh | sudo bash
sudo neurader init
```

The install script auto-detects your architecture and downloads the correct binary.

### Supported architectures

| Binary | Runs on |
|---|---|
| `neurader-linux-amd64` | Intel/AMD servers, most VMs, cloud instances |
| `neurader-linux-arm64` | AWS Graviton, GCP Tau T2A, Raspberry Pi 4/5, Oracle Ampere |
| `neurader-linux-arm`   | Raspberry Pi 2/3, older embedded ARM controllers |

### Supported Linux distros

Ubuntu, Debian, RHEL, CentOS Stream, Rocky Linux, AlmaLinux, Fedora,
Amazon Linux 2/2023, openSUSE, Arch Linux, Alpine Linux —
and any distro with Ansible installed via `apt`, `dnf`, `yum`, `pip`, or `pipx`.

---

## Setup

```bash
sudo neurader init
```

The wizard handles everything automatically:
- Detects your Ansible install and callback plugin directory
- Creates `/var/log/neurader/` and `/etc/neurader/`
- Writes `/etc/neurader/neurader.conf`
- Installs `neurader_callback.py` into the correct Ansible plugin directory
- Patches `ansible.cfg` to enable the callback
- Installs a systemd timer (or cron job) for automatic log cleanup
- Optionally creates a Grafana datasource and imports the pre-built dashboard

You never touch a config file or edit `ansible.cfg` manually.

---

## CLI Commands

| Command | Needs Root | Description |
|---|---|---|
| `neurader init` | ✅ | Full setup wizard — run once after install |
| `neurader list` | ❌ | List all recorded playbook runs (newest first) |
| `neurader show <file>` | ❌ | Show detailed per-host result of a run |
| `neurader clean` | ❌ | Delete logs older than retention period |
| `neurader push` | ❌ | Push all logs to Grafana |
| `neurader grafana-setup` | ❌ | Create datasource and import dashboard in Grafana |
| `neurader status` | ❌ | Show current config and health |
| `neurader reset-callback` | ✅ | Restore default callback plugin |
| `neurader uninstall` | ✅ | Remove neurader completely |
| `neurader version` | ❌ | Print binary version |

---

## Log format

Each run produces a file like `/var/log/neurader/site.yml_2025-01-24_14-30-00.json`:

```json
{
  "playbook":    "site.yml",
  "start_time":  "2025-01-24T14:29:45Z",
  "end_time":    "2025-01-24T14:30:02Z",
  "total_hosts": 3,
  "hosts": {
    "web01": {
      "status": "success",
      "error_output": null,
      "summary": { "ok": 12, "failures": 0, "changed": 3, "unreachable": 0, "skipped": 1 }
    },
    "db01": {
      "status": "failed",
      "error_output": {
        "msg":    "Permission denied",
        "stdout": "",
        "stderr": "sudo: a password is required",
        "rc":     1,
        "module": "ansible.builtin.command"
      },
      "summary": { "ok": 5, "failures": 1, "changed": 0, "unreachable": 0, "skipped": 0 }
    }
  }
}
```

---

## Customising the log format

The callback plugin is plain Python installed at:
```
<callback_dir>/neurader_callback.py
```

Edit the `v2_playbook_on_stats` method to add or remove fields from the JSON payload.
To restore the default at any time:

```bash
sudo neurader reset-callback
```

---

## Configuration

`/etc/neurader/neurader.conf` (written by `neurader init`, mode 0600):

```json
{
  "retention_days":   3,
  "grafana_endpoint": "http://grafana.company.com:3000",
  "grafana_api_key":  "eyJr...",
  "grafana_org_id":   "1",
  "log_dir":          "/var/log/neurader",
  "callback_dir":     "/usr/lib/python3/dist-packages/ansible/plugins/callback",
  "ansible_cfg_path": "/etc/ansible/ansible.cfg"
}
```

---

## Building from source

```bash
git clone <your-private-repo>
cd neurader
go mod tidy

# Build for current machine
make build

# Cross-compile all 3 architectures
make all
```

Requires Go 1.22+.

---

## License

Apache License Version 2.0