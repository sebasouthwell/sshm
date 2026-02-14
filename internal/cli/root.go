package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
	"github.com/Sebasouthwell/sshm/internal/config"
	"github.com/Sebasouthwell/sshm/internal/inventory"
)

var (
	invDir      string
	defaultBase string
	manager     *inventory.Manager
)

const Version = "2.0.0"

// rootCmd represents the base command
var rootCmd = &cobra.Command{
	Use:     "sshm",
	Short:   "Dynamic multi-environment session manager",
	Version: Version,
	Long: `SSHM v2 - A cross-environment session launcher and inventory manager.

Supports SSH, Terraform, AWS SSM, Docker, and Kubernetes providers.
Use 'sshm <provider>.<alias>' for quick connect or 'sshm ui' for interactive selection.`,
	SilenceUsage:  true,
	SilenceErrors: true,
}

// Execute runs the CLI
func Execute() error {
	// Initialize inventory manager
	initManager()

	// Handle no arguments - launch UI (default action)
	if len(os.Args) == 1 {
		return handleUI("")
	}

	// Check for help/version flags first
	if len(os.Args) == 2 {
		arg := os.Args[1]
		if arg == "--help" || arg == "-h" || arg == "--version" || arg == "-v" {
			return rootCmd.Execute()
		}
	}

	// Check for quick connect pattern: <provider>.<alias>
	if len(os.Args) > 1 {
		firstArg := os.Args[1]
		if matched, _ := regexp.MatchString(`^(\w+)\.(\w+)$`, firstArg); matched {
			// Quick connect pattern detected
			return handleQuickConnect(firstArg, os.Args[2:])
		}

		// Check if first arg is a known alias (direct alias access)
		// Skip if it's a flag or known subcommand
		if !strings.HasPrefix(firstArg, "-") && !isSubcommand(firstArg) {
			if _, err := manager.Find(firstArg); err == nil {
				// Found alias, route to open
				return handleOpen(firstArg, os.Args[2:])
			}
		}
	}

	return rootCmd.Execute()
}

// isSubcommand checks if the argument is a known subcommand
func isSubcommand(arg string) bool {
	subcommands := map[string]bool{
		"open":     true,
		"ui":       true,
		"ls":       true,
		"show":     true,
		"add":      true,
		"rm":       true,
		"edit":     true,
		"cd":       true,
		"history":  true,
		"cache":    true,
		"test":     true,
		"scp":      true,
		"export":   true,
		"import":   true,
		"tf":       true,
		"ssh":      true,
		"ssm":      true,
		"docker":   true,
		"kube":     true,
		"completion": true,
		"version":  true,
	}
	return subcommands[arg]
}

// initManager initializes the inventory manager with config/environment
func initManager() {
	// Load config
	cfg, _ := config.LoadConfig()

	// Get inventory directory (precedence: env > config > default)
	invDir = os.Getenv("SSHM_INV_DIR")
	if invDir == "" && cfg != nil && cfg.InventoryDir != "" {
		invDir = cfg.InventoryDir
	}
	if invDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			// Fallback to current directory
			invDir = ".ssh/inventory.d"
		} else {
			invDir = filepath.Join(home, ".ssh", "inventory.d")
		}
	}

	// Get default filebase (precedence: env > config > default)
	defaultBase = os.Getenv("SSHM_DEFAULT_FILEBASE")
	if defaultBase == "" && cfg != nil && cfg.DefaultFilebase != "" {
		defaultBase = cfg.DefaultFilebase
	}
	if defaultBase == "" {
		defaultBase = "default"
	}

	manager = inventory.NewManager(invDir, defaultBase)
}

// handleQuickConnect handles the quick connect pattern: <provider>.<alias>
func handleQuickConnect(pattern string, remainingArgs []string) error {
	parts := strings.Split(pattern, ".")
	if len(parts) != 2 {
		return fmt.Errorf("invalid quick connect pattern: %s (expected <provider>.<alias>)", pattern)
	}

	providerType := strings.ToLower(parts[0])
	alias := parts[1]

	// Validate provider type
	validProviders := map[string]bool{
		"ssh":    true,
		"tf":     true,
		"ssm":    true,
		"docker": true,
		"kube":   true,
	}
	if !validProviders[providerType] {
		return fmt.Errorf("invalid provider type: %s (valid: ssh, tf, ssm, docker, kube)", providerType)
	}

	// Find entry
	entry, err := manager.Find(alias)
	if err != nil {
		return err
	}

	// Validate provider type matches
	if entry.Type != providerType {
		return fmt.Errorf("provider type mismatch: entry '%s' is type '%s', not '%s'", alias, entry.Type, providerType)
	}

	// Route to open command
	return handleOpen(alias, remainingArgs)
}

func init() {
	// Add subcommands
	rootCmd.AddCommand(openCmd)
	rootCmd.AddCommand(uiCmd)
	rootCmd.AddCommand(lsCmd)
	rootCmd.AddCommand(showCmd)
	rootCmd.AddCommand(addCmd)
	rootCmd.AddCommand(rmCmd)
	rootCmd.AddCommand(editCmd)
	rootCmd.AddCommand(cdCmd)
	rootCmd.AddCommand(historyCmd)
	rootCmd.AddCommand(cacheCmd)
	rootCmd.AddCommand(testCmd) // Defined in cmd_testcmd.go
	rootCmd.AddCommand(scpCmd)
	rootCmd.AddCommand(exportCmd)
	rootCmd.AddCommand(importCmd)
	rootCmd.AddCommand(migrateCmd)
	rootCmd.AddCommand(sshCmd)
	rootCmd.AddCommand(tfWizardCmd) // Terraform wizard command (handles both wizard and provider convenience)
	rootCmd.AddCommand(ssmCmd)
	rootCmd.AddCommand(dockerCmd)
	rootCmd.AddCommand(kubeCmd)
	// Note: tfCmd from cmd_providers.go is not added to avoid conflict with tfWizardCmd
}
