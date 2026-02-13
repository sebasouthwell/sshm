package provider

import (
	"os"
	"strings"

	"github.com/Sebasouthwell/sshm/internal/inventory"
	"github.com/Sebasouthwell/sshm/internal/errors"
	"github.com/Sebasouthwell/sshm/internal/resolver"
)

// TFProvider implements the Provider interface for Terraform-backed SSH connections
type TFProvider struct {
	sshProvider *SSHProvider
}

// NewTFProvider creates a new Terraform provider
func NewTFProvider() *TFProvider {
	return &TFProvider{
		sshProvider: NewSSHProvider(),
	}
}

// Name returns the provider name
func (p *TFProvider) Name() string {
	return "tf"
}

// Supports returns true if the provider supports the given action
func (p *TFProvider) Supports(action Action) bool {
	// TF provider supports same actions as SSH
	return p.sshProvider.Supports(action)
}

// Resolve resolves Terraform resource address to IP
func (p *TFProvider) Resolve(entry *inventory.Entry, opts RuntimeOpts) (*Resolved, error) {
	// Parse target: <terraform_address>:<public|private>
	parts := strings.SplitN(entry.Target, ":", 2)
	if len(parts) != 2 {
		return nil, errors.NewResolveError("invalid tf target format: %s (expected <address>:<mode>)", entry.Target)
	}

	tfaddr := parts[0]
	mode := parts[1]

	if mode != "public" && mode != "private" {
		return nil, errors.NewResolveError("invalid mode: %s (expected 'public' or 'private')", mode)
	}

	// Determine workdir
	workdir := opts.Workdir
	if workdir == "" {
		workdir = entry.Workdir
	}
	if workdir == "" {
		return nil, errors.NewResolveError("terraform workdir not set for entry %s", entry.Alias)
	}

	// Resolve IP via terraform
	ip, err := resolver.ResolveTerraformIP(workdir, tfaddr, mode)
	if err != nil {
		return nil, errors.NewResolveError("failed to resolve terraform address %s: %w", tfaddr, err)
	}

	resolved := &Resolved{
		Target:     ip,
		User:       entry.User,
		Port:       entry.Port,
		Additional: make(map[string]string),
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

// Build creates an ExecPlan for Terraform-backed SSH connections
func (p *TFProvider) Build(action Action, entry *inventory.Entry, resolved *Resolved, opts RuntimeOpts) (*ExecPlan, error) {
	// TF provider builds SSH commands, so delegate to SSH provider
	// But we need to create a temporary entry with resolved IP
	sshEntry := &inventory.Entry{
		Alias:    entry.Alias,
		Type:     "ssh",
		Target:   resolved.Target,
		User:     resolved.User,
		Port:     resolved.Port,
		Key:      entry.Key,
		Workdir:  entry.Workdir,
		Tags:     entry.Tags,
		Meta:     entry.Meta, // Inherit SSH meta keys
		File:     entry.File,
		Filebase: entry.Filebase,
	}

	return p.sshProvider.Build(action, sshEntry, resolved, opts)
}

// Validate validates a Terraform entry
func (p *TFProvider) Validate(entry *inventory.Entry) error {
	if entry.Key == "" {
		return errors.NewValidationError("TF provider requires 'key' field")
	}

	// Check if key file exists
	if _, err := os.Stat(entry.Key); os.IsNotExist(err) {
		return errors.NewValidationError("SSH key file not found: %s", entry.Key)
	}

	if entry.Target == "" {
		return errors.NewValidationError("TF provider requires 'target' field")
	}

	// Validate target format
	parts := strings.SplitN(entry.Target, ":", 2)
	if len(parts) != 2 {
		return errors.NewValidationError("invalid tf target format: %s (expected <address>:<mode>)", entry.Target)
	}

	mode := parts[1]
	if mode != "public" && mode != "private" {
		return errors.NewValidationError("invalid mode: %s (expected 'public' or 'private')", mode)
	}

	if entry.Workdir == "" {
		return errors.NewValidationError("TF provider requires 'workdir' field")
	}

	// Check if workdir exists (warning only, not error)
	if _, err := os.Stat(entry.Workdir); os.IsNotExist(err) {
		// Warning: workdir doesn't exist, but allow it (might be created later)
		// This will error on open, but not on add
	}

	return nil
}
