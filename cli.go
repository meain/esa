package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/sashabaranov/go-openai" // Added for message types
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
	HistoryIndex int    // Index for show-history command
	OutputFormat string // Output format for show-history (text, markdown, json)
}

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
		for _, fn := range agent.Functions {
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
	ListHistory
	ShowHistory
)

func parseFlags() (CLIOptions, CommandType) {
	opts := CLIOptions{}

	// Define command-line flags
	flag.BoolVar(&opts.DebugMode, "debug", false, "Enable debug mode")
	flag.BoolVar(&opts.ContinueChat, "c", false, "Continue last conversation")
	flag.BoolVar(&opts.ContinueChat, "continue", false, "Continue last conversation")
	flag.BoolVar(&opts.RetryChat, "r", false, "Retry last command")
	flag.BoolVar(&opts.RetryChat, "retry", false, "Retry last command")
	agentPath := flag.String("agent", "", "Path to the agent config file")
	configPath := flag.String("config", "", "Path to the global config file (default: ~/.config/esa/config.toml)")
	flag.StringVar(&opts.Model, "m", "", "Model to use (e.g., openai/gpt-4)")
	flag.StringVar(&opts.Model, "model", "", "Model to use (e.g., openai/gpt-4)")
	flag.StringVar(&opts.AskLevel, "ask", "none", "Ask level (none, unsafe, all)")
	flag.BoolVar(&opts.ShowCommands, "show-commands", false, "Show executed commands")
	flag.BoolVar(&opts.HideProgress, "hide-progress", false, "Disable LLM-generated progress summary for each function")
	flag.StringVar(&opts.OutputFormat, "output", "text", "Output format for show-history (text, markdown, json)")
	help := flag.Bool("help", false, "Show help message")
	flag.Parse()

	// Validate output format
	validFormats := map[string]bool{"text": true, "markdown": true, "json": true}
	if !validFormats[opts.OutputFormat] {
		printHelp()
		color.Red("\nError: Invalid output format %q. Must be one of: text, markdown, json.", opts.OutputFormat)
		os.Exit(1)
	}

	// Handle help flag
	if *help {
		printHelp()
		os.Exit(0)
	}

	// Process command arguments
	args := flag.Args()
	opts.CommandStr = strings.Join(args, " ")
	opts.AgentPath = *agentPath
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
			opts.AgentPath = fmt.Sprintf("%s/%s.toml", DefaultAgentsDir, agentName)
		}
		return ShowAgent
	}

	// Check for list-agents command
	if strings.HasPrefix(opts.CommandStr, "list-agents") {
		return ListAgents
	}

	// Check for list-history command
	if strings.HasPrefix(opts.CommandStr, "list-history") {
		return ListHistory
	}

	// Check for show-history command
	if strings.HasPrefix(opts.CommandStr, "show-history") {
		parts := strings.SplitN(opts.CommandStr, " ", 2)
		if len(parts) == 2 {
			index, err := strconv.Atoi(parts[1])
			if err == nil && index > 0 {
				opts.HistoryIndex = index
				return ShowHistory
			}
		}
		// If format is wrong or index is invalid, print help and exit
		printHelp()
		color.Red("\nError: Invalid format for show-history. Use 'show-history <number>'.")
		os.Exit(1)
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

	// Set agent path based on agent name
	opts.AgentPath = fmt.Sprintf("%s/%s.toml", DefaultAgentsDir, opts.AgentName)
}

func printHelp() {
	fmt.Println("Usage: esa <command> [--debug] [--agent <path>] [--config <path>] [--ask <level>] [--show-progress]")
	fmt.Println("\nOptions:")
	fmt.Println("  --debug         Enable debug mode")
	fmt.Println("  -c, --continue  Continue last conversation")
	fmt.Println("  -r, --retry     Retry the last command")
	fmt.Println("  --agent         Path to the agent config file")
	fmt.Println("  --config        Path to the global config file (default: ~/.config/esa/config.toml)")
	fmt.Println("  -m, --model     Model to use (e.g., openai/gpt-4)")
	fmt.Println("  --ask           Ask level (none, unsafe, all)")
	fmt.Println("  --show-commands Show executed commands")
	fmt.Println("  --hide-progress Disable progress summary for each function (enabled by default)")
	fmt.Println("  --output        Output format for show-history (text, markdown, json)")
	fmt.Println("\nCommands:")
	fmt.Println("  list-agents          List all available agents")
	fmt.Println("  list-history         List all saved conversation histories")
	fmt.Println("  show-history <num>   Show details of a specific conversation history")
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

	agentNameStyle := color.New(color.FgHiCyan, color.Bold).SprintFunc()
	timeStyle := color.New(color.FgYellow).SprintFunc()
	fileStyle := color.New(color.FgHiBlack).SprintFunc()

	fmt.Println("Available conversation histories (most recent first):")
	for i, fileName := range sortedFiles { // Add index 'i'
		// Attempt to parse agent name and timestamp from filename
		// Format: agentName-YYYYMMDD-HHMMSS.json
		parts := strings.SplitN(strings.TrimSuffix(fileName, ".json"), "-", 2)
		agentName := "unknown"
		timestampStr := "unknown"
		if len(parts) == 2 {
			agentName = parts[0]
			timestampStr = parts[1]
			// Attempt to parse the timestamp for better formatting
			parsedTime, err := time.Parse("20060102-150405", timestampStr)
			if err == nil {
				timestampStr = parsedTime.Format("2006-01-02 15:04:05") // More readable format
			}
		}

		// Add index (i+1) to the output format
		fmt.Printf(" %2d: %s %s %s\n",
			i+1,
			agentNameStyle(agentName),
			timeStyle(timestampStr),
			fileStyle("("+fileName+")"),
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
		printHistoryMarkdown(fileName, history)
	default: // "text"
		printHistoryText(fileName, history)
	}
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
				var argsMap map[string]interface{}
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

			var contentMap map[string]interface{}
			var contentSlice []interface{}
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

	// --- Define Styles (copied from original implementation) ---
	systemStyle := color.New(color.FgMagenta, color.Italic).SprintFunc()
	userStyle := color.New(color.FgGreen).SprintFunc()
	assistantStyle := color.New(color.FgBlue).SprintFunc()
	toolStyle := color.New(color.FgYellow).SprintFunc()
	toolDataStyle := color.New(color.FgHiBlack).SprintFunc()
	errorStyle := color.New(color.FgRed).SprintFunc()
	labelStyle := color.New(color.Bold).SprintFunc()

	// --- Print Header (copied from original implementation) ---
	fmt.Printf("%s %s\n", labelStyle("History File:"), fileName)
	if agentPath != "" {
		// Try to extract agent name from path for display
		agentName := strings.TrimSuffix(filepath.Base(agentPath), ".toml")
		fmt.Printf("%s +%s (%s)\n", labelStyle("Agent:"), agentName, agentPath)
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
					var argsMap map[string]interface{}
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
			var contentMap map[string]interface{}
			var contentSlice []interface{}
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
