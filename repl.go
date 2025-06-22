package main

import (
	"context"
	"fmt"
	"io"
	"os"
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

	cyan := color.New(color.FgCyan).SprintFunc()
	green := color.New(color.FgGreen).SprintFunc()
	labelStyle := color.New(color.FgHiCyan, color.Bold).SprintFunc()

	switch command {
	case "/help":
		fmt.Fprintf(os.Stderr, "%s %s\n", cyan("[REPL]"), "Available commands:")
		fmt.Fprintf(os.Stderr, "  %s - Exit the session\n", green("/exit, /quit"))
		fmt.Fprintf(os.Stderr, "  %s - Show this help message\n", green("/help"))
		fmt.Fprintf(os.Stderr, "  %s - Show current configuration\n", green("/config"))
		fmt.Fprintf(os.Stderr, "  %s - Show or set model (e.g., /model openai/gpt-4)\n", green("/model <provider/model>"))
		fmt.Fprintf(os.Stderr, "\n")
		return true

	case "/config":
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

	case "/model":
		if len(args) == 0 {
			provider, model, _ := app.parseModel()
			fmt.Fprintf(os.Stderr, "%s %s: %s/%s\n", cyan("[REPL]"), "Current model", provider, model)
		} else {
			if err := validateAndSetModel(app, opts, args[0]); err != nil {
				fmt.Fprintf(os.Stderr, "%s %s\n", color.New(color.FgRed).Sprint("[ERROR]"), err.Error())
				return true
			}

			provider, model, _ := app.parseModel()
			fmt.Fprintf(os.Stderr, "%s %s: %s/%s\n", cyan("[REPL]"), "Model updated to", provider, model)
		}
		return true

	default:
		if strings.HasPrefix(command, "/") {
			fmt.Fprintf(os.Stderr, "%s %s '%s'. Type /help for available commands.\n",
				color.New(color.FgRed).Sprint("[ERROR]"), "Unknown command", command)
			return true
		}
		return false
	}
}

// validateAndSetModel validates a model string (including aliases) and sets it if valid
func validateAndSetModel(app *Application, opts *CLIOptions, modelStr string) error {
	// First check if it's an alias
	resolvedModel := modelStr
	if app.config != nil {
		if aliasedModel, ok := app.config.ModelAliases[modelStr]; ok {
			resolvedModel = aliasedModel
		}
	}

	// Validate the resolved model format
	parts := strings.SplitN(resolvedModel, "/", 2)
	if len(parts) != 2 {
		if resolvedModel != modelStr {
			// It was an alias that resolved to an invalid format
			return fmt.Errorf("model alias '%s' resolves to invalid format '%s'. Must be provider/model (e.g., openai/gpt-4)", modelStr, resolvedModel)
		} else {
			// Direct model string is invalid
			return fmt.Errorf("invalid model format '%s'. Must be provider/model (e.g., openai/gpt-4) or a valid alias", modelStr)
		}
	}

	// Update the model flag in the application and options
	app.modelFlag = modelStr // Store the original input (might be an alias)
	opts.Model = modelStr

	return nil
}
