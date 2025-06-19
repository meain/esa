package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

type Agent struct {
	Name           string                     `toml:"name"`
	Description    string                     `toml:"description"`
	Functions      []FunctionConfig           `toml:"functions"`
	MCPServers     map[string]MCPServerConfig `toml:"mcp_servers"`
	Ask            string                     `toml:"ask"`
	SystemPrompt   string                     `toml:"system_prompt"`
	InitialMessage string                     `toml:"initial_message"`
	DefaultModel   string                     `toml:"default_model"`
}

// MCPServerConfig represents the configuration for an MCP server
type MCPServerConfig struct {
	Command string   `toml:"command"`
	Args    []string `toml:"args"`
}

type FunctionConfig struct {
	Name        string            `toml:"name"`
	Description string            `toml:"description"`
	Command     string            `toml:"command"`
	Parameters  []ParameterConfig `toml:"parameters"`
	Safe        bool              `toml:"safe"`
	Stdin       string            `toml:"stdin,omitempty"`
	Output      string            `toml:"output"`
	Pwd         string            `toml:"pwd,omitempty"`
	Timeout     int               `toml:"timeout"`
}

type ParameterConfig struct {
	Name        string   `toml:"name"`
	Type        string   `toml:"type"`
	Description string   `toml:"description"`
	Required    bool     `toml:"required"`
	Format      string   `toml:"format,omitempty"`
	Options     []string `toml:"options,omitempty"`
}

func loadAgent(agentPath string) (Agent, error) {
	var agent Agent
	_, err := toml.DecodeFile(agentPath, &agent)
	if err != nil {
		return agent, err
	}

	return agent, err
}

func loadConfiguration(opts *CLIOptions) (Agent, error) {
	if conf, exists := builtinAgents[opts.AgentName]; exists {
		var agent Agent
		if _, err := toml.Decode(conf, &agent); err != nil {
			return Agent{}, fmt.Errorf("error loading embedded '%s' agent config: %v", opts.AgentName, err)
		}
		return agent, nil
	}

	agentPath := expandHomePath(opts.AgentPath)
	_, err := os.Stat(agentPath)
	if err != nil {
		if os.IsNotExist(err) && opts.AgentName == "" && opts.AgentPath == DefaultAgentPath {
			var agent Agent
			if _, err := toml.Decode(defaultAgentToml, &agent); err != nil {
				return Agent{}, fmt.Errorf("error loading embedded new agent config: %v", err)
			}
			return agent, nil
		}
	}

	return loadAgent(agentPath)
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
