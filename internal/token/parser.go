package token

import (
	"strings"
)

// ParsedTokens contains parsed tokens and passthrough arguments
type ParsedTokens struct {
	Tokens      map[string]string // Friendly tokens (user=, p=, etc.)
	Passthrough []string          // Raw arguments after --
	Command     []string          // Remote command after ::
}

// Parse parses command-line tokens into structured format
// Handles friendly tokens, -- passthrough, and :: command separators
func Parse(args []string) *ParsedTokens {
	result := &ParsedTokens{
		Tokens:      make(map[string]string),
		Passthrough: []string{},
		Command:     []string{},
	}

	mode := "tokens" // tokens, passthrough, command

	for _, arg := range args {
		switch arg {
		case "--":
			mode = "passthrough"
			continue
		case "::":
			mode = "command"
			continue
		}

		switch mode {
		case "command":
			result.Command = append(result.Command, arg)
		case "passthrough":
			result.Passthrough = append(result.Passthrough, arg)
		case "tokens":
			// Try to parse as key=value token
			if key, value, ok := parseToken(arg); ok {
				result.Tokens[key] = value
			} else {
				// Unknown token - treat as passthrough for now
				// (providers can handle these)
				result.Passthrough = append(result.Passthrough, arg)
			}
		}
	}

	return result
}

// parseToken attempts to parse arg as key=value token
// Returns key, value, and true if successful
func parseToken(arg string) (string, string, bool) {
	// Check for = separator
	idx := strings.Index(arg, "=")
	if idx == -1 {
		return "", "", false
	}

	key := strings.ToLower(arg[:idx])
	value := arg[idx+1:]

	// Validate key format (should be alphanumeric + some special chars)
	if key == "" {
		return "", "", false
	}

	return key, value, true
}

// GetString returns a token value as string, or empty string if not found
func (pt *ParsedTokens) GetString(key string) string {
	return pt.Tokens[strings.ToLower(key)]
}

// GetStringOrDefault returns a token value or default if not found
func (pt *ParsedTokens) GetStringOrDefault(key, defaultValue string) string {
	if value := pt.GetString(key); value != "" {
		return value
	}
	return defaultValue
}

// HasToken checks if a token exists
func (pt *ParsedTokens) HasToken(key string) bool {
	_, exists := pt.Tokens[strings.ToLower(key)]
	return exists
}

// GetBoolToken checks if a boolean token exists (e.g., "dry", "agent")
func (pt *ParsedTokens) GetBoolToken(key string) bool {
	value := pt.GetString(key)
	return value == "true" || value == "yes" || value == "1" || value == ""
}
