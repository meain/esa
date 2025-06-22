package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/fatih/color"
	"github.com/sashabaranov/go-openai"
)

// runReplMode starts the REPL (Read-Eval-Print Loop) mode
func runReplMode(opts *CLIOptions, args []string) error {
	// Handle agent selection with + prefix in the initial query
	initialQuery := strings.Join(args, " ")
	if strings.HasPrefix(initialQuery, "+") {
		opts.CommandStr = initialQuery
		parseAgentCommand(opts)
		initialQuery = opts.CommandStr
	}

	// Initialize application
	app, err := NewApplication(opts)
	if err != nil {
		return fmt.Errorf("failed to initialize application: %v", err)
	}

	// Start MCP servers if configured
	if len(app.agent.MCPServers) > 0 {
		ctx := context.Background()
		if err := app.mcpManager.StartServers(ctx, app.agent.MCPServers); err != nil {
			return fmt.Errorf("failed to start MCP servers: %v", err)
		}

		defer app.mcpManager.StopAllServers()
		app.debugPrint("MCP Servers", fmt.Sprintf("Started %d MCP servers", len(app.agent.MCPServers)))
	}

	prompt, err := app.getSystemPrompt()
	if err != nil {
		return fmt.Errorf("error processing system prompt: %v", err)
	}

	if app.messages == nil {
		app.messages = []openai.ChatCompletionMessage{{
			Role:    "system",
			Content: prompt,
		}}
	}

	// Debug prints before starting communication
	app.debugPrint("System Message", app.messages[0].Content)

	cyan := color.New(color.FgCyan).SprintFunc()
	green := color.New(color.FgGreen).SprintFunc()
	red := color.New(color.FgRed).SprintFunc()

	fmt.Fprintf(
		os.Stderr,
		"%s %s\n",
		cyan("[REPL]"),
		strings.Join([]string{
			"Starting interactive mode",
			"- '/exit' or '/quit' to end the session",
			"- '/help' for available commands",
			"- Press enter twice to send your message.",
		}, "\n"),
	)
	// Handle initial query if provided
	if initialQuery != "" {
		fmt.Fprintf(os.Stderr, "\n%s %s\n", green("you>"), initialQuery)
		app.messages = append(app.messages, openai.ChatCompletionMessage{
			Role:    "user",
			Content: initialQuery,
		})

		fmt.Fprintf(os.Stderr, "\n%s ", red("esa>"))
		app.runConversationLoop(*opts)
	}

	// Main REPL loop
	for {
		fmt.Fprintf(os.Stderr, "\n%s ", green("you>"))

		input, err := readUserInput("")
		if err != nil {
			if err == io.EOF {
				fmt.Fprintf(os.Stderr, "\n%s %s\n", cyan("[REPL]"), "Goodbye!")
				break
			}
			return fmt.Errorf("error reading input: %v", err)
		}

		input = strings.TrimSpace(input)
		if input == "/exit" || input == "/quit" || input == "" {
			fmt.Fprintf(os.Stderr, "%s %s\n", cyan("[REPL]"), "Goodbye!")
			break
		}

		// Handle REPL commands
		if strings.HasPrefix(input, "/") {
			if handleReplCommand(input, app, opts) {
				continue
			}
		}

		fmt.Fprintf(os.Stderr, "%s ", red("esa>"))
		app.messages = append(app.messages, openai.ChatCompletionMessage{
			Role:    "user",
			Content: input,
		})

		app.runConversationLoop(*opts)
	}

	return nil
}

// handleReplCommand handles special REPL commands
// Returns true if the command was handled (and should continue REPL loop)
func handleReplCommand(input string, app *Application, opts *CLIOptions) bool {
	parts := strings.Fields(input)
	if len(parts) == 0 {
		return false
	}

	command := parts[0]
	args := parts[1:]

	switch command {
	case "/help":
		return handleHelpCommand()
	case "/config":
		return handleConfigCommand(app)
	case "/model":
		return handleModelCommand(args, app, opts)
	case "/agent":
		return handleAgentCommand(args, app, opts)
	case "/editor":
		return handleEditorCommand(app, opts)
	default:
		return handleUnknownCommand(command)
	}
}

func handleHelpCommand() bool {
	cyan := color.New(color.FgCyan).SprintFunc()
	green := color.New(color.FgGreen).SprintFunc()

	fmt.Fprintf(os.Stderr, "%s %s\n", cyan("[REPL]"), "Available commands:")
	fmt.Fprintf(os.Stderr, "  %s - Exit the session\n", green("/exit, /quit"))
	fmt.Fprintf(os.Stderr, "  %s - Show this help message\n", green("/help"))
	fmt.Fprintf(os.Stderr, "  %s - Show current configuration\n", green("/config"))
	fmt.Fprintf(os.Stderr, "  %s - Show or set model (e.g., /model openai/gpt-4)\n", green("/model <provider/model>"))
	fmt.Fprintf(os.Stderr, "  %s - Show or set agent (e.g., /agent +k8s, /agent myagent)\n", green("/agent <agent>"))
	fmt.Fprintf(os.Stderr, "  %s - Open the default editor\n", green("/editor"))
	return true
}

func handleConfigCommand(app *Application) bool {
	cyan := color.New(color.FgCyan).SprintFunc()
	labelStyle := color.New(color.FgHiCyan, color.Bold).SprintFunc()

	fmt.Fprintf(os.Stderr, "%s %s\n", cyan("[REPL]"), "Current configuration:")

	// Show agent information
	if app.agent.Name != "" {
		fmt.Fprintf(os.Stderr, "%s %s\n", labelStyle("Agent Name:"), app.agent.Name)
	}
	if app.agent.Description != "" {
		fmt.Fprintf(os.Stderr, "%s %s\n", labelStyle("Agent Description:"), app.agent.Description)
	}
	fmt.Fprintf(os.Stderr, "%s %s\n", labelStyle("Agent Path:"), app.agentPath)

	provider, model, info := app.parseModel()
	askLevel := app.getEffectiveAskLevel()

	fmt.Fprintf(os.Stderr, "%s %s/%s\n", labelStyle("Current Model:"), provider, model)
	fmt.Fprintf(os.Stderr, "%s %s\n", labelStyle("Base URL:"), info.baseURL)
	fmt.Fprintf(os.Stderr, "%s %s\n", labelStyle("API Key Env:"), info.apiKeyEnvar)
	fmt.Fprintf(os.Stderr, "%s %s\n", labelStyle("Ask Level:"), askLevel)
	fmt.Fprintf(os.Stderr, "%s %v\n", labelStyle("Debug Mode:"), app.debug)
	fmt.Fprintf(os.Stderr, "%s %d\n", labelStyle("Functions:"), len(app.agent.Functions))
	fmt.Fprintf(os.Stderr, "%s %d\n", labelStyle("MCP Servers:"), len(app.agent.MCPServers))

	return true
}

func handleModelCommand(args []string, app *Application, opts *CLIOptions) bool {
	cyan := color.New(color.FgCyan).SprintFunc()

	if len(args) == 0 {
		provider, model, _ := app.parseModel()
		fmt.Fprintf(os.Stderr, "%s %s: %s/%s\n", cyan("[REPL]"), "Current model", provider, model)
		return true
	}

	if err := validateAndSetModel(app, opts, args[0]); err != nil {
		fmt.Fprintf(os.Stderr, "%s %s\n", color.New(color.FgRed).Sprint("[ERROR]"), err.Error())
		return true
	}

	provider, model, _ := app.parseModel()
	fmt.Fprintf(os.Stderr, "%s %s: %s/%s\n", cyan("[REPL]"), "Model updated to", provider, model)
	return true
}

func handleAgentCommand(args []string, app *Application, opts *CLIOptions) bool {
	cyan := color.New(color.FgCyan).SprintFunc()
	green := color.New(color.FgGreen).SprintFunc()
	labelStyle := color.New(color.FgHiCyan, color.Bold).SprintFunc()

	if len(args) == 0 {
		// Show current agent information
		fmt.Fprintf(os.Stderr, "%s %s:\n", cyan("[REPL]"), "Current agent")

		// Show agent information
		if app.agent.Name != "" {
			fmt.Fprintf(os.Stderr, "%s %s\n", labelStyle("Name:"), app.agent.Name)
		}
		if app.agent.Description != "" {
			fmt.Fprintf(os.Stderr, "%s %s\n", labelStyle("Description:"), app.agent.Description)
		}
		fmt.Fprintf(os.Stderr, "%s %s\n", labelStyle("Path:"), app.agentPath)

		if app.agent.DefaultModel != "" {
			fmt.Fprintf(os.Stderr, "%s %s\n", labelStyle("Default Model:"), app.agent.DefaultModel)
		}

		fmt.Fprintf(os.Stderr, "%s %d\n", labelStyle("Functions:"), len(app.agent.Functions))
		fmt.Fprintf(os.Stderr, "%s %d\n", labelStyle("MCP Servers:"), len(app.agent.MCPServers))

		return true
	}

	agentStr := args[0]
	if err := validateAndSetAgent(app, opts, agentStr); err != nil {
		fmt.Fprintf(os.Stderr, "%s %s\n", color.New(color.FgRed).Sprint("[ERROR]"), err.Error())
		return true
	}

	// Show confirmation of the switch
	agentName := app.agent.Name
	if agentName == "" {
		agentName = agentStr
	}
	fmt.Fprintf(os.Stderr, "%s %s: %s\n", cyan("[REPL]"), "Agent switched to", green(agentName))
	return true
}

// handleEditorCommand handles the /editor command to open the default text editor
func handleEditorCommand(app *Application, opts *CLIOptions) bool {
	cyan := color.New(color.FgCyan).SprintFunc()
	red := color.New(color.FgRed).SprintFunc()

	// Get editor from environment variable or default to nano
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "nano"
	}

	// Create a temporary file
	tmpFile, err := os.CreateTemp("", "esa_prompt_*.txt")
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s Failed to create temporary file: %v\n", red("[ERROR]"), err)
		return true
	}
	defer os.Remove(tmpFile.Name()) // Clean up

	// Close the file so the editor can open it
	tmpFile.Close()

	fmt.Fprintf(os.Stderr, "%s Opening editor: %s\n", cyan("[REPL]"), editor)

	// Open the editor
	cmd := exec.Command(editor, tmpFile.Name())
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "%s Failed to run editor: %v\n", red("[ERROR]"), err)
		return true
	}

	// Read the content back
	content, err := os.ReadFile(tmpFile.Name())
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s Failed to read temporary file: %v\n", red("[ERROR]"), err)
		return true
	}

	// Process the content
	finalContent := strings.TrimSpace(string(content))

	if finalContent == "" {
		fmt.Fprintf(os.Stderr, "%s No content entered, canceling.\n", cyan("[REPL]"))
		return true
	}

	// Add the message and run the conversation
	fmt.Fprintf(os.Stderr, "%s Prompt entered via editor\n", cyan("[REPL]"))
	app.messages = append(app.messages, openai.ChatCompletionMessage{
		Role:    "user",
		Content: finalContent,
	})

	fmt.Fprintf(os.Stderr, "%s ", color.New(color.FgRed).SprintFunc()("esa>"))
	app.runConversationLoop(*opts)

	return true
}

func handleUnknownCommand(command string) bool {
	if strings.HasPrefix(command, "/") {
		fmt.Fprintf(os.Stderr, "%s %s '%s'. Type /help for available commands.\n",
			color.New(color.FgRed).Sprint("[ERROR]"), "Unknown command", command)
		return true
	}
	return false
}

// validateAndSetModel validates a model string (including aliases) and sets it if valid
func validateAndSetModel(app *Application, opts *CLIOptions, modelStr string) error {
	app.modelFlag = modelStr
	opts.Model = modelStr

	client, err := setupOpenAIClient(modelStr, app.config)
	if err != nil {
		return fmt.Errorf("failed to set model '%s': %v", modelStr, err)
	}

	app.client = client
	return nil
}

// validateAndSetAgent validates an agent string and sets it if valid
func validateAndSetAgent(app *Application, opts *CLIOptions, agentStr string) error {
	// Parse the agent string to determine the agent name and path
	agentName, agentPath := parseAgentString(agentStr)

	// Create a temporary CLIOptions to use with loadConfiguration
	tempOpts := &CLIOptions{
		AgentName: agentName,
		AgentPath: agentPath,
	}

	// Load the agent using the existing loadConfiguration function
	agent, err := loadConfiguration(tempOpts)
	if err != nil {
		return fmt.Errorf("failed to load agent '%s': %v", agentStr, err)
	}

	// Update the application and options
	app.agent = agent
	app.agentPath = tempOpts.AgentPath // Use the resolved path from loadConfiguration
	opts.AgentPath = tempOpts.AgentPath
	if agentName != "" {
		opts.AgentName = agentName
	}

	// Restart MCP servers if needed
	if len(agent.MCPServers) > 0 {
		// Stop existing servers
		app.mcpManager.StopAllServers()

		// Start new servers
		ctx := context.Background()
		if err := app.mcpManager.StartServers(ctx, agent.MCPServers); err != nil {
			return fmt.Errorf("failed to start MCP servers for agent: %v", err)
		}
	} else {
		// Stop all servers if the new agent doesn't have any
		app.mcpManager.StopAllServers()
	}

	return nil
}

// parseAgentString parses an agent string and returns the agent name and path
func parseAgentString(agentStr string) (agentName, agentPath string) {
	// Handle +agent syntax and direct paths
	if strings.HasPrefix(agentStr, "+") {
		// Handle +agent syntax
		agentName = agentStr[1:] // Remove + prefix
		agentPath = expandHomePath(fmt.Sprintf("%s/%s.toml", DefaultAgentsDir, agentName))
	} else if strings.Contains(agentStr, "/") || strings.HasSuffix(agentStr, ".toml") {
		// Treat as direct path
		agentPath = agentStr
		if !strings.HasPrefix(agentPath, "/") {
			agentPath = expandHomePath(agentPath)
		}
	} else {
		// Treat as agent name without + prefix
		agentName = agentStr
		agentPath = expandHomePath(fmt.Sprintf("%s/%s.toml", DefaultAgentsDir, agentName))
	}

	// Check for builtin agents
	if agentName != "" {
		if _, exists := builtinAgents[agentName]; exists {
			agentPath = "builtin:" + agentName
		}
	}

	return agentName, agentPath
}
