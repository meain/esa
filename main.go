package main

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/fatih/color"
	"github.com/sashabaranov/go-openai"
)

const systemPrompt = `You are Esa, a professional assistant capable of performing various tasks. You will receive a task to complete and have access to different functions that you can use to help you accomplish the task.

When responding to tasks:
1. Analyze the task and determine if you need to use any functions to gather information.
2. If needed, make function calls to gather necessary information.
3. Process the information and formulate your response.
4. Provide only concise responses that directly address the task.

Other information:
- Date: {{date '+%Y-%m-%d %A'}}
- OS: {{uname}}
- Current directory: {{pwd}}

Remember to keep your responses brief and to the point. Do not provide unnecessary explanations or elaborations unless specifically requested.`

type FunctionConfig struct {
	Name        string            `toml:"name"`
	Description string            `toml:"description"`
	Command     string            `toml:"command"`
	Parameters  []ParameterConfig `toml:"parameters"`
	Safe        bool              `toml:"safe"`
	Stdin       string            `toml:"stdin,omitempty"`
}

type ParameterConfig struct {
	Name        string `toml:"name"`
	Type        string `toml:"type"`
	Description string `toml:"description"`
	Required    bool   `toml:"required"`
}

type Config struct {
	Functions    []FunctionConfig `toml:"functions"`
	Ask          string           `toml:"ask"`
	SystemPrompt string           `toml:"system_prompt"`
}

type ConversationMessage struct {
	Role      string            `json:"role"`
	Content   string            `json:"content"`
	ToolCalls []openai.ToolCall `json:"tool_calls,omitempty"`
	Name      string            `json:"name,omitempty"`
}

func expandHomePath(path string) string {
	if strings.HasPrefix(path, "~") {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(homeDir, path[2:])
	}
	return path
}

func loadConfig(configPath string) (Config, error) {
	var config Config
	_, err := toml.DecodeFile(expandHomePath(configPath), &config)
	return config, err
}

func convertToOpenAIFunction(fc FunctionConfig) openai.FunctionDefinition {
	properties := make(map[string]interface{})
	required := []string{}

	for _, param := range fc.Parameters {
		properties[param.Name] = map[string]string{
			"type":        param.Type,
			"description": param.Description,
		}
		if param.Required {
			required = append(required, param.Name)
		}
	}

	return openai.FunctionDefinition{
		Name:        fc.Name,
		Description: fc.Description,
		Parameters: map[string]interface{}{
			"type":       "object",
			"properties": properties,
			"required":   required,
		},
	}
}

func getEnvWithFallback(primary, fallback string) string {
	if value, exists := os.LookupEnv(primary); exists {
		return value
	}
	return os.Getenv(fallback)
}

func main() {
	debugMode := flag.Bool("debug", false, "Enable debug mode")
	continueChat := flag.Bool("c", false, "Continue last conversation")
	flag.BoolVar(continueChat, "continue", false, "Continue last conversation")
	configPathFromCLI := flag.String("config", "~/.config/esa/config.toml", "Path to the config file")
	ask := flag.String("ask", "none", "Ask level (none, unsafe, all)")
	showCommands := flag.Bool("show-commands", false, "Show executed commands")
	help := flag.Bool("help", false, "Show help message")
	flag.Parse()

	args := flag.Args()
	if *help {
		fmt.Println("Usage: esa <command> [--debug] [--config <path>] [--ask <level>]")
		fmt.Println("\nCommands:")
		fmt.Println("  list-functions    List all available functions")
		fmt.Println("  <text>            Send text command to the assistant")
		os.Exit(1)
	}

	// Handle list-functions command
	// Get cache directory
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		log.Fatalf("Error getting cache directory: %v", err)
	}
	esaDir := filepath.Join(cacheDir, "esa")
	if err := os.MkdirAll(esaDir, 0755); err != nil {
		log.Fatalf("Error creating cache directory: %v", err)
	}

	if len(args) > 0 && args[0] == "list-functions" {
		config, err := loadConfig(*configPathFromCLI)
		if err != nil {
			log.Fatalf("Error loading config: %v", err)
		}

		fmt.Println("Available Functions:")
		fmt.Println()

		for _, fn := range config.Functions {
			fmt.Printf("%s\n", fn.Name)
			fmt.Printf("  %s\n", fn.Description)

			if len(fn.Parameters) > 0 {
				for _, p := range fn.Parameters {
					required := ""
					if p.Required {
						required = " (required)"
					}
					fmt.Printf("  • %s: %s%s\n", p.Name, p.Description, required)
				}
			}
			fmt.Println()
		}
		os.Exit(0)
	}

	var (
		configPath string
		agentName  = ""
		commandStr = strings.Join(args, " ")
	)

	if strings.HasPrefix(commandStr, "+") {
		parts := strings.SplitN(commandStr, " ", 2)
		if len(parts) < 2 {
			fmt.Println("Usage: esa +<config> <command>")
			os.Exit(1)
		}

		agentName = parts[0][1:]
		commandStr = parts[1]
		configPath = fmt.Sprintf("~/.config/esa/%s.toml", agentName)
	} else {
		configPath = *configPathFromCLI
		commandStr = strings.Join(args, " ")
	}

	// Add debug print function
	debugPrint := func(section string, v ...interface{}) {
		if *debugMode {
			width := 80
			headerColor := color.New(color.FgHiCyan, color.Bold)
			borderColor := color.New(color.FgCyan)
			labelColor := color.New(color.FgYellow)

			// Print top border with section
			borderColor.Printf("+--- ")
			headerColor.Printf("DEBUG: %s", section)
			borderColor.Printf(" %s\n", strings.Repeat("-", width-13-len(section)))

			// Print content with proper formatting
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

	var (
		config Config
	)

	// Load configuration
	if agentName == "new" {
		// Load embedded new agent config
		if _, err := toml.DecodeReader(bytes.NewReader([]byte(newAgentToml)), &config); err != nil {
			log.Fatalf("Error loading embedded new agent config: %v", err)
		}
	} else {
		config, err = loadConfig(configPath)
		if err != nil {
			log.Fatalf("Error loading config: %v", err)
		}
	}

	// Override ask level if provided via flag
	if *ask != "none" {
		config.Ask = *ask
	}

	// Initialize OpenAI client with configuration from environment
	apiKey := getEnvWithFallback("ESA_API_KEY", "OPENAI_API_KEY")
	baseURL := getEnvWithFallback("ESA_BASE_URL", "OPENAI_BASE_URL")
	model := getEnvWithFallback("ESA_MODEL", "OPENAI_MODEL")

	debugPrint("Configuration",
		fmt.Sprintf("Config Path: %s", configPath),
		fmt.Sprintf("Model: %s", model),
		fmt.Sprintf("Base URL: %s", baseURL),
		fmt.Sprintf("Ask Level: %s", config.Ask))

	if len(model) == 0 {
		model = "gpt-4o-mini"
	}

	llmConfig := openai.DefaultConfig(apiKey)
	if len(baseURL) > 0 {
		llmConfig.BaseURL = baseURL
	}

	client := openai.NewClientWithConfig(llmConfig)

	// Convert function configs to OpenAI function definitions
	var openAITools []openai.Tool
	for _, fc := range config.Functions {
		function := convertToOpenAIFunction(fc)
		openAITools = append(
			openAITools,
			openai.Tool{
				Type:     openai.ToolTypeFunction,
				Function: &function,
			})
	}

	// Initialize conversation history
	systemMessage := systemPrompt
	if config.SystemPrompt != "" {
		systemMessage = config.SystemPrompt
	}

	// System message with contain {{}} placeholders to be replaced
	// with the output of running these shell commands
	blocksRegex := regexp.MustCompile(`{{\$(.*?)}}`)
	systemMessage = blocksRegex.ReplaceAllStringFunc(systemMessage, func(match string) string {
		command := match[2 : len(match)-2]
		cmd := exec.Command("sh", "-c", command)
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Sprintf("Error: %v", err)
		}
		return strings.TrimSpace(string(output))
	})

	var messages []openai.ChatCompletionMessage

	// Generate cache key based on config path
	cacheKey := fmt.Sprintf("%x", md5.Sum([]byte(configPath)))
	historyFile := filepath.Join(esaDir, cacheKey+".json")

	if *continueChat {
		// Try to load previous conversation
		if data, err := os.ReadFile(historyFile); err == nil {
			if err := json.Unmarshal(data, &messages); err == nil {
				debugPrint("History", fmt.Sprintf("Loaded %d previous messages", len(messages)))
			}
		}
	}

	// Initialize new conversation if no history or not continuing
	if len(messages) == 0 {
		messages = []openai.ChatCompletionMessage{{
			Role:    "system",
			Content: systemMessage,
		}}
	}

	input := readStdin()
	if len(input) > 0 {
		messages = append(messages, openai.ChatCompletionMessage{
			Role:    "user",
			Content: input,
		})
	}

	if len(commandStr) != 0 {
		messages = append(messages, openai.ChatCompletionMessage{
			Role:    "user",
			Content: commandStr,
		})
	}

	debugPrint("Input",
		fmt.Sprintf("Command: %s", commandStr),
		fmt.Sprintf("Stdin: %s", input))

	// Main conversation loop
	for {
		// Create streaming chat completion request
		stream, err := client.CreateChatCompletionStream(
			context.Background(),
			openai.ChatCompletionRequest{
				Model:      model,
				Messages:   messages,
				Tools:      openAITools,
				ToolChoice: "auto",
			})

		if err != nil {
			log.Fatalf("ChatCompletionStream error: %v", err)
		}
		defer stream.Close()

		var assistantMsg openai.ChatCompletionMessage
		var fullContent strings.Builder

		// Stream the response
		hasContent := false
		for {
			response, err := stream.Recv()
			if err == io.EOF {
				break
			}
			if err != nil {
				log.Fatalf("Stream error: %v", err)
			}

			// Some providers be crazy (looking at you, GitHub Models)
			if len(response.Choices) == 0 {
				continue
			}

			if response.Choices[0].Delta.ToolCalls != nil {
				// Accumulate tool calls
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
				// Print content as it arrives
				content := response.Choices[0].Delta.Content
				if content != "" {
					hasContent = true
					fmt.Print(content)
					fullContent.WriteString(content)
				}
			}
		}

		if hasContent {
			fmt.Println() // New line after streaming completes
		}

		debugPrint("Assistant Response",
			fmt.Sprintf("Content: %s", fullContent.String()),
			fmt.Sprintf("Tool Calls: %+v", assistantMsg.ToolCalls))

		// Construct final message for history
		assistantMsg.Role = "assistant"
		assistantMsg.Content = fullContent.String()
		messages = append(messages, assistantMsg)

		// If no tool calls are made, we're done
		if len(assistantMsg.ToolCalls) == 0 {
			// Save conversation history before exiting
			if data, err := json.Marshal(messages); err == nil {
				if err := os.WriteFile(historyFile, data, 0644); err != nil {
					debugPrint("Error", fmt.Sprintf("Failed to save history: %v", err))
				}
			}
			break
		}

		// Process each tool call
		for _, toolCall := range assistantMsg.ToolCalls {
			if toolCall.Type != "function" {
				continue
			}

			if toolCall.Function.Name == "" {
				continue
			}

			// Find the corresponding function config
			var matchedFunc FunctionConfig
			for _, fc := range config.Functions {
				if fc.Name == toolCall.Function.Name {
					matchedFunc = fc
					break
				}
			}

			if matchedFunc.Name == "" {
				log.Fatalf("No matching function found for: %s", toolCall.Function.Name)
			}

			// Execute the function
			command, result, err := executeFunction(config.Ask, matchedFunc, toolCall.Function.Arguments, *showCommands)

			debugPrint("Function Execution",
				fmt.Sprintf("Function: %s", matchedFunc.Name),
				fmt.Sprintf("Command: %s", command),
				fmt.Sprintf("Output: %s", result))
			if err != nil {
				debugPrint("Function Error", err)
			}

			if err != nil {
				// Add error message to conversation
				messages = append(messages, openai.ChatCompletionMessage{
					Role:       "tool",
					Name:       toolCall.Function.Name,
					Content:    fmt.Sprintf("Error: %v", err),
					ToolCallID: toolCall.ID,
				})
				continue
			}

			// Add function result to conversation
			messages = append(messages, openai.ChatCompletionMessage{
				Role:       "tool",
				Name:       toolCall.Function.Name,
				Content:    result,
				ToolCallID: toolCall.ID,
			})
		}
	}
}

func confirm(prompt string) bool {
	var response string
	fmt.Printf("%s (y/n): ", prompt)
	fmt.Scanln(&response)
	return strings.ToLower(response) == "y"
}

func executeFunction(askLevel string, fc FunctionConfig, args string, showCommands bool) (string, string, error) {
	// Set defaults
	if askLevel == "" {
		askLevel = "unsafe"
	}

	// Parse the JSON arguments
	var parsedArgs map[string]interface{}
	if args != "" {
		if err := json.Unmarshal([]byte(args), &parsedArgs); err != nil {
			return "", "", fmt.Errorf("error parsing arguments: %v", err)
		}
	}

	// Validate required parameters
	var missingParams []string
	for _, param := range fc.Parameters {
		if param.Required {
			if value, exists := parsedArgs[param.Name]; !exists || value == nil {
				missingParams = append(missingParams, param.Name)
			}
		}
	}

	if len(missingParams) > 0 {
		return "", "", fmt.Errorf("missing required parameters: %s", strings.Join(missingParams, ", "))
	}

	// Prepare the command by replacing placeholders
	command := fc.Command
	// Replace parameters with their values, using empty space for missing optional parameters
	for _, param := range fc.Parameters {
		placeholder := fmt.Sprintf("{{%s}}", param.Name)
		if value, exists := parsedArgs[param.Name]; exists {
			command = strings.ReplaceAll(command, placeholder, fmt.Sprintf("%v", value))
		} else if !param.Required {
			command = strings.ReplaceAll(command, placeholder, "")
		}
	}
	// Clean up any extra spaces from removed optional parameters
	command = strings.Join(strings.Fields(command), " ")

	origCommand := command

	// Expand any home path in the command
	command = expandHomePath(command)

	// Check if confirmation is needed
	if askLevel == "all" || (askLevel == "unsafe" && !fc.Safe) {
		if !confirm(fmt.Sprintf("Execute '%s'?", command)) {
			return command, "Command execution cancelled by user.", nil
		}
	}

	if showCommands {
		fmt.Fprintf(os.Stderr, "Command: %s\n", command)
	}

	// Execute the command
	cmd := exec.Command("sh", "-c", command)

	// Handle stdin if specified
	if fc.Stdin != "" {
		stdinContent := fc.Stdin
		// Replace all {{variable}} placeholders with their values
		for key, value := range parsedArgs {
			placeholder := fmt.Sprintf("{{%s}}", key)
			stdinContent = strings.ReplaceAll(stdinContent, placeholder, fmt.Sprintf("%v", value))
		}
		cmd.Stdin = strings.NewReader(stdinContent)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return command, "", fmt.Errorf("%v\nCommand: %s\nOutput: %s", err, command, output)
	}

	return origCommand, strings.TrimSpace(string(output)), nil
}

func readStdin() string {
	var input bytes.Buffer
	// Check if stdin is not a terminal and has data
	stat, err := os.Stdin.Stat()
	if err == nil && (stat.Mode()&os.ModeCharDevice) == 0 {
		if _, err := io.Copy(&input, os.Stdin); err != nil {
			return ""
		}
	}
	return input.String()
}
