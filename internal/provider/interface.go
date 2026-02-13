package provider

import (
	"github.com/Sebasouthwell/sshm/internal/inventory"
)

// Action represents the type of action to perform
type Action string

const (
	ActionOpen      Action = "open"      // Open interactive shell
	ActionCommand   Action = "command"   // Execute command (:: cmd...)
	ActionPortFwd   Action = "portfwd"   // Port forwarding
	ActionTest      Action = "test"      // Test connection
	ActionShow      Action = "show"      // Show details
)

// Resolved contains runtime-resolved target information
type Resolved struct {
	Target     string            // Resolved target (IP, instance-id, pod name, etc.)
	User       string            // Resolved user
	Port       string            // Resolved port
	Additional map[string]string // Provider-specific resolved data
}

// ExecPlan represents a command execution plan
type ExecPlan struct {
	Argv []string          // Command arguments
	Env  map[string]string // Environment variables
	Cwd  string            // Working directory
	TTY  bool              // Whether TTY is required
}

// RuntimeOpts contains runtime options for provider operations
type RuntimeOpts struct {
	Tokens      map[string]string // Parsed tokens (user=, p=, etc.)
	Passthrough []string          // Raw passthrough arguments (after --)
	Command     []string          // Remote command (after ::)
	DryRun      bool              // If true, don't execute, just build plan
	Workdir     string            // Override workdir
}

// Provider defines the interface for all providers
type Provider interface {
	// Name returns the provider name (ssh, tf, ssm, docker, kube)
	Name() string

	// Supports returns true if the provider supports the given action
	Supports(action Action) bool

	// Resolve converts a logical target into a resolved target
	// For static providers (ssh), this may be a no-op
	// For dynamic providers (tf, ssm, kube), this performs actual resolution
	Resolve(entry *inventory.Entry, opts RuntimeOpts) (*Resolved, error)

	// Build creates an ExecPlan for the given action
	Build(action Action, entry *inventory.Entry, resolved *Resolved, opts RuntimeOpts) (*ExecPlan, error)

	// Validate checks if an entry is valid for this provider
	Validate(entry *inventory.Entry) error
}
