package history

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

var (
	maxEvents    = 2000
	recordCmd    = false
	recordResolved = true
)

func init() {
	if val := os.Getenv("SSHM_HISTORY_MAX_EVENTS"); val != "" {
		if n, err := strconv.Atoi(val); err == nil {
			maxEvents = n
		}
	}
	if val := os.Getenv("SSHM_HISTORY_RECORD_CMD"); val == "true" {
		recordCmd = true
	}
	if val := os.Getenv("SSHM_HISTORY_RECORD_RESOLVED"); val == "false" {
		recordResolved = false
	}
}

// Event represents a history event
type Event struct {
	TS       int64             `json:"ts"`
	Alias    string            `json:"alias"`
	Type     string            `json:"type"`
	Action   string            `json:"action"`
	Resolved *ResolvedInfo     `json:"resolved,omitempty"`
	Meta     map[string]string `json:"meta,omitempty"`
	Cmd      *CmdInfo          `json:"cmd,omitempty"`
	Status   string            `json:"status"`
	ExitCode int               `json:"exit_code,omitempty"`
	Duration int64             `json:"duration_ms,omitempty"`
}

// ResolvedInfo contains resolved target information
type ResolvedInfo struct {
	Target string            `json:"target"`
	User   string            `json:"user,omitempty"`
	Port   string            `json:"port,omitempty"`
	Additional map[string]string `json:"additional,omitempty"`
}

// CmdInfo contains command information
type CmdInfo struct {
	Kind string   `json:"kind"`
	Argv []string `json:"argv,omitempty"`
}

// Logger handles history logging
type Logger struct {
	historyFile string
}

// NewLogger creates a new history logger
func NewLogger(historyFile string) *Logger {
	if historyFile == "" {
		// Default location
		home, _ := os.UserHomeDir()
		historyFile = filepath.Join(home, ".ssh", "inventory.d", "history.jsonl")
	}

	return &Logger{
		historyFile: historyFile,
	}
}

// Log logs an event to history
func (l *Logger) Log(event *Event) error {
	// Set timestamp if not set
	if event.TS == 0 {
		event.TS = time.Now().Unix()
	}

	// Don't record cmd if privacy setting disabled
	if !recordCmd && event.Cmd != nil {
		event.Cmd = nil
	}

	// Don't record resolved if privacy setting disabled
	if !recordResolved && event.Resolved != nil {
		event.Resolved = nil
	}

	// Ensure directory exists
	dir := filepath.Dir(l.historyFile)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create history directory: %w", err)
	}

	// Append event to file
	file, err := os.OpenFile(l.historyFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open history file: %w", err)
	}
	defer file.Close()

	// Encode as JSON
	encoder := json.NewEncoder(file)
	if err := encoder.Encode(event); err != nil {
		return fmt.Errorf("failed to encode event: %w", err)
	}

	// Periodically compact (1% of the time)
	if event.TS%100 == 0 {
		go l.compact()
	}

	return nil
}

// compact removes old events, keeping only the most recent maxEvents
func (l *Logger) compact() {
	// Read all events
	events, err := l.readAll()
	if err != nil {
		return // Silently fail
	}

	// Keep only recent events
	if len(events) <= maxEvents {
		return
	}

	// Sort by timestamp (newest first) and keep top N
	// Simple approach: keep last maxEvents lines
	lines, err := readLines(l.historyFile)
	if err != nil {
		return
	}

	if len(lines) <= maxEvents {
		return
	}

	// Keep only last maxEvents lines
	keepLines := lines[len(lines)-maxEvents:]

	// Write back
	tmpFile := l.historyFile + ".tmp"
	file, err := os.Create(tmpFile)
	if err != nil {
		return
	}
	defer file.Close()

	for _, line := range keepLines {
		fmt.Fprintln(file, line)
	}

	// Atomic rename
	os.Rename(tmpFile, l.historyFile)
}

// readAll reads all events from history file
func (l *Logger) readAll() ([]*Event, error) {
	data, err := ioutil.ReadFile(l.historyFile)
	if err != nil {
		if os.IsNotExist(err) {
			return []*Event{}, nil
		}
		return nil, err
	}

	var events []*Event
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		var event Event
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			continue // Skip invalid lines
		}
		events = append(events, &event)
	}

	return events, nil
}

// readLines reads all lines from a file
func readLines(filename string) ([]string, error) {
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}

	lines := strings.Split(string(data), "\n")
	// Remove empty last line if file ends with newline
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	return lines, nil
}
