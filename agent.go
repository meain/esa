package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

type Agent struct {
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
	Output      string            `toml:"output"`
}

type ParameterConfig struct {
	Name        string `toml:"name"`
	Type        string `toml:"type"`
	Description string `toml:"description"`
	Required    bool   `toml:"required"`
	Format      string `toml:"format,omitempty"`
}

func loadAgent(agentPath string) (Agent, error) {
	var agent Agent
	_, err := toml.DecodeFile(expandHomePath(agentPath), &agent)
	return agent, err
}

func loadConfiguration(opts *CLIOptions) (Agent, error) {
	if opts.AgentName == "new" {
		var agent Agent
		if _, err := toml.Decode(newAgentToml, &agent); err != nil {
			return Agent{}, fmt.Errorf("error loading embedded new agent config: %v", err)
		}
		return agent, nil
	}
	return loadAgent(opts.AgentPath)
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
