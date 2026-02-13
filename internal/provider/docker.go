package provider

import (
	"os/exec"
	"strings"

	"github.com/Sebasouthwell/sshm/internal/inventory"
	"github.com/Sebasouthwell/sshm/internal/errors"
)

// DockerProvider implements the Provider interface for Docker container connections
type DockerProvider struct{}

// NewDockerProvider creates a new Docker provider
func NewDockerProvider() *DockerProvider {
	return &DockerProvider{}
}

// Name returns the provider name
func (p *DockerProvider) Name() string {
	return "docker"
}

// Supports returns true if the provider supports the given action
func (p *DockerProvider) Supports(action Action) bool {
	switch action {
	case ActionOpen, ActionCommand, ActionTest, ActionShow:
		return true
	default:
		return false
	}
}

// Resolve resolves Docker container name/ID
func (p *DockerProvider) Resolve(entry *inventory.Entry, opts RuntimeOpts) (*Resolved, error) {
	resolved := &Resolved{
		Target:     entry.Target,
		User:       entry.User,
		Port:       entry.Port,
		Additional: make(map[string]string),
	}

	// Container name/ID is the target
	resolved.Additional["container"] = entry.Target

	// Apply token overrides
	if opts.Tokens != nil {
		if container := opts.Tokens["container"]; container != "" {
			resolved.Additional["container"] = container
			resolved.Target = container
		}
		if shell := opts.Tokens["shell"]; shell != "" {
			resolved.Additional["shell"] = shell
		}
	}

	// Apply meta overrides
	if shell := entry.GetMeta("shell"); shell != "" {
		resolved.Additional["shell"] = shell
	}
	if context := entry.GetMeta("context"); context != "" {
		resolved.Additional["docker_context"] = context
	}

	// Default shell
	if resolved.Additional["shell"] == "" {
		resolved.Additional["shell"] = "/bin/sh"
	}

	return resolved, nil
}

// Build creates an ExecPlan for Docker exec
func (p *DockerProvider) Build(action Action, entry *inventory.Entry, resolved *Resolved, opts RuntimeOpts) (*ExecPlan, error) {
	if err := p.Validate(entry); err != nil {
		return nil, err
	}

	plan := &ExecPlan{
		Argv: []string{"docker"},
		Env:  make(map[string]string),
		Cwd:  opts.Workdir,
		TTY:  action != ActionCommand, // TTY required unless running command
	}

	// Add docker context if specified
	if context := resolved.Additional["docker_context"]; context != "" {
		plan.Argv = append(plan.Argv, "--context", context)
	}

	// Build exec command
	plan.Argv = append(plan.Argv, "exec")

	// Add TTY flag
	if plan.TTY {
		plan.Argv = append(plan.Argv, "-it")
	} else {
		plan.Argv = append(plan.Argv, "-i")
	}

	// Add user if specified
	if resolved.User != "" {
		plan.Argv = append(plan.Argv, "--user", resolved.User)
	}

	// Add workdir if specified
	if workdir := opts.Workdir; workdir != "" {
		plan.Argv = append(plan.Argv, "-w", workdir)
	} else if entry.Workdir != "" {
		plan.Argv = append(plan.Argv, "-w", entry.Workdir)
	}

	// Add environment variables from meta
	if envVars := entry.GetMeta("env"); envVars != "" {
		for _, envVar := range strings.Split(envVars, ",") {
			envVar = strings.TrimSpace(envVar)
			if envVar != "" {
				plan.Argv = append(plan.Argv, "-e", envVar)
			}
		}
	}

	// Add container name/ID
	container := resolved.Additional["container"]
	plan.Argv = append(plan.Argv, container)

	// Add command or shell
	if len(opts.Command) > 0 {
		plan.Argv = append(plan.Argv, opts.Command...)
		plan.TTY = false
	} else {
		shell := resolved.Additional["shell"]
		plan.Argv = append(plan.Argv, shell)
	}

	// Add passthrough arguments
	plan.Argv = append(plan.Argv, opts.Passthrough...)

	return plan, nil
}

// Validate validates a Docker entry
func (p *DockerProvider) Validate(entry *inventory.Entry) error {
	if entry.Target == "" {
		return errors.NewValidationError("Docker provider requires 'target' field (container name/ID)")
	}

	// Check if docker is available
	if _, err := exec.LookPath("docker"); err != nil {
		return errors.NewDependencyError("docker not found", "install via: https://docs.docker.com/get-docker/")
	}

	return nil
}
