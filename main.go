package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/sashabaranov/go-openai"
)

const systemPrompt = `You are a professional assistant capable of performing various tasks. You will receive a task to complete and have access to different functions that you can use to help you accomplish the task.

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
}

type ParameterConfig struct {
	Name        string `toml:"name"`
	Type        string `toml:"type"`
	Description string `toml:"description"`
	Required    bool   `toml:"required"`
}

type Config struct {
	Functions []FunctionConfig `toml:"functions"`
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

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: esa <command>")
		os.Exit(1)
	}

	// Load configuration
	configPath := "~/.config/esa/config.toml"
	config, err := loadConfig(configPath)
	if err != nil {
		log.Fatalf("Error loading config: %v", err)
	}

	fullCommand := os.Args[1:]
	commandStr := String(fullCommand)

	// Initialize OpenAI client with configuration from environment
	llmConfig := openai.DefaultConfig(os.Getenv("OPENAI_API_KEY"))

	// Override base URL if specified
	if baseURL := os.Getenv("OPENAI_BASE_URL"); baseURL != "" {
		llmConfig.BaseURL = baseURL
	}

	// Set model with default fallback
	model := os.Getenv("OPENAI_MODEL")
	if model == "" {
		model = "gpt-4o-mini"
	}

	client := openai.NewClientWithConfig(llmConfig)

	// Convert function configs to OpenAI function definitions
	var openAIFunctions []openai.FunctionDefinition
	for _, fc := range config.Functions {
		openAIFunctions = append(openAIFunctions, convertToOpenAIFunction(fc))
	}

	// Initialize conversation history
	messages := []openai.ChatCompletionMessage{
		{
			Role:    "system",
			Content: systemPrompt,
		},
		{
			Role:    "user",
			Content: commandStr,
		},
	}

	// Main conversation loop
	for {
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

		// Get the assistant's response
		assistantMsg := resp.Choices[0].Message

		// Add assistant's response to conversation history
		messages = append(messages, assistantMsg)

		// If no function call is made, we're done
		if assistantMsg.FunctionCall == nil {
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

		// Execute the function
		command, result, err := executeFunction(matchedFunc, assistantMsg.FunctionCall.Arguments)
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

func executeFunction(fc FunctionConfig, args string) (string, string, error) {
	// Parse the JSON arguments
	var parsedArgs map[string]interface{}
	if err := json.Unmarshal([]byte(args), &parsedArgs); err != nil {
		return "", "", fmt.Errorf("error parsing arguments: %v", err)
	}

	// Prepare the command by replacing placeholders
	command := fc.Command
	for key, value := range parsedArgs {
		placeholder := fmt.Sprintf("{{%s}}", key)
		command = strings.ReplaceAll(command, placeholder, fmt.Sprintf("%v", value))
	}

	origCommand := command

	// Expand any home path in the command
	command = expandHomePath(command)

	// Execute the command
	cmd := exec.Command("sh", "-c", command)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", "", fmt.Errorf("%v\nCommand: %s\nOutput: %s", err, command, output)
	}

	return origCommand, strings.TrimSpace(string(output)), nil
}

func String(args []string) string {
	return strings.Join(args, " ")
}
