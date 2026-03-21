package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	// Create a temporary directory for test config
	tmpDir, err := os.MkdirTemp("", "esa-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	configPath := filepath.Join(tmpDir, "config.toml")

	// Test loading default config when file doesn't exist
	config, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	// Test loading custom config
	customConfig := `
model_aliases = { "custom" = "custom/model" }
[providers]
[providers.custom]
base_url = "https://custom.api/v1"
api_key_envar = "CUSTOM_API_KEY"
`
	if err := os.WriteFile(configPath, []byte(customConfig), 0644); err != nil {
		t.Fatalf("Failed to write custom config: %v", err)
	}

	config, err = LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed for custom config: %v", err)
	}

	// Verify custom model alias
	if config.ModelAliases["custom"] != "custom/model" {
		t.Errorf("Expected custom alias to be custom/model, got %s", config.ModelAliases["custom"])
	}

	// Verify custom provider
	custom, exists := config.Providers["custom"]
	if !exists {
		t.Error("Expected custom provider to exist")
	}
	if custom.BaseURL != "https://custom.api/v1" {
		t.Errorf("Expected custom BaseURL to be https://custom.api/v1, got %s", custom.BaseURL)
	}
	if custom.APIKeyEnvar != "CUSTOM_API_KEY" {
		t.Errorf("Expected custom APIKeyEnvar to be CUSTOM_API_KEY, got %s", custom.APIKeyEnvar)
	}
}

func TestValidateConfig_CircularAliases(t *testing.T) {
	config := &Config{
		ModelAliases: map[string]string{
			"a": "b",
			"b": "c",
			"c": "a", // circular
		},
		Providers: make(map[string]ProviderConfig),
	}

	err := validateConfig(config)
	if err == nil {
		t.Error("Expected error for circular aliases, got nil")
	}
	if err != nil && !strings.Contains(err.Error(), "circular") {
		t.Errorf("Expected circular alias error, got: %v", err)
	}
}

func TestValidateConfig_NonCircularAliases(t *testing.T) {
	config := &Config{
		ModelAliases: map[string]string{
			"fast": "openai/gpt-4o-mini",
			"smart": "anthropic/claude-sonnet-4-20250514",
		},
		Providers: make(map[string]ProviderConfig),
	}

	err := validateConfig(config)
	if err != nil {
		t.Errorf("Expected no error for non-circular aliases, got: %v", err)
	}
}

func TestValidateConfig_InvalidProviderURL(t *testing.T) {
	config := &Config{
		ModelAliases: make(map[string]string),
		Providers: map[string]ProviderConfig{
			"bad": {
				BaseURL: "ftp://invalid.com",
			},
		},
	}

	err := validateConfig(config)
	if err == nil {
		t.Error("Expected error for invalid provider URL, got nil")
	}
	if err != nil && !strings.Contains(err.Error(), "invalid base_url") {
		t.Errorf("Expected invalid base_url error, got: %v", err)
	}
}

func TestValidateConfig_ValidProviderURL(t *testing.T) {
	config := &Config{
		ModelAliases: make(map[string]string),
		Providers: map[string]ProviderConfig{
			"good": {
				BaseURL: "https://api.example.com/v1",
			},
		},
	}

	err := validateConfig(config)
	if err != nil {
		t.Errorf("Expected no error for valid provider URL, got: %v", err)
	}
}
