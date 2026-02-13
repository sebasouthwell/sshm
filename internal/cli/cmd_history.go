package cli

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/Sebasouthwell/sshm/internal/history"
)

var historyCmd = &cobra.Command{
	Use:   "history [N]",
	Short: "Show command history",
	Long: `Display recent command history.

Example:
  sshm history
  sshm history 20`,
	RunE: func(cmd *cobra.Command, args []string) error {
		count := 10
		if len(args) > 0 {
			if _, err := fmt.Sscanf(args[0], "%d", &count); err != nil {
				return fmt.Errorf("invalid count: %s", args[0])
			}
		}
		return handleHistory(count)
	},
}

// handleHistory handles the history command
func handleHistory(count int) error {
	// Get history file path
	historyFile := os.Getenv("SSHM_HISTORY_FILE")
	if historyFile == "" {
		home, _ := os.UserHomeDir()
		historyFile = filepath.Join(home, ".ssh", "inventory.d", "history.jsonl")
	}

	// Read history file
	events, err := readHistoryEvents(historyFile)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("No history found")
			return nil
		}
		return err
	}

	if len(events) == 0 {
		fmt.Println("No history found")
		return nil
	}

	// Sort by timestamp (newest first)
	sort.Slice(events, func(i, j int) bool {
		return events[i].TS > events[j].TS
	})

	// Show last N
	if count > len(events) {
		count = len(events)
	}

	fmt.Printf("Recent history (showing last %d entries):\n\n", count)
	for i := 0; i < count; i++ {
		event := events[i]
		dt := time.Unix(event.TS, 0).Format("2006-01-02 15:04:05")
		
		statusIcon := "✓"
		if event.Status != "ok" {
			statusIcon = "✗"
		}

		fmt.Printf("%s [%s] %s (%s) - %s", statusIcon, dt, event.Alias, event.Type, event.Action)
		
		if event.Resolved != nil && event.Resolved.Target != "" {
			fmt.Printf(" → %s", event.Resolved.Target)
		}
		
		if event.ExitCode != 0 {
			fmt.Printf(" [exit: %d]", event.ExitCode)
		}
		
		fmt.Println()
	}

	return nil
}

// readHistoryEvents reads events from history file
func readHistoryEvents(filename string) ([]*history.Event, error) {
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	var events []*history.Event
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var event history.Event
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			continue // Skip invalid lines
		}
		events = append(events, &event)
	}

	return events, nil
}
