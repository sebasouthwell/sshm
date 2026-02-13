package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/Sebasouthwell/sshm/internal/inventory"
)

var (
	lsTagFilter string
	lsJSON      bool
)

var lsCmd = &cobra.Command{
	Use:   "ls [filter]",
	Short: "List inventory entries",
	Long: `List all inventory entries with optional filtering.

Examples:
  sshm ls
  sshm ls prod
  sshm ls --tag web`,
	RunE: func(cmd *cobra.Command, args []string) error {
		filter := ""
		if len(args) > 0 {
			filter = args[0]
		}
		return handleLs(filter, lsTagFilter, lsJSON)
	},
}

func init() {
	lsCmd.Flags().StringVar(&lsTagFilter, "tag", "", "Filter by tag")
	lsCmd.Flags().BoolVar(&lsJSON, "json", false, "Output in JSON format")
}

// handleLs handles the ls command
func handleLs(filter, tagFilter string, jsonOutput bool) error {
	entries, err := manager.LoadAll()
	if err != nil {
		return err
	}

	if len(entries) == 0 {
		fmt.Println("No entries found")
		return nil
	}

	// Filter entries
	var filtered []*inventory.Entry
	for _, entry := range entries {
		// Apply filters
		if filter != "" {
			if !strings.Contains(strings.ToLower(entry.Alias), strings.ToLower(filter)) &&
				!strings.Contains(strings.ToLower(entry.Target), strings.ToLower(filter)) {
				continue
			}
		}

		if tagFilter != "" {
			if !entry.HasTag(tagFilter) {
				continue
			}
		}

		filtered = append(filtered, entry)
	}

	// Output in JSON format if requested
	if jsonOutput {
		return outputLsJSON(filtered)
	}

	// Print header
	fmt.Printf("%-20s %-10s %-30s %-10s %-8s %-20s %s\n",
		"ALIAS", "TYPE", "TARGET", "USER", "PORT", "TAGS", "FILE")
	fmt.Println(strings.Repeat("-", 120))

	// Print entries
	for _, entry := range filtered {
		// Format output
		tags := entry.Tags
		if tags == "" {
			tags = "-"
		}
		user := entry.User
		if user == "" {
			user = "-"
		}
		port := entry.Port
		if port == "" {
			port = "-"
		}

		// Truncate long targets
		target := entry.Target
		if len(target) > 28 {
			target = target[:25] + "..."
		}

		fmt.Printf("%-20s %-10s %-30s %-10s %-8s %-20s %s\n",
			entry.Alias,
			entry.Type,
			target,
			user,
			port,
			tags,
			entry.Filebase,
		)
	}

	return nil
}

// outputLsJSON outputs entries in JSON format
func outputLsJSON(entries []*inventory.Entry) error {
	type EntryJSON struct {
		Alias   string `json:"alias"`
		Type    string `json:"type"`
		Target  string `json:"target"`
		User    string `json:"user,omitempty"`
		Port    string `json:"port,omitempty"`
		Tags    string `json:"tags,omitempty"`
		Filebase string `json:"filebase,omitempty"`
	}

	var jsonEntries []EntryJSON
	for _, entry := range entries {
		jsonEntries = append(jsonEntries, EntryJSON{
			Alias:    entry.Alias,
			Type:     entry.Type,
			Target:   entry.Target,
			User:     entry.User,
			Port:     entry.Port,
			Tags:     entry.Tags,
			Filebase: entry.Filebase,
		})
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(jsonEntries)
}
