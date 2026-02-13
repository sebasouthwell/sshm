package inventory

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	// aliasRegex validates alias format: ^[a-zA-Z0-9._-]+$
	aliasRegex = regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)
)

// ParseFile parses an inventory file and returns entries
func ParseFile(filePath string) ([]*Entry, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open inventory file: %w", err)
	}
	defer file.Close()

	var entries []*Entry
	scanner := bufio.NewScanner(file)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := strings.TrimRight(scanner.Text(), " \t")

		// Skip comments and empty lines
		if strings.HasPrefix(line, "#") || line == "" {
			continue
		}

		entry, err := ParseLine(line, filePath)
		if err != nil {
			return nil, fmt.Errorf("line %d in %s: %w", lineNum, filePath, err)
		}
		if entry != nil {
			entries = append(entries, entry)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading inventory file: %w", err)
	}

	return entries, nil
}

// ParseLine parses a single inventory line
// Supports both v1 (7 fields) and v2 (9 fields) formats
func ParseLine(line, filePath string) (*Entry, error) {
	// Try tab-separated first (canonical format)
	fields := strings.Split(line, "\t")

	// Fallback to whitespace if no tabs found
	if len(fields) == 1 {
		fields = strings.Fields(line)
	}

	// Determine format based on field count
	if len(fields) == 7 {
		// v1 format: alias key host user? port? workdir? tags?
		return parseV1Line(fields, filePath)
	} else if len(fields) >= 9 {
		// v2 format: alias type target user? port? key? workdir? tags? meta?
		return parseV2Line(fields, filePath)
	}

	return nil, fmt.Errorf("invalid line format: expected 7 (v1) or 9 (v2) fields, got %d", len(fields))
}

// parseV1Line parses v1 format: alias key host user? port? workdir? tags?
func parseV1Line(fields []string, filePath string) (*Entry, error) {
	entry := NewEntry()
	entry.Alias = fields[0]
	entry.Type = "ssh" // v1 is always SSH
	entry.Key = expandPath(fields[1])
	entry.Target = fields[2]
	entry.User = fields[3]
	entry.Port = fields[4]
	entry.Workdir = expandPath(fields[5])
	entry.Tags = fields[6]
	entry.File = filePath
	entry.Filebase = getFilebase(filePath)

	// Validate alias format
	if !aliasRegex.MatchString(entry.Alias) {
		return nil, fmt.Errorf("invalid alias format: %s", entry.Alias)
	}

	return entry, nil
}

// parseV2Line parses v2 format: alias type target user? port? key? workdir? tags? meta?
func parseV2Line(fields []string, filePath string) (*Entry, error) {
	entry := NewEntry()
	entry.Alias = fields[0]
	entry.Type = fields[1]
	entry.Target = fields[2]
	entry.User = fields[3]
	entry.Port = fields[4]
	entry.Key = expandPath(fields[5])
	entry.Workdir = expandPath(fields[6])
	entry.Tags = fields[7]
	entry.File = filePath
	entry.Filebase = getFilebase(filePath)

	// Parse meta field (9th field, optional)
	if len(fields) >= 9 && fields[8] != "" {
		meta, err := parseMeta(fields[8])
		if err != nil {
			return nil, fmt.Errorf("failed to parse meta: %w", err)
		}
		entry.Meta = meta
	}

	// Validate alias format
	if !aliasRegex.MatchString(entry.Alias) {
		return nil, fmt.Errorf("invalid alias format: %s", entry.Alias)
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
		return nil, fmt.Errorf("invalid provider type: %s", entry.Type)
	}

	return entry, nil
}

// parseMeta parses meta field: k=v;k=v;k=v
func parseMeta(metaStr string) (map[string]string, error) {
	meta := make(map[string]string)
	if metaStr == "" {
		return meta, nil
	}

	pairs := strings.Split(metaStr, ";")
	for _, pair := range pairs {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}

		parts := strings.SplitN(pair, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid meta pair: %s (expected k=v)", pair)
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		// Validate key format: [a-zA-Z0-9_.-]+
		keyRegex := regexp.MustCompile(`^[a-zA-Z0-9_.-]+$`)
		if !keyRegex.MatchString(key) {
			return nil, fmt.Errorf("invalid meta key format: %s", key)
		}

		meta[key] = value
	}

	return meta, nil
}

// expandPath expands ~ to home directory and resolves relative paths
func expandPath(path string) string {
	if path == "" {
		return ""
	}

	// Expand ~
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			path = filepath.Join(home, path[2:])
		}
	} else if path == "~" {
		home, err := os.UserHomeDir()
		if err == nil {
			path = home
		}
	}

	// Resolve to absolute path if relative
	if !filepath.IsAbs(path) && path != "" {
		abs, err := filepath.Abs(path)
		if err == nil {
			path = abs
		}
	}

	return path
}

// getFilebase extracts filebase from file path (filename without .inv extension)
func getFilebase(filePath string) string {
	base := filepath.Base(filePath)
	return strings.TrimSuffix(base, ".inv")
}

// FormatEntry formats an entry as a v2 inventory line
func FormatEntry(entry *Entry) string {
	fields := []string{
		entry.Alias,
		entry.Type,
		entry.Target,
		entry.User,
		entry.Port,
		entry.Key,
		entry.Workdir,
		entry.Tags,
		formatMeta(entry.Meta),
	}
	return strings.Join(fields, "\t")
}

// formatMeta formats meta map as k=v;k=v string
func formatMeta(meta map[string]string) string {
	if len(meta) == 0 {
		return ""
	}

	var pairs []string
	for k, v := range meta {
		pairs = append(pairs, fmt.Sprintf("%s=%s", k, v))
	}
	return strings.Join(pairs, ";")
}
