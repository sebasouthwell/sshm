package token

import (
	"testing"
)

func TestParseTokens(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantUser string
		wantPort string
		wantCmd  []string
		wantPass []string
	}{
		{
			name:     "simple tokens",
			args:     []string{"user=admin", "p=2222"},
			wantUser: "admin",
			wantPort: "2222",
		},
		{
			name:     "passthrough args",
			args:     []string{"user=admin", "--", "-v", "-o", "StrictHostKeyChecking=no"},
			wantUser: "admin",
			wantPass: []string{"-v", "-o", "StrictHostKeyChecking=no"},
		},
		{
			name:     "remote command",
			args:     []string{"user=admin", "::", "uptime", "-p"},
			wantUser: "admin",
			wantCmd:  []string{"uptime", "-p"},
		},
		{
			name:     "all together",
			args:     []string{"user=admin", "p=2222", "--", "-v", "::", "uptime"},
			wantUser: "admin",
			wantPort: "2222",
			wantPass: []string{"-v"},
			wantCmd:  []string{"uptime"},
		},
		{
			name:     "port forwarding",
			args:     []string{"L=8080:localhost:8080", "R=9090:localhost:9090"},
			wantPort: "",
		},
		{
			name:     "boolean flags",
			args:     []string{"A", "t", "v"},
			wantUser: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Parse(tt.args)

			if tt.wantUser != "" {
				if result.Tokens["user"] != tt.wantUser {
					t.Errorf("User = %v, want %v", result.Tokens["user"], tt.wantUser)
				}
			}

			if tt.wantPort != "" {
				if result.Tokens["p"] != tt.wantPort {
					t.Errorf("Port = %v, want %v", result.Tokens["p"], tt.wantPort)
				}
			}

			if len(tt.wantPass) > 0 {
				if len(result.Passthrough) != len(tt.wantPass) {
					t.Errorf("Passthrough length = %v, want %v", len(result.Passthrough), len(tt.wantPass))
				}
			}

			if len(tt.wantCmd) > 0 {
				if len(result.Command) != len(tt.wantCmd) {
					t.Errorf("Command length = %v, want %v", len(result.Command), len(tt.wantCmd))
				}
			}
		})
	}
}

func TestGetStringOrDefault(t *testing.T) {
	result := Parse([]string{"user=admin", "p=2222"})

	if result.GetString("user") != "admin" {
		t.Errorf("GetString(user) = %v, want admin", result.GetString("user"))
	}

	if result.GetStringOrDefault("p", "22") != "2222" {
		t.Errorf("GetStringOrDefault(p) = %v, want 2222", result.GetStringOrDefault("p", "22"))
	}

	if result.GetStringOrDefault("missing", "default") != "default" {
		t.Errorf("GetStringOrDefault(missing) = %v, want default", result.GetStringOrDefault("missing", "default"))
	}

	if !result.HasToken("user") {
		t.Errorf("HasToken(user) = false, want true")
	}
}
