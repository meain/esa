package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/sashabaranov/go-openai"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: esa <command>")
		os.Exit(1)
	}

	fullCommand := os.Args[1:]
	commandStr := String(fullCommand)

	// Initialize OpenAI client
	client := openai.NewClient(os.Getenv("OPENAI_API_KEY"))

	// Create chat completion request with function calling
	resp, err := client.CreateChatCompletion(
		context.Background(),
		openai.ChatCompletionRequest{
			Model: "gpt-4o-mini",
			Messages: []openai.ChatCompletionMessage{
				{
					Role:    "user",
					Content: commandStr,
				},
			},
			Functions: []openai.FunctionDefinition{
				SetAlarmFunction(),
				OpenWebsiteFunction(),
				AdjustBrightnessFunction(),
				SendMessageFunction(),
			},
		})

	if err != nil {
		log.Fatalf("Chat completion error: %v", err)
	}

	// Check if a function call was suggested
	if resp.Choices[0].Message.FunctionCall != nil {
		functionCall := resp.Choices[0].Message.FunctionCall

		// Execute the appropriate function
		result, err := ExecuteFunctionCall(functionCall)
		if err != nil {
			log.Fatalf("Function execution error: %v", err)
		}

		fmt.Println("Action completed:", result)
	} else {
		fmt.Println("No specific action could be determined.")
	}
}

func String(args []string) string {
	result := ""
	for _, arg := range args {
		result += arg + " "
	}
	return result
}
