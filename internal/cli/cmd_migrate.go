package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/Sebasouthwell/sshm/internal/inventory"
)

var (
	migrateDryRun bool
)

var migrateCmd = &cobra.Command{
	Use:   "migrate [--dry-run]",
	Short: "Migrate legacy inventory files to JSON format",
	Long: `Migrate legacy .inv files to JSON format.

Scans for all .inv files and converts them to .json format.
Creates backups of original files.

Examples:
  sshm migrate
  sshm migrate --dry-run`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return handleMigrate(migrateDryRun)
	},
}

func init() {
	migrateCmd.Flags().BoolVar(&migrateDryRun, "dry-run", false, "Show what would be migrated without doing it")
}

// handleMigrate handles the migrate command
func handleMigrate(dryRun bool) error {
	invDir := manager.GetInvDir()

	// Find all .inv files
	pattern := filepath.Join(invDir, "*.inv")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return fmt.Errorf("failed to glob inventory files: %w", err)
	}

	// Filter out backup files
	var invFiles []string
	for _, filePath := range matches {
		if !strings.HasSuffix(filePath, ".bak") && !strings.Contains(filePath, ".bak.") {
			invFiles = append(invFiles, filePath)
		}
	}

	if len(invFiles) == 0 {
		fmt.Println("No legacy .inv files found to migrate")
		return nil
	}

	fmt.Printf("Found %d legacy file(s) to migrate:\n\n", len(invFiles))

	migratedCount := 0
	skippedCount := 0
	errorCount := 0

	for _, filePath := range invFiles {
		filebase := getFilebaseFromPath(filePath, ".inv")
		jsonPath := filepath.Join(invDir, filebase+".json")

		// Check if JSON file already exists
		if _, err := os.Stat(jsonPath); err == nil {
			fmt.Printf("  ⏭  %s -> %s (JSON already exists, skipping)\n", filepath.Base(filePath), filepath.Base(jsonPath))
			skippedCount++
			continue
		}

		if dryRun {
			fmt.Printf("  [DRY RUN] Would migrate: %s -> %s\n", filepath.Base(filePath), filepath.Base(jsonPath))
			migratedCount++
			continue
		}

		// Parse legacy file
		entries, err := inventory.ParseFile(filePath)
		if err != nil {
			fmt.Printf("  ✗  %s (parse error: %v)\n", filepath.Base(filePath), err)
			errorCount++
			continue
		}

		// Write JSON file
		if err := inventory.WriteJSONFile(jsonPath, entries); err != nil {
			fmt.Printf("  ✗  %s -> %s (write error: %v)\n", filepath.Base(filePath), filepath.Base(jsonPath), err)
			errorCount++
			continue
		}

		// Backup legacy file
		backupPath := filePath + ".bak"
		if err := os.Rename(filePath, backupPath); err != nil {
			fmt.Printf("  ⚠  %s -> %s (migrated, but backup failed: %v)\n", filepath.Base(filePath), filepath.Base(jsonPath), err)
		} else {
			fmt.Printf("  ✓  %s -> %s (backed up to %s)\n", filepath.Base(filePath), filepath.Base(jsonPath), filepath.Base(backupPath))
		}

		migratedCount++
	}

	fmt.Println()
	if dryRun {
		fmt.Printf("Dry run complete: Would migrate %d file(s)\n", migratedCount)
	} else {
		fmt.Printf("Migration complete:\n")
		fmt.Printf("  Migrated: %d file(s)\n", migratedCount)
		if skippedCount > 0 {
			fmt.Printf("  Skipped: %d file(s) (JSON already exists)\n", skippedCount)
		}
		if errorCount > 0 {
			fmt.Printf("  Errors: %d file(s)\n", errorCount)
		}
	}

	return nil
}

// getFilebaseFromPath extracts filebase from file path
func getFilebaseFromPath(filePath, ext string) string {
	base := filepath.Base(filePath)
	return strings.TrimSuffix(base, ext)
}
