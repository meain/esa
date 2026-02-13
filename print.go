package main

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
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

// tryPrettyJSON attempts to format a string as indented JSON.
// Returns the formatted string and true if successful, or the original string and false.
func tryPrettyJSON(s string, indent string) (string, bool) {
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err == nil {
		if pretty, err := json.MarshalIndent(m, indent, "  "); err == nil {
			return string(pretty), true
		}
	}
	var sl []any
	if err := json.Unmarshal([]byte(s), &sl); err == nil {
		if pretty, err := json.MarshalIndent(sl, indent, "  "); err == nil {
			return string(pretty), true
		}
	}
	return s, false
}

// printHistoryJSON prints the raw history data as JSON.
func printHistoryJSON(history ConversationHistory) {
	if out, err := json.MarshalIndent(history, "", "  "); err == nil {
		fmt.Println(string(out))
	} else {
		fmt.Println(history)
	}
}

// printHistoryMarkdown prints the history in Markdown format.
func printHistoryMarkdown(fileName string, history ConversationHistory) {
	agentName := ""
	if history.AgentPath != "" {
		agentName = strings.TrimSuffix(filepath.Base(history.AgentPath), ".toml")
		agentName = strings.TrimPrefix(agentName, "builtin:")
	}

	fmt.Printf("# Conversation: %s\n\n", filepath.Base(fileName))
	if agentName != "" {
		fmt.Printf("**Agent:** +%s  \n", agentName)
	}
	if history.Model != "" {
		fmt.Printf("**Model:** %s  \n", history.Model)
	}
	fmt.Print("\n---\n\n")

	for _, msg := range history.Messages {
		switch msg.Role {
		case openai.ChatMessageRoleSystem:
			fmt.Printf("### 🔧 System\n\n")
			fmt.Printf("<details>\n<summary>System prompt</summary>\n\n%s\n\n</details>\n\n", msg.Content)

		case openai.ChatMessageRoleUser:
			fmt.Printf("### 👤 User\n\n%s\n\n", msg.Content)

		case openai.ChatMessageRoleAssistant:
			fmt.Printf("### 🤖 Assistant\n\n")
			if msg.Content != "" {
				fmt.Printf("%s\n\n", msg.Content)
			}
			if len(msg.ToolCalls) > 0 {
				for _, tc := range msg.ToolCalls {
					fmt.Printf("**⚙ Tool Call:** `%s`\n\n", tc.Function.Name)
					argsStr := tc.Function.Arguments
					if prettyArgs, ok := tryPrettyJSON(argsStr, ""); ok {
						argsStr = prettyArgs
					}
					fmt.Printf("```json\n%s\n```\n\n", argsStr)
				}
			}

		case openai.ChatMessageRoleTool:
			isError := strings.HasPrefix(msg.Content, "Error:")
			label := fmt.Sprintf("📎 Result: `%s`", msg.Name)
			if isError {
				label = fmt.Sprintf("❌ Error: `%s`", msg.Name)
			}
			fmt.Printf("**%s**\n\n", label)
			if formatted, ok := tryPrettyJSON(msg.Content, ""); ok {
				fmt.Printf("```json\n%s\n```\n\n", formatted)
			} else if len(msg.Content) > 200 || strings.Contains(msg.Content, "\n") {
				fmt.Printf("```\n%s\n```\n\n", msg.Content)
			} else {
				fmt.Printf("`%s`\n\n", msg.Content)
			}

		default:
			fmt.Printf("### %s\n\n%s\n\n", strings.ToUpper(msg.Role), msg.Content)
		}
	}
}

// printHistoryText prints the history in the default colored text format.
func printHistoryText(fileName string, history ConversationHistory) {
	messages := history.Messages
	agentPath := history.AgentPath
	model := history.Model

	systemStyle := color.New(color.FgMagenta, color.Italic).SprintFunc()
	userStyle := color.New(color.FgGreen, color.Bold).SprintFunc()
	assistantStyle := color.New(color.FgBlue, color.Bold).SprintFunc()
	toolStyle := color.New(color.FgYellow).SprintFunc()
	toolDataStyle := color.New(color.FgHiBlack).SprintFunc()
	errorStyle := color.New(color.FgRed).SprintFunc()
	labelStyle := color.New(color.FgHiCyan, color.Bold).SprintFunc()
	dimStyle := color.New(color.FgHiBlack).SprintFunc()

	// --- Print Header ---
	fmt.Printf("%s %s\n", labelStyle("File:"), filepath.Base(fileName))
	if agentPath != "" {
		agentName := strings.TrimSuffix(filepath.Base(agentPath), ".toml")
		agentName = strings.TrimPrefix(agentName, "builtin:")
		fmt.Printf("%s +%s\n", labelStyle("Agent:"), agentName)
	}
	if model != "" {
		fmt.Printf("%s %s\n", labelStyle("Model:"), model)
	}

	fmt.Println(dimStyle(strings.Repeat("─", 60)))

	for _, msg := range messages {
		switch msg.Role {
		case openai.ChatMessageRoleSystem:
			fmt.Printf("\n%s\n", systemStyle("── system ──"))
			// Truncate long system prompts
			content := msg.Content
			lines := strings.Split(content, "\n")
			if len(lines) > 5 {
				for _, l := range lines[:5] {
					fmt.Printf("  %s\n", dimStyle(l))
				}
				fmt.Printf("  %s\n", dimStyle(fmt.Sprintf("... (%d more lines)", len(lines)-5)))
			} else {
				for _, l := range lines {
					fmt.Printf("  %s\n", dimStyle(l))
				}
			}

		case openai.ChatMessageRoleUser:
			fmt.Printf("\n%s\n%s\n", userStyle("── you ──"), msg.Content)

		case openai.ChatMessageRoleAssistant:
			fmt.Printf("\n%s\n", assistantStyle("── esa ──"))
			if msg.Content != "" {
				fmt.Printf("%s\n", msg.Content)
			}
			if len(msg.ToolCalls) > 0 {
				for _, tc := range msg.ToolCalls {
					fmt.Printf("\n  %s %s\n", toolStyle("⚙"), toolStyle(tc.Function.Name))
					argsStr := tc.Function.Arguments
					if prettyArgs, ok := tryPrettyJSON(argsStr, "    "); ok {
						argsStr = prettyArgs
					}
					for _, line := range strings.Split(argsStr, "\n") {
						fmt.Printf("    %s\n", toolDataStyle(line))
					}
				}
			}

		case openai.ChatMessageRoleTool:
			isError := strings.HasPrefix(msg.Content, "Error:")
			if isError {
				fmt.Printf("  %s %s\n", errorStyle("✗"), errorStyle(msg.Name))
			} else {
				fmt.Printf("  %s %s\n", toolStyle("↳"), toolStyle(msg.Name))
			}
			contentStr, _ := tryPrettyJSON(msg.Content, "    ")
			lines := strings.Split(contentStr, "\n")
			maxLines := 20
			for i, line := range lines {
				if i >= maxLines {
					fmt.Printf("    %s\n", dimStyle(fmt.Sprintf("... (%d more lines)", len(lines)-maxLines)))
					break
				}
				if isError {
					fmt.Printf("    %s\n", errorStyle(line))
				} else {
					fmt.Printf("    %s\n", toolDataStyle(line))
				}
			}

		default:
			fmt.Printf("\n[%s]\n%s\n", strings.ToUpper(msg.Role), msg.Content)
		}
	}
	fmt.Println()
}

// printHistoryHTML prints the history as a self-contained HTML document.
func printHistoryHTML(fileName string, history ConversationHistory) {
	agentName := ""
	if history.AgentPath != "" {
		agentName = strings.TrimSuffix(filepath.Base(history.AgentPath), ".toml")
		agentName = strings.TrimPrefix(agentName, "builtin:")
	}

	var b strings.Builder
	b.WriteString(`<!DOCTYPE html>
<html lang="en" data-theme="dark">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>esa - `)
	b.WriteString(html.EscapeString(filepath.Base(fileName)))
	b.WriteString(`</title>
<style>
:root, [data-theme="dark"] {
    --bg-primary: #1a1b26;
    --bg-secondary: #16161e;
    --bg-tertiary: #24283b;
    --text-primary: #c0caf5;
    --text-secondary: #a9b1d6;
    --text-muted: #565f89;
    --accent: #7aa2f7;
    --green: #9ece6a;
    --red: #f7768e;
    --orange: #ff9e64;
    --cyan: #7dcfff;
    --border: #292e42;
}
@media (prefers-color-scheme: light) {
    :root {
        --bg-primary: #f5f5f5;
        --bg-secondary: #ffffff;
        --bg-tertiary: #e8e8e8;
        --text-primary: #1a1b26;
        --text-secondary: #3b4261;
        --text-muted: #8690a7;
        --accent: #2e5cb8;
        --green: #4d7a2a;
        --red: #c0392b;
        --orange: #d4740a;
        --cyan: #1a6ea0;
        --border: #d4d4d4;
    }
}
*, *::before, *::after { margin: 0; padding: 0; box-sizing: border-box; }
body {
    background: var(--bg-primary);
    color: var(--text-primary);
    font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', system-ui, sans-serif;
    line-height: 1.6;
    padding: 24px;
}
.container { max-width: 800px; margin: 0 auto; }
.header {
    padding: 16px 0;
    margin-bottom: 24px;
    border-bottom: 1px solid var(--border);
}
.header h1 { font-size: 18px; color: var(--accent); margin-bottom: 4px; }
.header .meta { font-size: 13px; color: var(--text-muted); }
.message { margin-bottom: 20px; }
.message-role {
    font-size: 12px;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    margin-bottom: 6px;
    padding: 4px 10px;
    border-radius: 4px;
    display: inline-block;
}
.role-system { color: var(--text-muted); background: var(--bg-tertiary); }
.role-user { color: var(--green); background: var(--bg-tertiary); }
.role-assistant { color: var(--accent); background: var(--bg-tertiary); }
.role-tool { color: var(--orange); background: var(--bg-tertiary); }
.role-error { color: var(--red); background: var(--bg-tertiary); }
.message-content {
    padding: 12px 16px;
    background: var(--bg-secondary);
    border-radius: 8px;
    border: 1px solid var(--border);
    white-space: pre-wrap;
    word-wrap: break-word;
    font-size: 14px;
}
.tool-call {
    margin: 8px 0;
    padding: 10px 14px;
    background: var(--bg-tertiary);
    border-radius: 6px;
    border-left: 3px solid var(--orange);
    font-family: 'SF Mono', 'Fira Code', monospace;
    font-size: 13px;
}
.tool-call .tool-name { color: var(--orange); font-weight: 600; }
.tool-call pre {
    margin-top: 6px;
    color: var(--text-muted);
    white-space: pre-wrap;
    font-size: 12px;
}
.tool-result {
    margin: 8px 0;
    padding: 10px 14px;
    background: var(--bg-tertiary);
    border-radius: 6px;
    border-left: 3px solid var(--green);
    font-family: 'SF Mono', 'Fira Code', monospace;
    font-size: 12px;
    white-space: pre-wrap;
    word-wrap: break-word;
    color: var(--text-secondary);
    max-height: 400px;
    overflow-y: auto;
}
.tool-result.error { border-left-color: var(--red); color: var(--red); }
.system-content {
    color: var(--text-muted);
    font-size: 13px;
    max-height: 120px;
    overflow-y: auto;
}
</style>
</head>
<body>
<div class="container">
`)

	// Header
	b.WriteString(`<div class="header">`)
	b.WriteString(fmt.Sprintf(`<h1>esa conversation</h1>`))
	b.WriteString(`<div class="meta">`)
	if agentName != "" {
		b.WriteString(fmt.Sprintf(`Agent: +%s`, html.EscapeString(agentName)))
	}
	if history.Model != "" {
		if agentName != "" {
			b.WriteString(` &middot; `)
		}
		b.WriteString(fmt.Sprintf(`Model: %s`, html.EscapeString(history.Model)))
	}
	b.WriteString(`</div></div>`)

	// Messages
	for _, msg := range history.Messages {
		b.WriteString(`<div class="message">`)

		switch msg.Role {
		case openai.ChatMessageRoleSystem:
			b.WriteString(`<div class="message-role role-system">system</div>`)
			b.WriteString(`<div class="message-content system-content">`)
			b.WriteString(html.EscapeString(msg.Content))
			b.WriteString(`</div>`)

		case openai.ChatMessageRoleUser:
			b.WriteString(`<div class="message-role role-user">you</div>`)
			b.WriteString(`<div class="message-content">`)
			b.WriteString(html.EscapeString(msg.Content))
			b.WriteString(`</div>`)

		case openai.ChatMessageRoleAssistant:
			b.WriteString(`<div class="message-role role-assistant">esa</div>`)
			if msg.Content != "" {
				b.WriteString(`<div class="message-content">`)
				b.WriteString(html.EscapeString(msg.Content))
				b.WriteString(`</div>`)
			}
			for _, tc := range msg.ToolCalls {
				b.WriteString(`<div class="tool-call">`)
				b.WriteString(fmt.Sprintf(`<span class="tool-name">⚙ %s</span>`, html.EscapeString(tc.Function.Name)))
				argsStr := tc.Function.Arguments
				if prettyArgs, ok := tryPrettyJSON(argsStr, ""); ok {
					argsStr = prettyArgs
				}
				b.WriteString(fmt.Sprintf(`<pre>%s</pre>`, html.EscapeString(argsStr)))
				b.WriteString(`</div>`)
			}

		case openai.ChatMessageRoleTool:
			isError := strings.HasPrefix(msg.Content, "Error:")
			cls := "tool-result"
			if isError {
				cls = "tool-result error"
			}
			contentStr, _ := tryPrettyJSON(msg.Content, "")
			b.WriteString(fmt.Sprintf(`<div class="%s">`, cls))
			b.WriteString(html.EscapeString(contentStr))
			b.WriteString(`</div>`)

		default:
			b.WriteString(fmt.Sprintf(`<div class="message-role">%s</div>`, html.EscapeString(strings.ToUpper(msg.Role))))
			b.WriteString(`<div class="message-content">`)
			b.WriteString(html.EscapeString(msg.Content))
			b.WriteString(`</div>`)
		}

		b.WriteString("</div>\n")
	}

	b.WriteString("</div>\n</body>\n</html>\n")
	fmt.Print(b.String())
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
