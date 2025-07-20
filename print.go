package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/glamour"
	"github.com/fatih/color"
	"github.com/sashabaranov/go-openai"
)

func printFunctionInfo(fn FunctionConfig) {
	functionName := color.New(color.FgHiGreen, color.Bold).SprintFunc()
	paramName := color.New(color.FgYellow).SprintFunc()
	requiredTag := color.New(color.FgRed, color.Bold).SprintFunc()

	fmt.Printf("%s\n", functionName(fn.Name))
	fmt.Printf("%s\n", fn.Description)
	if len(fn.Parameters) > 0 {
		for _, p := range fn.Parameters {
			required := ""
			if p.Required {
				required = requiredTag(" (required)")
			}
			fmt.Printf("  • %s: %s%s\n", paramName(p.Name), p.Description, required)
		}
	}
	fmt.Println()
}

// printAgentInfo prints basic information about an agent (name and description)
// Used for listing agents in CLI commands
func printAgentInfo(agent Agent, agentName string) {
	nameStyle := color.New(color.FgHiGreen).SprintFunc()
	agentNameStyle := color.New(color.FgHiCyan, color.Bold).SprintFunc()
	noDescStyle := color.New(color.FgHiBlack, color.Italic).SprintFunc()

	// Print agent filename and name from config
	if agent.Name != "" {
		fmt.Printf("  %s (%s): ", nameStyle(agent.Name), agentNameStyle(agentName))
	} else {
		fmt.Printf("  %s: ", agentNameStyle(agentName))
	}

	// Print description
	if agent.Description != "" {
		fmt.Println(agent.Description)
	} else {
		fmt.Printf("%s\n", noDescStyle("(No description available)"))
	}
}

// printDetailedAgentInfo prints detailed information about an agent
// Used for --show-agent and /config commands
func printDetailedAgentInfo(agent Agent, agentPath string) {
	labelStyle := color.New(color.FgHiCyan, color.Bold).SprintFunc()

	// Print agent name and path
	if agent.Name != "" {
		fmt.Printf("  %s %s\n", labelStyle("Name:"), agent.Name)
	}
	if agent.Description != "" {
		fmt.Printf("  %s %s\n", labelStyle("Description:"), agent.Description)
	}
	fmt.Printf("  %s %s\n", labelStyle("Path:"), agentPath)

	if agent.DefaultModel != "" {
		fmt.Printf("  %s %s\n", labelStyle("Default Model:"), agent.DefaultModel)
	}

	fmt.Printf("  %s %d\n", labelStyle("Functions:"), len(agent.Functions))
	fmt.Printf("  %s %d\n", labelStyle("MCP Servers:"), len(agent.MCPServers))
}

// printError prints an error message with consistent formatting
func printError(msg string) {
	errorStyle := color.New(color.FgRed, color.Bold).SprintFunc()
	fmt.Printf("%s %s\n", errorStyle("[ERROR]"), msg)
}

// printWarning prints a warning message with consistent formatting
func printWarning(msg string) {
	warnStyle := color.New(color.FgYellow).SprintFunc()
	fmt.Printf("%s %s\n", warnStyle("[WARNING]"), msg)
}

// printInfo prints an informational message with consistent formatting
func printInfo(msg string) {
	infoStyle := color.New(color.FgCyan).SprintFunc()
	fmt.Printf("%s %s\n", infoStyle("[INFO]"), msg)
}

// printJSONError prints an error message in JSON format.
func printJSONError(message string) {
	errJSON, _ := json.Marshal(map[string]string{"error": message})
	fmt.Println(string(errJSON))
}

// printHistoryJSON prints the raw history data as JSON.
func printHistoryJSON(historyData []byte) {
	// Pretty print the JSON
	var prettyJSON bytes.Buffer
	if err := json.Indent(&prettyJSON, historyData, "", "  "); err == nil {
		fmt.Println(prettyJSON.String())
	} else {
		// Fallback to raw data if indent fails
		fmt.Println(string(historyData))
	}
}

// printHistoryMarkdown prints the history in Markdown format.
func printHistoryMarkdown(fileName string, history ConversationHistory) {
	fmt.Printf("# History: %s\n\n", fileName)
	if history.AgentPath != "" {
		agentName := strings.TrimSuffix(filepath.Base(history.AgentPath), ".toml")
		agentName = strings.TrimPrefix(agentName, "builtin:")
		fmt.Printf("**Agent:** +%s (`%s`)\n\n", agentName, history.AgentPath)
	}
	fmt.Println("---")

	for _, msg := range history.Messages {
		role := strings.ToUpper(msg.Role)
		fmt.Printf("## %s\n\n", role)

		if msg.Content != "" && msg.Role != openai.ChatMessageRoleTool {
			fmt.Printf("%s\n\n", msg.Content)
		}

		if len(msg.ToolCalls) > 0 {
			fmt.Println("**Tool Calls**")
			for _, tc := range msg.ToolCalls {
				fmt.Printf("\n\nTool: `%s` (ID: `%s`)\n", tc.Function.Name, tc.ID)
				var argsMap map[string]string
				argsStr := tc.Function.Arguments
				if err := json.Unmarshal([]byte(argsStr), &argsMap); err == nil {
					for key, value := range argsMap {
						fmt.Printf("\n**`%s`**", key)
						if len(value) > 100 || strings.Contains(value, "\n") {
							fmt.Printf("\n```\n%s\n```\n\n", value)
						} else {
							fmt.Printf(": %s\n\n", value)
						}
					}
				} else {
					fmt.Printf("```json\n%s\n```\n", argsStr)
				}
			}
			fmt.Println()
		}

		if msg.Role == openai.ChatMessageRoleTool {
			fmt.Printf("**Tool Result** (for call `%s`, tool `%s`):\n\n", msg.ToolCallID, msg.Name)
			// Check if content looks like JSON or code for formatting
			isError := strings.HasPrefix(msg.Content, "Error:")
			prefix := ""
			if isError {
				prefix = "**ERROR:** "
			}

			var contentMap map[string]any
			var contentSlice []any
			contentStr := msg.Content
			if err := json.Unmarshal([]byte(contentStr), &contentMap); err == nil {
				prettyJSON, _ := json.MarshalIndent(contentMap, "", "  ")
				contentStr = string(prettyJSON)
				fmt.Printf("```json\n%s%s\n```\n\n", prefix, contentStr)
			} else if err := json.Unmarshal([]byte(contentStr), &contentSlice); err == nil {
				prettyJSON, _ := json.MarshalIndent(contentSlice, "", "  ")
				contentStr = string(prettyJSON)
				fmt.Printf("```json\n%s%s\n```\n\n", prefix, contentStr)
			} else {
				// Treat as plain text or potentially other code
				lang := ""                              // Auto-detect or leave blank
				if strings.Contains(contentStr, "\n") { // Basic check for multi-line content
					fmt.Printf("```%s\n%s%s\n```\n\n", lang, prefix, contentStr)
				} else {
					fmt.Printf("%s%s\n\n", prefix, contentStr)
				}
			}
		}
	}
}

// printHistoryText prints the history in the default colored text format.
func printHistoryText(fileName string, history ConversationHistory) {
	messages := history.Messages
	agentPath := history.AgentPath
	model := history.Model

	// --- Define Styles (copied from original implementation) ---
	systemStyle := color.New(color.FgMagenta, color.Italic).SprintFunc()
	userStyle := color.New(color.FgGreen).SprintFunc()
	assistantStyle := color.New(color.FgBlue).SprintFunc()
	toolStyle := color.New(color.FgYellow).SprintFunc()
	toolDataStyle := color.New(color.FgHiBlack).SprintFunc()
	errorStyle := color.New(color.FgRed).SprintFunc()
	labelStyle := color.New(color.Bold).SprintFunc()

	// --- Print Header ---
	fmt.Printf("%s %s\n", labelStyle("History File:"), fileName)
	if agentPath != "" {
		// Try to extract agent name from path for display
		agentName := strings.TrimSuffix(filepath.Base(agentPath), ".toml")
		agentName = strings.TrimPrefix(agentName, "builtin:")
		fmt.Printf("%s +%s (%s)\n", labelStyle("Agent:"), agentName, agentPath)
	}
	if model != "" {
		fmt.Printf("%s %s\n", labelStyle("Model:"), model)
	}

	fmt.Println(strings.Repeat("-", 40)) // Separator

	// --- Print Messages (copied from original implementation) ---
	for _, msg := range messages {
		switch msg.Role {
		case openai.ChatMessageRoleSystem:
			fmt.Printf("%s\n%s\n\n", systemStyle("[SYSTEM]"), msg.Content)
		case openai.ChatMessageRoleUser:
			fmt.Printf("%s\n%s\n\n", userStyle("[USER]"), msg.Content)
		case openai.ChatMessageRoleAssistant:
			fmt.Printf("%s\n", assistantStyle("[ASSISTANT]"))
			if msg.Content != "" {
				fmt.Printf("%s\n", msg.Content)
			}
			if len(msg.ToolCalls) > 0 {
				fmt.Printf("%s\n", toolStyle("Tool Calls:"))
				for _, tc := range msg.ToolCalls {
					// Pretty print arguments JSON
					var argsMap map[string]any
					argsStr := tc.Function.Arguments
					if err := json.Unmarshal([]byte(argsStr), &argsMap); err == nil {
						prettyJSON, _ := json.MarshalIndent(argsMap, "  ", "  ")
						argsStr = string(prettyJSON)
					}
					// Indent arguments slightly more for clarity
					indentedArgs := strings.ReplaceAll(argsStr, "\n", "\n    ")
					fmt.Printf("  - %s (%s):\n    %s\n", toolStyle(tc.Function.Name), tc.ID, toolDataStyle(indentedArgs))
				}
			}
			fmt.Println() // Add newline after assistant message
		case openai.ChatMessageRoleTool:
			// Check if content indicates an error
			isError := strings.HasPrefix(msg.Content, "Error:")
			prefix := toolStyle(fmt.Sprintf("[TOOL: %s (%s)]", msg.Name, msg.ToolCallID))
			contentStyle := toolDataStyle
			if isError {
				prefix = errorStyle(fmt.Sprintf("[TOOL ERROR: %s (%s)]", msg.Name, msg.ToolCallID))
				contentStyle = errorStyle
			}

			// Try to pretty print JSON content if possible
			var contentMap map[string]any
			var contentSlice []any
			contentStr := msg.Content
			if err := json.Unmarshal([]byte(contentStr), &contentMap); err == nil {
				prettyJSON, _ := json.MarshalIndent(contentMap, "  ", "  ")
				contentStr = string(prettyJSON)
			} else if err := json.Unmarshal([]byte(contentStr), &contentSlice); err == nil {
				prettyJSON, _ := json.MarshalIndent(contentSlice, "  ", "  ")
				contentStr = string(prettyJSON)
			}
			// Indent content for clarity
			indentedContent := strings.ReplaceAll(contentStr, "\n", "\n  ")
			fmt.Printf("%s\n  %s\n\n", prefix, contentStyle(indentedContent))
		default:
			fmt.Printf("[%s]\n%s\n\n", strings.ToUpper(msg.Role), msg.Content)
		}
	}
}

// printOutput prints last output of a history file
func printOutput(history ConversationHistory, pretty bool) {
	if len(history.Messages) < 1 {
		fmt.Println("No messages found in this history.")
		return
	}

	lastMessage := history.Messages[len(history.Messages)-1]
	if pretty {
		printPrettyOutput(lastMessage.Content)
	} else {
		fmt.Println(lastMessage.Content)
	}
}

// printMCPServerInfo prints information about an MCP server
// It attempts to start the server temporarily to discover available tools
func printMCPServerInfo(name string, server MCPServerConfig) {
	nameStyle := color.New(color.FgHiGreen, color.Bold).SprintFunc()
	descStyle := color.New(color.FgHiBlack).SprintFunc()
	commandStyle := color.New(color.FgYellow).SprintFunc()
	errorStyle := color.New(color.FgRed).SprintFunc()
	toolStyle := color.New(color.FgCyan).SprintFunc()

	fmt.Printf("  %s: %s\n", nameStyle(name), descStyle("MCP Server"))
	fmt.Printf("    %s %s %s\n", commandStyle("Command:"), server.Command, strings.Join(server.Args, " "))

	// Try to discover MCP tools by temporarily starting the server
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client := NewMCPClient(name, server)
	if err := client.Start(ctx); err != nil {
		fmt.Printf("    %s %s\n", errorStyle("Error:"), fmt.Sprintf("Failed to start MCP server: %v", err))
		return
	}
	defer client.Stop()

	tools := client.GetTools()
	if len(tools) > 0 {
		fmt.Printf("    %s\n", commandStyle("Available Tools:"))
		for _, tool := range tools {
			if tool.Function != nil {
				displayName := strings.TrimPrefix(tool.Function.Name, "mcp_"+name+"_")
				fmt.Printf("      %s: %s\n", toolStyle(displayName), tool.Function.Description)
			}
		}
	} else {
		fmt.Printf("    %s %s\n", descStyle("Tools:"), "No tools discovered")
	}
	fmt.Println()
}

func printPrettyOutput(content string) {
	renderer, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(80),
	)
	if err != nil {
		fmt.Println(content)
		return
	}

	out, err := renderer.Render(content)
	if err != nil {
		fmt.Println(content)
		return
	}

	fmt.Print(out)
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
