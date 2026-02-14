package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
)

var exportCmd = &cobra.Command{
	Use:   "export [file]",
	Short: "Export inventory to archive",
	Long: `Export all inventory files to a tar.gz archive.

Examples:
  sshm export
  sshm export ~/backup/sshm-inventory.tar.gz`,
	RunE: func(cmd *cobra.Command, args []string) error {
		outputFile := ""
		if len(args) > 0 {
			outputFile = args[0]
		} else {
			// Default filename with timestamp
			timestamp := time.Now().Format("20060102-150405")
			outputFile = fmt.Sprintf("sshm-inventory-%s.tar.gz", timestamp)
		}
		return handleExport(outputFile)
	},
}

// handleExport handles the export command
func handleExport(outputFile string) error {
	invDir := manager.GetInvDir()

	// Check if inventory directory exists
	if _, err := os.Stat(invDir); os.IsNotExist(err) {
		return fmt.Errorf("inventory directory not found: %s", invDir)
	}

	// Check if tar is available
	if _, err := exec.LookPath("tar"); err != nil {
		return fmt.Errorf("tar not found (needed for export)")
	}

	// Get absolute path for output file
	absOutput, err := filepath.Abs(outputFile)
	if err != nil {
		return fmt.Errorf("failed to resolve output path: %w", err)
	}

	// Create tar.gz archive
	// tar -czf output.tar.gz -C parent_dir inventory_dir_name/*.inv
	parentDir := filepath.Dir(invDir)
	invDirName := filepath.Base(invDir)

	args := []string{
		"-czf",
		absOutput,
		"-C",
		parentDir,
		invDirName + "/*.inv",
	}

	cmd := exec.Command("tar", args...)
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("export failed: %w", err)
	}

	fmt.Printf("Exported inventory to: %s\n", absOutput)
	return nil
}
