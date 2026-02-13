package cli

import (
	"fmt"
	osexec "os/exec"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/Sebasouthwell/sshm/internal/errors"
	"github.com/Sebasouthwell/sshm/internal/exec"
	"github.com/Sebasouthwell/sshm/internal/history"
	"github.com/Sebasouthwell/sshm/internal/inventory"
	"github.com/Sebasouthwell/sshm/internal/provider"
	"github.com/Sebasouthwell/sshm/internal/token"
)

var openCmd = &cobra.Command{
	Use:   "open <alias> [tokens...] [-- passthrough...] [:: cmd...]",
	Short: "Open a session to an entry",
	Long: `Open an interactive session to an entry.

Examples:
  sshm open prod-web
  sshm open prod-web user=admin p=2222
  sshm open prod-web L=8080:localhost:8080
  sshm open prod-web -- -v
  sshm open prod-web :: uptime`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		alias := args[0]
		return handleOpen(alias, args[1:])
	},
}

// handleOpen handles the open command
func handleOpen(alias string, remainingArgs []string) error {
	// Find entry
	entry, err := manager.Find(alias)
	if err != nil {
		return errors.NewNotFoundError(alias)
	}

	// Get provider
	prov, err := GetProvider(entry.Type)
	if err != nil {
		return err
	}

	// Parse tokens
	parsed := token.Parse(remainingArgs)

	// Build runtime options
	opts := provider.RuntimeOpts{
		Tokens:      parsed.Tokens,
		Passthrough: parsed.Passthrough,
		Command:     parsed.Command,
		DryRun:      parsed.GetBoolToken("dry"),
		Workdir:     parsed.GetStringOrDefault("wdir", ""),
	}

	// Check for dry run
	if opts.DryRun {
		return handleDryRun(entry, prov, opts)
	}

	// Resolve entry
	resolved, err := prov.Resolve(entry, opts)
	if err != nil {
		return err
	}

	// Build exec plan
	plan, err := prov.Build(provider.ActionOpen, entry, resolved, opts)
	if err != nil {
		return err
	}

	// Log to history
	logger := history.NewLogger("")
	logEvent := &history.Event{
		TS:     time.Now().Unix(),
		Alias:  entry.Alias,
		Type:   entry.Type,
		Action: "open",
		Status: "ok",
	}
	if resolved != nil {
		logEvent.Resolved = &history.ResolvedInfo{
			Target: resolved.Target,
			User:   resolved.User,
			Port:   resolved.Port,
		}
	}
	
	startTime := time.Now()
	
	// Execute
	runner := exec.NewRunner()
	err = runner.Execute(plan)
	
	// Update event with result
	duration := time.Since(startTime).Milliseconds()
	logEvent.Duration = duration
	if err != nil {
		logEvent.Status = "error"
		if exitErr, ok := err.(*osexec.ExitError); ok {
			logEvent.ExitCode = exitErr.ExitCode()
		}
	} else {
		logEvent.ExitCode = 0
	}
	
	// Log asynchronously
	go logger.Log(logEvent)
	
	return err
}

// handleDryRun prints the exec plan without executing
func handleDryRun(entry *inventory.Entry, prov provider.Provider, opts provider.RuntimeOpts) error {
	resolved, err := prov.Resolve(entry, opts)
	if err != nil {
		return err
	}

	plan, err := prov.Build(provider.ActionOpen, entry, resolved, opts)
	if err != nil {
		return err
	}

	fmt.Println("Dry run - would execute:")
	fmt.Printf("Command: %s\n", formatCommand(plan.Argv))
	if len(plan.Env) > 0 {
		fmt.Println("Environment:")
		for k, v := range plan.Env {
			fmt.Printf("  %s=%s\n", k, v)
		}
	}
	if plan.Cwd != "" {
		fmt.Printf("Working directory: %s\n", plan.Cwd)
	}
	fmt.Printf("TTY required: %v\n", plan.TTY)

	return nil
}

func formatCommand(argv []string) string {
	var parts []string
	for _, arg := range argv {
		if needsQuoting(arg) {
			parts = append(parts, fmt.Sprintf("%q", arg))
		} else {
			parts = append(parts, arg)
		}
	}
	return strings.Join(parts, " ")
}

func needsQuoting(s string) bool {
	if s == "" {
		return true
	}
	for _, r := range s {
		if r == ' ' || r == '\t' || r == '\n' || r == '"' || r == '\'' || r == '\\' {
			return true
		}
	}
	return false
}
