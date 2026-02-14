package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/Sebasouthwell/sshm/internal/inventory"
)

var (
	addType     string
	addTarget   string
	addUser     string
	addPort     string
	addKey      string
	addWorkdir  string
	addTags     string
	addMeta     string
	addFilebase string
)

var addCmd = &cobra.Command{
	Use:   "add <alias> [key] [host] [user] [port] [workdir] [tags] [filebase]",
	Short: "Add an entry to inventory",
	Long: `Add an entry to the inventory.

V1 format (SSH only):
  sshm add <alias> <key> <host> [user] [port] [workdir] [tags] [filebase]

V2 format:
  sshm add <alias> --type <type> --target <target> [options...]

Examples:
  sshm add prod-web ~/.ssh/key.pem 1.2.3.4 ubuntu 22 "" prod,web
  sshm add myapp --type docker --target myapp:latest --tags dev,docker`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		alias := args[0]

		// Determine format based on flags
		if addType != "" || addTarget != "" {
			// V2 format
			return handleAddV2(alias, args[1:])
		} else {
			// V1 format
			return handleAddV1(alias, args[1:])
		}
	},
}

func init() {
	addCmd.Flags().StringVar(&addType, "type", "", "Provider type (ssh, tf, ssm, docker, kube)")
	addCmd.Flags().StringVar(&addTarget, "target", "", "Target string")
	addCmd.Flags().StringVar(&addUser, "user", "", "User")
	addCmd.Flags().StringVar(&addPort, "port", "", "Port")
	addCmd.Flags().StringVar(&addKey, "key", "", "Key path")
	addCmd.Flags().StringVar(&addWorkdir, "workdir", "", "Working directory")
	addCmd.Flags().StringVar(&addTags, "tags", "", "Tags (comma-separated)")
	addCmd.Flags().StringVar(&addMeta, "meta", "", "Meta (k=v;k=v)")
	addCmd.Flags().StringVar(&addFilebase, "filebase", "", "Inventory filebase")
}

// handleAddV1 handles v1 format: alias key host user? port? workdir? tags? filebase?
func handleAddV1(alias string, args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("v1 format requires at least: <alias> <key> <host>")
	}

	entry := inventory.NewEntry()
	entry.Alias = alias
	entry.Type = "ssh"
	entry.Key = args[0]
	entry.Target = args[1]

	if len(args) > 2 {
		entry.User = args[2]
	}
	if len(args) > 3 {
		entry.Port = args[3]
	}
	if len(args) > 4 {
		entry.Workdir = args[4]
	}
	if len(args) > 5 {
		entry.Tags = args[5]
	}

	filebase := defaultBase
	if len(args) > 6 {
		filebase = args[6]
	}

	return manager.Add(entry, filebase)
}

// handleAddV2 handles v2 format with flags
func handleAddV2(alias string, args []string) error {
	if addType == "" {
		return fmt.Errorf("--type is required for v2 format")
	}
	if addTarget == "" {
		return fmt.Errorf("--target is required for v2 format")
	}

	// Warn if positional args are provided (they're ignored in v2 format)
	if len(args) > 0 {
		fmt.Fprintf(os.Stderr, "warning: positional arguments are ignored when using v2 format with flags. Use --key, --user, etc. instead.\n")
	}

	entry := inventory.NewEntry()
	entry.Alias = alias
	entry.Type = addType
	entry.Target = addTarget
	entry.User = addUser
	entry.Port = addPort
	entry.Key = addKey
	entry.Workdir = addWorkdir
	entry.Tags = addTags

	// For terraform entries, validate target format
	if addType == "tf" && !strings.Contains(addTarget, ":") {
		return fmt.Errorf("terraform target must include mode (e.g., aws_instance.hf:public or aws_instance.hf:private)")
	}

	// Parse meta if provided
	if addMeta != "" {
		// Simple parsing - can be enhanced
		meta, err := parseMetaString(addMeta)
		if err != nil {
			return fmt.Errorf("failed to parse meta: %w", err)
		}
		entry.Meta = meta
	}

	filebase := addFilebase
	if filebase == "" {
		filebase = defaultBase
	}

	if err := manager.Add(entry, filebase); err != nil {
		return err
	}

	filePath := filepath.Join(manager.GetInvDir(), filebase+".json")
	fmt.Printf("Added entry: %s (%s) -> %s\n", alias, addType, filePath)
	return nil
}

func parseMetaString(metaStr string) (map[string]string, error) {
	// This is a temporary workaround - we should extract meta parsing
	// For now, simple implementation
	meta := make(map[string]string)
	// TODO: Use proper meta parsing from parser package
	return meta, nil
}
