package main

import (
	"fmt"
	"os"

	"github.com/BurntSushi/toml"
)

type Agent struct {
	Name           string           `toml:"name"`
	Description    string           `toml:"description"`
	Functions      []FunctionConfig `toml:"functions"`
	Ask            string           `toml:"ask"`
	SystemPrompt   string           `toml:"system_prompt"`
	InitialMessage string           `toml:"initial_message"`
	DefaultModel   string           `toml:"default_model"`
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
	Default     any      `toml:"default,omitempty"`
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

	// Validate ask level
	validAskLevels := map[string]bool{"": true, "none": true, "unsafe": true, "all": true}
	if !validAskLevels[agent.Ask] {
		return agent, fmt.Errorf("agent '%s' has invalid ask level: %q (must be one of: none, unsafe, all)", agent.Name, agent.Ask)
	}

	// Check function name uniqueness
	funcNames := make(map[string]bool)

	// Validate each function configuration
	for i, fc := range agent.Functions {
		if fc.Name == "" {
			return agent, fmt.Errorf("function %d in agent '%s' has no name", i+1, agent.Name)
		}
		if funcNames[fc.Name] {
			return agent, fmt.Errorf("duplicate function name '%s' in agent '%s'", fc.Name, agent.Name)
		}
		funcNames[fc.Name] = true

		if fc.Command == "" {
			return agent, fmt.Errorf("function %s in agent '%s' has no command defined", fc.Name, agent.Name)
		}
		if fc.Timeout < 0 || fc.Timeout > 3600 {
			return agent, fmt.Errorf("function '%s' in agent '%s' has invalid timeout %d (must be 0-3600)", fc.Name, agent.Name, fc.Timeout)
		}

		agent.Functions[i].Description, err = processShellBlocks(fc.Description)
		if err != nil {
			return agent, fmt.Errorf("error processing shell blocks in function %s: %v", fc.Name, err)
		}

		// Validate parameters
		paramNames := make(map[string]bool)
		for j, param := range fc.Parameters {
			if param.Name == "" {
				return agent, fmt.Errorf("parameter %d in function '%s' has no name", j+1, fc.Name)
			}
			if paramNames[param.Name] {
				return agent, fmt.Errorf("duplicate parameter name '%s' in function '%s'", param.Name, fc.Name)
			}
			paramNames[param.Name] = true
			if param.Type == "" {
				return agent, fmt.Errorf("parameter %s in function '%s' has no type defined", param.Name, fc.Name)
			}

			// Validate parameter type
			validTypes := map[string]bool{
				"string":  true,
				"number":  true,
				"boolean": true,
				"array":   true,
				"object":  true,
			}
			if !validTypes[param.Type] {
				return agent, fmt.Errorf("parameter %s in function '%s' has invalid type: %s", param.Name, fc.Name, param.Type)
			}

			agent.Functions[i].Parameters[j].Description, err = processShellBlocks(param.Description)
			if err != nil {
				return agent, fmt.Errorf("error processing shell blocks in parameter %s of function %s: %v",
					param.Name, fc.Name, err)
			}
		}
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
