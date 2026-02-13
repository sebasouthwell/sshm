package utils

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"
)

// BackupFile creates a backup of a file before modification
func BackupFile(filePath string) error {
	// Check if file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return nil // No backup needed if file doesn't exist
	}

	// Create backup filename with timestamp
	backupPath := filePath + ".bak"
	
	// If backup already exists, add timestamp
	if _, err := os.Stat(backupPath); err == nil {
		timestamp := time.Now().Format("20060102_150405")
		backupPath = fmt.Sprintf("%s.%s.bak", filePath, timestamp)
	}

	// Read original file
	data, err := ioutil.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read file for backup: %w", err)
	}

	// Write backup
	if err := ioutil.WriteFile(backupPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write backup file: %w", err)
	}

	return nil
}

// AtomicWrite writes data to a file atomically
func AtomicWrite(filePath string, data []byte, perm os.FileMode) error {
	// Create temporary file in same directory
	dir := filepath.Dir(filePath)
	tmpFile, err := ioutil.TempFile(dir, filepath.Base(filePath)+".tmp.")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath) // Clean up on error

	// Write data to temp file
	if _, err := tmpFile.Write(data); err != nil {
		tmpFile.Close()
		return fmt.Errorf("failed to write temp file: %w", err)
	}

	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("failed to close temp file: %w", err)
	}

	// Set permissions
	if err := os.Chmod(tmpPath, perm); err != nil {
		return fmt.Errorf("failed to set permissions: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tmpPath, filePath); err != nil {
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	return nil
}

// AtomicWriteWithBackup writes data to a file atomically with backup
func AtomicWriteWithBackup(filePath string, data []byte, perm os.FileMode) error {
	// Create backup first
	if err := BackupFile(filePath); err != nil {
		return fmt.Errorf("backup failed: %w", err)
	}

	// Atomic write
	return AtomicWrite(filePath, data, perm)
}
