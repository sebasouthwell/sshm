package cli

import (
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"
)

var editCmd = &cobra.Command{
	Use:   "edit [filebase]",
	Short: "Edit inventory file",
	Long: `Open inventory file or directory in $EDITOR.

Examples:
  sshm edit
  sshm edit terraform`,
	RunE: func(cmd *cobra.Command, args []string) error {
		filebase := ""
		if len(args) > 0 {
			filebase = args[0]
		}
		return handleEdit(filebase)
	},
}

// HandleEdit handles the edit command (exported for use from UI)
func HandleEdit(filebase string) error {
	return handleEdit(filebase)
}

// handleEdit handles the edit command
func handleEdit(filebase string) error {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi" // Default fallback
	}

	var target string
	if filebase == "" {
		// Open directory
		target = manager.GetInvDir()
	} else {
		// Open specific file (JSON format)
		target = filepath.Join(manager.GetInvDir(), filebase+".json")
		// If JSON file doesn't exist, check for legacy .inv file
		if _, err := os.Stat(target); os.IsNotExist(err) {
			legacyPath := filepath.Join(manager.GetInvDir(), filebase+".inv")
			if _, err := os.Stat(legacyPath); err == nil {
				target = legacyPath
			}
		}
	}

	cmd := exec.Command(editor, target)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}
