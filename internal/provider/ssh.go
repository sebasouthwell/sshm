package provider

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/Sebasouthwell/sshm/internal/inventory"
	"github.com/Sebasouthwell/sshm/internal/errors"
)

// SSHProvider implements the Provider interface for SSH connections
type SSHProvider struct{}

// NewSSHProvider creates a new SSH provider
func NewSSHProvider() *SSHProvider {
	return &SSHProvider{}
}

// Name returns the provider name
func (p *SSHProvider) Name() string {
	return "ssh"
}

// Supports returns true if the provider supports the given action
func (p *SSHProvider) Supports(action Action) bool {
	switch action {
	case ActionOpen, ActionCommand, ActionPortFwd, ActionTest, ActionShow:
		return true
	default:
		return false
	}
}

// Resolve resolves the entry target (for SSH, this is mostly a no-op)
func (p *SSHProvider) Resolve(entry *inventory.Entry, opts RuntimeOpts) (*Resolved, error) {
	resolved := &Resolved{
		Target:     entry.Target,
		User:       entry.User,
		Port:       entry.Port,
		Additional: make(map[string]string),
	}

	// Parse target if it contains user@ or :port
	if user, host, port := parseTarget(entry.Target); user != "" || port != "" {
		if user != "" {
			resolved.User = user
		}
		if port != "" {
			resolved.Port = port
		}
		resolved.Target = host
	}

	// Apply token overrides
	if opts.Tokens != nil {
		if user := opts.Tokens["user"]; user != "" {
			resolved.User = user
		}
		if port := opts.Tokens["p"]; port != "" {
			resolved.Port = port
		}
		if port := opts.Tokens["port"]; port != "" {
			resolved.Port = port
		}
	}

	// Apply entry defaults if not set
	if resolved.User == "" {
		resolved.User = entry.User
	}
	if resolved.Port == "" {
		resolved.Port = entry.Port
	}

	return resolved, nil
}

// Build creates an ExecPlan for SSH connections
func (p *SSHProvider) Build(action Action, entry *inventory.Entry, resolved *Resolved, opts RuntimeOpts) (*ExecPlan, error) {
	if err := p.Validate(entry); err != nil {
		return nil, err
	}

	plan := &ExecPlan{
		Argv: []string{"ssh"},
		Env:  make(map[string]string),
		Cwd:  opts.Workdir,
		TTY:  action != ActionCommand, // TTY required unless running command
	}

	// Add key file
	if entry.Key != "" {
		plan.Argv = append(plan.Argv, "-i", entry.Key)
		plan.Argv = append(plan.Argv, "-o", "IdentitiesOnly=yes")
	}

	// Add port
	if resolved.Port != "" {
		plan.Argv = append(plan.Argv, "-p", resolved.Port)
	}

	// Handle jump host
	if jump := entry.GetMeta("jump"); jump != "" {
		// TODO: Resolve jump alias if needed
		plan.Argv = append(plan.Argv, "-J", jump)
	}

	// Handle strict host key checking
	strict := entry.GetMeta("strict")
	if strict == "" {
		strict = "yes" // Default to strict
	}
	switch strict {
	case "yes":
		plan.Argv = append(plan.Argv, "-o", "StrictHostKeyChecking=yes")
	case "no":
		plan.Argv = append(plan.Argv, "-o", "StrictHostKeyChecking=no")
	case "accept-new":
		plan.Argv = append(plan.Argv, "-o", "StrictHostKeyChecking=accept-new")
	}

	// Handle known hosts override
	if knownHosts := entry.GetMeta("knownhosts"); knownHosts != "" {
		plan.Argv = append(plan.Argv, "-o", fmt.Sprintf("UserKnownHostsFile=%s", knownHosts))
	}

	// Handle proxy command
	if proxyCmd := entry.GetMeta("proxycmd"); proxyCmd != "" {
		plan.Argv = append(plan.Argv, "-o", fmt.Sprintf("ProxyCommand=%s", proxyCmd))
	}

	// Handle SSH options from meta
	if sshOpts := entry.GetMeta("sshopt"); sshOpts != "" {
		optsList := strings.Split(sshOpts, ",")
		for _, opt := range optsList {
			opt = strings.TrimSpace(opt)
			if strings.HasPrefix(opt, "-o") {
				plan.Argv = append(plan.Argv, opt)
			} else if strings.HasPrefix(opt, "-") {
				plan.Argv = append(plan.Argv, opt)
			} else {
				plan.Argv = append(plan.Argv, "-o", opt)
			}
		}
	}

	// Handle tokens
	if opts.Tokens != nil {
		// Port forwarding tokens
		if l := opts.Tokens["l"]; l != "" {
			plan.Argv = append(plan.Argv, "-L", l)
		}
		if l := opts.Tokens["L"]; l != "" {
			plan.Argv = append(plan.Argv, "-L", l)
		}
		if r := opts.Tokens["r"]; r != "" {
			plan.Argv = append(plan.Argv, "-R", r)
		}
		if r := opts.Tokens["R"]; r != "" {
			plan.Argv = append(plan.Argv, "-R", r)
		}
		if d := opts.Tokens["d"]; d != "" {
			plan.Argv = append(plan.Argv, "-D", d)
		}
		if d := opts.Tokens["D"]; d != "" {
			plan.Argv = append(plan.Argv, "-D", d)
		}
		if j := opts.Tokens["j"]; j != "" {
			plan.Argv = append(plan.Argv, "-J", j)
		}
		if j := opts.Tokens["J"]; j != "" {
			plan.Argv = append(plan.Argv, "-J", j)
		}

		// Agent forwarding
		if opts.Tokens["agent"] == "true" || opts.Tokens["A"] == "true" {
			plan.Argv = append(plan.Argv, "-A")
		}
		if opts.Tokens["noagent"] == "true" || opts.Tokens["a"] == "true" {
			plan.Argv = append(plan.Argv, "-a")
		}

		// TTY control
		if opts.Tokens["tty"] == "true" || opts.Tokens["t"] == "true" {
			plan.Argv = append(plan.Argv, "-t")
		}
		if opts.Tokens["notty"] == "true" || opts.Tokens["T"] == "true" {
			plan.Argv = append(plan.Argv, "-T")
		}

		// Verbosity
		if opts.Tokens["v"] == "true" {
			plan.Argv = append(plan.Argv, "-v")
		}
		if opts.Tokens["v2"] == "true" {
			plan.Argv = append(plan.Argv, "-vv")
		}
		if opts.Tokens["v3"] == "true" {
			plan.Argv = append(plan.Argv, "-vvv")
		}

		// Strict override
		if strict := opts.Tokens["strict"]; strict != "" {
			switch strict {
			case "yes":
				plan.Argv = append(plan.Argv, "-o", "StrictHostKeyChecking=yes")
			case "no":
				plan.Argv = append(plan.Argv, "-o", "StrictHostKeyChecking=no")
			case "accept-new":
				plan.Argv = append(plan.Argv, "-o", "StrictHostKeyChecking=accept-new")
			}
		}
	}

	// Add passthrough arguments
	plan.Argv = append(plan.Argv, opts.Passthrough...)

	// Build destination
	dest := resolved.Target
	if resolved.User != "" {
		dest = resolved.User + "@" + dest
	}
	plan.Argv = append(plan.Argv, dest)

	// Add command if provided
	if len(opts.Command) > 0 {
		plan.Argv = append(plan.Argv, opts.Command...)
		plan.TTY = false // Commands don't need TTY
	}

	// Set workdir
	if opts.Workdir != "" {
		plan.Cwd = opts.Workdir
	} else if entry.Workdir != "" {
		plan.Cwd = entry.Workdir
	}

	return plan, nil
}

// Validate validates an SSH entry
func (p *SSHProvider) Validate(entry *inventory.Entry) error {
	if entry.Key == "" {
		return errors.NewValidationError("SSH provider requires 'key' field")
	}

	// Check if key file exists
	if _, err := os.Stat(entry.Key); os.IsNotExist(err) {
		return errors.NewValidationError("SSH key file not found: %s", entry.Key)
	}

	if entry.Target == "" {
		return errors.NewValidationError("SSH provider requires 'target' field")
	}

	return nil
}

// parseTarget parses a target string that may contain user@host:port or [ipv6]:port
func parseTarget(target string) (user, host, port string) {
	// Handle IPv6: [addr]:port
	ipv6Regex := regexp.MustCompile(`^\[(.+)\]:(\d+)$`)
	if matches := ipv6Regex.FindStringSubmatch(target); len(matches) == 3 {
		return "", matches[1], matches[2]
	}

	// Handle user@host:port
	if strings.Contains(target, "@") {
		parts := strings.SplitN(target, "@", 2)
		user = parts[0]
		target = parts[1]
	}

	// Handle host:port
	if strings.Contains(target, ":") {
		parts := strings.SplitN(target, ":", 2)
		host = parts[0]
		port = parts[1]
	} else {
		host = target
	}

	return user, host, port
}
