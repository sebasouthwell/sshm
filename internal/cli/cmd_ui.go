package cli

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/Sebasouthwell/sshm/internal/history"
	"github.com/Sebasouthwell/sshm/internal/inventory"
)

var uiCmd = &cobra.Command{
	Use:   "ui [filter]",
	Short: "Interactive TUI for selecting entries",
	Long: `Launch an interactive terminal UI for selecting and connecting to entries.

Uses fzf if available, falls back to simple list otherwise.

Example:
  sshm ui
  sshm ui prod`,
	RunE: func(cmd *cobra.Command, args []string) error {
		filter := ""
		if len(args) > 0 {
			filter = args[0]
		}
		return handleUI(filter)
	},
}

// handleUI handles the ui command
func handleUI(filter string) error {
	// Check if fzf is available
	if _, err := exec.LookPath("fzf"); err == nil {
		return handleUIFZF(filter)
	}

	// Fallback to simple interactive list
	return handleUISimple(filter)
}

// handleUIFZF handles UI with fzf
func handleUIFZF(filter string) error {
	entries, err := manager.LoadAll()
	if err != nil {
		return err
	}

	if len(entries) == 0 {
		fmt.Println("No entries found")
		return nil
	}

	// Rank entries by history
	ranker := history.NewRanker("")
	aliases := make([]string, len(entries))
	for i, e := range entries {
		aliases[i] = e.Alias
	}
	scores, _ := ranker.RankEntries(aliases)
	scoreMap := make(map[string]*history.EntryScore)
	for i := range scores {
		scoreMap[scores[i].Alias] = &scores[i]
	}

	// Sort entries by score
	sort.Slice(entries, func(i, j int) bool {
		scoreI := scoreMap[entries[i].Alias]
		scoreJ := scoreMap[entries[j].Alias]
		if scoreI == nil {
			return false
		}
		if scoreJ == nil {
			return true
		}
		return scoreI.TotalScore > scoreJ.TotalScore
	})

	// Build fzf command
	fzfCmd := exec.Command("fzf", "--height=85%", "--border", "--ansi")
	fzfCmd.Stderr = os.Stderr
	fzfIn, err := fzfCmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to create fzf stdin pipe: %w", err)
	}

	// Format entries for fzf with indicators
	go func() {
		defer fzfIn.Close()
		for _, entry := range entries {
			// Apply filter if provided
			if filter != "" {
				if !strings.Contains(strings.ToLower(entry.Alias), strings.ToLower(filter)) &&
					!strings.Contains(strings.ToLower(entry.Target), strings.ToLower(filter)) {
					continue
				}
			}

			// Get indicators
			indicators := ""
			if score := scoreMap[entry.Alias]; score != nil {
				if score.Recent > 0 {
					indicators += "⭐ "
				}
				if score.Frequent > 5 {
					indicators += "🔥 "
				}
			}

			// Format: [indicators] alias (type): target
			fmt.Fprintf(fzfIn, "%s%s (%s): %s\n", indicators, entry.Alias, entry.Type, entry.Target)
		}
	}()

	// Run fzf and capture output
	output, err := fzfCmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			if exitErr.ExitCode() == 130 {
				// User cancelled (Ctrl+C)
				return nil
			}
		}
		return fmt.Errorf("fzf error: %w", err)
	}

	// Parse selected entry
	selected := strings.TrimSpace(string(output))
	if selected == "" {
		return nil
	}

	// Extract alias (first part before space)
	parts := strings.Fields(selected)
	if len(parts) == 0 {
		return fmt.Errorf("invalid selection")
	}

	alias := parts[0]

	// Open the selected entry
	fmt.Printf("Opening %s...\n", alias)
	return handleOpen(alias, []string{})
}

// handleUISimple handles UI fallback with interactive selection
func handleUISimple(filter string) error {
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
		if filter != "" {
			if !strings.Contains(strings.ToLower(entry.Alias), strings.ToLower(filter)) &&
				!strings.Contains(strings.ToLower(entry.Target), strings.ToLower(filter)) {
				continue
			}
		}
		filtered = append(filtered, entry)
	}

	if len(filtered) == 0 {
		fmt.Printf("No entries match filter: %s\n", filter)
		return nil
	}

	// Display entries
	fmt.Println("\nSelect an entry:")
	fmt.Println(strings.Repeat("-", 80))
	for i, entry := range filtered {
		fmt.Printf("%2d. %-20s %-10s %s\n", i+1, entry.Alias, entry.Type, entry.Target)
	}
	fmt.Println(strings.Repeat("-", 80))

	// Prompt for selection
	fmt.Print("\nEnter number (or 'q' to quit): ")
	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("failed to read input: %w", err)
	}

	input = strings.TrimSpace(input)
	if input == "q" || input == "Q" {
		return nil
	}

	// Parse selection
	choice, err := strconv.Atoi(input)
	if err != nil {
		return fmt.Errorf("invalid selection: %s", input)
	}

	if choice < 1 || choice > len(filtered) {
		return fmt.Errorf("selection out of range: %d", choice)
	}

	selected := filtered[choice-1]

	// Open the selected entry
	fmt.Printf("\nOpening %s...\n", selected.Alias)
	return handleOpen(selected.Alias, []string{})
}
