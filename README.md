# NeuRader

> Ansible Execution Monitor — automatically logs every playbook run with per-host results, full error capture, and optional Grafana integration.

No daemons. No ports. No agents on managed nodes. Just drop a binary on your Ansible controller and every `ansible-playbook` run is logged automatically.

---

## How it works

```
ansible-playbook site.yml
        ↓
neurader_callback.py (loaded by Ansible automatically)
        ↓
/var/log/neurader/site.yml_2025-01-24_14-30-00.json
        ↓
neurader list  →  neurader show  →  Grafana dashboard
```

For every playbook run, neurader captures:

- Per-host status — success, failed, or unreachable
- Full error output for failed hosts — msg, stdout, stderr, rc, module name
- Task summary per host — ok, changed, failed, skipped counts
- Start time, end time, total hosts

---

## Installation

```bash
# detect your architecture and download
curl -L https://neurader.operman.in/neurader/releases/latest/neurader-linux-amd64 -o neurader
chmod +x neurader
sudo mv neurader /usr/local/bin/neurader

# run the setup wizard
sudo neurader init
```

### Download by architecture

| Binary | Architecture | Runs on |
|---|---|---|
| `neurader-linux-amd64` | Intel / AMD 64-bit | Most servers, cloud VMs, AWS EC2, GCP, Azure |
| `neurader-linux-arm64` | ARM 64-bit | AWS Graviton, GCP Tau T2A, Raspberry Pi 4/5, Oracle Ampere |
| `neurader-linux-arm`   | ARM 32-bit | Raspberry Pi 2/3, older embedded ARM controllers |

Direct links:

```
https://neurader.operman.in/neurader/releases/latest/neurader-linux-amd64
https://neurader.operman.in/neurader/releases/latest/neurader-linux-arm64
https://neurader.operman.in/neurader/releases/latest/neurader-linux-arm
```

### Verify checksum

```bash
curl -L https://neurader.operman.in/neurader/releases/latest/checksums.sha256 -o checksums.sha256
sha256sum -c checksums.sha256
```

### Supported Linux distributions

Works on any distro where Ansible is installed — regardless of how it was installed:

| Install method | Supported |
|---|---|
| `apt install ansible` | ✅ Ubuntu, Debian |
| `dnf install ansible` | ✅ RHEL, CentOS Stream, Rocky, AlmaLinux, Fedora, Amazon Linux 2023 |
| `yum install ansible` | ✅ Amazon Linux 2, older RHEL/CentOS |
| `pip install ansible` | ✅ User install (`~/.local`) and system install |
| `pipx install ansible` | ✅ |
| `conda install ansible` | ✅ |

---

## Setup

```bash
sudo neurader init
```

The setup wizard detects your environment automatically and handles everything:

- Finds your Ansible installation — binary, Python interpreter, and plugin directory
- Creates `/var/log/neurader/` with correct permissions
- Creates `/etc/neurader/` and writes `neurader.conf`
- Installs `neurader_callback.py` into the correct Ansible callback directory
- Patches `ansible.cfg` to enable the callback
- Installs a systemd timer (or cron fallback) for automatic log cleanup
- Optionally connects to Grafana and imports a pre-built dashboard

You never touch a config file or edit `ansible.cfg` manually.

---

## Commands

| Command | Root | Description |
|---|---|---|
| `neurader init` | ✅ | Setup wizard — run once after install |
| `neurader list` | ❌ | List all recorded playbook runs, newest first |
| `neurader show <file>` | ❌ | Show detailed per-host result of a specific run |
| `neurader status` | ❌ | Show current config and health check |
| `neurader push` | ❌ | Backfill all existing logs to Grafana |
| `neurader grafana-setup` | ❌ | Create Grafana datasource and import dashboard |
| `neurader clean` | ❌ | Delete logs older than retention period |
| `neurader reset-callback` | ✅ | Restore default callback plugin |
| `neurader uninstall` | ✅ | Remove neurader completely |
| `neurader version` | ❌ | Print binary version |

---

## Usage

After `neurader init` runs once, everything is automatic:

```bash
# just run your playbooks as normal
ansible-playbook site.yml

# then check what happened
neurader list
```

Example `neurader list` output:

```
  FILE                              PLAYBOOK    HOSTS   STATUS    STARTED
  site.yml_2025-01-24_14-30-00     site.yml    3       success   2025-01-24 14:30
  site.yml_2025-01-23_09-15-42     site.yml    3       failed    2025-01-23 09:15
  deploy.yml_2025-01-22_18-00-01   deploy.yml  5       success   2025-01-22 18:00
```

Drill into a specific run:

```bash
neurader show site.yml_2025-01-23_09-15-42.json
```

```
  Playbook : site.yml
  Started  : 2025-01-23 09:15:42
  Ended    : 2025-01-23 09:16:01
  Hosts    : 3 total

  ✓ node1    success   (ok=8 changed=2 skipped=0)
  ✓ node3    success   (ok=8 changed=2 skipped=0)
  ✗ node2    FAILED
    Module : ansible.builtin.command
    Msg    : Permission denied
    Stderr : sudo: a password is required
    RC     : 1
```

---

## Log format

Logs are written to `/var/log/neurader/<playbook>_<date>_<time>.json`:

```json
{
  "playbook":    "site.yml",
  "start_time":  "2025-01-24T14:29:45Z",
  "end_time":    "2025-01-24T14:30:02Z",
  "total_hosts": 3,
  "hosts": {
    "node1": {
      "status": "success",
      "error_output": null,
      "summary": { "ok": 12, "failures": 0, "changed": 3, "unreachable": 0, "skipped": 1 }
    },
    "node2": {
      "status": "failed",
      "error_output": {
        "msg":    "Permission denied",
        "stdout": "",
        "stderr": "sudo: a password is required",
        "rc":     1,
        "module": "ansible.builtin.command"
      },
      "summary": { "ok": 5, "failures": 1, "changed": 0, "unreachable": 0, "skipped": 0 }
    },
    "node3": {
      "status": "unreachable",
      "error_output": {
        "msg": "Failed to connect to the host via ssh: Connection timed out"
      },
      "summary": { "ok": 0, "failures": 0, "changed": 0, "unreachable": 1, "skipped": 0 }
    }
  }
}
```

---

## Grafana integration

neurader can push logs to a Grafana instance automatically after every playbook run.

**One-time setup:**

```bash
# add Grafana details during init, or edit the config directly
sudo nano /etc/neurader/neurader.conf

# create datasource and import the pre-built dashboard
neurader grafana-setup

# backfill any runs that happened before Grafana was connected
neurader push
```

**After setup — fully automatic.** Every `ansible-playbook` run pushes to Grafana without any manual steps.

---

## Configuration

`/etc/neurader/neurader.conf`:

```json
{
  "retention_days":   3,
  "grafana_endpoint": "http://grafana.company.com:3000",
  "grafana_api_key":  "eyJr...",
  "grafana_org_id":   "1",
  "log_dir":          "/var/log/neurader",
  "callback_dir":     "/home/ec2-user/.local/lib/python3.12/site-packages/ansible/plugins/callback",
  "ansible_cfg_path": "/etc/ansible/ansible.cfg"
}
```

All fields are written automatically by `neurader init`. Edit directly to change Grafana credentials or log retention without re-running init.

---

## Customising the callback plugin

The callback plugin is plain Python installed at `<callback_dir>/neurader_callback.py`. Edit the `v2_playbook_on_stats` method to add or remove fields from the JSON payload.

To restore the default at any time:

```bash
sudo neurader reset-callback
```

---

## Updating

Just replace the binary — config, logs, and callback plugin are untouched:

```bash
curl -L https://neurader.operman.in/neurader/releases/latest/neurader-linux-amd64 -o neurader-new
chmod +x neurader-new
sudo mv neurader-new /usr/local/bin/neurader
neurader version
```

No need to re-run `neurader init` after an update.

---

## Uninstall

```bash
sudo neurader uninstall
```

Removes the callback plugin, reverts `ansible.cfg`, removes the systemd timer, and deletes `/etc/neurader/` and `/var/log/neurader/`.

---

## License

Apache License 2.0
