package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

var cacheCmd = &cobra.Command{
	Use:   "cache [command]",
	Short: "Manage SSHM caches",
	Long: `Manage SSHM caches (Terraform resolution cache, etc.).

Examples:
  sshm cache clear        # Clear all caches
  sshm cache clear tf     # Clear only Terraform cache
  sshm cache stats       # Show cache statistics`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return cmd.Help()
		}
		return handleCache(args[0], args[1:])
	},
}

func init() {
	cacheCmd.AddCommand(&cobra.Command{
		Use:   "clear [type]",
		Short: "Clear caches",
		Long:  "Clear all caches or a specific cache type (tf, ssm, etc.)",
		RunE: func(cmd *cobra.Command, args []string) error {
			cacheType := ""
			if len(args) > 0 {
				cacheType = args[0]
			}
			return handleCacheClear(cacheType)
		},
	})
	cacheCmd.AddCommand(&cobra.Command{
		Use:   "stats",
		Short: "Show cache statistics",
		RunE: func(cmd *cobra.Command, args []string) error {
			return handleCacheStats()
		},
	})
}

// handleCache handles cache subcommands
func handleCache(subcmd string, args []string) error {
	switch subcmd {
	case "clear":
		cacheType := ""
		if len(args) > 0 {
			cacheType = args[0]
		}
		return handleCacheClear(cacheType)
	case "stats":
		return handleCacheStats()
	default:
		return fmt.Errorf("unknown cache command: %s", subcmd)
	}
}

// handleCacheClear clears caches
func handleCacheClear(cacheType string) error {
	tmpDir := os.TempDir()
	if tmpDir == "" {
		tmpDir = "/tmp"
	}

	var patterns []string
	if cacheType == "" {
		// Clear all caches
		patterns = []string{"sshm-tf-*", "sshm-ssm-*", "sshm-*"}
	} else if cacheType == "tf" {
		patterns = []string{"sshm-tf-*"}
	} else if cacheType == "ssm" {
		patterns = []string{"sshm-ssm-*"}
	} else {
		return fmt.Errorf("unknown cache type: %s (supported: tf, ssm)", cacheType)
	}

	cleared := 0
	for _, pattern := range patterns {
		matches, err := filepath.Glob(filepath.Join(tmpDir, pattern))
		if err != nil {
			continue
		}
		for _, match := range matches {
			if err := os.Remove(match); err == nil {
				cleared++
			}
		}
	}

	if cacheType == "" {
		fmt.Printf("Cleared %d cache files\n", cleared)
	} else {
		fmt.Printf("Cleared %d %s cache files\n", cleared, cacheType)
	}

	return nil
}

// handleCacheStats shows cache statistics
func handleCacheStats() error {
	tmpDir := os.TempDir()
	if tmpDir == "" {
		tmpDir = "/tmp"
	}

	patterns := map[string]string{
		"Terraform": "sshm-tf-*",
		"SSM":       "sshm-ssm-*",
	}

	totalSize := int64(0)
	totalFiles := 0

	fmt.Println("Cache Statistics:")
	fmt.Println(strings.Repeat("-", 50))

	for name, pattern := range patterns {
		matches, err := filepath.Glob(filepath.Join(tmpDir, pattern))
		if err != nil {
			continue
		}

		count := len(matches)
		size := int64(0)
		for _, match := range matches {
			if info, err := os.Stat(match); err == nil {
				size += info.Size()
			}
		}

		fmt.Printf("%-15s: %d files, %s\n", name, count, formatBytes(size))
		totalFiles += count
		totalSize += size
	}

	fmt.Println(strings.Repeat("-", 50))
	fmt.Printf("%-15s: %d files, %s\n", "Total", totalFiles, formatBytes(totalSize))

	return nil
}

// formatBytes formats bytes as human-readable string
func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
