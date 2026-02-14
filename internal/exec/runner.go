package exec

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"

	"github.com/Sebasouthwell/sshm/internal/provider"
)

// Runner executes command plans
type Runner struct{}

// NewRunner creates a new exec runner
func NewRunner() *Runner {
	return &Runner{}
}

// Execute executes an ExecPlan
func (r *Runner) Execute(plan *provider.ExecPlan) error {
	// Check TTY requirement
	if plan.TTY {
		if !isTerminal(os.Stdin) {
			return fmt.Errorf("interactive session requires a TTY")
		}
	}

	// Find the command binary (use LookPath to find in PATH)
	cmdPath := plan.Argv[0]
	if foundPath, err := exec.LookPath(cmdPath); err == nil {
		cmdPath = foundPath
	}

	// Create command
	cmd := exec.Command(cmdPath, plan.Argv[1:]...)

	// Set environment
	cmd.Env = os.Environ()
	for k, v := range plan.Env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	// Set working directory
	if plan.Cwd != "" {
		cmd.Dir = plan.Cwd
	}

	// Set up stdio
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Handle TTY - only set Setsid, avoid Setctty which can fail in WSL2
	if plan.TTY {
		// Setsid creates a new session, but don't use Setctty in WSL2
		// This allows SSH to work properly without permission errors
		cmd.SysProcAttr = &syscall.SysProcAttr{
			Setsid: true,
		}
	}

	// Handle signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		sig := <-sigChan
		if cmd.Process != nil {
			cmd.Process.Signal(sig)
		}
	}()

	// Print command preview
	fmt.Printf("→ %s\n", formatCommand(plan.Argv))

	// Execute
	err := cmd.Run()

	// Get exit code
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			os.Exit(exitError.ExitCode())
		}
		return err
	}

	return nil
}

// formatCommand formats command arguments for display
func formatCommand(argv []string) string {
	var parts []string
	for _, arg := range argv {
		// Quote arguments with spaces or special characters
		if needsQuoting(arg) {
			parts = append(parts, fmt.Sprintf("%q", arg))
		} else {
			parts = append(parts, arg)
		}
	}
	return strings.Join(parts, " ")
}

// needsQuoting checks if an argument needs shell quoting
func needsQuoting(s string) bool {
	if s == "" {
		return true
	}
	// Check for spaces, quotes, or other special characters
	for _, r := range s {
		if r == ' ' || r == '\t' || r == '\n' || r == '"' || r == '\'' || r == '\\' {
			return true
		}
	}
	return false
}

// isTerminal checks if a file descriptor is a terminal
func isTerminal(f *os.File) bool {
	stat, err := f.Stat()
	if err != nil {
		return false
	}
	return (stat.Mode() & os.ModeCharDevice) != 0
}
