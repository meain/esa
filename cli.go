package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fatih/color"
)

// DefaultConfigDir is the default directory for agent configuration files
const DefaultConfigDir = "~/.config/esa"

// DefaultConfigPath is the default location for the configuration file
const DefaultConfigPath = DefaultConfigDir + "/default.toml"

type CLIOptions struct {
	DebugMode    bool
	ContinueChat bool
	ConfigPath   string
	AskLevel     string
	ShowCommands bool
	HideProgress bool
	CommandStr   string
	AgentName    string
}

func handleShowAgent(configPath string) {
	config, err := loadConfig(configPath)
	if err != nil {
		color.Red("Error loading config: %v\n", err)
		return
	}

	labelStyle := color.New(color.FgHiCyan, color.Bold).SprintFunc()

	// Print agent name and description if available
	if config.Name != "" {
		fmt.Printf("%s %s (%s)\n", labelStyle("Agent:"), config.Name, filepath.Base(configPath))
	} else {
		fmt.Printf("%s %s\n", labelStyle("Agent:"), filepath.Base(configPath))
	}

	if config.Description != "" {
		fmt.Printf("%s %s\n", labelStyle("Description:"), config.Description)
	}
	fmt.Println()

	// Print available functions
	if len(config.Functions) > 0 {
		fmt.Printf("%s\n\n", labelStyle("Available functions"))
		for _, fn := range config.Functions {
			printFunctionInfo(fn)
		}
	} else {
		noFuncStyle := color.New(color.FgYellow, color.Italic).SprintFunc()
		fmt.Printf("%s\n", noFuncStyle("No functions available."))
	}
}

// CommandType represents the type of command to execute
type CommandType int

const (
	NormalExecution CommandType = iota
	ShowAgent
	ListAgents
)

func parseFlags() (CLIOptions, CommandType) {
	opts := CLIOptions{}

	// Define command-line flags
	flag.BoolVar(&opts.DebugMode, "debug", false, "Enable debug mode")
	flag.BoolVar(&opts.ContinueChat, "c", false, "Continue last conversation")
	flag.BoolVar(&opts.ContinueChat, "continue", false, "Continue last conversation")
	configPath := flag.String("config", "", "Path to the config file")
	flag.StringVar(&opts.AskLevel, "ask", "none", "Ask level (none, unsafe, all)")
	flag.BoolVar(&opts.ShowCommands, "show-commands", false, "Show executed commands")
	flag.BoolVar(&opts.HideProgress, "hide-progress", false, "Disable LLM-generated progress summary for each function")
	help := flag.Bool("help", false, "Show help message")
	flag.Parse()

	// Handle help flag
	if *help {
		printHelp()
		os.Exit(0)
	}

	// Process command arguments
	args := flag.Args()
	opts.CommandStr = strings.Join(args, " ")
	opts.ConfigPath = *configPath

	// Determine command type and parse agent information
	commandType := parseCommandType(&opts)

	return opts, commandType
}

// parseCommandType determines what type of command is being executed and
// updates the options accordingly
func parseCommandType(opts *CLIOptions) CommandType {
	// Check for show-agent command
	if strings.HasPrefix(opts.CommandStr, "show-agent") {
		parts := strings.SplitN(opts.CommandStr, " ", 2)
		if len(parts) > 1 && strings.HasPrefix(parts[1], "+") {
			// Extract agent name (remove + prefix)
			agentName := parts[1][1:]
			opts.AgentName = agentName
			opts.ConfigPath = fmt.Sprintf("%s/%s.toml", DefaultConfigDir, agentName)
		}
		return ShowAgent
	}

	// Check for list-agents command
	if strings.HasPrefix(opts.CommandStr, "list-agents") {
		return ListAgents
	}

	// Check for agent selection with + prefix
	if strings.HasPrefix(opts.CommandStr, "+") {
		parseAgentCommand(opts)
	}

	return NormalExecution
}

// parseAgentCommand handles the +agent syntax, extracting agent name and remaining command
func parseAgentCommand(opts *CLIOptions) {
	parts := strings.SplitN(opts.CommandStr, " ", 2)

	// Extract agent name (remove + prefix)
	opts.AgentName = parts[0][1:]

	// Update command string if there's content after the agent name
	if len(parts) < 2 {
		// Clear CommandStr so it can use initial_message
		opts.CommandStr = ""
	} else {
		opts.CommandStr = parts[1]
	}

	// Set config path based on agent name
	opts.ConfigPath = fmt.Sprintf("%s/%s.toml", DefaultConfigDir, opts.AgentName)
}

func printHelp() {
	fmt.Println("Usage: esa <command> [--debug] [--config <path>] [--ask <level>] [--show-progress]")
	fmt.Println("\nOptions:")
	fmt.Println("  --debug         Enable debug mode")
	fmt.Println("  --config        Path to the config file")
	fmt.Println("  --ask           Ask level (none, unsafe, all)")
	fmt.Println("  --show-commands Show executed commands")
	fmt.Println("  --hide-progress Disable progress summary for each function (enabled by default)")
	fmt.Println("\nCommands:")
	fmt.Println("  list-agents          List all available agents")
	fmt.Println("  show-agent +<agent>  Show agent details and available functions")
	fmt.Println("  +<agent> <text>      Use specific agent with the given command")
	fmt.Println("  <text>               Send text command to the assistant")
}

func printFunctionInfo(fn FunctionConfig) {
	functionName := color.New(color.FgHiGreen, color.Bold).SprintFunc()
	paramName := color.New(color.FgYellow).SprintFunc()
	requiredTag := color.New(color.FgRed, color.Bold).SprintFunc()

	fmt.Printf("%s\n", functionName(fn.Name))
	fmt.Printf("%s\n", fn.Description)
	if len(fn.Parameters) > 0 {
		for _, p := range fn.Parameters {
			required := ""
			if p.Required {
				required = requiredTag(" (required)")
			}
			fmt.Printf("  • %s: %s%s\n", paramName(p.Name), p.Description, required)
		}
	}
	fmt.Println()
}

// listAgents lists all available agents in the default config directory
func listAgents() {
	// Expand the default config directory
	configDir := expandHomePath(DefaultConfigDir)

	// Check if the directory exists
	if _, err := os.Stat(configDir); os.IsNotExist(err) {
		color.Red("Config directory does not exist: %s\n", configDir)
		return
	}

	// Read all .toml files in the directory
	files, err := os.ReadDir(configDir)
	if err != nil {
		color.Red("Error reading config directory: %v\n", err)
		return
	}

	foundAgents := false

	agentNameStyle := color.New(color.FgHiCyan, color.Bold).SprintFunc()
	configNameStyle := color.New(color.FgHiGreen).SprintFunc()
	noDescStyle := color.New(color.FgHiBlack, color.Italic).SprintFunc()

	for _, file := range files {
		if !file.IsDir() && strings.HasSuffix(file.Name(), ".toml") {
			foundAgents = true
			agentName := strings.TrimSuffix(file.Name(), ".toml")

			// Load the config to get the description
			configPath := filepath.Join(configDir, file.Name())
			config, err := loadConfig(configPath)

			if err != nil {
				color.Red("%s: Error loading config\n", agentName)
				continue
			}

			// Print agent filename and name from config
			if config.Name != "" {
				fmt.Printf("%s (%s)\n", agentNameStyle(agentName), configNameStyle(config.Name))
			} else {
				fmt.Printf("%s\n", agentNameStyle(agentName))
			}

			// Print description
			if config.Description != "" {
				fmt.Println(config.Description)
			} else {
				fmt.Printf("%s\n", noDescStyle("(No description available)"))
			}
			fmt.Println()
		}
	}

	if !foundAgents {
		color.Yellow("No agents found in the config directory.")
	}
}
