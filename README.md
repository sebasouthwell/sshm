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

# Optional: Add short alias and shell wrapper functions
# Add to your ~/.bashrc or ~/.zshrc:
alias sm=sshm

# Shell wrapper for cd command (enables direct directory changes)
sshm-cd() {
    local dir
    dir=$(sshm cd "$@" 2>/dev/null) && cd "$dir"
}
```

**Note**: The `cd` command prints the directory path for shell evaluation. Use `eval "$(sshm cd alias)"` or the `sshm-cd` wrapper function above to actually change directories.

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
# Quick connect (multiple ways)
sshm ssh.prod-web
sshm docker.myapp
sshm prod-web              # Direct alias access

# Or use open command
sshm open prod-web

# Interactive selection (default action)
sshm                       # Launches UI
sshm ui                    # Explicit UI command

# With short alias (if configured)
sm prod-web                # Same as sshm prod-web
sm                         # Same as sshm (launches UI)
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

### Core Commands
- `sshm` or `sshm ui` - Interactive TUI selection (default action)
- `sshm <alias>` - Direct alias access (opens connection)
- `sshm <provider>.<alias>` - Quick connect with provider
- `sshm open <alias>` - Open a session
- `sshm ls` - List entries
- `sshm show <alias>` - Show entry details
- `sshm add` - Add entry
- `sshm rm <alias>` - Remove entry
- `sshm edit` - Edit inventory files
- `sshm cd <alias>` - Print entry's workdir (use `eval "$(sshm cd alias)"` or `sshm-cd alias` wrapper function to change directory)
- `sshm test <alias>` - Test connection
- `sshm history` - Show command history

### File Transfer
- `sshm scp <alias> <src> <dst>` - Copy files via SCP (SSH/TF only)

### Inventory Management
- `sshm export [file]` - Export inventory to archive
- `sshm import <file>` - Import inventory from archive
- `sshm migrate [--dry-run]` - Migrate legacy .inv files to JSON format

### Terraform Integration
- `sshm tf` - Interactive terraform add wizard
- `sshm tf list [--details] [--fzf]` - List terraform resources
- `sshm tf add [args...]` - Add terraform entry

### Provider Convenience
- `sshm ssh <alias>` - Force SSH provider
- `sshm tf <alias>` - Force Terraform provider
- `sshm ssm <alias>` - Force SSM provider
- `sshm docker <alias>` - Force Docker provider
- `sshm kube <alias>` - Force Kubernetes provider

### Utilities
- `sshm cache clear` - Clear caches
- `sshm cache stats` - Show cache statistics

## Inventory Format

Entries are stored in `~/.ssh/inventory.d/*.json` files (JSON format):

```json
[
  {
    "alias": "prod-web",
    "type": "ssh",
    "target": "host.example.com",
    "user": "ubuntu",
    "port": "22",
    "key": "/home/user/.ssh/key.pem",
    "workdir": "/home/user/work",
    "tags": "prod,web",
    "meta": {
      "jump": "bastion",
      "strict": "yes"
    }
  },
  {
    "alias": "tf-web",
    "type": "tf",
    "target": "module.web.instance:public",
    "user": "ubuntu",
    "port": "22",
    "key": "/home/user/.ssh/key.pem",
    "workdir": "/home/user/terraform",
    "tags": "prod,terraform",
    "meta": {}
  }
]
```

**Fields:**
- `alias` (required): Unique identifier, regex `^[a-zA-Z0-9._-]+$`
- `type` (required): Provider type (`ssh`, `tf`, `ssm`, `docker`, `kube`)
- `target` (required): Provider-specific target string
- `user` (optional): Default user
- `port` (optional): Default port
- `key` (optional): Key path (required for SSH/TF)
- `workdir` (optional): Working directory
- `tags` (optional): Comma-separated tags
- `meta` (optional): Provider-specific metadata (key-value map)

**Legacy Support:**
- Old `.inv` files (tab-separated format) are automatically migrated to JSON on first load
- Legacy files are backed up as `.inv.bak` after migration
- Use `sshm migrate` to manually migrate all legacy files

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
- Default inventory file: `default.json` (override with `SSHM_DEFAULT_FILEBASE`)
- History file: `~/.ssh/inventory.d/history.jsonl`
- Cache directory: `$TMPDIR/sshm-*`

**Note:** Legacy `.inv` files are automatically migrated to `.json` format on first load. Use `sshm migrate` to manually migrate all files.

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
