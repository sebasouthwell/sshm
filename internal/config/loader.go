package config

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v2"
)

// Config represents the SSHM configuration
type Config struct {
	InventoryDir  string            `yaml:"inventory_dir,omitempty"`
	DefaultFilebase string          `yaml:"default_filebase,omitempty"`
	AWS           AWSConfig         `yaml:"aws,omitempty"`
	Docker        DockerConfig      `yaml:"docker,omitempty"`
	Kubernetes    KubernetesConfig  `yaml:"kubernetes,omitempty"`
	TUI           TUIConfig          `yaml:"tui,omitempty"`
	History       HistoryConfig     `yaml:"history,omitempty"`
}

// AWSConfig contains AWS-specific settings
type AWSConfig struct {
	DefaultProfile string `yaml:"default_profile,omitempty"`
	DefaultRegion  string `yaml:"default_region,omitempty"`
}

// DockerConfig contains Docker-specific settings
type DockerConfig struct {
	DefaultContext string `yaml:"default_context,omitempty"`
	DefaultShell   string `yaml:"default_shell,omitempty"`
}

// KubernetesConfig contains Kubernetes-specific settings
type KubernetesConfig struct {
	DefaultContext   string `yaml:"default_context,omitempty"`
	DefaultNamespace string `yaml:"default_namespace,omitempty"`
}

// TUIConfig contains TUI-specific settings
type TUIConfig struct {
	Height      int    `yaml:"height,omitempty"`
	BorderStyle string `yaml:"border_style,omitempty"`
}

// HistoryConfig contains history-specific settings
type HistoryConfig struct {
	MaxEvents      int  `yaml:"max_events,omitempty"`
	RecordCommand  bool `yaml:"record_command,omitempty"`
	RecordResolved bool `yaml:"record_resolved,omitempty"`
}

var (
	globalConfig *Config
	configLoaded bool
)

// LoadConfig loads configuration from file or environment
func LoadConfig() (*Config, error) {
	if configLoaded {
		return globalConfig, nil
	}

	config := &Config{
		// Defaults
		History: HistoryConfig{
			MaxEvents:      2000,
			RecordCommand:  false,
			RecordResolved: true,
		},
		TUI: TUIConfig{
			Height: 85,
		},
	}

	// Load from config file
	configFile := getConfigPath()
	if data, err := ioutil.ReadFile(configFile); err == nil {
		if err := yaml.Unmarshal(data, config); err != nil {
			return nil, fmt.Errorf("failed to parse config file: %w", err)
		}
	}

	// Override with environment variables (highest precedence)
	if invDir := os.Getenv("SSHM_INV_DIR"); invDir != "" {
		config.InventoryDir = invDir
	}
	if filebase := os.Getenv("SSHM_DEFAULT_FILEBASE"); filebase != "" {
		config.DefaultFilebase = filebase
	}
	if profile := os.Getenv("AWS_PROFILE"); profile != "" {
		config.AWS.DefaultProfile = profile
	}
	if region := os.Getenv("AWS_REGION"); region != "" {
		config.AWS.DefaultRegion = region
	}

	globalConfig = config
	configLoaded = true

	return config, nil
}

// GetConfig returns the global config (loads if not already loaded)
func GetConfig() *Config {
	if !configLoaded {
		config, _ := LoadConfig()
		if config != nil {
			return config
		}
		return &Config{} // Return empty config on error
	}
	return globalConfig
}

// getConfigPath returns the path to the config file
func getConfigPath() string {
	// Check environment override
	if path := os.Getenv("SSHM_CONFIG"); path != "" {
		return path
	}

	// Default: ~/.config/sshm/config.yaml
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	return filepath.Join(home, ".config", "sshm", "config.yaml")
}

// SaveConfig saves the configuration to file
func SaveConfig(config *Config) error {
	configFile := getConfigPath()
	if configFile == "" {
		return fmt.Errorf("cannot determine config file path")
	}

	// Ensure directory exists
	dir := filepath.Dir(configFile)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Marshal to YAML
	data, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Write file
	if err := ioutil.WriteFile(configFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	globalConfig = config
	configLoaded = true

	return nil
}
