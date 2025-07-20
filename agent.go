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
	Command          string   `toml:"command"`
	Args             []string `toml:"args"`
	Safe             bool     `toml:"safe"`              // Whether tools from this server are considered safe by default
	SafeFunctions    []string `toml:"safe_functions"`    // List of specific functions that are safe (overrides server-level safe setting)
	AllowedFunctions []string `toml:"allowed_functions"` // List of functions to expose to the LLM (if empty, all functions are allowed)
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

	return validateAgent(agent)
}

// validateAgent performs validation on an agent configuration
// to ensure all required fields are present and properly formatted.
func validateAgent(agent Agent) (Agent, error) {
	var err error

	// Validate agent level configuration
	if agent.Name == "" {
		return agent, fmt.Errorf("agent has no name defined")
	}

	// Validate model if specified
	if agent.DefaultModel != "" {
		// This could be expanded to validate against a list of known models
	}

	// Validate each function configuration
	for i, fc := range agent.Functions {
		if fc.Name == "" {
			return agent, fmt.Errorf("function %d in agent '%s' has no name", i+1, agent.Name)
		}
		if fc.Command == "" {
			return agent, fmt.Errorf("function %s in agent '%s' has no command defined", fc.Name, agent.Name)
		}
		if fc.Description == "" {
			fc.Description = "No description provided"
		}

		agent.Functions[i].Description, err = processShellBlocks(fc.Description)
		if err != nil {
			return agent, fmt.Errorf("error processing shell blocks in function %s: %v", fc.Name, err)
		}

		// Validate parameters
		for j, param := range fc.Parameters {
			if param.Name == "" {
				return agent, fmt.Errorf("parameter %d in function '%s' has no name", j+1, fc.Name)
			}
			if param.Type == "" {
				return agent, fmt.Errorf("parameter %s in function '%s' has no type defined", param.Name, fc.Name)
			}

			// Validate parameter type
			validTypes := map[string]bool{"string": true, "number": true, "boolean": true, "array": true, "object": true}
			if !validTypes[param.Type] {
				return agent, fmt.Errorf("parameter %s in function '%s' has invalid type: %s", param.Name, fc.Name, param.Type)
			}

			if param.Description == "" {
				param.Description = "No description provided"
			}

			agent.Functions[i].Parameters[j].Description, err = processShellBlocks(param.Description)
			if err != nil {
				return agent, fmt.Errorf("error processing shell blocks in parameter %s of function %s: %v",
					param.Name, fc.Name, err)
			}
		}
	}

	// Validate MCP server configurations
	for serverName, serverConfig := range agent.MCPServers {
		if serverConfig.Command == "" {
			return agent, fmt.Errorf("MCP server '%s' has no command defined", serverName)
		}

		// Check that any safe or allowed functions referenced actually exist in the server
		// This would require knowledge of what functions each server exposes
	}

	return agent, nil
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
