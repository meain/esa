package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/sashabaranov/go-openai"
)

const (
	historyTimeFormat    = "20060102-150405"
	defaultModel         = "openai/gpt-4o-mini"
	toolCallCommandColor      = color.FgCyan
	toolCallOutputColor       = color.FgWhite
	toolCallErrorCommandColor = color.FgRed
	maxRetryCount        = 5
	baseRetryDelay       = 1 * time.Second
	maxRetryDelay        = 1 * time.Minute
)

// Common error messages
const (
	errFailedToLoadConfig    = "failed to load global config"
	errFailedToSetupCache    = "failed to setup cache directory"
	errFailedToLoadHistory   = "failed to load conversation history"
	errFailedToUnmarshalHist = "failed to unmarshal conversation history"
	errFailedToLoadAgent     = "failed to load agent configuration"
	errFailedToSetupClient   = "failed to setup OpenAI client"
)

type Application struct {
	agent           Agent
	agentPath       string
	client          LLMClient
	debug           bool
	historyFile     string
	messages        []openai.ChatCompletionMessage
	debugPrint      func(section string, v ...any)
	showCommands    bool
	showToolCalls   bool
	showProgress    bool
	lastProgressLen int
	modelFlag       string
	config          *Config
	cliAskLevel     string
	prettyOutput    bool
	startTime       time.Time
	maxTurns        int
}

// providerInfo contains provider-specific configuration
type providerInfo struct {
	baseURL           string
	apiKeyEnvar       string
	apiKeyCanBeEmpty  bool
	additionalHeaders map[string]string
}

// parseModel parses model string in format "provider/model" and
// returns provider, model name, base URL and API key environment
// variable
func (app *Application) parseModel() (provider string, model string, info providerInfo) {
	return parseModel(app.modelFlag, app.agent, app.config)
}

// isRateLimitError checks if the error is a rate limit error (429)
func isRateLimitError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "429") ||
		strings.Contains(errStr, "Too Many Requests") ||
		strings.Contains(errStr, "rate limit")
}

// createChatCompletionWithRetry creates a chat completion stream with retry logic for rate limiting
func (app *Application) createChatCompletionWithRetry(tools []openai.Tool) (LLMStream, error) {
	var stream LLMStream
	var err error

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	_ = ctx // context threaded through client when supported
	defer cancel()

	// Retry logic for rate limiting
	for attempt := 0; attempt <= maxRetryCount; attempt++ {
		stream, err = app.client.CreateChatCompletionStream(
			app.getModel(),
			app.messages,
			tools,
		)

		if err == nil {
			return stream, nil // Success
		}

		if !isRateLimitError(err) {
			// Not a rate limit error, return immediately
			return nil, err
		}

		if attempt == maxRetryCount {
			// Last attempt failed
			return nil, fmt.Errorf("ChatCompletionStream error after %d retries: %w", maxRetryCount, err)
		}

		// Calculate delay and wait
		delay := calculateRetryDelay(attempt)
		app.debugPrint("Rate Limit",
			fmt.Sprintf("Rate limit hit, retrying in %v (attempt %d/%d)", delay, attempt+1, maxRetryCount))

		time.Sleep(delay)
	}

	return nil, err // Should never reach here, but for safety
}

// prepareRetryMessages prepares messages for retry mode by keeping all messages
// up to the last user message, optionally replacing its content.
func prepareRetryMessages(allMessages []openai.ChatCompletionMessage, commandStr string) []openai.ChatCompletionMessage {
	if len(allMessages) == 0 {
		return nil
	}

	// Find the last user message in the history
	lastUserIdx := -1
	for i := len(allMessages) - 1; i >= 0; i-- {
		if allMessages[i].Role == "user" {
			lastUserIdx = i
			break
		}
	}

	if lastUserIdx < 0 {
		// No user message found, just use the system message
		return []openai.ChatCompletionMessage{allMessages[0]}
	}

	// Keep all messages up to and including the last user message
	messages := allMessages[:lastUserIdx+1]
	if commandStr != "" {
		messages[lastUserIdx].Content = commandStr
	}
	return messages
}

// loadHistoryMessages loads and processes messages from conversation history.
// Returns the messages, and updates opts with agent path and model from history.
func loadHistoryMessages(opts *CLIOptions, historyFile string, debugPrint func(string, ...any)) ([]openai.ChatCompletionMessage, error) {
	data, err := os.ReadFile(historyFile)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", errFailedToLoadHistory, err)
	}

	var history ConversationHistory
	if err := json.Unmarshal(data, &history); err != nil {
		return nil, fmt.Errorf("%s: %w", errFailedToUnmarshalHist, err)
	}

	var messages []openai.ChatCompletionMessage
	if opts.RetryChat && len(history.Messages) > 1 {
		messages = prepareRetryMessages(history.Messages, opts.CommandStr)
		debugPrint("Retry Mode",
			fmt.Sprintf("Keeping %d messages", len(messages)),
			fmt.Sprintf("Agent: %s", history.AgentPath),
			fmt.Sprintf("History file: %q", historyFile),
		)
	} else {
		messages = history.Messages
		debugPrint("History",
			fmt.Sprintf("Loaded %d messages from history", len(messages)),
			fmt.Sprintf("Agent: %s", history.AgentPath),
			fmt.Sprintf("History file: %q", historyFile),
		)
	}

	if history.AgentPath != "" && opts.AgentPath == "" {
		opts.AgentPath = history.AgentPath
	}
	if history.Model != "" && opts.Model == "" {
		opts.Model = history.Model
	}

	return messages, nil
}

func NewApplication(opts *CLIOptions) (*Application, error) {
	// Load global config first
	config, err := LoadConfig(opts.ConfigPath)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", errFailedToLoadConfig, err)
	}

	cacheDir, err := setupCacheDir()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", errFailedToSetupCache, err)
	}

	var messages []openai.ChatCompletionMessage

	// If conversation index is set without retry, also set continue chat
	if len(opts.Conversation) > 0 && !opts.RetryChat {
		if _, err := findHistoryFile(cacheDir, opts.Conversation); err == nil {
			opts.ContinueChat = true
		}
	}

	if opts.ContinueChat || opts.RetryChat {
		if opts.Conversation == "" {
			opts.Conversation = "1"
		}
	}

	historyFile, hasHistory := getHistoryFilePath(cacheDir, opts)
	if hasHistory && (opts.ContinueChat || opts.RetryChat) {
		debugPrint := createDebugPrinter(opts.DebugMode)
		messages, err = loadHistoryMessages(opts, historyFile, debugPrint)
		if err != nil {
			return nil, err
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
		return nil, fmt.Errorf("%s: %w", errFailedToLoadAgent, err)
	}

	// If SystemPrompt is set in CLI options, override agent's SystemPrompt
	if opts.SystemPrompt != "" {
		agent.SystemPrompt = opts.SystemPrompt
	}

	client, err := setupLLMClient(opts.Model, agent, config)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", errFailedToSetupClient, err)
	}

	showCommands := opts.ShowCommands || config.Settings.ShowCommands
	showToolCalls := opts.ShowToolCalls || config.Settings.ShowToolCalls

	app := &Application{
		agent:        agent,
		agentPath:    opts.AgentPath,
		client:       client,
		historyFile:  historyFile,
		messages:     messages,
		modelFlag:    opts.Model,
		config:       config,
		cliAskLevel:  opts.AskLevel,
		prettyOutput: opts.Pretty,
		startTime:    time.Now(),
		maxTurns:     resolveMaxTurns(opts.MaxTurns, config.Settings.MaxTurns),

		debug:         opts.DebugMode,
		showCommands:  showCommands && !showToolCalls && !opts.DebugMode,
		showToolCalls: showToolCalls && !opts.DebugMode,
		showProgress:  !opts.HideProgress && !opts.DebugMode && !(showCommands || showToolCalls),
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
		fmt.Sprintf("Show commands: %v", app.showCommands),
		fmt.Sprintf("Show tool calls: %v", app.showToolCalls),
		fmt.Sprintf("Show progress: %v", app.showProgress),
	)

	return app, nil
}

// initializeRuntime sets up the system prompt.
// Returns a cleanup function that should be deferred by the caller.
func (app *Application) initializeRuntime() (cleanup func(), err error) {
	cleanup = func() {}

	prompt, err := app.getSystemPrompt()
	if err != nil {
		return cleanup, fmt.Errorf("error processing system prompt: %w", err)
	}

	if app.messages == nil {
		app.messages = []openai.ChatCompletionMessage{{
			Role:    "system",
			Content: prompt,
		}}
	}

	app.debugPrint("System Message", app.messages[0].Content)
	return cleanup, nil
}

func (app *Application) Run(opts CLIOptions) {
	cleanup, err := app.initializeRuntime()
	if err != nil {
		log.Fatalf("%v", err)
	}
	defer cleanup()

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

// resolveMaxTurns returns the effective max turns: CLI flag takes priority over
// config, 0 means unlimited.
func resolveMaxTurns(cliFlag, configVal int) int {
	if cliFlag > 0 {
		return cliFlag
	}
	return configVal
}

func (app *Application) runConversationLoop(opts CLIOptions) {
	openAITools := convertFunctionsToTools(app.agent.Functions)
	turns := 0

	for {
		if app.maxTurns > 0 && turns >= app.maxTurns {
			app.clearProgress()
			color.New(color.FgYellow).Fprintf(os.Stderr, "Max turns (%d) reached. Use -c to continue.\n", app.maxTurns)
			break
		}

		stream, err := app.createChatCompletionWithRetry(openAITools)
		if err != nil {
			log.Fatalf("ChatCompletionStream error: %v", err)
		}

		assistantMsg := app.handleStreamResponse(stream)
		app.messages = append(app.messages, assistantMsg)
		turns++

		// Save history after each assistant response
		app.saveConversationHistory()

		if len(assistantMsg.ToolCalls) == 0 {
			break
		}

		app.handleToolCalls(assistantMsg.ToolCalls, opts)

		// Save history after processing tool calls
		app.saveConversationHistory()
	}

	app.runOnComplete()
}

// CompletionSession is the JSON payload passed to the on_complete script via stdin.
type CompletionSession struct {
	AgentPath    string                         `json:"agent_path"`
	Model        string                         `json:"model"`
	WorkDir      string                         `json:"work_dir"`
	StartTime    time.Time                      `json:"start_time"`
	EndTime      time.Time                      `json:"end_time"`
	DurationMs   int64                          `json:"duration_ms"`
	ToolCallCount int                           `json:"tool_call_count"`
	Messages     []openai.ChatCompletionMessage `json:"messages"`
}

func (app *Application) runOnComplete() {
	script := app.config.Settings.OnComplete
	if script == "" {
		return
	}

	endTime := time.Now()
	provider, model, _ := app.parseModel()
	workDir, _ := os.Getwd()

	toolCallCount := 0
	for _, msg := range app.messages {
		toolCallCount += len(msg.ToolCalls)
	}

	session := CompletionSession{
		AgentPath:     app.agentPath,
		Model:         fmt.Sprintf("%s/%s", provider, model),
		WorkDir:       workDir,
		StartTime:     app.startTime,
		EndTime:       endTime,
		DurationMs:    endTime.Sub(app.startTime).Milliseconds(),
		ToolCallCount: toolCallCount,
		Messages:      app.messages,
	}

	data, err := json.Marshal(session)
	if err != nil {
		app.debugPrint("OnComplete", fmt.Sprintf("Failed to marshal session: %v", err))
		return
	}

	script = expandHomePath(script)
	cmd := exec.Command("sh", "-c", script)
	cmd.Stdin = strings.NewReader(string(data))
	cmd.Stdout = os.Stderr // route script output to stderr to avoid polluting stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		app.debugPrint("OnComplete", fmt.Sprintf("Script error: %v", err))
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

func (app *Application) handleStreamResponse(stream LLMStream) openai.ChatCompletionMessage {
	defer stream.Close()

	var assistantMsg openai.ChatCompletionMessage
	var fullContent strings.Builder
	hasContent := false

	for {
		delta, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Fatalf("Stream error: %v", err)
		}

		if len(delta.ToolCalls) > 0 {
			for _, toolCall := range delta.ToolCalls {
				if toolCall.ID != "" {
					assistantMsg.ToolCalls = append(assistantMsg.ToolCalls, toolCall)
				} else {
					lastToolCall := assistantMsg.ToolCalls[len(assistantMsg.ToolCalls)-1]
					lastToolCall.Function.Arguments += toolCall.Function.Arguments
					assistantMsg.ToolCalls[len(assistantMsg.ToolCalls)-1] = lastToolCall
				}
			}
		} else {
			app.clearProgress()

			if delta.Content != "" {
				hasContent = true
				if !app.prettyOutput {
					fmt.Print(delta.Content)
				}
				fullContent.WriteString(delta.Content)
			}
		}
	}

	if hasContent {
		if app.prettyOutput {
			// TODO: Add support for rendering pretty markdown in a
			// streming manner (charmbracelet/glow/issues/601)
			printPrettyOutput(fullContent.String())
		} else {
			fmt.Println()
		}
	}

	assistantMsg.Role = "assistant"
	assistantMsg.Content = fullContent.String()
	return assistantMsg
}

type ConversationHistory struct {
	AgentPath string                         `json:"agent_path"`
	Model     string                         `json:"model"`
	WorkDir   string                         `json:"work_dir,omitempty"`
	Messages  []openai.ChatCompletionMessage `json:"messages"`
}

func (app *Application) saveConversationHistory() {
	provider, model, _ := app.parseModel()
	modelString := fmt.Sprintf("%s/%s", provider, model)
	workDir, _ := os.Getwd()
	history := ConversationHistory{
		AgentPath: app.agentPath,
		Model:     modelString,
		WorkDir:   workDir,
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

// clearProgress clears the progress line from stderr if one is currently displayed
func (app *Application) clearProgress() {
	if app.showProgress && app.lastProgressLen > 0 {
		fmt.Fprintf(os.Stderr, "\r%s\r", strings.Repeat(" ", app.lastProgressLen))
		app.lastProgressLen = 0
	}
}

// showToolProgress displays a progress indicator for a tool call being executed
func (app *Application) showToolProgress(funcName string, args string) {
	if !app.showProgress {
		return
	}
	summary := app.generateProgressSummary(funcName, args)
	if summary == "" {
		return
	}
	// Clear previous line if exists
	if app.lastProgressLen > 0 {
		fmt.Fprintf(os.Stderr, "\r%s\r", strings.Repeat(" ", app.lastProgressLen))
	}
	msg := fmt.Sprintf("⋮ %s", summary)
	color.New(color.FgBlue).Fprint(os.Stderr, msg)
	app.lastProgressLen = len(msg)
}

// appendToolError appends an error message for a tool call to the conversation and displays it if configured
func (app *Application) appendToolError(toolCall openai.ToolCall, err error, displayCommand string) {
	app.clearProgress()
	if displayCommand != "" && (app.showCommands || app.showToolCalls) {
		color.New(toolCallErrorCommandColor).Fprintf(os.Stderr, "%s\n", displayCommand)
	}
	if app.showToolCalls {
		color.New(toolCallErrorCommandColor).Fprintf(os.Stderr, "Error: %v\n", err)
	}
	app.messages = append(app.messages, openai.ChatCompletionMessage{
		Role:       "tool",
		Name:       toolCall.Function.Name,
		Content:    fmt.Sprintf("Error: %v", err),
		ToolCallID: toolCall.ID,
	})
}

// appendToolResult appends a tool result to the conversation and displays it if configured
func (app *Application) appendToolResult(toolCall openai.ToolCall, content string, displayCommand string, displayOutput string) {
	if app.showCommands || app.showToolCalls {
		color.New(toolCallCommandColor).Fprintf(os.Stderr, "%s\n", displayCommand)
	}
	if app.showToolCalls && displayOutput != "" {
		color.New(toolCallOutputColor).Fprintf(os.Stderr, "%s\n", displayOutput)
	}
	app.messages = append(app.messages, openai.ChatCompletionMessage{
		Role:       "tool",
		Name:       toolCall.Function.Name,
		Content:    content,
		ToolCallID: toolCall.ID,
	})
}

func (app *Application) handleToolCalls(toolCalls []openai.ToolCall, opts CLIOptions) {
	for _, toolCall := range toolCalls {
		if toolCall.Type != "function" || toolCall.Function.Name == "" {
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

		if len(matchedFunc.Output) == 0 {
			app.showToolProgress(matchedFunc.Name, toolCall.Function.Arguments)
		}

		// Set the provider and model env so that nested esa calls
		// make use of it. Users can override this by setting the
		// value explicitly in the nested esa calls.
		provider, model, _ := app.parseModel()
		os.Setenv("ESA_MODEL", fmt.Sprintf("%s/%s", provider, model))

		approved, command, stdin, result, err := executeFunction(
			app.getEffectiveAskLevel(),
			matchedFunc,
			toolCall.Function.Arguments,
		)
		app.debugPrint("Function Execution",
			fmt.Sprintf("Function: %s", matchedFunc.Name),
			fmt.Sprintf("Approved: %s", fmt.Sprint(approved)),
			fmt.Sprintf("Command: %s", command),
			fmt.Sprintf("Stdin: %s", stdin),
			fmt.Sprintf("Output: %s", result))

		if err != nil {
			app.debugPrint("Function Error", err)
			app.appendToolError(toolCall, err, fmt.Sprintf("$ %s", command))
			continue
		}

		content := fmt.Sprintf("Command: %s\n\nOutput: \n%s", command, result)
		app.appendToolResult(toolCall, content, fmt.Sprintf("$ %s", command), result)
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
