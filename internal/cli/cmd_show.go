package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/Sebasouthwell/sshm/internal/errors"
	"github.com/Sebasouthwell/sshm/internal/inventory"
)

var (
	showJSON bool
)

var showCmd = &cobra.Command{
	Use:   "show <alias>",
	Short: "Show entry details",
	Long: `Display detailed information about an entry.

Examples:
  sshm show prod-web
  sshm show prod-web --json`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return handleShow(args[0], showJSON)
	},
}

func init() {
	showCmd.Flags().BoolVar(&showJSON, "json", false, "Output in JSON format")
}

// HandleShow handles the show command (exported for use from UI)
func HandleShow(alias string) error {
	return handleShow(alias, false)
}

// handleShow handles the show command
func handleShow(alias string, jsonOutput bool) error {
	entry, err := manager.Find(alias)
	if err != nil {
		return errors.NewNotFoundError(alias)
	}

	if jsonOutput {
		return outputShowJSON(entry)
	}

	fmt.Printf("Alias    : %s\n", entry.Alias)
	fmt.Printf("Type     : %s\n", entry.Type)
	fmt.Printf("Target   : %s\n", entry.Target)
	fmt.Printf("User     : %s\n", orDash(entry.User))
	fmt.Printf("Port     : %s\n", orDash(entry.Port))
	fmt.Printf("Key      : %s\n", orDash(entry.Key))
	fmt.Printf("Workdir  : %s\n", orDash(entry.Workdir))
	fmt.Printf("Tags     : %s\n", orDash(entry.Tags))
	fmt.Printf("File     : %s\n", entry.File)
	fmt.Printf("Filebase : %s\n", entry.Filebase)

	if len(entry.Meta) > 0 {
		fmt.Println("\nMeta:")
		for k, v := range entry.Meta {
			fmt.Printf("  %s = %s\n", k, v)
		}
	}

	return nil
}

// outputShowJSON outputs entry in JSON format
func outputShowJSON(entry *inventory.Entry) error {
	type EntryJSON struct {
		Alias    string            `json:"alias"`
		Type     string            `json:"type"`
		Target   string            `json:"target"`
		User     string            `json:"user,omitempty"`
		Port     string            `json:"port,omitempty"`
		Key      string            `json:"key,omitempty"`
		Workdir  string            `json:"workdir,omitempty"`
		Tags     string            `json:"tags,omitempty"`
		Meta     map[string]string `json:"meta,omitempty"`
		File     string            `json:"file"`
		Filebase string            `json:"filebase"`
	}

	jsonEntry := EntryJSON{
		Alias:    entry.Alias,
		Type:     entry.Type,
		Target:   entry.Target,
		User:     entry.User,
		Port:     entry.Port,
		Key:      entry.Key,
		Workdir:  entry.Workdir,
		Tags:     entry.Tags,
		Meta:     entry.Meta,
		File:     entry.File,
		Filebase: entry.Filebase,
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(jsonEntry)
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
