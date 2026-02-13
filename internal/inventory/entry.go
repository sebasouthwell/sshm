package inventory

import (
	"fmt"
	"strings"
)

// Entry represents a single inventory entry
type Entry struct {
	Alias    string            // Unique identifier
	Type     string            // Provider type: ssh, tf, ssm, docker, kube
	Target   string            // Provider-specific target string
	User     string            // Default user (optional)
	Port     string            // Default port (optional)
	Key      string            // Key path (optional/required depending on type)
	Workdir  string            // Working directory (optional)
	Tags     string            // Comma-separated tags (optional)
	Meta     map[string]string // Provider-specific metadata
	File     string            // Source inventory file path
	Filebase string            // Inventory filebase (filename without .inv)
}

// NewEntry creates a new Entry with initialized Meta map
func NewEntry() *Entry {
	return &Entry{
		Meta: make(map[string]string),
	}
}

// String returns a string representation of the entry
func (e *Entry) String() string {
	return fmt.Sprintf("%s (%s): %s", e.Alias, e.Type, e.Target)
}

// HasTag checks if the entry has a specific tag
func (e *Entry) HasTag(tag string) bool {
	if e.Tags == "" {
		return false
	}
	tags := strings.Split(e.Tags, ",")
	for _, t := range tags {
		if strings.TrimSpace(strings.ToLower(t)) == strings.ToLower(tag) {
			return true
		}
	}
	return false
}

// GetMeta returns a meta value or empty string if not found
func (e *Entry) GetMeta(key string) string {
	if e.Meta == nil {
		return ""
	}
	return e.Meta[key]
}

// SetMeta sets a meta value
func (e *Entry) SetMeta(key, value string) {
	if e.Meta == nil {
		e.Meta = make(map[string]string)
	}
	e.Meta[key] = value
}
