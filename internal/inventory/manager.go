package inventory

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Sebasouthwell/sshm/internal/utils"
)

// Manager handles inventory file operations
type Manager struct {
	invDir       string
	defaultBase  string
	entriesCache map[string][]*Entry // Cache by filebase
}

// NewManager creates a new inventory manager
func NewManager(invDir, defaultBase string) *Manager {
	return &Manager{
		invDir:       invDir,
		defaultBase:  defaultBase,
		entriesCache: make(map[string][]*Entry),
	}
}

// LoadAll loads all entries from all inventory files
func (m *Manager) LoadAll() ([]*Entry, error) {
	var allEntries []*Entry

	// Ensure inventory directory exists
	if err := os.MkdirAll(m.invDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create inventory directory: %w", err)
	}

	// Find all .inv files
	pattern := filepath.Join(m.invDir, "*.inv")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("failed to glob inventory files: %w", err)
	}

	// Load entries from each file
	for _, filePath := range matches {
		entries, err := ParseFile(filePath)
		if err != nil {
			// Log error but continue with other files
			fmt.Fprintf(os.Stderr, "warning: failed to parse %s: %v\n", filePath, err)
			continue
		}
		allEntries = append(allEntries, entries...)
	}

	return allEntries, nil
}

// Find finds an entry by alias
func (m *Manager) Find(alias string) (*Entry, error) {
	entries, err := m.LoadAll()
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if entry.Alias == alias {
			return entry, nil
		}
	}

	return nil, fmt.Errorf("alias not found: %s", alias)
}

// Add adds or updates an entry
// If alias exists, removes it from all files first, then adds to specified filebase
func (m *Manager) Add(entry *Entry, filebase string) error {
	// Remove alias from all files if it exists
	if err := m.Remove(entry.Alias); err != nil {
		// Ignore "not found" errors
		if !strings.Contains(err.Error(), "not found") {
			return err
		}
	}

	// Determine target file
	if filebase == "" {
		filebase = m.defaultBase
	}
	filePath := m.getFilePath(filebase)

	// Ensure directory exists
	if err := os.MkdirAll(m.invDir, 0755); err != nil {
		return fmt.Errorf("failed to create inventory directory: %w", err)
	}

	// Create backup before writing
	if err := utils.BackupFile(filePath); err != nil {
		// Log warning but continue
		fmt.Fprintf(os.Stderr, "warning: failed to create backup: %v\n", err)
	}

	// Append entry to file
	file, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open inventory file: %w", err)
	}
	defer file.Close()

	line := FormatEntry(entry)
	if _, err := fmt.Fprintln(file, line); err != nil {
		return fmt.Errorf("failed to write entry: %w", err)
	}

	// Invalidate cache
	delete(m.entriesCache, filebase)

	return nil
}

// Remove removes an entry by alias from all inventory files
func (m *Manager) Remove(alias string) error {
	entries, err := m.LoadAll()
	if err != nil {
		return err
	}

	// Find entry
	var found bool
	for _, entry := range entries {
		if entry.Alias == alias {
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("alias not found: %s", alias)
	}

	// Remove from all files
	pattern := filepath.Join(m.invDir, "*.inv")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return fmt.Errorf("failed to glob inventory files: %w", err)
	}

	for _, filePath := range matches {
		if err := m.removeFromFile(filePath, alias); err != nil {
			return fmt.Errorf("failed to remove from %s: %w", filePath, err)
		}
	}

	return nil
}

// removeFromFile removes an alias from a specific file
func (m *Manager) removeFromFile(filePath, alias string) error {
	entries, err := ParseFile(filePath)
	if err != nil {
		return err
	}

	// Filter out the alias
	var filtered []*Entry
	for _, entry := range entries {
		if entry.Alias != alias {
			filtered = append(filtered, entry)
		}
	}

	// Create backup before writing
	if err := utils.BackupFile(filePath); err != nil {
		// Log warning but continue
		fmt.Fprintf(os.Stderr, "warning: failed to create backup: %v\n", err)
	}

	// Build file content
	var lines []string
	for _, entry := range filtered {
		lines = append(lines, FormatEntry(entry))
	}
	content := strings.Join(lines, "\n")
	if len(lines) > 0 {
		content += "\n"
	}

	// Atomic write
	if err := utils.AtomicWrite(filePath, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

// getFilePath returns the full path for an inventory file
func (m *Manager) getFilePath(filebase string) string {
	return filepath.Join(m.invDir, filebase+".inv")
}

// GetInvDir returns the inventory directory path
func (m *Manager) GetInvDir() string {
	return m.invDir
}
