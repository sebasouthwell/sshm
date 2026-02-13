# SSHM v2 - Dynamic Multi-Environment Session Manager

SSHM is a cross-environment session launcher and inventory manager that allows DevOps engineers to open interactive shells into many target types (VMs, cloud instances via SSM, containers, pods) with a unified interface.

## Features

- **Multiple Providers**: SSH, Terraform, AWS SSM, Docker, Kubernetes
- **Dynamic Resolution**: Terraform addresses → IPs, SSM tag selectors → instance IDs
- **Interactive TUI**: Fast selection with fzf integration
- **Quick Connect**: `sshm <provider>.<alias>` syntax
- **History Tracking**: JSONL-based event logging
- **Inventory Management**: Add, remove, list, and edit entries

## Installation

```bash
git clone https://github.com/Sebasouthwell/sshm.git
cd sshm
go build -o sshm cmd/sshm/main.go
sudo mv sshm /usr/local/bin/
```

## Quick Start

### 1. Add an entry

```bash
# SSH entry
sshm add prod-web ssh host.example.com ubuntu 22 ~/.ssh/key.pem ~/work prod,web

# Terraform entry
sshm add tf-web tf module.web.instance:public ubuntu 22 ~/.ssh/key.pem ~/terraform prod

# SSM entry
sshm add ssm-prod ssm i-1234567890abcdef0

# Docker entry
sshm add docker-app docker myapp-container

# Kubernetes entry
sshm add kube-pod kube default/myapp-pod-abc123
```

### 2. Connect

```bash
# Quick connect
sshm ssh.prod-web
sshm docker.myapp

# Or use open command
sshm open prod-web

# Interactive selection
sshm ui
```

### 3. List entries

```bash
# List all
sshm ls

# Filter
sshm ls prod
sshm ls --tag web

# JSON output
sshm ls --json
```

## Commands

- `sshm open <alias>` - Open a session
- `sshm <provider>.<alias>` - Quick connect
- `sshm ui` - Interactive TUI selection
- `sshm ls` - List entries
- `sshm show <alias>` - Show entry details
- `sshm add` - Add entry
- `sshm rm <alias>` - Remove entry
- `sshm edit` - Edit inventory files
- `sshm cd <alias>` - Change to entry's workdir
- `sshm history` - Show command history
- `sshm cache clear` - Clear caches
- `sshm cache stats` - Show cache statistics

## Inventory Format

Entries are stored in `~/.ssh/inventory.d/*.inv` files (tab-separated):

```
alias<TAB>type<TAB>target<TAB>user<TAB>port<TAB>key<TAB>workdir<TAB>tags<TAB>meta
```

**Example:**
```
prod-web	ssh	host.example.com	ubuntu	22	~/.ssh/key.pem	~/work	prod,web	jump=bastion;strict=yes
```

## Providers

### SSH (`ssh`)
Direct SSH connections with jump host support.

**Meta keys:**
- `jump=<alias|host>` - Bastion jump host
- `strict=<yes|no|accept-new>` - Host key checking
- `sshopt=<csv>` - Extra SSH options

### Terraform (`tf`)
Terraform-backed SSH with IP resolution.

**Target format:** `<terraform_address>:<public|private>`

**Example:** `module.web.aws_instance.app:public`

### AWS SSM (`ssm`)
AWS Systems Manager sessions.

**Target format:**
- Instance ID: `i-1234567890abcdef0`
- Tag selector: `tag:Name=prod-web`

**Meta keys:**
- `profile=<aws_profile>` - AWS profile
- `region=<aws_region>` - AWS region
- `pf=<localPort:remoteHost:remotePort>` - Port forwarding

### Docker (`docker`)
Docker container exec.

**Meta keys:**
- `shell=<shell>` - Shell to use (default: /bin/sh)
- `context=<context>` - Docker context
- `env=<var1,var2>` - Environment variables

### Kubernetes (`kube`)
Kubernetes pod exec.

**Target format:**
- Pod name: `myapp-pod-abc123`
- Namespace/pod: `default/myapp-pod-abc123`

**Meta keys:**
- `namespace=<namespace>` - Kubernetes namespace
- `container=<container>` - Container name
- `context=<context>` - Kubernetes context

## Token Syntax

Tokens allow runtime overrides:

```bash
# User override
sshm open prod-web user=admin

# Port override
sshm open prod-web p=2222

# Port forwarding (SSH)
sshm open prod-web L=8080:localhost:8080

# Passthrough arguments
sshm open prod-web -- -v

# Remote command
sshm open prod-web :: uptime
```

## Examples

```bash
# SSH with jump host
sshm open prod-web

# Terraform with private IP
sshm open tf-web

# SSM with tag selector
sshm add ssm-prod ssm tag:Name=prod-web profile=prod
sshm open ssm-prod first=true

# Docker exec
sshm docker.myapp shell=/bin/bash

# Kubernetes pod exec
sshm kube.myapp-pod container=app

# Port forwarding (SSM)
sshm open ssm-prod pf=5432:localhost:5432
```

## Configuration

- Inventory directory: `~/.ssh/inventory.d` (override with `SSHM_INV_DIR`)
- Default inventory file: `default.inv` (override with `SSHM_DEFAULT_FILEBASE`)
- History file: `~/.ssh/inventory.d/history.jsonl`
- Cache directory: `$TMPDIR/sshm-*`

## Requirements

- Go 1.13+ (for building)
- External tools (validated at runtime):
  - `ssh` (for SSH/TF providers)
  - `terraform` (for TF provider)
  - `aws` CLI + `session-manager-plugin` (for SSM provider)
  - `docker` (for Docker provider)
  - `kubectl` (for Kubernetes provider)
  - `fzf` (optional, for enhanced UI)

## License

MIT

## Contributing

Contributions welcome! Please open an issue or submit a pull request.
