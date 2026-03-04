# NeuRader

> Ansible Execution Monitor — automatically captures every playbook run as structured JSON with per-host results, full error output, and optional Grafana integration.

---

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                        Ansible Controller                       │
│                                                                 │
│   ansible-playbook site.yml                                     │
│          │                                                      │
│          ▼                                                      │
│   ┌─────────────────────────────────────────────┐               │
│   │           Ansible Callback System            │              │
│   │                                              │              │
│   │   ansible.cfg                                │              │
│   │   callbacks_enabled = neurader  ◄── patched  │              │
│   │            │                   by init       │              │
│   │            ▼                                 │              │
│   │   neurader_callback.py                       │              │
│   │   (Python plugin, auto-loaded by Ansible)    │              │
│   └─────────────────────────────────────────────┘               │
│          │                                                      │
│          │  captures every host result in real time             │
│          ▼                                                      │
│   /var/log/neurader/                                            │
│   └── site.yml_2025-01-24_14-30-00.json                         │
│                                                                 │
│   neurader binary (/usr/local/bin/neurader)                     │
│   ├── reads logs  →  neurader list / show / staus               │
│   └── pushes logs →  Grafana (optional)                         │
└─────────────────────────────────────────────────────────────────┘
          │  SSH                │  SSH                │  SSH
          ▼                     ▼                     ▼
   ┌────────────┐       ┌────────────┐       ┌────────────┐
   │  node1     │       │  node2     │       │  node3     │
   │  managed   │       │  managed   │       │  managed   │
   │  node      │       │  node      │       │  node      │
   │            │       │            │       │            │
   │ no neurader│       │ no neurader│       │ no neurader│
   │ no agents  │       │ no agents  │       │ no agents  │
   └────────────┘       └────────────┘       └────────────┘
```

neurader lives entirely on the **controller**. Managed nodes have no awareness of it.

---

## Components

### neurader binary
A single statically linked Go binary at `/usr/local/bin/neurader`. It sets up the environment via `neurader init` and provides CLI commands to read and push logs. It never runs as a daemon and opens no ports.

### neurader_callback.py
A Python callback plugin that Ansible loads automatically before every playbook run. It hooks into Ansible's internal event system and captures every host result in real time. When the playbook finishes it writes a structured JSON file to `/var/log/neurader/`. It runs inside the Ansible process — no subprocess, no network calls, no side effects.

### /var/log/neurader/
The log directory where every playbook run is stored as a JSON file. Files are named `<playbook>_<date>_<time>.json` and retained for a configurable number of days. The callback plugin writes here directly — the neurader binary reads from here.

### /etc/neurader/neurader.conf
The configuration file written by `neurader init`. Stores log retention, Grafana credentials, and detected paths for the callback directory and ansible.cfg.

---

## Data flow

```
ansible-playbook runs
        │
        ▼
Ansible loads neurader_callback.py from callback_dir
        │
        ▼
v2_runner_on_ok / v2_runner_on_failed / v2_runner_on_unreachable
capture each host result as playbook executes
        │
        ▼
v2_playbook_on_stats fires when playbook completes
        │
        ▼
JSON written to /var/log/neurader/<playbook>_<date>_<time>.json
        │
        ├──► neurader list     reads log index
        ├──► neurader show     reads specific log
        └──► neurader push     sends to Grafana API
```

---

## Log structure

Every run produces one JSON file:

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

## Ansible compatibility

neurader uses the standard Ansible callback interface that has been stable since Ansible 2.0. `neurader init` detects the install method automatically and places the callback plugin in the correct directory.

| Install method | Controller OS | Supported |
|---|---|---|
| `apt install ansible` | Ubuntu, Debian | ✅ |
| `dnf install ansible` | RHEL, Rocky, AlmaLinux, Fedora, Amazon Linux 2023 | ✅ |
| `yum install ansible` | Amazon Linux 2, older RHEL/CentOS | ✅ |
| `pip install ansible` | Any Linux | ✅ |
| `pipx install ansible` | Any Linux | ✅ |
| `conda install ansible` | Any Linux | ✅ |

---

## Binary compatibility

neurader is a statically linked Go binary — it carries all dependencies inside itself and requires no runtime, no Python, and no system libraries. The same binary runs on Ubuntu 20, Ubuntu 24, Amazon Linux, Alpine, Arch, and any other Linux distro.

| Binary | CPU | Common machines |
|---|---|---|
| `neurader-linux-amd64` | x86-64 | Most servers, AWS EC2, GCP, Azure VMs |
| `neurader-linux-arm64` | ARM 64-bit | AWS Graviton, GCP Tau T2A, Raspberry Pi 4/5 |
| `neurader-linux-arm`   | ARM 32-bit | Raspberry Pi 2/3, older embedded controllers |

---

## What neurader does not do

- Does not run on managed nodes — controller only
- Does not open any ports or expose any API
- Does not run as a background daemon or service
- Does not modify playbooks or inventory
- Does not require changes to how you run `ansible-playbook`
- Does not touch managed nodes in any way

---

## License

Apache License 2.0

