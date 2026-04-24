package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// DefaultConfigPath is the default location for the global config file
const DefaultConfigPath = "~/.config/esa/config.toml"

// Settings represents global settings that can be overridden by CLI flags
type Settings struct {
	ShowCommands  bool   `toml:"show_commands"`
	ShowToolCalls bool   `toml:"show_tool_calls"`
	DefaultModel  string `toml:"default_model"`
	OnComplete    string `toml:"on_complete"`
	MaxTurns      int    `toml:"max_turns"`
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

	if err := validateConfig(config); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return config, nil
}

// validateConfig validates the loaded configuration for common errors.
func validateConfig(config *Config) error {
	// Detect circular model aliases
	for alias := range config.ModelAliases {
		visited := make(map[string]bool)
		current := alias
		for {
			visited[current] = true
			next, ok := config.ModelAliases[current]
			if !ok {
				break // resolved to a non-alias, good
			}
			if visited[next] {
				return fmt.Errorf("circular model alias detected: %s", alias)
			}
			current = next
		}
	}

	// Validate provider BaseURLs
	for name, provider := range config.Providers {
		if provider.BaseURL != "" &&
			!strings.HasPrefix(provider.BaseURL, "http://") &&
			!strings.HasPrefix(provider.BaseURL, "https://") {
			return fmt.Errorf("provider %q has invalid base_url %q: must start with http:// or https://", name, provider.BaseURL)
		}
	}

	return nil
}
