package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/Sebasouthwell/sshm/internal/errors"
)

var cdCmd = &cobra.Command{
	Use:   "cd <alias>",
	Short: "Change to entry's working directory",
	Long: `Change to the entry's working directory, or key directory if workdir not set.

Example:
  sshm cd prod-web`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return handleCd(args[0])
	},
}

// HandleCD handles the cd command (exported for use from UI)
func HandleCD(alias string) error {
	return handleCd(alias)
}

// handleCd handles the cd command
func handleCd(alias string) error {
	entry, err := manager.Find(alias)
	if err != nil {
		return errors.NewNotFoundError(alias)
	}

	var targetDir string
	if entry.Workdir != "" {
		targetDir = entry.Workdir
	} else if entry.Key != "" {
		targetDir = filepath.Dir(entry.Key)
	} else {
		return fmt.Errorf("no workdir or key directory available for alias: %s", alias)
	}

	// Check if directory exists
	if _, err := os.Stat(targetDir); os.IsNotExist(err) {
		return fmt.Errorf("directory not found: %s", targetDir)
	}

	// Print directory (shell will handle actual cd via eval)
	fmt.Println(targetDir)
	return nil
}
