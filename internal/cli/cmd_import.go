package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"
)

var importCmd = &cobra.Command{
	Use:   "import <file>",
	Short: "Import inventory from archive",
	Long: `Import inventory files from a tar.gz archive.

Examples:
  sshm import ~/backup/sshm-inventory.tar.gz`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return handleImport(args[0])
	},
}

// handleImport handles the import command
func handleImport(inputFile string) error {
	// Check if file exists
	if _, err := os.Stat(inputFile); os.IsNotExist(err) {
		return fmt.Errorf("file not found: %s", inputFile)
	}

	// Check if tar is available
	if _, err := exec.LookPath("tar"); err != nil {
		return fmt.Errorf("tar not found (needed for import)")
	}

	invDir := manager.GetInvDir()

	// Ensure inventory directory exists
	if err := os.MkdirAll(invDir, 0755); err != nil {
		return fmt.Errorf("failed to create inventory directory: %w", err)
	}

	// Extract tar.gz archive
	// tar -xzf input.tar.gz -C parent_dir
	parentDir := filepath.Dir(invDir)

	args := []string{
		"-xzf",
		inputFile,
		"-C",
		parentDir,
	}

	cmd := exec.Command("tar", args...)
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("import failed (check file format): %w", err)
	}

	fmt.Printf("Imported inventory from: %s\n", inputFile)
	return nil
}
