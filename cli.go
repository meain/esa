package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/fatih/color"
	"github.com/sashabaranov/go-openai"
	"github.com/spf13/cobra"
)

// DefaultAgentsDir is the default directory for agent configuration files
const DefaultAgentsDir = "~/.config/esa/agents"

// DefaultAgentPath is the default location for the agent configuration file
const DefaultAgentPath = DefaultAgentsDir + "/default.toml"

type CLIOptions struct {
	DebugMode         bool
	ContinueChat      bool
	ConversationIndex int // continue non-last one
	RetryChat         bool
	ReplMode          bool // Flag for REPL mode
	AgentPath         string
	AskLevel          string
	ShowCommands      bool
	ShowToolCalls     bool
	HideProgress      bool
	CommandStr        string
	AgentName         string
	Model             string
	ConfigPath        string
	OutputFormat      string // Output format for show-history (text, markdown, json)
	ShowAgent         bool   // Flag for showing agent details
	ListAgents        bool   // Flag for listing agents
	ListUserAgents    bool   // Flag for listing only user agents
	ListHistory       bool   // Flag for listing history
	ShowHistory       bool   // Flag for showing specific history
	ShowOutput        bool   // Flag for showing just output from history
	ShowStats         bool   // Flag for showing usage statistics
	ShowAll           bool   // Flag for showing both stats and history
	SystemPrompt      string // System prompt override from CLI
	Pretty            bool   // Pretty print markdown output using glow
}

func createRootCommand() *cobra.Command {
	opts := &CLIOptions{}

	rootCmd := &cobra.Command{
		Use:          "esa [text]",
		SilenceUsage: true,
		Short:        "Personalized micro agents",
		Long: "Esa is a command-line tool for interacting with personalized micro agents" +
			" that can execute tasks, answer questions, and assist with various functions.",
		Example: `  esa Will it rain tomorrow
  esa +coder How do I write a function in Go
  esa --repl
  esa --repl "initial query"
  esa --list-agents
  esa --show-agent +coder
  esa --show-agent ~/.config/esa/agents/custom.toml
  esa --list-history
  esa --show-history 1
  esa --show-history 1 --output json
  esa --show-output 1
  esa --show-stats`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Handle REPL mode first
			if opts.ReplMode {
				return runReplMode(opts, args)
			}

			if opts.AskLevel != "" &&
				!slices.Contains([]string{"none", "unsafe", "all"}, opts.AskLevel) {
				return fmt.Errorf(
					"invalid ask level: %s. Must be one of: none, unsafe, all",
					opts.AskLevel,
				)
			}

			if opts.OutputFormat == "" &&
				!slices.Contains([]string{"text", "markdown", "json"}, opts.OutputFormat) {
				return fmt.Errorf(
					"invalid output format: %s. Must be one of: text, markdown, json",
					opts.OutputFormat,
				)
			}

			// Handle list/show flags first
			if opts.ListAgents {
				listAgents()
				return nil
			}

			if opts.ListUserAgents {
				listUserAgents()
				return nil
			}

			if opts.ListHistory {
				listHistory(opts.ShowAll)
				return nil
			}

			if opts.ShowHistory {
				// Require positional argument for history index
				if len(args) == 0 {
					return fmt.Errorf("history index must be provided as argument: esa --show-history <index>")
				}

				idx, err := strconv.Atoi(args[0])
				if err != nil || idx <= 0 {
					return fmt.Errorf("invalid history index: %s (must be a positive number)", args[0])
				}

				handleShowHistory(idx, opts.OutputFormat)
				return nil
			}

			if opts.ShowOutput {
				// Require positional argument for history index
				if len(args) == 0 {
					return fmt.Errorf("history index must be provided as argument: esa --show-output <index>")
				}

				idx, err := strconv.Atoi(args[0])
				if err != nil || idx <= 0 {
					return fmt.Errorf("invalid history index: %s (must be a positive number)", args[0])
				}

				handleShowOutput(idx, opts.Pretty)
				return nil
			}

			if opts.ShowStats {
				handleShowStats(opts.ShowAll)
				return nil
			}

			if opts.ShowAgent {
				// Require positional argument for agent
				if len(args) == 0 {
					return fmt.Errorf("agent must be provided as argument: esa --show-agent <agent> or esa --show-agent +<agent>")
				}

				_, agentPath := ParseAgentString(args[0])
				handleShowAgent(agentPath)
				return nil
			}

			// Normal execution - join args as command string
			opts.CommandStr = strings.Join(args, " ")

			// Handle agent selection with + prefix
			if strings.HasPrefix(opts.CommandStr, "+") {
				parseAgentCommand(opts)
			}

			app, err := NewApplication(opts)
			if err != nil {
				return fmt.Errorf("failed to initialize application: %v", err)
			}

			app.Run(*opts)
			return nil
		},
	}

	// Add flags
	rootCmd.Flags().BoolVar(&opts.DebugMode, "debug", false, "Enable debug mode")
	rootCmd.Flags().BoolVarP(&opts.ContinueChat, "continue", "c", false, "Continue last conversation")
	rootCmd.Flags().IntVarP(&opts.ConversationIndex, "conversation", "C", 0, "Specify the conversation to continue or retry")
	rootCmd.Flags().BoolVarP(&opts.RetryChat, "retry", "r", false, "Retry last command")
	rootCmd.Flags().BoolVar(&opts.ReplMode, "repl", false, "Start in REPL mode for interactive conversation")
	rootCmd.Flags().StringVar(&opts.AgentPath, "agent", "", "Path to agent config file")
	rootCmd.Flags().StringVar(&opts.ConfigPath, "config", "", "Path to the global config file (default: ~/.config/esa/config.toml)")
	rootCmd.Flags().StringVarP(&opts.Model, "model", "m", "", "Model to use (e.g., openai/gpt-4)")
	rootCmd.Flags().StringVar(&opts.AskLevel, "ask", "", "Ask level (none, unsafe, all)")
	rootCmd.Flags().BoolVar(&opts.ShowCommands, "show-commands", false, "Show executed commands during run")
	rootCmd.Flags().BoolVar(&opts.ShowToolCalls, "show-tool-calls", false, "Show executed commands and their outputs during run")
	rootCmd.Flags().BoolVar(&opts.HideProgress, "hide-progress", false, "Disable progress info for each function")
	rootCmd.Flags().StringVar(&opts.OutputFormat, "output", "text", "Output format for --show-history (text, markdown, json)")
	rootCmd.Flags().BoolVarP(&opts.Pretty, "pretty", "p", false, "Pretty print markdown output (disables streaming)")
	rootCmd.Flags().StringVar(&opts.SystemPrompt, "system-prompt", "", "Override the system prompt for the agent")

	// List/show flags
	rootCmd.Flags().BoolVar(&opts.ListAgents, "list-agents", false, "List all available agents")
	rootCmd.Flags().BoolVar(&opts.ListUserAgents, "list-user-agents", false, "List only user agents")
	rootCmd.Flags().BoolVar(&opts.ListHistory, "list-history", false, "List all saved conversation histories")
	rootCmd.Flags().BoolVar(&opts.ShowAgent, "show-agent", false, "Show agent details (requires agent name/path as argument)")
	rootCmd.Flags().BoolVar(&opts.ShowHistory, "show-history", false, "Show conversation history (requires history index as argument)")
	rootCmd.Flags().BoolVar(&opts.ShowOutput, "show-output", false, "Show just the output from a history entry (requires history index as argument)")
	rootCmd.Flags().BoolVar(&opts.ShowStats, "show-stats", false, "Show usage statistics based on conversation history")
	rootCmd.Flags().BoolVar(&opts.ShowAll, "all", false, "Show all items when used with --list-history or --show-stats")

	// Make history-index required when show-history is used
	rootCmd.PreRunE = func(cmd *cobra.Command, args []string) error {
		// Validate output format
		validFormats := map[string]bool{"text": true, "markdown": true, "json": true}
		if !validFormats[opts.OutputFormat] {
			return fmt.Errorf("invalid output format %q. Must be one of: text, markdown, json", opts.OutputFormat)
		}

		return nil
	}

	return rootCmd
}

// parseAgentCommand handles the +agent syntax, extracting agent name and remaining command
func parseAgentCommand(opts *CLIOptions) {
	parts := strings.SplitN(opts.CommandStr, " ", 2)

	// Extract agent string (with + prefix)
	agentStr := parts[0]

	// Update command string if there's content after the agent name
	if len(parts) < 2 {
		// Clear CommandStr so it can use initial_message
		opts.CommandStr = ""
	} else {
		opts.CommandStr = parts[1]
	}

	// Parse agent string
	agentName, agentPath := ParseAgentString(agentStr)
	opts.AgentName = agentName
	opts.AgentPath = agentPath

	// Check if this is a user agent that overrides a builtin
	if strings.HasPrefix(agentPath, "builtin:") && opts.DebugMode {
		userAgentPath := expandHomePath(fmt.Sprintf("%s/%s.toml", DefaultAgentsDir, agentName))
		if _, err := os.Stat(userAgentPath); err == nil {
			fmt.Printf("Note: Using user agent '%s' which overrides the built-in agent with the same name\n", agentName)
			opts.AgentPath = userAgentPath
		}
	}
}

// getUserAgents gets a list of user agents from the default config directory
func getUserAgents(showErrors bool) ([]Agent, []string, bool) {
	var agents []Agent
	var names []string

	// Expand the default config directory
	agentDir := expandHomePath(DefaultAgentsDir)

	// Check if the directory exists
	if _, err := os.Stat(agentDir); os.IsNotExist(err) {
		if showErrors {
			color.Red("Agent directory does not exist: %s\n", agentDir)
		}
		return agents, names, false
	}

	// Read all .toml files in the directory
	files, err := os.ReadDir(agentDir)
	if err != nil {
		if showErrors {
			color.Red("Error reading agent directory: %v\n", err)
		}
		return agents, names, false
	}

	userAgentsFound := false

	for _, file := range files {
		if !file.IsDir() && strings.HasSuffix(file.Name(), ".toml") {
			userAgentsFound = true
			agentName := strings.TrimSuffix(file.Name(), ".toml")
			names = append(names, agentName)

			// Load the agent config to get the description
			agentPath := filepath.Join(agentDir, file.Name())
			agent, err := loadAgent(agentPath)

			if err != nil {
				if showErrors {
					color.Red("  %s: Error loading agent\n", agentName)
				}
				continue
			}

			agents = append(agents, agent)
		}
	}

	return agents, names, userAgentsFound
}

// listUserAgents lists only user agents in the default config directory
func listUserAgents() {
	builtinStyle := color.New(color.FgHiMagenta, color.Bold).SprintFunc()
	fmt.Println(builtinStyle("User Agents:"))

	agents, names, userAgentsFound := getUserAgents(true)

	for i := range agents {
		printAgentInfo(agents[i], names[i])
	}

	if !userAgentsFound {
		color.Yellow("  No user agents found in the agent directory.")
	}
}

// listAgents lists all available agents in the default config directory and built-in agents
func listAgents() {
	builtinStyle := color.New(color.FgHiMagenta, color.Bold).SprintFunc()
	foundAgents := false

	// First list built-in agents
	fmt.Println(builtinStyle("Built-in Agents:"))
	for name, tomlContent := range builtinAgents {
		foundAgents = true

		// Parse the agent from TOML content
		var agent Agent
		if _, err := toml.Decode(tomlContent, &agent); err != nil {
			color.Red("%s: Error loading built-in agent\n", name)
			continue
		}

		printAgentInfo(agent, name)
	}

	fmt.Println()
	fmt.Println(builtinStyle("User Agents:"))

	agents, names, userAgentsFound := getUserAgents(false)

	for i := range agents {
		foundAgents = true
		printAgentInfo(agents[i], names[i])
	}

	if !userAgentsFound {
		color.Yellow("  No user agents found in the agent directory.")
	}

	if !foundAgents {
		color.Yellow("No agents found.")
	}
}

// listHistory lists available history files in the cache directory
func listHistory(showAll bool) {
	sortedFiles, _, err := getSortedHistoryFiles() // Use blank identifier for unused historyItems
	if err != nil {
		// Handle specific errors or just print the message
		if strings.Contains(err.Error(), "no history files found") || strings.Contains(err.Error(), "cache directory does not exist") {
			printWarning(err.Error())
		} else {
			printError(err.Error())
		}
		return
	}

	highPriStyle := color.New(color.FgHiCyan, color.Bold).SprintFunc()
	// medPriStyle := color.New(color.FgHiBlack).SprintFunc()
	lowPriStyle := color.New(color.FgHiWhite, color.Italic).SprintFunc()

	fmt.Printf("Available conversation histories (total: %d):\n", len(sortedFiles))

	// Determine how many items to show
	itemsToShow := sortedFiles
	if !showAll {
		if len(sortedFiles) > 15 {
			itemsToShow = sortedFiles[:15]
		}
	}

	for i, fileName := range itemsToShow {
		parts := strings.SplitN(strings.TrimSuffix(fileName, ".json"), "-", 2)
		agentName := "unknown"
		timestampStr := "unknown"
		if len(parts) == 2 {
			agentName = parts[0]
			timestampStr = parts[1]
			if parsedTime, err := time.Parse("20060102-150405", timestampStr); err == nil {
				timestampStr = parsedTime.Format("2006-01-02 15:04:05")
			}
		}

		// Get first user query
		cacheDir, _ := setupCacheDir()
		historyFilePath := filepath.Join(cacheDir, fileName)
		var query string
		if historyData, err := os.ReadFile(historyFilePath); err == nil {
			var history ConversationHistory
			if err := json.Unmarshal(historyData, &history); err == nil {
				prevMessage := ""
				for _, msg := range history.Messages {
					if msg.Role == openai.ChatMessageRoleAssistant {
						query = strings.ReplaceAll(prevMessage, "\n", " ")
						if len(query) > 60 {
							query = query[:57] + "..."
						}
						break
					}

					prevMessage = msg.Content
				}
			}
		}

		fmt.Printf(" %2d: %s %s %s\n",
			i+1,
			highPriStyle("+"+agentName),
			query,
			lowPriStyle(timestampStr),
		)

	}
}

// handleShowHistory displays the content of a specific history file in the specified format.
func handleShowHistory(index int, outputFormat string) {
	sortedFiles, _, err := getSortedHistoryFiles()
	if err != nil {
		// For JSON output, print error as JSON
		if outputFormat == "json" {
			printJSONError(fmt.Sprintf("Error getting history files: %v", err))
			return
		}
		if strings.Contains(err.Error(), "no history files found") || strings.Contains(err.Error(), "cache directory does not exist") {
			color.Yellow(err.Error())
		} else {
			color.Red(err.Error())
		}
		return
	}

	if index <= 0 || index > len(sortedFiles) {
		errMsg := fmt.Sprintf("Error: Invalid history number %d. Please choose a number between 1 and %d.", index, len(sortedFiles))
		if outputFormat == "json" {
			printJSONError(errMsg)
		} else {
			color.Red(errMsg)
		}
		return
	}

	fileName := sortedFiles[index-1] // Adjust index to be 0-based

	cacheDir, _ := setupCacheDir() // Error already checked in getSortedHistoryFiles
	historyFilePath := filepath.Join(cacheDir, fileName)

	// Load the conversation history data
	historyData, err := os.ReadFile(historyFilePath)
	if err != nil {
		errMsg := fmt.Sprintf("Error reading history file %s: %v", fileName, err)
		if outputFormat == "json" {
			printJSONError(errMsg)
		} else {
			color.Red(errMsg)
		}
		return
	}

	var history ConversationHistory
	err = json.Unmarshal(historyData, &history)
	if err != nil {
		errMsg := fmt.Sprintf("Error unmarshalling conversation history from %s: %v", fileName, err)
		if outputFormat == "json" {
			printJSONError(errMsg)
		} else {
			color.Red(errMsg)
		}
		return
	}

	// --- Output based on format ---
	switch outputFormat {
	case "json":
		printHistoryJSON(historyData)
	case "markdown":
		printHistoryMarkdown(historyFilePath, history)
	default: // "text"
		printHistoryText(historyFilePath, history)
	}
}

// handleShowOutput displays output from a specific history file.
func handleShowOutput(index int, pretty bool) {
	sortedFiles, _, err := getSortedHistoryFiles()
	if err != nil {
		if strings.Contains(err.Error(), "no history files found") || strings.Contains(err.Error(), "cache directory does not exist") {
			color.Yellow(err.Error())
		} else {
			color.Red(err.Error())
		}
		return
	}

	if index <= 0 || index > len(sortedFiles) {
		color.Red("Error: Invalid history number %d. Please choose a number between 1 and %d.", index, len(sortedFiles))
		return
	}

	fileName := sortedFiles[index-1] // Adjust index to be 0-based

	cacheDir, _ := setupCacheDir() // Error already checked in getSortedHistoryFiles
	historyFilePath := filepath.Join(cacheDir, fileName)

	// Load the conversation history data
	historyData, err := os.ReadFile(historyFilePath)
	if err != nil {
		color.Red("Error reading history file %s: %v", fileName, err)
		return
	}

	var history ConversationHistory
	err = json.Unmarshal(historyData, &history)
	if err != nil {
		color.Red("Error unmarshalling conversation history from %s: %v", fileName, err)
		return
	}

	printOutput(history, pretty)
}

// handleShowStats analyzes history files and displays usage statistics
func handleShowStats(showAll bool) {
	// Get all history files
	sortedFiles, fileInfo, err := getSortedHistoryFiles()
	if err != nil {
		if strings.Contains(err.Error(), "no history files found") || strings.Contains(err.Error(), "cache directory does not exist") {
			color.Yellow(err.Error())
		} else {
			color.Red(err.Error())
		}
		return
	}

	cacheDir, _ := setupCacheDir()
	collector := NewStatsCollector()

	// Process each history file
	for _, fileName := range sortedFiles {
		historyFilePath := filepath.Join(cacheDir, fileName)
		fileModTime := fileInfo[fileName].ModTime()

		if err := collector.ProcessHistoryFile(historyFilePath, fileName, fileModTime); err != nil {
			color.Red("Error processing history file %s: %v", fileName, err)
		}
	}

	collector.PrintStatistics(showAll)
}

// handleShowAgent displays the details of the agent specified by the agentPath.
func handleShowAgent(agentPath string) {
	agent, err := loadAgent(agentPath)
	if err != nil {
		printError(fmt.Sprintf("Error loading agent: %v", err))
		return
	}

	labelStyle := color.New(color.FgHiCyan, color.Bold).SprintFunc()

	// Print agent header
	if agent.Name != "" {
		fmt.Printf("%s %s (%s)\n", labelStyle("Agent:"), agent.Name, filepath.Base(agentPath))
	} else {
		fmt.Printf("%s %s\n", labelStyle("Agent:"), filepath.Base(agentPath))
	}

	if agent.Description != "" {
		fmt.Printf("%s %s\n", labelStyle("Description:"), agent.Description)
	}
	fmt.Println()

	// Print available functions
	if len(agent.Functions) > 0 {
		fmt.Printf("%s\n", labelStyle("Functions:"))
		for _, fn := range agent.Functions {
			printFunctionInfo(fn)
		}
	}

	// Print MCP servers
	if len(agent.MCPServers) > 0 {
		fmt.Printf("%s\n", labelStyle("MCP Servers:"))
		for name, server := range agent.MCPServers {
			printMCPServerInfo(name, server)
		}
	}

	if len(agent.Functions) == 0 && len(agent.MCPServers) == 0 {
		noFuncStyle := color.New(color.FgYellow, color.Italic).SprintFunc()
		fmt.Printf("%s\n", noFuncStyle("No functions or MCP servers available."))
	}
}
