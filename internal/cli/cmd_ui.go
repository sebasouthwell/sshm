package cli

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/Sebasouthwell/sshm/internal/history"
	"github.com/Sebasouthwell/sshm/internal/inventory"
	"github.com/Sebasouthwell/sshm/internal/provider"
	"github.com/Sebasouthwell/sshm/internal/utils"
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

// handleUIFZF handles UI with fzf and rich bindings
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

	// Sort entries by recency (recent first, then alphabetical)
	sort.Slice(entries, func(i, j int) bool {
		scoreI := scoreMap[entries[i].Alias]
		scoreJ := scoreMap[entries[j].Alias]
		
		// Recent entries first
		if scoreI != nil && scoreI.Recent > 0 && (scoreJ == nil || scoreJ.Recent == 0) {
			return true
		}
		if scoreJ != nil && scoreJ.Recent > 0 && (scoreI == nil || scoreI.Recent == 0) {
			return false
		}
		
		// Then by total score
		if scoreI != nil && scoreJ != nil {
			if scoreI.TotalScore != scoreJ.TotalScore {
				return scoreI.TotalScore > scoreJ.TotalScore
			}
		}
		
		// Finally alphabetical
		return entries[i].Alias < entries[j].Alias
	})

	// Build fzf command with bindings
	header := "Enter=ssh | Ctrl-d=cd | Ctrl-h=host | Ctrl-k=key | Ctrl-u=user@host | Ctrl-p=cmd | Ctrl-s=show | Ctrl-e=edit | Ctrl-t=test | Ctrl-w=workdir | ?=help"
	fzfArgs := []string{
		"--height=85%",
		"--border",
		"--ansi",
		"--layout=reverse",
		"--prompt=sm> ",
		"--header=" + header,
		"--expect=enter,ctrl-d,ctrl-h,ctrl-k,ctrl-u,ctrl-p,ctrl-s,ctrl-e,ctrl-t,ctrl-w",
		"--preview-window=right:40%:wrap",
		"--preview=echo 'Use Ctrl-s to show full details'",
		"--bind=?:toggle-preview",
	}

	fzfCmd := exec.Command("fzf", fzfArgs...)
	fzfCmd.Stderr = os.Stderr
	fzfIn, err := fzfCmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to create fzf stdin pipe: %w", err)
	}

	// Format entries for fzf with enhanced display
	go func() {
		defer fzfIn.Close()
		for _, entry := range entries {
			// Apply filter if provided
			if filter != "" {
				lowerFilter := strings.ToLower(filter)
				if !strings.Contains(strings.ToLower(entry.Alias), lowerFilter) &&
					!strings.Contains(strings.ToLower(entry.Target), lowerFilter) &&
					!strings.Contains(strings.ToLower(entry.Tags), lowerFilter) {
					continue
				}
			}

			// Get indicators and colors
			icon := ""
			isRecent := false
			
			if score := scoreMap[entry.Alias]; score != nil {
				if score.Recent > 0 {
					isRecent = true
				}
			}

			// Determine icon and color based on type/target
			if strings.HasPrefix(entry.Target, "tf:") {
				icon = "\033[35m⛏\033[0m" // Magenta for TF
			} else if matched, _ := regexp.MatchString(`^\d+\.\d+\.\d+\.\d+$`, entry.Target); matched {
				icon = "\033[36m●\033[0m" // Cyan for IP
			} else {
				icon = "\033[32m◌\033[0m" // Green for DNS
			}

			// Add star for recently used
			if isRecent {
				icon = "\033[33m⭐\033[0m " + icon
			}

			// Format fields
			user := entry.User
			if user == "" {
				user = "-"
			}
			port := entry.Port
			if port == "" {
				port = "-"
			}

			// Key basename
			keyBase := "-"
			if entry.Key != "" {
				keyBase = filepath.Base(entry.Key)
				if len(keyBase) > 16 {
					keyBase = keyBase[:13] + "..."
				}
			}

			// Workdir basename
			workdirBase := "-"
			if entry.Workdir != "" {
				workdirBase = filepath.Base(entry.Workdir)
				if len(workdirBase) > 14 {
					workdirBase = workdirBase[:11] + "..."
				}
			}

			// Tags
			tags := entry.Tags
			if tags == "" {
				tags = "-"
			} else if len(tags) > 12 {
				tags = tags[:9] + "..."
			}

			// Filebase (category)
			filebase := entry.Filebase
			if filebase == "" {
				filebase = "default"
			}

			// Shorten target display
			targetDisplay := entry.Target
			if len(targetDisplay) > 46 {
				targetDisplay = targetDisplay[:43] + "..."
			}

			// Format: icon alias<TAB>target<TAB>user<TAB>port<TAB>key<TAB>workdir<TAB>tags<TAB>filebase
			// Use separator that won't conflict with display
			line := fmt.Sprintf("%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s",
				entry.Alias, // For extraction
				icon,
				entry.Alias,
				targetDisplay,
				user,
				port,
				keyBase,
				workdirBase,
				tags,
				"\033[2m["+filebase+"]\033[0m",
			)
			fmt.Fprintf(fzfIn, "%s\n", line)
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

	// Parse output - first line is keypress, second is selection
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) < 2 {
		return nil // No selection
	}

	keypress := strings.TrimSpace(lines[0])
	selected := strings.TrimSpace(lines[1])

	if selected == "" {
		return nil
	}

	// Extract alias (first field before tab)
	parts := strings.Split(selected, "\t")
	if len(parts) == 0 {
		return fmt.Errorf("invalid selection")
	}

	alias := parts[0]

	// Find entry for bindings
	entry, err := manager.Find(alias)
	if err != nil {
		return fmt.Errorf("alias not found: %s", alias)
	}

	// Handle keypress bindings
	switch keypress {
	case "ctrl-d":
		// Change to workdir or key directory
		return HandleCD(alias)
	case "ctrl-h":
		// Copy host/IP to clipboard
		return handleUICopyHost(entry)
	case "ctrl-k":
		// Copy key path to clipboard
		return handleUICopyKey(entry)
	case "ctrl-u":
		// Copy user@host to clipboard
		return handleUICopyUserHost(entry)
	case "ctrl-p":
		// Copy full SSH command to clipboard
		return handleUICopyCommand(entry)
	case "ctrl-s":
		// Show full entry details
		return HandleShow(alias)
	case "ctrl-e":
		// Edit inventory file
		return HandleEdit(entry.Filebase)
	case "ctrl-t":
		// Test connection
		return HandleTest(alias)
	case "ctrl-w":
		// Copy workdir path to clipboard
		return handleUICopyWorkdir(entry)
	case "enter", "":
		// Open connection
		fmt.Printf("Opening %s...\n", alias)
		return handleOpen(alias, []string{})
	default:
		// Unknown keypress, treat as enter
		fmt.Printf("Opening %s...\n", alias)
		return handleOpen(alias, []string{})
	}
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

// handleUICopyHost copies host/IP to clipboard
func handleUICopyHost(entry *inventory.Entry) error {
	// For TF entries, try to resolve IP
	target := entry.Target
	if strings.HasPrefix(target, "tf:") {
		// Try to resolve, but don't fail if we can't
		prov, err := GetProvider("tf")
		if err == nil {
			opts := provider.RuntimeOpts{}
			resolved, err := prov.Resolve(entry, opts)
			if err == nil && resolved != nil {
				target = resolved.Target
			}
		}
	}

	if err := utils.CopyToClipboard(target); err != nil {
		fmt.Printf("Host/IP: %s (no clipboard tool found)\n", target)
		return nil
	}
	fmt.Printf("Copied: %s\n", target)
	return nil
}

// handleUICopyKey copies key path to clipboard
func handleUICopyKey(entry *inventory.Entry) error {
	if entry.Key == "" {
		fmt.Println("No key set for this entry")
		return nil
	}
	if err := utils.CopyToClipboard(entry.Key); err != nil {
		fmt.Printf("Key: %s (no clipboard tool found)\n", entry.Key)
		return nil
	}
	fmt.Printf("Copied key: %s\n", entry.Key)
	return nil
}

// handleUICopyUserHost copies user@host to clipboard
func handleUICopyUserHost(entry *inventory.Entry) error {
	// Resolve target if needed
	target := entry.Target
	if strings.HasPrefix(target, "tf:") {
		prov, err := GetProvider("tf")
		if err == nil {
			opts := provider.RuntimeOpts{}
			resolved, err := prov.Resolve(entry, opts)
			if err == nil && resolved != nil {
				target = resolved.Target
			}
		}
	}

	userHost := target
	if entry.User != "" {
		userHost = entry.User + "@" + target
	}

	if err := utils.CopyToClipboard(userHost); err != nil {
		fmt.Printf("User@Host: %s (no clipboard tool found)\n", userHost)
		return nil
	}
	fmt.Printf("Copied: %s\n", userHost)
	return nil
}

// handleUICopyCommand copies full SSH command to clipboard
func handleUICopyCommand(entry *inventory.Entry) error {
	if entry.Type != "ssh" && entry.Type != "tf" {
		fmt.Println("Command copy only supported for SSH/TF entries")
		return nil
	}

	// Build command using dry-run
	prov, err := GetProvider(entry.Type)
	if err != nil {
		return fmt.Errorf("failed to get provider: %w", err)
	}

	opts := provider.RuntimeOpts{
		DryRun: true,
	}

	resolved, err := prov.Resolve(entry, opts)
	if err != nil {
		return fmt.Errorf("failed to resolve entry: %w", err)
	}

	plan, err := prov.Build(provider.ActionOpen, entry, resolved, opts)
	if err != nil {
		return fmt.Errorf("failed to build command: %w", err)
	}

	// Format command
	cmdStr := strings.Join(plan.Argv, " ")
	if err := utils.CopyToClipboard(cmdStr); err != nil {
		fmt.Printf("Command: %s (no clipboard tool found)\n", cmdStr)
		return nil
	}
	fmt.Printf("Copied command: %s\n", cmdStr)
	return nil
}

// handleUICopyWorkdir copies workdir path to clipboard
func handleUICopyWorkdir(entry *inventory.Entry) error {
	if entry.Workdir == "" {
		fmt.Println("No workdir set for this alias")
		return nil
	}
	if err := utils.CopyToClipboard(entry.Workdir); err != nil {
		fmt.Printf("Workdir: %s (no clipboard tool found)\n", entry.Workdir)
		return nil
	}
	fmt.Printf("Copied workdir: %s\n", entry.Workdir)
	return nil
}
