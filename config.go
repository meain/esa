package main

import (
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// DefaultConfigPath is the default location for the global config file
const DefaultConfigPath = "~/.config/esa/config.toml"

// Settings represents global settings that can be overridden by CLI flags
type Settings struct {
	ShowCommands   bool   `toml:"show_commands"`
	ShowToolCalls  bool   `toml:"show_tool_calls"`
	DefaultModel   string `toml:"default_model"`
}

// Config represents the global configuration structure
type Config struct {
	ModelAliases map[string]string         `toml:"model_aliases"`
	Providers    map[string]ProviderConfig `toml:"providers"`
	Settings     Settings                  `toml:"settings"`
}

// ProviderConfig represents the configuration for a model provider
type ProviderConfig struct {
	BaseURL           string            `toml:"base_url"`
	APIKeyEnvar       string            `toml:"api_key_envar"`
	AdditionalHeaders map[string]string `toml:"additional_headers"`
}

// LoadConfig loads the configuration from the specified path
func LoadConfig(configPath string) (*Config, error) {
	config := &Config{
		ModelAliases: make(map[string]string),
		Providers:    make(map[string]ProviderConfig),
	}

	// Expand home directory if needed
	if configPath == "" {
		configPath = DefaultConfigPath
	}
	configPath = expandHomePath(configPath)

	// Create default config directory if it doesn't exist
	configDir := filepath.Dir(configPath)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return nil, err
	}

	// Check if config file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		// Create default config file
		defaultConfig := Config{
			ModelAliases: map[string]string{},
			Providers:    map[string]ProviderConfig{},
			Settings:     Settings{ShowCommands: false, ShowToolCalls: false, DefaultModel: ""},
		}
		file, err := os.Create(configPath)
		if err != nil {
			return nil, err
		}
		defer file.Close()

		if err := toml.NewEncoder(file).Encode(defaultConfig); err != nil {
			return nil, err
		}
		return &defaultConfig, nil
	}

	// Load existing config file
	if _, err := toml.DecodeFile(configPath, config); err != nil {
		return nil, err
	}

	return config, nil
}
