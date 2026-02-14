package cli

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
	"github.com/Sebasouthwell/sshm/internal/errors"
	"github.com/Sebasouthwell/sshm/internal/provider"
	"github.com/Sebasouthwell/sshm/internal/token"
)

var scpCmd = &cobra.Command{
	Use:   "scp <alias> <src> <dst> [-- passthrough...]",
	Short: "Copy files via SCP (SSH/TF only)",
	Long: `Copy files using SCP. Only supported for SSH and Terraform providers.

Examples:
  sshm scp prod-web /local/file.txt :/remote/path/
  sshm scp prod-web :/remote/file.txt /local/path/
  sshm scp prod-web file.txt :/remote/path/ -- -r`,
	Args: cobra.MinimumNArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		alias := args[0]
		src := args[1]
		dst := args[2]
		
		// Parse passthrough args
		passthrough := []string{}
		if len(args) > 3 {
			if args[3] == "--" {
				passthrough = args[4:]
			} else {
				// Try to find -- separator
				for i, arg := range args[3:] {
					if arg == "--" {
						passthrough = args[3+i+1:]
						break
					}
				}
			}
		}
		
		return handleSCP(alias, src, dst, passthrough)
	},
}

// handleSCP handles the scp command
func handleSCP(alias string, src string, dst string, passthrough []string) error {
	entry, err := manager.Find(alias)
	if err != nil {
		return errors.NewNotFoundError(alias)
	}

	// Only SSH and TF providers support SCP
	if entry.Type != "ssh" && entry.Type != "tf" {
		return fmt.Errorf("scp only supported for ssh and tf providers (entry type: %s)", entry.Type)
	}

	prov, err := GetProvider(entry.Type)
	if err != nil {
		return fmt.Errorf("failed to get provider: %s", entry.Type)
	}

	// Parse tokens (empty for scp)
	parsed := token.Parse([]string{})
	opts := provider.RuntimeOpts{
		Tokens:      parsed.Tokens,
		Passthrough: parsed.Passthrough,
		Command:     parsed.Command,
	}

	// Resolve entry
	resolved, err := prov.Resolve(entry, opts)
	if err != nil {
		return fmt.Errorf("failed to resolve entry: %w", err)
	}

	// Determine if src/dst are remote
	srcIsRemote := strings.HasPrefix(src, ":")
	dstIsRemote := strings.HasPrefix(dst, ":")

	// If neither is marked, try to detect
	if !srcIsRemote && !dstIsRemote {
		// Check if src exists locally
		if _, err := os.Stat(src); err == nil {
			// src exists locally, so dst must be remote
			dstIsRemote = true
		} else if _, err := os.Stat(dst); err == nil {
			// dst exists locally, so src must be remote
			srcIsRemote = true
		} else {
			// Neither exists, assume dst is remote
			dstIsRemote = true
		}
	}

	// Build remote prefix
	remotePrefix := resolved.Target
	if entry.User != "" {
		remotePrefix = entry.User + "@" + resolved.Target
	}

	// Format paths
	if srcIsRemote {
		src = remotePrefix + ":" + strings.TrimPrefix(src, ":")
	}
	if dstIsRemote {
		dst = remotePrefix + ":" + strings.TrimPrefix(dst, ":")
	}

	// Build scp command
	scpArgs := []string{"-i", entry.Key}
	if entry.Port != "" {
		scpArgs = append(scpArgs, "-P", entry.Port)
	}
	scpArgs = append(scpArgs, "-o", "IdentitiesOnly=yes")
	scpArgs = append(scpArgs, passthrough...)
	scpArgs = append(scpArgs, src, dst)

	// Print command
	fmt.Printf("→ scp %s\n", strings.Join(scpArgs, " "))

	// Execute
	cmd := exec.Command("scp", scpArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}
