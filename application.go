package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/fatih/color"
	"github.com/sashabaranov/go-openai"
)

const (
	historyTimeFormat = "20060102-150405"
	defaultModel      = "openai/gpt-4o-mini"
)

type Application struct {
	agent           Agent
	agentPath       string
	client          *openai.Client
	debug           bool
	historyFile     string
	messages        []openai.ChatCompletionMessage
	debugPrint      func(section string, v ...any)
	showCommands    bool
	showProgress    bool
	lastProgressLen int
	modelFlag       string
	config          *Config
	mcpManager      *MCPManager
	cliAskLevel     string
}

// providerInfo contains provider-specific configuration
type providerInfo struct {
	baseURL           string
	apiKeyEnvar       string
	additionalHeaders map[string]string
}

// parseModel parses model string in format "provider/model" and
// returns provider, model name, base URL and API key environment
// variable
func (app *Application) parseModel() (provider string, model string, info providerInfo) {
	modelStr := app.modelFlag
	if modelStr == "" && app.agent.DefaultModel != "" {
		modelStr = app.agent.DefaultModel
	}

	if modelStr == "" && app.config != nil {
		modelStr = app.config.Settings.DefaultModel
	}

	return parseModel(modelStr, app.config)
}

func parseModel(modelStr string, config *Config) (provider string, model string, info providerInfo) {
	if modelStr == "" {
		modelStr = defaultModel
	}

	// Check if the model string is an alias
	if config != nil {
		if aliasedModel, ok := config.ModelAliases[modelStr]; ok {
			modelStr = aliasedModel
		}
	}

	parts := strings.SplitN(modelStr, "/", 2)
	if len(parts) != 2 {
		log.Fatalf("invalid model format %q - must be provider/model", modelStr)
	}

	provider = parts[0]
	model = parts[1]

	// Start with default provider info
	switch provider {
	case "openai":
		info = providerInfo{
			baseURL:     "https://api.openai.com/v1",
			apiKeyEnvar: "OPENAI_API_KEY",
		}
	case "ollama":
		info = providerInfo{
			baseURL:     "http://localhost:11434/v1",
			apiKeyEnvar: "OLLAMA_API_KEY",
		}
	case "openrouter":
		info = providerInfo{
			baseURL:     "https://openrouter.ai/api/v1",
			apiKeyEnvar: "OPENROUTER_API_KEY",
		}
	case "groq":
		info = providerInfo{
			baseURL:     "https://api.groq.com/openai/v1",
			apiKeyEnvar: "GROQ_API_KEY",
		}
	case "github":
		info = providerInfo{
			baseURL:     "https://models.inference.ai.azure.com",
			apiKeyEnvar: "GITHUB_MODELS_API_KEY",
		}
	case "copilot":
		info = providerInfo{
			baseURL:     "https://api.githubcopilot.com",
			apiKeyEnvar: "COPILOT_API_KEY",
			additionalHeaders: map[string]string{
				"Content-Type":           "application/json",
				"Copilot-Integration-Id": "vscode-chat",
			},
		}
	}

	// Override with config if present
	if config != nil {
		if providerCfg, ok := config.Providers[provider]; ok {
			// Only override non-empty values
			if providerCfg.BaseURL != "" {
				info.baseURL = providerCfg.BaseURL
			}

			if providerCfg.APIKeyEnvar != "" {
				info.apiKeyEnvar = providerCfg.APIKeyEnvar
			}

			if len(providerCfg.AdditionalHeaders) > 0 {
				if info.additionalHeaders == nil {
					info.additionalHeaders = make(map[string]string)
				}

				for key, value := range providerCfg.AdditionalHeaders {
					info.additionalHeaders[key] = value
				}
			}
		}
	}

	return provider, model, info
}

func NewApplication(opts *CLIOptions) (*Application, error) {
	// Load global config first
	config, err := LoadConfig(opts.ConfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load global config: %v", err)
	}

	cacheDir, err := setupCacheDir()
	if err != nil {
		return nil, fmt.Errorf("failed to setup cache directory: %v", err)
	}

	var (
		messages []openai.ChatCompletionMessage
	)

	// If continue conversation is set, also set continue chat
	if opts.ContinueConversation > 0 {
		opts.ContinueChat = true
	} else if opts.ContinueChat {
		opts.ContinueConversation = 1
	}

	historyFile, hasHistory := getHistoryFilePath(cacheDir, opts)
	if hasHistory && (opts.ContinueChat || opts.RetryChat) {
		var history ConversationHistory
		data, err := os.ReadFile(historyFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load conversation history: %v", err)
		}
		err = json.Unmarshal(data, &history)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal conversation history: %v", err)
		}

		allMessages := history.Messages
		agentPath := history.AgentPath

		app := &Application{debug: opts.DebugMode}
		app.debugPrint = createDebugPrinter(app.debug)

		if opts.RetryChat && len(allMessages) > 1 {
			// In retry mode, keep all messages up until the last user message
			var lastUserMessageIndex int = -1

			// Find the last user message in the history
			for i := len(allMessages) - 1; i >= 0; i-- {
				if allMessages[i].Role == "user" {
					lastUserMessageIndex = i
					break
				}
			}

			if lastUserMessageIndex >= 0 {
				// Keep all messages up to and including the last user message
				messages = allMessages[:lastUserMessageIndex+1]

				// If a command string was provided with -r, replace the last user message content
				if opts.CommandStr != "" {
					messages[lastUserMessageIndex].Content = opts.CommandStr
					app.debugPrint("Retry Mode",
						fmt.Sprintf("Keeping %d messages", len(messages)),
						fmt.Sprintf("Replacing last user message with: %q", opts.CommandStr),
						fmt.Sprintf("Agent: %s", agentPath), // Note: Agent might be overridden later if specified in opts
						fmt.Sprintf("History file: %q", historyFile),
					)
				} else {
					app.debugPrint("Retry Mode",
						fmt.Sprintf("Keeping %d messages", len(messages)),
						fmt.Sprintf("Agent: %s", agentPath), // Note: Agent might be overridden later if specified in opts
						fmt.Sprintf("History file: %q", historyFile),
					)
				}
			} else {
				// If we couldn't find a user message, just use the system message
				messages = []openai.ChatCompletionMessage{allMessages[0]}

				app.debugPrint("Retry Mode",
					fmt.Sprintf("No user messages found"),
					fmt.Sprintf("Agent: %s", agentPath),
					fmt.Sprintf("History file: %q", historyFile),
				)
			}
		} else {
			// In continue mode, use all messages
			messages = allMessages
			app.debugPrint("History",
				fmt.Sprintf("Loaded %d messages from history", len(messages)),
				fmt.Sprintf("Agent: %s", agentPath),
				fmt.Sprintf("History file: %q", historyFile),
			)
		}

		if agentPath != "" && opts.AgentPath == "" {
			opts.AgentPath = agentPath
		}

		// Use model from history if none specified in opts
		if history.Model != "" && opts.Model == "" {
			opts.Model = history.Model
		}
	}

	if opts.AgentPath == "" {
		opts.AgentPath = DefaultAgentPath
	}

	if strings.HasPrefix(opts.AgentPath, "builtin:") {
		opts.AgentName = strings.TrimPrefix(opts.AgentPath, "builtin:")
	}

	agent, err := loadConfiguration(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to load agent configuration: %v", err)
	}

	client, err := setupOpenAIClient(opts.Model, config)
	if err != nil {
		return nil, fmt.Errorf("failed to setup OpenAI client: %v", err)
	}

	showCommands := opts.ShowCommands || config.Settings.ShowCommands

	// Initialize MCP manager
	mcpManager := NewMCPManager()

	app := &Application{
		agent:       agent,
		agentPath:   opts.AgentPath,
		client:      client,
		historyFile: historyFile,
		messages:    messages,
		modelFlag:   opts.Model,
		config:      config,
		mcpManager:  mcpManager,
		cliAskLevel: opts.AskLevel,

		debug:        opts.DebugMode,
		showCommands: showCommands && !opts.DebugMode,
		showProgress: !opts.HideProgress && !opts.DebugMode && !showCommands,
	}

	app.debugPrint = createDebugPrinter(app.debug)
	provider, model, info := app.parseModel()

	app.debugPrint("Configuration",
		fmt.Sprintf("Provider: %q", provider),
		fmt.Sprintf("Model: %q", model),
		fmt.Sprintf("Base URL: %q", info.baseURL),
		fmt.Sprintf("API key envar: %q", info.apiKeyEnvar),
		fmt.Sprintf("Agent path: %q", opts.AgentPath),
		fmt.Sprintf("History file: %q", historyFile),
		fmt.Sprintf("Debug mode: %v", app.debug),
		fmt.Sprintf("Show progress: %v", app.showProgress),
	)

	return app, nil
}

func (app *Application) Run(opts CLIOptions) {
	// Start MCP servers if configured
	if len(app.agent.MCPServers) > 0 {
		ctx := context.Background()
		if err := app.mcpManager.StartServers(ctx, app.agent.MCPServers); err != nil {
			log.Fatalf("Failed to start MCP servers: %v", err)
		}
		// Ensure MCP servers are stopped when the application exits
		defer app.mcpManager.StopAllServers()

		app.debugPrint("MCP Servers", fmt.Sprintf("Started %d MCP servers", len(app.agent.MCPServers)))
	}

	prompt, err := app.getSystemPrompt()
	if err != nil {
		log.Fatalf("Error processing system prompt: %v", err)
	}

	if app.messages == nil {
		app.messages = []openai.ChatCompletionMessage{{
			Role:    "system",
			Content: prompt,
		}}
	}

	// Debug prints before starting communication
	app.debugPrint("System Message", app.messages[0].Content)

	input := readStdin()
	app.debugPrint("Input State",
		fmt.Sprintf("Command string: %q", opts.CommandStr),
		fmt.Sprintf("Stdin: %q", input),
	)

	// If in retry mode and a command string was provided,
	// it means we replaced the last user message content during loading.
	// Don't process input again.
	if !(opts.RetryChat && opts.CommandStr != "") {
		app.processInput(opts.CommandStr, input)
	}

	app.runConversationLoop(opts)
}

func (app *Application) processInput(commandStr, input string) {
	if len(input) > 0 {
		app.messages = append(app.messages, openai.ChatCompletionMessage{
			Role:    "user",
			Content: input,
		})
	}

	if len(commandStr) > 0 {
		app.messages = append(app.messages, openai.ChatCompletionMessage{
			Role:    "user",
			Content: commandStr,
		})
	}

	// If no input from stdin or command line, use initial message from agent config
	prompt, err := app.processInitialMessage(app.agent.InitialMessage)
	if err != nil {
		log.Fatalf("Error processing initial message: %v", err)
	}

	if len(input) == 0 && len(commandStr) == 0 && app.agent.InitialMessage != "" {
		app.messages = append(app.messages, openai.ChatCompletionMessage{
			Role:    "user",
			Content: prompt,
		})
	}
}

func (app *Application) processInitialMessage(message string) (string, error) {
	// Use the same processing logic as system prompt
	return app.processSystemPrompt(message)
}

func (app *Application) runConversationLoop(opts CLIOptions) {
	openAITools := convertFunctionsToTools(app.agent.Functions)

	// Add MCP tools
	mcpTools := app.mcpManager.GetAllTools()
	openAITools = append(openAITools, mcpTools...)

	for {
		stream, err := app.client.CreateChatCompletionStream(
			context.Background(),
			openai.ChatCompletionRequest{
				Model:    app.getModel(),
				Messages: app.messages,
				Tools:    openAITools,
			})

		if err != nil {
			log.Fatalf("ChatCompletionStream error: %v", err)
		}

		assistantMsg := app.handleStreamResponse(stream)
		app.messages = append(app.messages, assistantMsg)

		if len(assistantMsg.ToolCalls) == 0 {
			app.saveConversationHistory()
			break
		}

		app.handleToolCalls(assistantMsg.ToolCalls, opts)
	}
}

func (app *Application) getModel() string {
	_, model, _ := app.parseModel()
	return model
}

// getEffectiveAskLevel returns the ask level to use, with CLI flag taking priority over agent config
func (app *Application) getEffectiveAskLevel() string {
	effectiveLevel := ""
	if app.cliAskLevel != "" {
		effectiveLevel = app.cliAskLevel
		app.debugPrint("Ask Level", fmt.Sprintf("Using CLI ask level: %s", effectiveLevel))
	} else if app.agent.Ask != "" {
		effectiveLevel = app.agent.Ask
		app.debugPrint("Ask Level", fmt.Sprintf("Using agent ask level: %s", effectiveLevel))
	} else {
		effectiveLevel = "unsafe"
		app.debugPrint("Ask Level", fmt.Sprintf("Using default ask level: %s", effectiveLevel))
	}
	return effectiveLevel
}

func (app *Application) handleStreamResponse(stream *openai.ChatCompletionStream) openai.ChatCompletionMessage {
	defer stream.Close()

	var assistantMsg openai.ChatCompletionMessage
	var fullContent strings.Builder
	hasContent := false

	for {
		response, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Fatalf("Stream error: %v", err)
		}

		if len(response.Choices) == 0 {
			continue
		}

		if response.Choices[0].Delta.ToolCalls != nil {
			for _, toolCall := range response.Choices[0].Delta.ToolCalls {
				if toolCall.ID != "" {
					assistantMsg.ToolCalls = append(assistantMsg.ToolCalls, toolCall)
				} else {
					lastToolCall := assistantMsg.ToolCalls[len(assistantMsg.ToolCalls)-1]
					lastToolCall.Function.Arguments += toolCall.Function.Arguments
					assistantMsg.ToolCalls[len(assistantMsg.ToolCalls)-1] = lastToolCall
				}
			}
		} else {
			// Clear progress line before showing result
			if app.showProgress && app.lastProgressLen > 0 {
				fmt.Fprintf(os.Stderr, "\r%s\r", strings.Repeat(" ", app.lastProgressLen))
				app.lastProgressLen = 0
			}

			content := response.Choices[0].Delta.Content
			if content != "" {
				hasContent = true
				fmt.Print(content)
				fullContent.WriteString(content)
			}
		}
	}

	if hasContent {
		fmt.Println()
	}

	assistantMsg.Role = "assistant"
	assistantMsg.Content = fullContent.String()
	return assistantMsg
}

type ConversationHistory struct {
	AgentPath string                         `json:"agent_path"`
	Model     string                         `json:"model"`
	Messages  []openai.ChatCompletionMessage `json:"messages"`
}

func (app *Application) saveConversationHistory() {
	provider, model, _ := app.parseModel()
	modelString := fmt.Sprintf("%s/%s", provider, model)
	history := ConversationHistory{
		AgentPath: app.agentPath,
		Model:     modelString,
		Messages:  app.messages,
	}

	if data, err := json.Marshal(history); err == nil {
		if err := os.WriteFile(app.historyFile, data, 0644); err != nil {
			app.debugPrint("Error", fmt.Sprintf("Failed to save history: %v", err))
		}
	}
}

func (app *Application) generateProgressSummary(funcName string, args string) string {
	return fmt.Sprintf("Calling %s...", funcName)
}

func (app *Application) handleToolCalls(toolCalls []openai.ToolCall, opts CLIOptions) {
	for _, toolCall := range toolCalls {
		if toolCall.Type != "function" || toolCall.Function.Name == "" {
			continue
		}

		// Check if it's an MCP tool (starts with "mcp_")
		// FIXME: This might not be reliable, the user might define a
		// function that starts with mcp_
		if strings.HasPrefix(toolCall.Function.Name, "mcp_") {
			app.handleMCPToolCall(toolCall, opts)
			continue
		}

		// Handle regular function
		var matchedFunc FunctionConfig
		for _, fc := range app.agent.Functions {
			if fc.Name == toolCall.Function.Name {
				matchedFunc = fc
				break
			}
		}

		if matchedFunc.Name == "" {
			log.Fatalf("No matching function found for: %s", toolCall.Function.Name)
		}

		if app.showProgress && len(matchedFunc.Output) == 0 {
			if summary := app.generateProgressSummary(matchedFunc.Name, toolCall.Function.Arguments); summary != "" {
				// Clear previous line if exists
				if app.lastProgressLen > 0 {
					fmt.Fprintf(os.Stderr, "\r%s\r", strings.Repeat(" ", app.lastProgressLen))
				}
				msg := fmt.Sprintf("⋮ %s", summary)
				color.New(color.FgBlue).Fprint(os.Stderr, msg)
				app.lastProgressLen = len(msg)
			}
		}

		// Set the provider and model env so that nested esa calls
		// make use of it. Users can override this by setting the
		// value explicitly in the nested esa calls.
		provider, model, _ := app.parseModel()
		os.Setenv("ESA_MODEL", fmt.Sprintf("%s/%s", provider, model))

		command, stdin, result, err := executeFunction(
			app.getEffectiveAskLevel(),
			matchedFunc,
			toolCall.Function.Arguments,
			app.showCommands,
		)
		app.debugPrint("Function Execution",
			fmt.Sprintf("Function: %s", matchedFunc.Name),
			fmt.Sprintf("Command: %s", command),
			fmt.Sprintf("Stdin: %s", stdin),
			fmt.Sprintf("Output: %s", result))

		if err != nil {
			app.debugPrint("Function Error", err)
			// Clear progress line before showing error
			if app.showProgress && app.lastProgressLen > 0 {
				fmt.Fprintf(os.Stderr, "\r%s\r", strings.Repeat(" ", app.lastProgressLen))
				app.lastProgressLen = 0
			}

			app.messages = append(app.messages, openai.ChatCompletionMessage{
				Role:       "tool",
				Name:       toolCall.Function.Name,
				Content:    fmt.Sprintf("Error: %v", err),
				ToolCallID: toolCall.ID,
			})
			continue
		}

		content := fmt.Sprintf("Command: %s\n\nOutput: \n%s", command, result)

		app.messages = append(app.messages, openai.ChatCompletionMessage{
			Role:       "tool",
			Name:       toolCall.Function.Name,
			Content:    content,
			ToolCallID: toolCall.ID,
		})
	}
}

// handleMCPToolCall handles tool calls for MCP servers
func (app *Application) handleMCPToolCall(toolCall openai.ToolCall, opts CLIOptions) {
	if app.showProgress {
		if summary := app.generateProgressSummary(toolCall.Function.Name, toolCall.Function.Arguments); summary != "" {
			// Clear previous line if exists
			if app.lastProgressLen > 0 {
				fmt.Fprintf(os.Stderr, "\r%s\r", strings.Repeat(" ", app.lastProgressLen))
			}
			msg := fmt.Sprintf("⋮ %s", summary)
			color.New(color.FgBlue).Fprint(os.Stderr, msg)
			app.lastProgressLen = len(msg)
		}
	}

	// Parse the arguments
	var arguments interface{}
	if toolCall.Function.Arguments != "" {
		if err := json.Unmarshal([]byte(toolCall.Function.Arguments), &arguments); err != nil {
			app.debugPrint("MCP Tool Error", fmt.Sprintf("Failed to parse arguments: %v", err))
			// Clear progress line before showing error
			if app.showProgress && app.lastProgressLen > 0 {
				fmt.Fprintf(os.Stderr, "\r%s\r", strings.Repeat(" ", app.lastProgressLen))
				app.lastProgressLen = 0
			}

			app.messages = append(app.messages, openai.ChatCompletionMessage{
				Role:       "tool",
				Name:       toolCall.Function.Name,
				Content:    fmt.Sprintf("Error: Failed to parse arguments: %v", err),
				ToolCallID: toolCall.ID,
			})
			return
		}
	}

	// Call the MCP tool with ask level and show commands options
	result, err := app.mcpManager.CallTool(toolCall.Function.Name, arguments, app.getEffectiveAskLevel(), app.showCommands)

	app.debugPrint("MCP Tool Execution",
		fmt.Sprintf("Tool: %s", toolCall.Function.Name),
		fmt.Sprintf("Arguments: %s", toolCall.Function.Arguments),
		fmt.Sprintf("Output: %s", result))

	if err != nil {
		app.debugPrint("MCP Tool Error", err)
		// Clear progress line before showing error
		if app.showProgress && app.lastProgressLen > 0 {
			fmt.Fprintf(os.Stderr, "\r%s\r", strings.Repeat(" ", app.lastProgressLen))
			app.lastProgressLen = 0
		}

		app.messages = append(app.messages, openai.ChatCompletionMessage{
			Role:       "tool",
			Name:       toolCall.Function.Name,
			Content:    fmt.Sprintf("Error: %v", err),
			ToolCallID: toolCall.ID,
		})
		return
	}

	app.messages = append(app.messages, openai.ChatCompletionMessage{
		Role:       "tool",
		Name:       toolCall.Function.Name,
		Content:    result,
		ToolCallID: toolCall.ID,
	})
}

type transportWithCustomHeaders struct {
	headers map[string]string
	base    http.RoundTripper
}

func (t *transportWithCustomHeaders) RoundTrip(req *http.Request) (*http.Response, error) {
	for key, value := range t.headers {
		req.Header.Set(key, value)
	}
	return t.base.RoundTrip(req)
}

func setupOpenAIClient(modelStr string, config *Config) (*openai.Client, error) {
	_, _, info := parseModel(modelStr, config)

	configuredAPIKey := os.Getenv(info.apiKeyEnvar)
	// Key name can be empty if we don't need any keys
	if info.apiKeyEnvar != "" && configuredAPIKey == "" {
		return nil, fmt.Errorf(info.apiKeyEnvar + " env not found")
	}

	llmConfig := openai.DefaultConfig(configuredAPIKey)
	llmConfig.BaseURL = info.baseURL

	if len(info.additionalHeaders) != 0 {
		httpClient := &http.Client{
			Transport: &transportWithCustomHeaders{
				headers: info.additionalHeaders,
				base:    http.DefaultTransport,
			},
		}

		llmConfig.HTTPClient = httpClient
	}

	client := openai.NewClientWithConfig(llmConfig)

	return client, nil
}

func createDebugPrinter(debugMode bool) func(string, ...any) {
	return func(section string, v ...any) {
		if !debugMode {
			return
		}

		width := 80
		headerColor := color.New(color.FgHiCyan, color.Bold)
		borderColor := color.New(color.FgCyan)
		labelColor := color.New(color.FgYellow)

		borderColor.Printf("+--- ")
		headerColor.Printf("DEBUG: %s", section)
		borderColor.Printf(" %s\n", strings.Repeat("-", width-13-len(section)))

		for _, item := range v {
			str := fmt.Sprintf("%v", item)
			if strings.Contains(str, ": ") {
				parts := strings.SplitN(str, ": ", 2)
				labelColor.Printf("%s: ", parts[0])
				fmt.Printf("%s\n", parts[1])
			} else {
				fmt.Printf("%s\n", str)
			}
		}
		fmt.Println()
	}
}

func (app *Application) getSystemPrompt() (string, error) {
	if app.agent.SystemPrompt != "" {
		return app.processSystemPrompt(app.agent.SystemPrompt)
	}
	return app.processSystemPrompt(systemPrompt)
}

func (app *Application) processSystemPrompt(prompt string) (string, error) {
	return processShellBlocks(prompt)
}
