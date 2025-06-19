package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/sashabaranov/go-openai"
	"github.com/spf13/cobra"
)

// DefaultAgentsDir is the default directory for agent configuration files
const DefaultAgentsDir = "~/.config/esa/agents"

// DefaultAgentPath is the default location for the agent configuration file
const DefaultAgentPath = DefaultAgentsDir + "/default.toml"

type CLIOptions struct {
	DebugMode    bool
	ContinueChat bool
	RetryChat    bool
	AgentPath    string
	AskLevel     string
	ShowCommands bool
	HideProgress bool
	CommandStr   string
	AgentName    string
	Model        string
	ConfigPath   string
	OutputFormat string // Output format for show-history (text, markdown, json)
	ShowAgent    bool   // Flag for showing agent details
	ListAgents   bool   // Flag for listing agents
	ListHistory  bool   // Flag for listing history
	ShowHistory  bool   // Flag for showing specific history
	ShowOutput   bool   // Flag for showing just output from history
}

func createRootCommand() *cobra.Command {
	opts := &CLIOptions{}

	rootCmd := &cobra.Command{
		Use:   "esa [text]",
		Short: "AI assistant with tool calling capabilities",
		Long:  `An AI assistant that can execute functions and tools to help with various tasks.`,
		Example: `  esa Will it rain tomorrow
  esa +coder How do I write a function in Go
  esa --list-agents
  esa --show-agent +coder
  esa --show-agent ~/.config/esa/agents/custom.toml
  esa --list-history
  esa --show-history 1
  esa --show-history 1 --output json
  esa --show-output 1`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Handle list/show flags first
			if opts.ListAgents {
				listAgents()
				return nil
			}

			if opts.ListHistory {
				listHistory()
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

				handleShowOutput(idx)
				return nil
			}

			if opts.ShowAgent {
				// Require positional argument for agent
				if len(args) == 0 {
					return fmt.Errorf("agent must be provided as argument: esa --show-agent <agent> or esa --show-agent +<agent>")
				}

				var agentPath string
				if strings.HasPrefix(args[0], "+") {
					// Handle +agent syntax
					agentName := args[0][1:] // Remove + prefix
					agentPath = expandHomePath(fmt.Sprintf("%s/%s.toml", DefaultAgentsDir, agentName))
				} else {
					// Treat as direct path
					agentPath = args[0]
				}

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
	rootCmd.Flags().BoolVarP(&opts.RetryChat, "retry", "r", false, "Retry last command")
	rootCmd.Flags().StringVar(&opts.AgentPath, "agent", "", "Path to agent config file")
	rootCmd.Flags().StringVar(&opts.ConfigPath, "config", "", "Path to the global config file (default: ~/.config/esa/config.toml)")
	rootCmd.Flags().StringVarP(&opts.Model, "model", "m", "", "Model to use (e.g., openai/gpt-4)")
	rootCmd.Flags().StringVar(&opts.AskLevel, "ask", "none", "Ask level (none, unsafe, all)")
	rootCmd.Flags().BoolVar(&opts.ShowCommands, "show-commands", false, "Show executed commands during run")
	rootCmd.Flags().BoolVar(&opts.HideProgress, "hide-progress", false, "Disable progress info for each function")
	rootCmd.Flags().StringVar(&opts.OutputFormat, "output", "text", "Output format for --show-history (text, markdown, json)")

	// List/show flags
	rootCmd.Flags().BoolVar(&opts.ListAgents, "list-agents", false, "List all available agents")
	rootCmd.Flags().BoolVar(&opts.ListHistory, "list-history", false, "List all saved conversation histories")
	rootCmd.Flags().BoolVar(&opts.ShowAgent, "show-agent", false, "Show agent details (requires agent name/path as argument)")
	rootCmd.Flags().BoolVar(&opts.ShowHistory, "show-history", false, "Show conversation history (requires history index as argument)")
	rootCmd.Flags().BoolVar(&opts.ShowOutput, "show-output", false, "Show just the output from a history entry (requires history index as argument)")

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

	// Extract agent name (remove + prefix)
	opts.AgentName = parts[0][1:]

	// Update command string if there's content after the agent name
	if len(parts) < 2 {
		// Clear CommandStr so it can use initial_message
		opts.CommandStr = ""
	} else {
		opts.CommandStr = parts[1]
	}

	// Separately handle builtin agents
	if _, exists := builtinAgents[opts.AgentName]; exists {
		opts.AgentPath = "builtin:" + opts.AgentName
		return
	}

	// Set agent path based on agent name
	opts.AgentPath = expandHomePath(fmt.Sprintf("%s/%s.toml", DefaultAgentsDir, opts.AgentName))
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
	agentDir := expandHomePath(DefaultAgentsDir)

	// Check if the directory exists
	if _, err := os.Stat(agentDir); os.IsNotExist(err) {
		color.Red("Agent directory does not exist: %s\n", agentDir)
		return
	}

	// Read all .toml files in the directory
	files, err := os.ReadDir(agentDir)
	if err != nil {
		color.Red("Error reading agent directory: %v\n", err)
		return
	}

	foundAgents := false

	agentNameStyle := color.New(color.FgHiCyan, color.Bold).SprintFunc()
	nameStyle := color.New(color.FgHiGreen).SprintFunc()
	noDescStyle := color.New(color.FgHiBlack, color.Italic).SprintFunc()

	for _, file := range files {
		if !file.IsDir() && strings.HasSuffix(file.Name(), ".toml") {
			foundAgents = true
			agentName := strings.TrimSuffix(file.Name(), ".toml")

			// Load the agent config to get the description
			agentPath := filepath.Join(agentDir, file.Name())
			agent, err := loadAgent(agentPath)

			if err != nil {
				color.Red("%s: Error loading agent\n", agentName)
				continue
			}

			// Print agent filename and name from config
			if agent.Name != "" {
				fmt.Printf("%s (%s): ", nameStyle(agent.Name), agentNameStyle(agentName))
			} else {
				fmt.Printf("%s: ", agentNameStyle(agentName))
			}

			// Print description
			if agent.Description != "" {
				fmt.Println(agent.Description)
			} else {
				fmt.Printf("%s\n", noDescStyle("(No description available)"))
			}
		}
	}

	if !foundAgents {
		color.Yellow("No agents found in the agent directory.")
	}
}

// getSortedHistoryFiles retrieves and sorts history files by modification time.
func getSortedHistoryFiles() ([]string, map[string]os.FileInfo, error) {
	cacheDir, err := setupCacheDir()
	if err != nil {
		return nil, nil, fmt.Errorf("error accessing cache directory: %v", err)
	}

	// Check if the directory exists
	if _, err := os.Stat(cacheDir); os.IsNotExist(err) {
		return nil, nil, fmt.Errorf("cache directory does not exist: %s", cacheDir)
	}

	// Read all .json files in the directory
	files, err := os.ReadDir(cacheDir)
	if err != nil {
		return nil, nil, fmt.Errorf("error reading cache directory: %v", err)
	}

	historyItems := make(map[string]os.FileInfo) // Store file info to sort by mod time later

	for _, file := range files {
		if !file.IsDir() && strings.HasSuffix(file.Name(), ".json") {
			info, err := file.Info()
			if err != nil {
				continue // Skip files we can't get info for
			}
			historyItems[file.Name()] = info
		}
	}

	if len(historyItems) == 0 {
		return nil, nil, fmt.Errorf("no history files found in the cache directory: %s", cacheDir)
	}

	// Sort files by modification time (most recent first)
	sortedFiles := make([]string, 0, len(historyItems))
	for name := range historyItems {
		sortedFiles = append(sortedFiles, name)
	}
	// Custom sort function
	sort.Slice(sortedFiles, func(i, j int) bool {
		return historyItems[sortedFiles[i]].ModTime().After(historyItems[sortedFiles[j]].ModTime())
	})

	return sortedFiles, historyItems, nil
}

// listHistory lists all available history files in the cache directory
func listHistory() {
	sortedFiles, _, err := getSortedHistoryFiles() // Use blank identifier for unused historyItems
	if err != nil {
		// Handle specific errors or just print the message
		if strings.Contains(err.Error(), "no history files found") || strings.Contains(err.Error(), "cache directory does not exist") {
			color.Yellow(err.Error())
		} else {
			color.Red(err.Error())
		}
		return
	}

	highPriStyle := color.New(color.FgHiCyan, color.Bold).SprintFunc()
	// medPriStyle := color.New(color.FgHiBlack).SprintFunc()
	lowPriStyle := color.New(color.FgHiWhite, color.Italic).SprintFunc()

	fmt.Printf("Available conversation histories (total: %d):\n", len(sortedFiles))

	// TODO(meain): Add additional flag to list all items
	for i, fileName := range sortedFiles[:15] {
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
func handleShowOutput(index int) {
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

	printOutput(history)
}

// printJSONError prints an error message in JSON format.
func printJSONError(message string) {
	errJSON, _ := json.Marshal(map[string]string{"error": message})
	fmt.Println(string(errJSON))
}

// printHistoryJSON prints the raw history data as JSON.
func printHistoryJSON(historyData []byte) {
	// Pretty print the JSON
	var prettyJSON bytes.Buffer
	if err := json.Indent(&prettyJSON, historyData, "", "  "); err == nil {
		fmt.Println(prettyJSON.String())
	} else {
		// Fallback to raw data if indent fails
		fmt.Println(string(historyData))
	}
}

// printHistoryMarkdown prints the history in Markdown format.
func printHistoryMarkdown(fileName string, history ConversationHistory) {
	fmt.Printf("# History: %s\n\n", fileName)
	if history.AgentPath != "" {
		agentName := strings.TrimSuffix(filepath.Base(history.AgentPath), ".toml")
		agentName = strings.TrimPrefix(agentName, "builtin:")
		fmt.Printf("**Agent:** +%s (`%s`)\n\n", agentName, history.AgentPath)
	}
	fmt.Println("---")

	for _, msg := range history.Messages {
		role := strings.ToUpper(msg.Role)
		fmt.Printf("## %s\n\n", role)

		if msg.Content != "" {
			// Check if content looks like code block for formatting
			if strings.Contains(msg.Content, "\n") && (strings.Contains(msg.Content, "```") || strings.Contains(msg.Content, "  ")) {
				fmt.Printf("```\n%s\n```\n\n", msg.Content)
			} else {
				fmt.Printf("%s\n\n", msg.Content)
			}
		}

		if len(msg.ToolCalls) > 0 {
			fmt.Println("**Tool Calls:**")
			for _, tc := range msg.ToolCalls {
				fmt.Printf("- **%s** (`%s`):\n", tc.Function.Name, tc.ID)
				// Attempt to format arguments as JSON code block
				var argsMap map[string]any
				argsStr := tc.Function.Arguments
				if err := json.Unmarshal([]byte(argsStr), &argsMap); err == nil {
					prettyJSON, _ := json.MarshalIndent(argsMap, "", "  ")
					argsStr = string(prettyJSON)
				}
				fmt.Printf("  ```json\n  %s\n  ```\n", argsStr)
			}
			fmt.Println()
		}

		if msg.Role == openai.ChatMessageRoleTool {
			fmt.Printf("**Tool Result** (for call `%s`, tool `%s`):\n\n", msg.ToolCallID, msg.Name)
			// Check if content looks like JSON or code for formatting
			isError := strings.HasPrefix(msg.Content, "Error:")
			prefix := ""
			if isError {
				prefix = "**ERROR:** "
			}

			var contentMap map[string]any
			var contentSlice []any
			contentStr := msg.Content
			if err := json.Unmarshal([]byte(contentStr), &contentMap); err == nil {
				prettyJSON, _ := json.MarshalIndent(contentMap, "", "  ")
				contentStr = string(prettyJSON)
				fmt.Printf("```json\n%s%s\n```\n\n", prefix, contentStr)
			} else if err := json.Unmarshal([]byte(contentStr), &contentSlice); err == nil {
				prettyJSON, _ := json.MarshalIndent(contentSlice, "", "  ")
				contentStr = string(prettyJSON)
				fmt.Printf("```json\n%s%s\n```\n\n", prefix, contentStr)
			} else {
				// Treat as plain text or potentially other code
				lang := ""                              // Auto-detect or leave blank
				if strings.Contains(contentStr, "\n") { // Basic check for multi-line content
					fmt.Printf("```%s\n%s%s\n```\n\n", lang, prefix, contentStr)
				} else {
					fmt.Printf("%s%s\n\n", prefix, contentStr)
				}
			}
		}
	}
}

// printHistoryText prints the history in the default colored text format.
func printHistoryText(fileName string, history ConversationHistory) {
	messages := history.Messages
	agentPath := history.AgentPath
	model := history.Model

	// --- Define Styles (copied from original implementation) ---
	systemStyle := color.New(color.FgMagenta, color.Italic).SprintFunc()
	userStyle := color.New(color.FgGreen).SprintFunc()
	assistantStyle := color.New(color.FgBlue).SprintFunc()
	toolStyle := color.New(color.FgYellow).SprintFunc()
	toolDataStyle := color.New(color.FgHiBlack).SprintFunc()
	errorStyle := color.New(color.FgRed).SprintFunc()
	labelStyle := color.New(color.Bold).SprintFunc()

	// --- Print Header ---
	fmt.Printf("%s %s\n", labelStyle("History File:"), fileName)
	if agentPath != "" {
		// Try to extract agent name from path for display
		agentName := strings.TrimSuffix(filepath.Base(agentPath), ".toml")
		agentName = strings.TrimPrefix(agentName, "builtin:")
		fmt.Printf("%s +%s (%s)\n", labelStyle("Agent:"), agentName, agentPath)
	}
	if model != "" {
		fmt.Printf("%s %s\n", labelStyle("Model:"), model)
	}

	fmt.Println(strings.Repeat("-", 40)) // Separator

	// --- Print Messages (copied from original implementation) ---
	for _, msg := range messages {
		switch msg.Role {
		case openai.ChatMessageRoleSystem:
			fmt.Printf("%s\n%s\n\n", systemStyle("[SYSTEM]"), msg.Content)
		case openai.ChatMessageRoleUser:
			fmt.Printf("%s\n%s\n\n", userStyle("[USER]"), msg.Content)
		case openai.ChatMessageRoleAssistant:
			fmt.Printf("%s\n", assistantStyle("[ASSISTANT]"))
			if msg.Content != "" {
				fmt.Printf("%s\n", msg.Content)
			}
			if len(msg.ToolCalls) > 0 {
				fmt.Printf("%s\n", toolStyle("Tool Calls:"))
				for _, tc := range msg.ToolCalls {
					// Pretty print arguments JSON
					var argsMap map[string]any
					argsStr := tc.Function.Arguments
					if err := json.Unmarshal([]byte(argsStr), &argsMap); err == nil {
						prettyJSON, _ := json.MarshalIndent(argsMap, "  ", "  ")
						argsStr = string(prettyJSON)
					}
					// Indent arguments slightly more for clarity
					indentedArgs := strings.ReplaceAll(argsStr, "\n", "\n    ")
					fmt.Printf("  - %s (%s):\n    %s\n", toolStyle(tc.Function.Name), tc.ID, toolDataStyle(indentedArgs))
				}
			}
			fmt.Println() // Add newline after assistant message
		case openai.ChatMessageRoleTool:
			// Check if content indicates an error
			isError := strings.HasPrefix(msg.Content, "Error:")
			prefix := toolStyle(fmt.Sprintf("[TOOL: %s (%s)]", msg.Name, msg.ToolCallID))
			contentStyle := toolDataStyle
			if isError {
				prefix = errorStyle(fmt.Sprintf("[TOOL ERROR: %s (%s)]", msg.Name, msg.ToolCallID))
				contentStyle = errorStyle
			}

			// Try to pretty print JSON content if possible
			var contentMap map[string]any
			var contentSlice []any
			contentStr := msg.Content
			if err := json.Unmarshal([]byte(contentStr), &contentMap); err == nil {
				prettyJSON, _ := json.MarshalIndent(contentMap, "  ", "  ")
				contentStr = string(prettyJSON)
			} else if err := json.Unmarshal([]byte(contentStr), &contentSlice); err == nil {
				prettyJSON, _ := json.MarshalIndent(contentSlice, "  ", "  ")
				contentStr = string(prettyJSON)
			}
			// Indent content for clarity
			indentedContent := strings.ReplaceAll(contentStr, "\n", "\n  ")
			fmt.Printf("%s\n  %s\n\n", prefix, contentStyle(indentedContent))
		default:
			fmt.Printf("[%s]\n%s\n\n", strings.ToUpper(msg.Role), msg.Content)
		}
	}
}

// handleShowAgent displays the details of the agent specified by the agentPath.
func handleShowAgent(agentPath string) {
	agent, err := loadAgent(agentPath)
	if err != nil {
		color.Red("Error loading agent: %v\n", err)
		return
	}

	labelStyle := color.New(color.FgHiCyan, color.Bold).SprintFunc()

	// Print agent name and description if available
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
			printMCPServerInfo(name, server, agent.MCPServers)
		}
	}

	if len(agent.Functions) == 0 && len(agent.MCPServers) == 0 {
		noFuncStyle := color.New(color.FgYellow, color.Italic).SprintFunc()
		fmt.Printf("%s\n", noFuncStyle("No functions or MCP servers available."))
	}
}

// printOutput prints last output of a history file
func printOutput(history ConversationHistory) {
	if len(history.Messages) < 1 {
		fmt.Println("No messages found in this history.")
		return
	}

	lastMessage := history.Messages[len(history.Messages)-1]
	fmt.Println(lastMessage.Content)
}

func printMCPServerInfo(name string, server MCPServerConfig, allServers map[string]MCPServerConfig) {
	nameStyle := color.New(color.FgHiGreen, color.Bold).SprintFunc()
	descStyle := color.New(color.FgHiBlack).SprintFunc()
	commandStyle := color.New(color.FgYellow).SprintFunc()
	errorStyle := color.New(color.FgRed).SprintFunc()
	toolStyle := color.New(color.FgCyan).SprintFunc()

	fmt.Printf("  %s: %s\n", nameStyle(name), descStyle("MCP Server"))
	fmt.Printf("    %s %s %s\n", commandStyle("Command:"), server.Command, strings.Join(server.Args, " "))

	// Try to discover MCP tools by temporarily starting the server
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client := NewMCPClient(name, server)
	if err := client.Start(ctx); err != nil {
		fmt.Printf("    %s %s\n", errorStyle("Error:"), fmt.Sprintf("Failed to start MCP server: %v", err))
	} else {
		defer client.Stop()
		tools := client.GetTools()
		if len(tools) > 0 {
			fmt.Printf("    %s\n", commandStyle("Available Tools:"))
			for _, tool := range tools {
				if tool.Function != nil {
					// Remove the mcp_{server_name}_ prefix for display
					displayName := tool.Function.Name
					prefix := fmt.Sprintf("mcp_%s_", name)
					if strings.HasPrefix(displayName, prefix) {
						displayName = strings.TrimPrefix(displayName, prefix)
					}
					fmt.Printf("      %s: %s\n", toolStyle(displayName), tool.Function.Description)
				}
			}
		} else {
			fmt.Printf("    %s %s\n", descStyle("Tools:"), "No tools discovered")
		}
	}
	fmt.Println()
}
