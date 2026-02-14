package inventory

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
)

// EntryJSON represents an entry in JSON format (for serialization)
// Internal fields (File, Filebase) are not serialized
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
}

// ParseJSONFile parses a JSON inventory file and returns entries
func ParseJSONFile(filePath string) ([]*Entry, error) {
	data, err := ioutil.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read JSON file: %w", err)
	}

	var jsonEntries []EntryJSON
	if err := json.Unmarshal(data, &jsonEntries); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	entries := make([]*Entry, 0, len(jsonEntries))
	filebase := getFilebase(filePath)

	for i, jsonEntry := range jsonEntries {
		entry, err := convertJSONToEntry(&jsonEntry, filePath, filebase)
		if err != nil {
			return nil, fmt.Errorf("entry %d: %w", i, err)
		}

		// Validate entry
		if err := validateEntry(entry); err != nil {
			return nil, fmt.Errorf("entry %d (%s): %w", i, entry.Alias, err)
		}

		entries = append(entries, entry)
	}

	return entries, nil
}

// WriteJSONFile writes entries to a JSON inventory file
func WriteJSONFile(filePath string, entries []*Entry) error {
	// Convert entries to JSON format
	jsonEntries := make([]EntryJSON, 0, len(entries))
	for _, entry := range entries {
		jsonEntry := convertEntryToJSON(entry)
		jsonEntries = append(jsonEntries, jsonEntry)
	}

	// Marshal with pretty printing
	data, err := json.MarshalIndent(jsonEntries, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}

	// Ensure directory exists
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Atomic write: write to temp file, then rename
	tmpFile := filePath + ".tmp"
	if err := ioutil.WriteFile(tmpFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write temp file: %w", err)
	}

	if err := os.Rename(tmpFile, filePath); err != nil {
		os.Remove(tmpFile) // Clean up on error
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	return nil
}

// convertEntryToJSON converts an Entry to EntryJSON format
func convertEntryToJSON(entry *Entry) EntryJSON {
	jsonEntry := EntryJSON{
		Alias:   entry.Alias,
		Type:    entry.Type,
		Target:  entry.Target,
		User:    entry.User,
		Port:    entry.Port,
		Key:     entry.Key,
		Workdir: entry.Workdir,
		Tags:    entry.Tags,
	}

	// Copy meta map (create new map to avoid sharing)
	if entry.Meta != nil && len(entry.Meta) > 0 {
		jsonEntry.Meta = make(map[string]string)
		for k, v := range entry.Meta {
			jsonEntry.Meta[k] = v
		}
	}

	return jsonEntry
}

// convertJSONToEntry converts EntryJSON to Entry format
func convertJSONToEntry(jsonEntry *EntryJSON, filePath, filebase string) (*Entry, error) {
	entry := NewEntry()
	entry.Alias = jsonEntry.Alias
	entry.Type = jsonEntry.Type
	entry.Target = jsonEntry.Target
	entry.User = jsonEntry.User
	entry.Port = jsonEntry.Port
	entry.Key = jsonEntry.Key
	entry.Workdir = jsonEntry.Workdir
	entry.Tags = jsonEntry.Tags
	entry.File = filePath
	entry.Filebase = filebase

	// Copy meta map
	if jsonEntry.Meta != nil {
		entry.Meta = make(map[string]string)
		for k, v := range jsonEntry.Meta {
			entry.Meta[k] = v
		}
	}

	return entry, nil
}

// validateEntry validates an entry after parsing
func validateEntry(entry *Entry) error {
	// Validate alias format
	if !aliasRegex.MatchString(entry.Alias) {
		return fmt.Errorf("invalid alias format: %s (must match ^[a-zA-Z0-9._-]+$)", entry.Alias)
	}

	// Validate type
	validTypes := map[string]bool{
		"ssh":    true,
		"tf":     true,
		"ssm":    true,
		"docker": true,
		"kube":   true,
	}
	if !validTypes[entry.Type] {
		return fmt.Errorf("invalid provider type: %s (valid: ssh, tf, ssm, docker, kube)", entry.Type)
	}

	// Validate required fields
	if entry.Target == "" {
		return fmt.Errorf("target field is required")
	}

	return nil
}
