package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/Sebasouthwell/sshm/internal/errors"
)

var rmCmd = &cobra.Command{
	Use:   "rm <alias>",
	Short: "Remove an entry from inventory",
	Long: `Remove an entry from all inventory files.

Example:
  sshm rm prod-web`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return handleRm(args[0])
	},
}

// handleRm handles the rm command
func handleRm(alias string) error {
	if err := manager.Remove(alias); err != nil {
		if strings.Contains(err.Error(), "not found") {
			return errors.NewNotFoundError(alias)
		}
		return err
	}

	fmt.Printf("Removed entry: %s\n", alias)
	return nil
}
