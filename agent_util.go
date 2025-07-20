package main

import (
	"fmt"
	"strings"
)

// ParseAgentString handles all agent string formats:
// - +name (built-in or user agent by name)
// - name (without + prefix, treated as agent name)
// - /path/to/agent.toml (direct file path)
// - builtin:name (builtin agent specification)
//
// Returns agentName and agentPath. If the input is a direct path, 
// agentName will be empty.
func ParseAgentString(input string) (agentName, agentPath string) {
	// Handle +agent syntax
	if strings.HasPrefix(input, "+") {
		agentName = input[1:] // Remove + prefix
		
		// Check for builtin agents first
		if _, exists := builtinAgents[agentName]; exists {
			agentPath = "builtin:" + agentName
			return
		}
		
		// Otherwise treat as user agent name
		agentPath = expandHomePath(fmt.Sprintf("%s/%s.toml", DefaultAgentsDir, agentName))
		return
	}
	
	// Handle direct path (contains / or ends with .toml)
	if strings.Contains(input, "/") || strings.HasSuffix(input, ".toml") {
		agentPath = input
		if !strings.HasPrefix(agentPath, "/") {
			agentPath = expandHomePath(agentPath)
		}
		return
	}
	
	// Handle plain name without + prefix
	agentName = input
	
	// Check for builtin agents
	if _, exists := builtinAgents[agentName]; exists {
		agentPath = "builtin:" + agentName
		return
	}
	
	// Treat as user agent name
	agentPath = expandHomePath(fmt.Sprintf("%s/%s.toml", DefaultAgentsDir, agentName))
	return
}
