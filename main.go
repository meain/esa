package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/sashabaranov/go-openai"
)

const systemPrompt = `You are Esa, a professional assistant capable of performing various tasks. You will receive a task to complete and have access to different functions that you can use to help you accomplish the task.

When responding to tasks:
1. Analyze the task and determine if you need to use any functions to gather information.
2. If needed, make function calls to gather necessary information.
3. Process the information and formulate your response.
4. Provide only concise responses that directly address the task.

Remember to keep your responses brief and to the point. Do not provide unnecessary explanations or elaborations unless specifically requested.`

type FunctionConfig struct {
	Name        string            `toml:"name"`
	Description string            `toml:"description"`
	Command     string            `toml:"command"`
	Parameters  []ParameterConfig `toml:"parameters"`
	Safe        bool              `toml:"safe"`
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
	Role         string               `json:"role"`
	Content      string               `json:"content"`
	FunctionCall *openai.FunctionCall `json:"function_call,omitempty"`
	Name         string               `json:"name,omitempty"`
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
	debug := flag.Bool("debug", false, "Enable debug mode")
	configPathFromCLI := flag.String("config", "~/.config/esa/config.toml", "Path to the config file")
	ask := flag.String("ask", "none", "Ask level (none, unsafe, all)")
	showCommands := flag.Bool("show-commands", false, "Show executed commands")
	flag.Parse()

	args := flag.Args()
	if len(args) < 1 {
		fmt.Println("Usage: esa <command> [--debug] [--config <path>] [--ask <level>]")
		os.Exit(1)
	}

	var configPath string
	commandStr := strings.Join(args, " ")

	if strings.HasPrefix(commandStr, "+") {
		parts := strings.SplitN(commandStr, " ", 2)
		if len(parts) < 2 {
			fmt.Println("Usage: esa +<config> <command>")
			os.Exit(1)
		}
		configName := parts[0][1:]
		commandStr = parts[1]
		configPath = fmt.Sprintf("~/.config/esa/%s.toml", configName)
	} else {
		configPath = *configPathFromCLI
		commandStr = strings.Join(args, " ")
	}

	debugMode := *debug

	// Load configuration
	config, err := loadConfig(configPath)
	if err != nil {
		log.Fatalf("Error loading config: %v", err)
	}

	// Override ask level if provided via flag
	if *ask != "none" {
		config.Ask = *ask
	}

	// Initialize OpenAI client with configuration from environment
	apiKey := getEnvWithFallback("ESA_API_KEY", "OPENAI_API_KEY")
	baseURL := getEnvWithFallback("ESA_BASE_URL", "OPENAI_BASE_URL")
	model := getEnvWithFallback("ESA_MODEL", "OPENAI_MODEL")

	if len(model) == 0 {
		model = "gpt-4o-mini"
	}

	llmConfig := openai.DefaultConfig(apiKey)
	if len(baseURL) > 0 {
		llmConfig.BaseURL = baseURL
	}

	client := openai.NewClientWithConfig(llmConfig)

	// Convert function configs to OpenAI function definitions
	var openAIFunctions []openai.FunctionDefinition
	for _, fc := range config.Functions {
		openAIFunctions = append(openAIFunctions, convertToOpenAIFunction(fc))
	}

	// Initialize conversation history
	systemMessage := systemPrompt
	if config.SystemPrompt != "" {
		systemMessage = config.SystemPrompt
	}
	messages := []openai.ChatCompletionMessage{{
		Role:    "system",
		Content: systemMessage,
	}}

	input := readStdin()
	if len(input) > 0 {
		messages = append(messages, openai.ChatCompletionMessage{
			Role:    "user",
			Content: input,
		})
	}

	messages = append(messages, openai.ChatCompletionMessage{
		Role:    "user",
		Content: commandStr,
	})

	// Main conversation loop
	for {
		if !debugMode {
			fmt.Fprintf(os.Stderr, "\033[2K")
			fmt.Fprintf(os.Stderr, "Talking to the assistant...\r")
		}

		// Create chat completion request with function calling
		resp, err := client.CreateChatCompletion(
			context.Background(),
			openai.ChatCompletionRequest{
				Model:     model,
				Messages:  messages,
				Functions: openAIFunctions,
			})

		if err != nil {
			log.Fatalf("Chat completion error: %v", err)
		}

		// Check if we got any response
		if len(resp.Choices) == 0 {
			log.Fatal("No response choices received from the API")
		}

		// Get the assistant's response
		assistantMsg := resp.Choices[0].Message

		if debugMode {
			fmt.Println("\n--- Assistant Response ---")
			fmt.Printf("Role: %s\n", assistantMsg.Role)
			fmt.Printf("Content: %s\n", assistantMsg.Content)
			if assistantMsg.FunctionCall != nil {
				fmt.Printf("Function Call: %+v\n", assistantMsg.FunctionCall)
			}
		}

		// Add assistant's response to conversation history
		messages = append(messages, assistantMsg)

		// If no function call is made, we're done
		if assistantMsg.FunctionCall == nil {
			fmt.Fprintf(os.Stderr, "\033[2K")
			fmt.Println(assistantMsg.Content)
			break
		}

		// Find the corresponding function config
		var matchedFunc FunctionConfig
		for _, fc := range config.Functions {
			if fc.Name == assistantMsg.FunctionCall.Name {
				matchedFunc = fc
				break
			}
		}

		if matchedFunc.Name == "" {
			log.Fatalf("No matching function found for: %s", assistantMsg.FunctionCall.Name)
		}

		if !debugMode {
			fmt.Fprintf(os.Stderr, "\033[2K")
			fmt.Fprintf(os.Stderr, "Executing function: %s\r", matchedFunc.Name)
		}

		// Execute the function
		command, result, err := executeFunction(config.Ask, matchedFunc, assistantMsg.FunctionCall.Arguments, *showCommands)

		if debugMode {
			fmt.Println("\n--- Function Execution ---")
			fmt.Printf("Function: %s\n", matchedFunc.Name)
			fmt.Printf("Command: %s\n", command)
			fmt.Printf("Output: %s\n", result)
			if err != nil {
				fmt.Printf("Error: %v\n", err)
			}
		}

		if err != nil {
			// Add error message to conversation
			messages = append(messages, openai.ChatCompletionMessage{
				Role:    "function",
				Name:    assistantMsg.FunctionCall.Name,
				Content: fmt.Sprintf("Error: %v", err),
			})
			continue
		}

		// Add function result to conversation
		messages = append(messages, openai.ChatCompletionMessage{
			Role:    "function",
			Name:    assistantMsg.FunctionCall.Name,
			Content: fmt.Sprintf("Command: %s\nOutput: %s", command, result),
		})
	}
}

func confirm(prompt string) bool {
	var response string
	fmt.Printf("%s (y/n): ", prompt)
	fmt.Scanln(&response)
	return strings.ToLower(response) == "y"
}

func executeFunction(askLevel string, fc FunctionConfig, args string, showCommands bool) (string, string, error) {
	// Parse the JSON arguments
	var parsedArgs map[string]interface{}
	if err := json.Unmarshal([]byte(args), &parsedArgs); err != nil {
		return "", "", fmt.Errorf("error parsing arguments: %v", err)
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
	output, err := cmd.CombinedOutput()
	if err != nil {
		return command, "", fmt.Errorf("%v\nCommand: %s\nOutput: %s", err, command, output)
	}

	return origCommand, strings.TrimSpace(string(output)), nil
}

func readStdin() string {
	var input bytes.Buffer
	_, err := io.Copy(&input, os.Stdin)
	if err != nil {
		return ""
	}

	return input.String()
}
