package main

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/fatih/color"
	"github.com/sashabaranov/go-openai"
)

type Application struct {
	config          Config
	client          *openai.Client
	debug           bool
	historyFile     string
	messages        []openai.ChatCompletionMessage
	debugPrint      func(section string, v ...interface{})
	showProgress    bool
	lastProgressLen int
}

func NewApplication(opts CLIOptions) (*Application, error) {
	cacheDir, err := setupCacheDir()
	if err != nil {
		return nil, fmt.Errorf("failed to setup cache directory: %v", err)
	}

	config, err := loadConfiguration(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to load configuration: %v", err)
	}

	client, err := setupOpenAIClient()
	if err != nil {
		return nil, fmt.Errorf("failed to setup OpenAI client: %v", err)
	}

	app := &Application{
		config:       config,
		client:       client,
		debug:        opts.DebugMode,
		showProgress: !opts.HideProgress && !opts.DebugMode, // Hide progress if debug mode is enabled
		historyFile:  filepath.Join(cacheDir, fmt.Sprintf("%x.json", md5.Sum([]byte(opts.ConfigPath)))),
	}

	app.debugPrint = createDebugPrinter(app.debug)
	return app, nil
}

func (app *Application) Run(opts CLIOptions) {
	app.loadConversationHistory(opts.ContinueChat)
	app.processInput(opts.CommandStr)
	app.runConversationLoop(opts)
}

func (app *Application) loadConversationHistory(continueChat bool) {
	if continueChat {
		if data, err := os.ReadFile(app.historyFile); err == nil {
			if err := json.Unmarshal(data, &app.messages); err == nil {
				app.debugPrint("History", fmt.Sprintf("Loaded %d previous messages", len(app.messages)))
				return
			}
		}
	}

	app.messages = []openai.ChatCompletionMessage{{
		Role:    "system",
		Content: app.getSystemPrompt(),
	}}
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
}

func (app *Application) runConversationLoop(opts CLIOptions) {
	openAITools := convertFunctionsToTools(app.config.Functions)

	for {
		stream, err := app.client.CreateChatCompletionStream(
			context.Background(),
			openai.ChatCompletionRequest{
				Model:      getEnvWithFallback("ESA_MODEL", "OPENAI_MODEL"),
				Messages:   app.messages,
				Tools:      openAITools,
				ToolChoice: "auto",
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

func (app *Application) saveConversationHistory() {
	if data, err := json.Marshal(app.messages); err == nil {
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
			Model: getEnvWithFallback("ESA_MODEL", "OPENAI_MODEL"),
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

func setupOpenAIClient() (*openai.Client, error) {
	apiKey := getEnvWithFallback("ESA_API_KEY", "OPENAI_API_KEY")
	baseURL := getEnvWithFallback("ESA_BASE_URL", "OPENAI_BASE_URL")

	if apiKey == "" {
		return nil, fmt.Errorf("API key not found in environment variables")
	}

	llmConfig := openai.DefaultConfig(apiKey)
	if baseURL != "" {
		llmConfig.BaseURL = baseURL
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
		cmd := exec.Command("sh", "-c", command)
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Sprintf("Error: %v", err)
		}
		return strings.TrimSpace(string(output))
	})
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
