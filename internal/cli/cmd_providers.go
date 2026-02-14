package cli

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/Sebasouthwell/sshm/internal/errors"
)

// Provider convenience commands - force provider-specific semantics

var sshCmd = &cobra.Command{
	Use:   "ssh <alias> [tokens...] [-- passthrough...] [:: cmd...]",
	Short: "Open SSH connection (forces ssh provider)",
	Long:  `Open a connection forcing SSH provider semantics.`,
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return handleProviderCommand("ssh", args[0], args[1:])
	},
}

var tfCmd = &cobra.Command{
	Use:   "tf <alias> [tokens...] [-- passthrough...] [:: cmd...]",
	Short: "Open Terraform connection (forces tf provider)",
	Long:  `Open a connection forcing Terraform provider semantics.`,
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return handleProviderCommand("tf", args[0], args[1:])
	},
}

var ssmCmd = &cobra.Command{
	Use:   "ssm <alias> [tokens...] [-- passthrough...] [:: cmd...]",
	Short: "Open SSM connection (forces ssm provider)",
	Long:  `Open a connection forcing SSM provider semantics.`,
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return handleProviderCommand("ssm", args[0], args[1:])
	},
}

var dockerCmd = &cobra.Command{
	Use:   "docker <alias> [tokens...] [-- passthrough...] [:: cmd...]",
	Short: "Open Docker connection (forces docker provider)",
	Long:  `Open a connection forcing Docker provider semantics.`,
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return handleProviderCommand("docker", args[0], args[1:])
	},
}

var kubeCmd = &cobra.Command{
	Use:   "kube <alias> [tokens...] [-- passthrough...] [:: cmd...]",
	Short: "Open Kubernetes connection (forces kube provider)",
	Long:  `Open a connection forcing Kubernetes provider semantics.`,
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return handleProviderCommand("kube", args[0], args[1:])
	},
}

// handleProviderCommand handles provider convenience commands
func handleProviderCommand(providerType string, alias string, remainingArgs []string) error {
	entry, err := manager.Find(alias)
	if err != nil {
		return errors.NewNotFoundError(alias)
	}

	// Validate provider type matches
	if entry.Type != providerType {
		return fmt.Errorf("provider type mismatch: entry '%s' is type '%s', not '%s'", alias, entry.Type, providerType)
	}

	// Route to open command
	return handleOpen(alias, remainingArgs)
}
