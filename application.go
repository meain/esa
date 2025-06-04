package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/sashabaranov/go-openai"
)

const historyTimeFormat = "20060102-150405"

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
}

// providerInfo contains provider-specific configuration
type providerInfo struct {
	baseURL     string
	apiKeyEnvar string
}

// parseModel parses model string in format "provider/model" and returns provider, model name, base URL and API key environment variable
func (app *Application) parseModel() (provider string, model string, info providerInfo) {
	modelStr := app.modelFlag
	if modelStr == "" && app.agent.DefaultModel != "" {
		modelStr = app.agent.DefaultModel
	}
	if modelStr == "" && app.config != nil {
		modelStr = app.config.Settings.DefaultModel
	}

	// Use a reasonable default model
	if modelStr == "" {
		modelStr = "openai/gpt-4o-mini"
	}

	// Check if the model string is an alias
	if app.config != nil {
		if aliasedModel, ok := app.config.ModelAliases[modelStr]; ok {
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
	}

	// Override with config if present
	if app.config != nil {
		if providerCfg, ok := app.config.Providers[provider]; ok {
			// Only override non-empty values
			if providerCfg.BaseURL != "" {
				info.baseURL = providerCfg.BaseURL
			}
			if providerCfg.APIKeyEnvar != "" {
				info.apiKeyEnvar = providerCfg.APIKeyEnvar
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

	client, err := setupOpenAIClient(opts, config)
	if err != nil {
		return nil, fmt.Errorf("failed to setup OpenAI client: %v", err)
	}

	showCommands := opts.ShowCommands || config.Settings.ShowCommands
	app := &Application{
		agent:       agent,
		agentPath:   opts.AgentPath,
		client:      client,
		historyFile: historyFile,
		messages:    messages,
		modelFlag:   opts.Model,
		config:      config,

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

		command, result, err := executeFunction(
			app.agent.Ask,
			matchedFunc,
			toolCall.Function.Arguments,
			app.showCommands,
		)
		app.debugPrint("Function Execution",
			fmt.Sprintf("Function: %s", matchedFunc.Name),
			fmt.Sprintf("Command: %s", command),
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

func setupOpenAIClient(opts *CLIOptions, config *Config) (*openai.Client, error) {
	// Get provider info
	app := &Application{config: config, modelFlag: opts.Model} // Temporary app instance just for parsing model
	_, _, info := app.parseModel()

	configuredAPIKey := os.Getenv(info.apiKeyEnvar)
	// Key name can be empty if we don't need any keys
	if info.apiKeyEnvar != "" && configuredAPIKey == "" {
		return nil, fmt.Errorf(info.apiKeyEnvar + " env not found")
	}

	llmConfig := openai.DefaultConfig(configuredAPIKey)
	llmConfig.BaseURL = info.baseURL

	return openai.NewClientWithConfig(llmConfig), nil
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

func createNewHistoryFile(cacheDir string, agentName string) string {
	if agentName == "" {
		agentName = "default"
	}
	timestamp := time.Now().Format(historyTimeFormat)
	return filepath.Join(cacheDir, fmt.Sprintf("%s-%s.json", agentName, timestamp))
}

func findLatestHistoryFile(cacheDir string) (string, error) {
	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		return "", err
	}

	var latestFile string
	var latestTime time.Time

	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".json") {
			info, err := entry.Info()
			if err != nil {
				continue
			}
			if latestFile == "" || info.ModTime().After(latestTime) {
				latestFile = entry.Name()
				latestTime = info.ModTime()
			}
		}
	}

	if latestFile == "" {
		return "", fmt.Errorf("no history files found")
	}
	return filepath.Join(cacheDir, latestFile), nil
}

func getHistoryFilePath(cacheDir string, opts *CLIOptions) (string, bool) {
	if !opts.ContinueChat && !opts.RetryChat {
		cacheDir, err := setupCacheDir()
		if err != nil {
			// Handle error appropriately, maybe log or return an error
			// For now, fallback or panic might occur depending on createNewHistoryFile
			fmt.Fprintf(os.Stderr, "Warning: could not get cache dir: %v\n", err)
		}
		return createNewHistoryFile(cacheDir, opts.AgentName), false
	}

	if latestFile, err := findLatestHistoryFile(cacheDir); err == nil {
		return latestFile, true
	}

	cacheDir, err := setupCacheDir()
	if err != nil {
		// Handle error appropriately
		fmt.Fprintf(os.Stderr, "Warning: could not get cache dir: %v\n", err)
	}
	return createNewHistoryFile(cacheDir, opts.AgentName), false
}

func readStdin() string {
	var input bytes.Buffer
	stat, err := os.Stdin.Stat()
	if err == nil && (stat.Mode()&os.ModeCharDevice) == 0 {
		if _, err := io.Copy(&input, os.Stdin); err != nil {
			return ""
		}
	}
	return input.String()
}
