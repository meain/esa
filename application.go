package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/sashabaranov/go-openai"
)

const historyTimeFormat = "20060102-150405"

type Application struct {
	config          Config
	configPath      string
	client          *openai.Client
	debug           bool
	historyFile     string
	messages        []openai.ChatCompletionMessage
	debugPrint      func(section string, v ...interface{})
	showProgress    bool
	lastProgressLen int
	modelFlag       string
}

// providerInfo contains provider-specific configuration
type providerInfo struct {
	baseURL     string
	apiKeyEnvar string
}

// parseModel parses model string in format "provider/model" and returns provider, model name, base URL and API key environment variable
func parseModel(modelStr string) (provider string, model string, info providerInfo) {
	parts := strings.SplitN(modelStr, "/", 2)
	if len(parts) != 2 {
		return "", modelStr, providerInfo{} // Default to just using model name if no provider specified
	}

	provider = parts[0]
	model = parts[1]

	// Map providers to their configurations
	switch provider {
	case "openai":
		info = providerInfo{
			baseURL:     "https://api.openai.com/v1",
			apiKeyEnvar: "OPENAI_API_KEY",
		}
	case "anthropic":
		info = providerInfo{
			baseURL:     "https://api.anthropic.com/v1",
			apiKeyEnvar: "ANTHROPIC_API_KEY",
		}
	case "azure":
		info = providerInfo{
			baseURL:     "https://api.azure.com/v1",
			apiKeyEnvar: "AZURE_OPENAI_API_KEY",
		}
	}

	return provider, model, info
}

func NewApplication(opts *CLIOptions) (*Application, error) {
	cacheDir, err := setupCacheDir()
	if err != nil {
		return nil, fmt.Errorf("failed to setup cache directory: %v", err)
	}

	var (
		messages   []openai.ChatCompletionMessage
		configPath string
	)

	historyFile, hasHistory := getHistoryFilePath(cacheDir, opts)
	if hasHistory && opts.ContinueChat {
		messages, configPath, err = loadConversationHistory(historyFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load conversation history: %v", err)
		}

		app := &Application{debug: opts.DebugMode}
		app.debugPrint = createDebugPrinter(app.debug)
		app.debugPrint("History",
			fmt.Sprintf("Loaded %d messages from history", len(messages)),
			fmt.Sprintf("Agent: %s", configPath),
			fmt.Sprintf("History file: %q", historyFile),
		)

		if configPath != "" && opts.ConfigPath == "" {
			opts.ConfigPath = configPath
		}
	}

	if opts.ConfigPath == "" {
		opts.ConfigPath = DefaultConfigPath
	}

	config, err := loadConfiguration(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to load configuration: %v", err)
	}

	client, err := setupOpenAIClient(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to setup OpenAI client: %v", err)
	}

	app := &Application{
		config:       config,
		configPath:   opts.ConfigPath,
		client:       client,
		debug:        opts.DebugMode,
		showProgress: !opts.HideProgress && !opts.DebugMode, // Hide progress if debug mode is enabled
		historyFile:  historyFile,
		messages:     messages,
		modelFlag:    opts.Model,
	}

	app.debugPrint = createDebugPrinter(app.debug)
	return app, nil
}

func (app *Application) Run(opts CLIOptions) {
	if app.messages == nil {
		app.messages = []openai.ChatCompletionMessage{{
			Role:    "system",
			Content: app.getSystemPrompt(),
		}}
	}

	// Debug prints before starting communication
	app.debugPrint("System Message", app.messages[0].Content)
	app.debugPrint("Input State",
		fmt.Sprintf("Command string: %q", opts.CommandStr),
		fmt.Sprintf("Stdin: %q", readStdin()),
		fmt.Sprintf("Config path: %q", opts.ConfigPath),
	)

	app.processInput(opts.CommandStr)
	app.runConversationLoop(opts)
}

func loadConversationHistory(historyFile string) ([]openai.ChatCompletionMessage, string, error) {
	data, err := os.ReadFile(historyFile)
	if err != nil {
		return nil, "", err
	}

	var history ConversationHistory

	err = json.Unmarshal(data, &history)
	if err != nil {
		return nil, "", err
	}

	return history.Messages, history.ConfigPath, nil
}

func (app *Application) processInput(commandStr string) {
	input := readStdin()
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

	// If no input from stdin or command line, use initial message from config
	if len(input) == 0 && len(commandStr) == 0 && app.config.InitialMessage != "" {
		app.messages = append(app.messages, openai.ChatCompletionMessage{
			Role:    "user",
			Content: app.processInitialMessage(app.config.InitialMessage),
		})
	}
}

func (app *Application) processInitialMessage(message string) string {
	// Use the same processing logic as system prompt
	return app.processSystemPrompt(message)
}

func (app *Application) runConversationLoop(opts CLIOptions) {
	openAITools := convertFunctionsToTools(app.config.Functions)

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
	modelStr := ""
	if app.modelFlag != "" {
		modelStr = app.modelFlag
	} else {
		modelStr = os.Getenv("ESA_MODEL")
	}
	_, model, _ := parseModel(modelStr)
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
	ConfigPath string                         `json:"config_path"`
	Messages   []openai.ChatCompletionMessage `json:"messages"`
}

func (app *Application) saveConversationHistory() {
	history := ConversationHistory{
		ConfigPath: app.configPath,
		Messages:   app.messages,
	}

	if data, err := json.Marshal(history); err == nil {
		if err := os.WriteFile(app.historyFile, data, 0644); err != nil {
			app.debugPrint("Error", fmt.Sprintf("Failed to save history: %v", err))
		}
	}
}

func (app *Application) generateProgressSummary(funcName string, args string) string {
	if !app.showProgress {
		return ""
	}

	prompt := fmt.Sprintf(`Summarize what this function is doing
Function: %s
Arguments: %s

Examples:
- Reading file main.go
- Accessing webpage from blog.meain.io
- Computing average of list of numbers
- Opening page google.com in browser
- Searching for 'improvements in AI'

Notes:
- Include names of files, URLs, or search queries
- Use present continuous tense (e.g., 'Reading file')
- Keep it concise (1 line, max 8 words)
- Do not include function name or arguments`, funcName, args)

	resp, err := app.client.CreateChatCompletion(
		context.Background(),
		openai.ChatCompletionRequest{
			Model: app.getModel(),
			Messages: []openai.ChatCompletionMessage{{
				Role:    "user",
				Content: prompt,
			}},
		},
	)

	if err != nil {
		app.debugPrint("Progress", fmt.Sprintf("Failed to generate progress summary: %v", err))
		return ""
	}

	if len(resp.Choices) > 0 {
		return resp.Choices[0].Message.Content
	}
	return ""
}

func (app *Application) handleToolCalls(toolCalls []openai.ToolCall, opts CLIOptions) {
	for _, toolCall := range toolCalls {
		if toolCall.Type != "function" || toolCall.Function.Name == "" {
			continue
		}

		var matchedFunc FunctionConfig
		for _, fc := range app.config.Functions {
			if fc.Name == toolCall.Function.Name {
				matchedFunc = fc
				break
			}
		}

		if matchedFunc.Name == "" {
			log.Fatalf("No matching function found for: %s", toolCall.Function.Name)
		}

		if app.showProgress {
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

		command, result, err := executeFunction(app.config.Ask, matchedFunc, toolCall.Function.Arguments, opts.ShowCommands)
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

		app.messages = append(app.messages, openai.ChatCompletionMessage{
			Role:       "tool",
			Name:       toolCall.Function.Name,
			Content:    result,
			ToolCallID: toolCall.ID,
		})
	}
}

func setupOpenAIClient(opts *CLIOptions) (*openai.Client, error) {
	modelStr := ""
	if opts != nil && opts.Model != "" {
		modelStr = opts.Model
	} else {
		modelStr = os.Getenv("ESA_MODEL")
	}
	configuredBaseURL := os.Getenv("ESA_BASE_URL")
	configuredAPIKey := getEnvWithFallback("ESA_API_KEY", "OPENAI_API_KEY")

	// Get provider info
	_, _, info := parseModel(modelStr)

	// If ESA_API_KEY is not set, try to use provider-specific API key
	if configuredAPIKey == "" && info.apiKeyEnvar != "" {
		configuredAPIKey = os.Getenv(info.apiKeyEnvar)
	}

	if configuredAPIKey == "" {
		return nil, fmt.Errorf("API key not found in environment variables")
	}

	llmConfig := openai.DefaultConfig(configuredAPIKey)

	// If base URL is explicitly configured, use it
	if configuredBaseURL != "" {
		llmConfig.BaseURL = configuredBaseURL
	} else if info.baseURL != "" {
		// Otherwise try to use provider-specific base URL
		llmConfig.BaseURL = info.baseURL
	}

	return openai.NewClientWithConfig(llmConfig), nil
}

func createDebugPrinter(debugMode bool) func(string, ...interface{}) {
	return func(section string, v ...interface{}) {
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

func (app *Application) getSystemPrompt() string {
	if app.config.SystemPrompt != "" {
		return app.processSystemPrompt(app.config.SystemPrompt)
	}
	return app.processSystemPrompt(systemPrompt)
}

func (app *Application) processSystemPrompt(prompt string) string {
	blocksRegex := regexp.MustCompile(`{{\$(.*?)}}`)
	return blocksRegex.ReplaceAllStringFunc(prompt, func(match string) string {
		command := match[2 : len(match)-2]
		cmd := exec.Command("sh", "-c", command[1:])
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Sprintf("Error: %v", err)
		}
		return strings.TrimSpace(string(output))
	})
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
	if !opts.ContinueChat {
		return createNewHistoryFile(cacheDir, opts.AgentName), false
	}

	if latestFile, err := findLatestHistoryFile(cacheDir); err == nil {
		return latestFile, true
	}

	return createNewHistoryFile(cacheDir, opts.AgentName), false
}

func setupCacheDir() (string, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	esaDir := filepath.Join(cacheDir, "esa")
	return esaDir, os.MkdirAll(esaDir, 0755)
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
