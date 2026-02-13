package provider

import (
	"os/exec"
	"strings"

	"github.com/Sebasouthwell/sshm/internal/inventory"
	"github.com/Sebasouthwell/sshm/internal/errors"
)

// KubeProvider implements the Provider interface for Kubernetes pod connections
type KubeProvider struct{}

// NewKubeProvider creates a new Kubernetes provider
func NewKubeProvider() *KubeProvider {
	return &KubeProvider{}
}

// Name returns the provider name
func (p *KubeProvider) Name() string {
	return "kube"
}

// Supports returns true if the provider supports the given action
func (p *KubeProvider) Supports(action Action) bool {
	switch action {
	case ActionOpen, ActionCommand, ActionTest, ActionShow:
		return true
	default:
		return false
	}
}

// Resolve resolves Kubernetes pod selector or pod name
func (p *KubeProvider) Resolve(entry *inventory.Entry, opts RuntimeOpts) (*Resolved, error) {
	resolved := &Resolved{
		Target:     entry.Target,
		User:       entry.User,
		Port:       entry.Port,
		Additional: make(map[string]string),
	}

	// Parse target format: label:key=value or pod/name or namespace/pod/name
	if strings.HasPrefix(entry.Target, "label:") {
		// Label selector - will be resolved by resolver
		resolved.Additional["selector_type"] = "label"
		resolved.Additional["selector"] = strings.TrimPrefix(entry.Target, "label:")
	} else {
		// Pod name format: pod/name or namespace/pod/name
		parts := strings.Split(entry.Target, "/")
		if len(parts) == 2 {
			resolved.Additional["pod"] = parts[1]
		} else if len(parts) == 3 {
			resolved.Additional["namespace"] = parts[0]
			resolved.Additional["pod"] = parts[2]
		} else {
			resolved.Additional["pod"] = entry.Target
		}
	}

	// Apply token overrides
	if opts.Tokens != nil {
		if pod := opts.Tokens["pod"]; pod != "" {
			resolved.Additional["pod"] = pod
		}
		if namespace := opts.Tokens["namespace"]; namespace != "" {
			resolved.Additional["namespace"] = namespace
		}
		if container := opts.Tokens["container"]; container != "" {
			resolved.Additional["container"] = container
		}
		if context := opts.Tokens["context"]; context != "" {
			resolved.Additional["kube_context"] = context
		}
	}

	// Apply meta overrides
	if namespace := entry.GetMeta("namespace"); namespace != "" {
		resolved.Additional["namespace"] = namespace
	}
	if container := entry.GetMeta("container"); container != "" {
		resolved.Additional["container"] = container
	}
	if context := entry.GetMeta("context"); context != "" {
		resolved.Additional["kube_context"] = context
	}

	return resolved, nil
}

// Build creates an ExecPlan for kubectl exec
func (p *KubeProvider) Build(action Action, entry *inventory.Entry, resolved *Resolved, opts RuntimeOpts) (*ExecPlan, error) {
	if err := p.Validate(entry); err != nil {
		return nil, err
	}

	plan := &ExecPlan{
		Argv: []string{"kubectl", "exec"},
		Env:  make(map[string]string),
		Cwd:  opts.Workdir,
		TTY:  action != ActionCommand, // TTY required unless running command
	}

	// Add namespace if specified
	if namespace := resolved.Additional["namespace"]; namespace != "" {
		plan.Argv = append(plan.Argv, "-n", namespace)
	}

	// Add context if specified
	if context := resolved.Additional["kube_context"]; context != "" {
		plan.Argv = append(plan.Argv, "--context", context)
	}

	// Add TTY flag
	if plan.TTY {
		plan.Argv = append(plan.Argv, "-it")
	} else {
		plan.Argv = append(plan.Argv, "-i")
	}

	// Add container if specified
	if container := resolved.Additional["container"]; container != "" {
		plan.Argv = append(plan.Argv, "-c", container)
	}

	// Add pod name
	pod := resolved.Additional["pod"]
	if pod == "" {
		pod = resolved.Target
	}
	plan.Argv = append(plan.Argv, pod)

	// Add command or shell
	if len(opts.Command) > 0 {
		plan.Argv = append(plan.Argv, "--")
		plan.Argv = append(plan.Argv, opts.Command...)
		plan.TTY = false
	} else {
		// Default shell
		shell := resolved.Additional["shell"]
		if shell == "" {
			shell = "/bin/sh"
		}
		plan.Argv = append(plan.Argv, "--", shell)
	}

	// Add passthrough arguments
	plan.Argv = append(plan.Argv, opts.Passthrough...)

	return plan, nil
}

// Validate validates a Kubernetes entry
func (p *KubeProvider) Validate(entry *inventory.Entry) error {
	if entry.Target == "" {
		return errors.NewValidationError("Kube provider requires 'target' field (pod name or label selector)")
	}

	// Check if kubectl is available
	if _, err := exec.LookPath("kubectl"); err != nil {
		return errors.NewDependencyError("kubectl not found", "install via: https://kubernetes.io/docs/tasks/tools/")
	}

	return nil
}
