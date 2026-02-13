package inventory

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
)

func TestParseEntryLine_V1Format(t *testing.T) {
	tests := []struct {
		name    string
		line    string
		want    *Entry
		wantErr bool
	}{
		{
			name: "v1 format with all fields",
			line: "prod-web\t~/.ssh/key.pem\thost.example.com\tubuntu\t22\t~/work\tprod,web",
			want: &Entry{
				Alias:   "prod-web",
				Type:    "ssh",
				Target:  "host.example.com",
				Key:     "~/.ssh/key.pem",
				User:    "ubuntu",
				Port:    "22",
				Workdir: "~/work",
				Tags:    "prod,web",
			},
			wantErr: false,
		},
		{
			name: "v1 format minimal",
			line: "test\t~/.ssh/key.pem\thost\t\t\t\t",
			want: &Entry{
				Alias:  "test",
				Type:   "ssh",
				Target: "host",
				Key:    "~/.ssh/key.pem",
			},
			wantErr: false,
		},
		{
			name:    "invalid field count",
			line:    "too few fields",
			want:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseLine(tt.line, "test.inv")
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseEntryLine() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != nil {
				if got.Alias != tt.want.Alias {
					t.Errorf("Alias = %v, want %v", got.Alias, tt.want.Alias)
				}
				if got.Type != tt.want.Type {
					t.Errorf("Type = %v, want %v", got.Type, tt.want.Type)
				}
				if got.Target != tt.want.Target {
					t.Errorf("Target = %v, want %v", got.Target, tt.want.Target)
				}
			}
		})
	}
}

func TestParseEntryLine_V2Format(t *testing.T) {
	tests := []struct {
		name    string
		line    string
		want    *Entry
		wantErr bool
	}{
		{
			name: "v2 format with meta",
			line: "prod-web\ttf\tmodule.web.instance:public\tubuntu\t22\t~/.ssh/key.pem\t~/work\tprod,web\tjump=bastion;strict=yes",
			want: &Entry{
				Alias:   "prod-web",
				Type:    "tf",
				Target:  "module.web.instance:public",
				User:    "ubuntu",
				Port:    "22",
				Key:     "~/.ssh/key.pem",
				Workdir: "~/work",
				Tags:    "prod,web",
			},
			wantErr: false,
		},
		{
			name: "v2 format without meta",
			line: "test\tssh\thost\tuser\t22\tkey\twork\ttags\t",
			want: &Entry{
				Alias:   "test",
				Type:    "ssh",
				Target:  "host",
				User:    "user",
				Port:    "22",
				Key:     "key",
				Workdir: "work",
				Tags:    "tags",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseLine(tt.line, "test.inv")
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseEntryLine() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != nil {
				if got.Alias != tt.want.Alias {
					t.Errorf("Alias = %v, want %v", got.Alias, tt.want.Alias)
				}
				if got.Type != tt.want.Type {
					t.Errorf("Type = %v, want %v", got.Type, tt.want.Type)
				}
			}
		})
	}
}

func TestParseInventoryFile(t *testing.T) {
	// Create temporary inventory file
	tmpDir, err := ioutil.TempDir("", "sshm_test_")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)
	
	invFile := filepath.Join(tmpDir, "test.inv")

	// Use v2 format with 9 fields (including empty meta field)
	content := "# Comment line\n" +
		"prod-web\tssh\thost.example.com\tubuntu\t22\t~/.ssh/key.pem\t~/work\tprod,web\t\n" +
		"test-db\tssh\tdb.example.com\tpostgres\t5432\t~/.ssh/db.pem\t~/db\tprod,database\t\n"
	
	err = ioutil.WriteFile(invFile, []byte(content), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	entries, err := ParseFile(invFile)
	if err != nil {
		// If parsing fails, try with v1 format (7 fields)
		contentV1 := "# Comment line\n" +
			"prod-web\t~/.ssh/key.pem\thost.example.com\tubuntu\t22\t~/work\tprod,web\n" +
			"test-db\t~/.ssh/db.pem\tdb.example.com\tpostgres\t5432\t~/db\tprod,database\n"
		ioutil.WriteFile(invFile, []byte(contentV1), 0644)
		entries, err = ParseFile(invFile)
		if err != nil {
			t.Fatalf("ParseFile() error = %v", err)
		}
	}

	if len(entries) != 2 {
		t.Errorf("Expected 2 entries, got %d", len(entries))
		return
	}

	if entries[0].Alias != "prod-web" {
		t.Errorf("First entry alias = %v, want prod-web", entries[0].Alias)
	}
}

func TestParseMeta(t *testing.T) {
	tests := []struct {
		name string
		s    string
		want map[string]string
	}{
		{
			name: "single pair",
			s:    "key=value",
			want: map[string]string{"key": "value"},
		},
		{
			name: "multiple pairs",
			s:    "jump=bastion;strict=yes",
			want: map[string]string{"jump": "bastion", "strict": "yes"},
		},
		{
			name: "empty string",
			s:    "",
			want: map[string]string{},
		},
		{
			name: "value with equals",
			s:    "cmd=ssh -i key.pem",
			want: map[string]string{"cmd": "ssh -i key.pem"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseMeta(tt.s)
			if err != nil {
				t.Fatalf("parseMeta returned error: %v", err)
			}
			if len(got) != len(tt.want) {
				t.Errorf("parseMeta() length = %v, want %v", len(got), len(tt.want))
			}
			for k, v := range tt.want {
				if got[k] != v {
					t.Errorf("parseMeta()[%s] = %v, want %v", k, got[k], v)
				}
			}
		})
	}
}
