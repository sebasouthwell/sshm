package provider

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/Sebasouthwell/sshm/internal/inventory"
	"github.com/Sebasouthwell/sshm/internal/errors"
	"github.com/Sebasouthwell/sshm/internal/resolver"
)

// SSMProvider implements the Provider interface for AWS SSM connections
type SSMProvider struct{}

// NewSSMProvider creates a new SSM provider
func NewSSMProvider() *SSMProvider {
	return &SSMProvider{}
}

// Name returns the provider name
func (p *SSMProvider) Name() string {
	return "ssm"
}

// Supports returns true if the provider supports the given action
func (p *SSMProvider) Supports(action Action) bool {
	switch action {
	case ActionOpen, ActionCommand, ActionPortFwd, ActionTest, ActionShow:
		return true
	default:
		return false
	}
}

// Resolve resolves SSM target (tag selector or instance-id)
func (p *SSMProvider) Resolve(entry *inventory.Entry, opts RuntimeOpts) (*Resolved, error) {
	resolved := &Resolved{
		Target:     entry.Target,
		User:       entry.User,
		Port:       entry.Port,
		Additional: make(map[string]string),
	}

	// Parse target format: tag:Key=Value or instance-id
	if strings.HasPrefix(entry.Target, "tag:") {
		// Tag selector - resolve to instance-id
		profile := entry.GetMeta("profile")
		region := entry.GetMeta("region")
		
		// Apply token overrides for profile/region
		if opts.Tokens != nil {
			if p := opts.Tokens["profile"]; p != "" {
				profile = p
			}
			if r := opts.Tokens["region"]; r != "" {
				region = r
			}
		}

		// Resolve tag selector to instance IDs
		instances, err := resolver.ResolveSSMInstance(entry.Target, profile, region)
		if err != nil {
			return nil, errors.NewResolveError("failed to resolve SSM tag selector %s: %w", entry.Target, err)
		}

		if len(instances) == 0 {
			return nil, errors.NewResolveError("no instances found for selector: %s", entry.Target)
		}

		if len(instances) > 1 {
			// Multiple matches - check for disambiguation flags
			first := false
			if opts.Tokens != nil {
				first = opts.Tokens["first"] == "true"
			}
			
			if !first {
				// Default: fail on multiple matches
				return nil, errors.NewResolveError("multiple instances found (%d) for selector %s; use first=true to select first match", len(instances), entry.Target)
			}
		}

		resolved.Additional["selector_type"] = "tag"
		resolved.Additional["instance_id"] = instances[0]
		resolved.Additional["resolved_from"] = entry.Target
	} else {
		// Assume instance-id
		resolved.Additional["selector_type"] = "instance-id"
		resolved.Additional["instance_id"] = entry.Target
	}

		// Apply token overrides
	if opts.Tokens != nil {
		if instanceID := opts.Tokens["instance-id"]; instanceID != "" {
			resolved.Additional["instance_id"] = instanceID
			resolved.Additional["selector_type"] = "instance-id"
		}
		if profile := opts.Tokens["profile"]; profile != "" {
			resolved.Additional["aws_profile"] = profile
		}
		if region := opts.Tokens["region"]; region != "" {
			resolved.Additional["aws_region"] = region
		}
	}

	// Apply meta overrides
	if profile := entry.GetMeta("profile"); profile != "" {
		resolved.Additional["aws_profile"] = profile
	}
	if region := entry.GetMeta("region"); region != "" {
		resolved.Additional["aws_region"] = region
	}

	return resolved, nil
}

// Build creates an ExecPlan for SSM connections
func (p *SSMProvider) Build(action Action, entry *inventory.Entry, resolved *Resolved, opts RuntimeOpts) (*ExecPlan, error) {
	if err := p.Validate(entry); err != nil {
		return nil, err
	}

	plan := &ExecPlan{
		Argv: []string{"aws", "ssm", "start-session"},
		Env:  make(map[string]string),
		Cwd:  opts.Workdir,
		TTY:  action != ActionCommand, // TTY required unless running command
	}

	// Build target argument
	target := "--target"
	instanceID := resolved.Additional["instance_id"]
	if instanceID == "" {
		return nil, fmt.Errorf("instance ID not resolved")
	}
	plan.Argv = append(plan.Argv, target, instanceID)

	// Add AWS profile if specified
	if profile := resolved.Additional["aws_profile"]; profile != "" {
		plan.Argv = append(plan.Argv, "--profile", profile)
	}

	// Add AWS region if specified
	if region := resolved.Additional["aws_region"]; region != "" {
		plan.Argv = append(plan.Argv, "--region", region)
	}

	// Handle port forwarding
	if opts.Tokens != nil {
		if pf := opts.Tokens["pf"]; pf != "" {
			// Port forwarding: pf=localPort:remoteHost:remotePort
			remotePort, localPort, remoteHost := parsePortForward(pf)
			plan.Argv = []string{"aws", "ssm", "start-session"}
			if instanceID := resolved.Additional["instance_id"]; instanceID != "" {
				plan.Argv = append(plan.Argv, "--target", instanceID)
			}
			plan.Argv = append(plan.Argv, "--document-name", "AWS-StartPortForwardingSessionToRemoteHost")
			plan.Argv = append(plan.Argv, "--parameters", fmt.Sprintf(`{"portNumber":["%s"],"localPortNumber":["%s"],"host":["%s"]}`, remotePort, localPort, remoteHost))
		}
	}

	// Handle command execution
	if len(opts.Command) > 0 {
		plan.Argv = []string{"aws", "ssm", "send-command"}
		if instanceID := resolved.Additional["instance_id"]; instanceID != "" {
			plan.Argv = append(plan.Argv, "--instance-ids", instanceID)
		}
		plan.Argv = append(plan.Argv, "--document-name", "AWS-RunShellScript")
		plan.Argv = append(plan.Argv, "--parameters", fmt.Sprintf(`{"commands":["%s"]}`, strings.Join(opts.Command, " ")))
		plan.TTY = false
	}

	// Add passthrough arguments
	plan.Argv = append(plan.Argv, opts.Passthrough...)

	return plan, nil
}

// Validate validates an SSM entry
func (p *SSMProvider) Validate(entry *inventory.Entry) error {
	if entry.Target == "" {
		return errors.NewValidationError("SSM provider requires 'target' field")
	}

	// Check if AWS CLI is available
	if _, err := exec.LookPath("aws"); err != nil {
		return errors.NewDependencyError("aws CLI not found", "install via: pip install awscli or brew install awscli")
	}

	// Check if session-manager-plugin is available
	if _, err := exec.LookPath("session-manager-plugin"); err != nil {
		return errors.NewDependencyError("session-manager-plugin not found", "install via: https://docs.aws.amazon.com/systems-manager/latest/userguide/session-manager-working-with-install-plugin.html")
	}

	return nil
}

// parsePortForward parses port forwarding token: localPort:remoteHost:remotePort
// Returns remotePort, localPort, remoteHost
func parsePortForward(pf string) (string, string, string) {
	parts := strings.Split(pf, ":")
	if len(parts) != 3 {
		return "", "", ""
	}
	return parts[2], parts[0], parts[1]
}
