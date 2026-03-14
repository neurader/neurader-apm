<div align="center">

<br/>

```
███╗   ██╗███████╗██╗   ██╗██████╗  █████╗ ██████╗ ███████╗██████╗
████╗  ██║██╔════╝██║   ██║██╔══██╗██╔══██╗██╔══██╗██╔════╝██╔══██╗
██╔██╗ ██║█████╗  ██║   ██║██████╔╝███████║██║  ██║█████╗  ██████╔╝
██║╚██╗██║██╔══╝  ██║   ██║██╔══██╗██╔══██║██║  ██║██╔══╝  ██╔══██╗
██║ ╚████║███████╗╚██████╔╝██║  ██║██║  ██║██████╔╝███████╗██║  ██║
╚═╝  ╚═══╝╚══════╝ ╚═════╝ ╚═╝  ╚═╝╚═╝  ╚═╝╚═════╝ ╚══════╝╚═╝  ╚═╝
```

**Lightweight Ansible execution monitoring. Without the AWX overhead.**

[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)
[![Go Version](https://img.shields.io/badge/go-1.22+-00ADD8.svg)](https://golang.org)
[![Version](https://img.shields.io/badge/version-v0.5.x-brightgreen.svg)]()
[![Org](https://img.shields.io/badge/org-neurader-1A1A2E.svg)](https://github.com/neurader)

<br/>

</div>

---

## The Problem

AWX and Ansible Tower are powerful. They are also heavy.

A minimum AWX deployment needs a `t3.large`. It needs a Kubernetes cluster or Docker Compose stack. It needs a PostgreSQL database, Redis, and a web server. You spend more time maintaining the monitoring infrastructure than the Ansible infrastructure it monitors.

For most teams running Ansible on a handful of nodes, this is too much.

**NeuRader runs on a `t3.micro`.**

---

## What is NeuRader

NeuRader is an open-source Ansible execution monitoring and alerting controller written in Go. It gives you full visibility into your Ansible runs — playbooks, hosts, tasks, failures — with a Grafana/Loki dashboard, a clean CLI, and Slack/PagerDuty/Jira alerting. No Kubernetes. No heavy stack. One binary.

```
                    ┌─────────────────────┐
                    │   NeuRader          │
                    │   Controller        │
                    │                     │
                    │  Grafana dashboard  │
                    │  Alerting           │
                    │  CLI                │
                    │  REST API           │
                    └────────┬────────────┘
                             │
          ┌──────────────────┼──────────────────┐
          │                  │                  │
   ┌──────▼──────┐   ┌───────▼─────┐   ┌───────▼─────┐
   │  Node 1     │   │  Node 2     │   │  Node 3     │
   │  NeuCell    │   │  NeuCell    │   │  NeuCell    │
   │  agent      │   │  agent      │   │  agent      │
   └─────────────┘   └─────────────┘   └─────────────┘
   cloud / on-prem / bare metal / VPS
```

---

## Features

### Execution Monitoring
- Full Ansible run history — every playbook, every host, every task
- Failed task capture with complete detail per host
- Sensitive key scrubbing — vault passwords and secrets never logged
- Run-centric Grafana/Loki dashboard with cascading dropdowns: **Playbook → Run → Host**

### Alerting (v0.5.x)
- **Slack** — run failure notifications with full context
- **PagerDuty** — incident creation on threshold breach
- **Jira** — automatic ticket creation on failure
- Fire-and-forget model — alerts dispatch without blocking execution

### CLI
```bash
neurader inventory    # view managed node inventory
neurader last         # show last playbook run result
neurader hosts        # list all hosts and their status
neurader ping         # ping all managed nodes
neurader show         # detailed view of a specific run
```

### NeuCell Integration (v1.0)
- NeuCell agent deployed as a Cell on every managed node
- Automatic Cell activation triggered by NeuRader on demand
- Threshold-based alerting from node metrics — CPU, memory, disk, TCP, FDs
- Scale-to-zero Cell management — Ansible, Terraform, and tool Cells activated only when needed

---

## Architecture

NeuRader is the **controller**. It runs on your management node and connects to all managed nodes via NeuCell agents.

```
NeuRader Controller
  │
  ├── Ansible callback plugin
  │     Captures run data in real time
  │     Pushes to NeuRader on every event
  │
  ├── Grafana + Loki
  │     Run-centric dashboard
  │     Playbook → Run → Host drill-down
  │
  ├── Alerting engine
  │     Slack / PagerDuty / Jira
  │     Fire-and-forget dispatch
  │
  └── NeuCell orchestration (v1.0)
        Activates / deactivates Cells on managed nodes
        Receives threshold breach events
        Triggers alerts from node metrics
```

---

## Why Not AWX

| | AWX / Tower | NeuRader |
|---|---|---|
| Minimum instance | t3.large (~$60/mo) | t3.micro (~$8/mo) |
| Dependencies | K8s or Docker Compose + Postgres + Redis | Single Go binary |
| Setup time | Hours | Minutes |
| Ansible callback | Built-in | Lightweight plugin |
| Dashboard | Built-in web UI | Grafana + Loki |
| Alerting | Notifications system | Slack / PagerDuty / Jira |
| Node agent | None | NeuCell |
| License | Apache 2.0 | Apache 2.0 |

NeuRader is not trying to replace AWX for large enterprise deployments. It is built for the developer or small team running Ansible on a VPS, EC2 instance, or bare metal server who wants visibility without operational overhead.

---

## Quick Start

### Install

```bash
curl -L https://neurader.cloud/install | sh
```

Or download the binary directly from [releases](https://github.com/neurader/neurader/releases).

### Configure

```yaml
# neurader.yaml
server:
  port: 8080
  host: "0.0.0.0"

loki:
  url: "http://localhost:3100"

alerting:
  slack:
    webhook: "https://hooks.slack.com/services/..."
    channel: "#infra-alerts"
  pagerduty:
    integration_key: "..."
  jira:
    url: "https://yourorg.atlassian.net"
    project: "OPS"
    token: "..."
```

### Ansible Callback

Add the NeuRader callback plugin to your Ansible project:

```ini
# ansible.cfg
[defaults]
callback_plugins   = ./callbacks
callbacks_enabled  = neurader
```

```bash
# Set controller URL
export NEURADER_URL=http://your-controller:8080
```

Run your playbook normally. NeuRader captures everything.

### Grafana Dashboard

Import the NeuRader dashboard from `dashboards/neurader.json`. The dashboard includes:

- Run timeline with status per playbook
- Per-run host breakdown
- Failed task detail with full output
- Cascading dropdowns: **Playbook → Run → Host**

---

## CLI Reference

```bash
neurader inventory              # list all hosts in managed inventory
neurader last                   # show result of the last playbook run
neurader last --playbook site   # last run for a specific playbook
neurader hosts                  # all hosts with current status
neurader ping                   # connectivity check across all nodes
neurader show <run-id>          # full detail for a specific run
neurader show --last            # full detail for the last run
```

---

## Alerting

NeuRader uses a **fire-and-forget** model for alerts. When a run fails or a threshold is breached, the alert is dispatched asynchronously — it never blocks the Ansible execution pipeline.

### Slack

```
[NeuRader] ❌ Playbook failed: site.yml
Run ID:    run-20240315-001
Host:      web-prod-01
Task:      Deploy application
Error:     Permission denied on /var/www/app
```

### PagerDuty

Incidents are created automatically with full run context. Resolved automatically when the next run succeeds on the same host.

### Jira

Tickets created in your configured project with playbook name, run ID, host, failed task, and full error output attached.

---

## NeuCell Integration

NeuRader v1.0 introduces full NeuCell integration — turning NeuRader into a complete infrastructure control plane.

With NeuCell deployed on managed nodes:

```bash
# From NeuRader controller — activate an Ansible Cell on a node
neurader cell activate --node web-prod-01 --cell ansible

# View Cell states across all nodes
neurader cell list

# Node metrics and threshold status
neurader metrics --node web-prod-01
```

Cells activate on demand and scale to zero when idle. The monitoring-cell on each node runs continuously, pushing threshold breach events to the NeuRader controller for alerting.

See **[NeuCell →](https://github.com/neurader/neucell)** for the full Cell runtime documentation.

---

## Roadmap

| Version | Scope |
|---|---|
| v0.4.0 ✅ | Grafana/Loki dashboard, callback upgrades, sensitive key scrubbing, `inventory` `last` `hosts` `ping` CLI commands |
| v0.5.x ✅ | Slack / PagerDuty / Jira alerting, fire-and-forget model, configurable log retention |
| v0.6.0 | REST API for external integrations, webhook support |
| v0.7.0 | Multi-controller federation, node groups |
| v1.0.0 | NeuCell full integration — Cell orchestration, node metrics, threshold alerting |

---

## Tech Stack

| Layer | Technology |
|---|---|
| Language | Go |
| Log storage | Loki |
| Dashboard | Grafana |
| Ansible integration | Callback plugin |
| Alerting | Slack / PagerDuty / Jira |
| Node agent | NeuCell |
| License | Apache 2.0 |

---

## Contributing

NeuRader is an independent open-source project. Issues, ideas, and pull requests are welcome.

```bash
git clone https://github.com/neurader/neurader
cd neurader
go build ./...
go test ./...
```

---

## License

Apache 2.0 — see [LICENSE](LICENSE)

---

<div align="center">

**[neurader.cloud](https://neurader.cloud)** · **[NeuRader](https://github.com/neurader/neurader)** · **[NeuCell](https://github.com/neurader/neucell)**

</div>
