package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Name           string           `toml:"name"`
	Description    string           `toml:"description"`
	Functions      []FunctionConfig `toml:"functions"`
	Ask            string           `toml:"ask"`
	SystemPrompt   string           `toml:"system_prompt"`
	InitialMessage string           `toml:"initial_message"`
}

type FunctionConfig struct {
	Name        string            `toml:"name"`
	Description string            `toml:"description"`
	Command     string            `toml:"command"`
	Parameters  []ParameterConfig `toml:"parameters"`
	Safe        bool              `toml:"safe"`
	Stdin       string            `toml:"stdin,omitempty"`
}

type ParameterConfig struct {
	Name        string `toml:"name"`
	Type        string `toml:"type"`
	Description string `toml:"description"`
	Required    bool   `toml:"required"`
	Format      string `toml:"format,omitempty"`
}

func loadConfig(configPath string) (Config, error) {
	var config Config
	_, err := toml.DecodeFile(expandHomePath(configPath), &config)
	return config, err
}

func loadConfiguration(opts CLIOptions) (Config, error) {
	if opts.AgentName == "new" {
		var config Config
		if _, err := toml.Decode(newAgentToml, &config); err != nil {
			return Config{}, fmt.Errorf("error loading embedded new agent config: %v", err)
		}
		return config, nil
	}
	return loadConfig(opts.ConfigPath)
}

func expandHomePath(path string) string {
	if strings.HasPrefix(path, "~") {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(homeDir, path[2:])
	}
	return path
}

func getEnvWithFallback(primary, fallback string) string {
	if value, exists := os.LookupEnv(primary); exists {
		return value
	}
	return os.Getenv(fallback)
}

const systemPrompt = `You are Esa, a professional assistant capable of performing various tasks. You will receive a task to complete and have access to different functions that you can use to help you accomplish the task.

When responding to tasks:
1. Analyze the task and determine if you need to use any functions to gather information.
2. If needed, make function calls to gather necessary information.
3. Process the information and formulate your response.
4. Provide only concise responses that directly address the task.

Other information:
- Date: {{$date '+%Y-%m-%d %A'}}
- OS: {{$uname}}
- Current directory: {{$pwd}}

Remember to keep your responses brief and to the point. Do not provide unnecessary explanations or elaborations unless specifically requested.`
