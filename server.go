package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/BurntSushi/toml"
	"github.com/gorilla/websocket"
	"github.com/sashabaranov/go-openai"
)

// WebSocket message types
const (
	wsMsgMessage     = "message"
	wsMsgContinue    = "continue"
	wsMsgToken       = "token"
	wsMsgToolCall    = "tool_call"
	wsMsgToolResult  = "tool_result"
	wsMsgApproval    = "approval"
	wsMsgDone        = "done"
	wsMsgError       = "error"
	wsMsgAgentList   = "agent_list"
	wsMsgHistoryList = "history_list"
)

// WSMessage represents a WebSocket message exchanged between client and server
type WSMessage struct {
	Type    string `json:"type"`
	Content string `json:"content,omitempty"`
	Agent   string `json:"agent,omitempty"`
	Model   string `json:"model,omitempty"`
	ID      string `json:"id,omitempty"`
	Name    string `json:"name,omitempty"`
	Command string `json:"command,omitempty"`
	Safe    bool   `json:"safe,omitempty"`
	Output  string `json:"output,omitempty"`
	Args    string `json:"args,omitempty"`

	// Approval fields
	Approved bool   `json:"approved,omitempty"`
	Message  string `json:"message,omitempty"`

	// List payloads
	Agents  []AgentInfo   `json:"agents,omitempty"`
	History []HistoryInfo `json:"history,omitempty"`
}

// AgentInfo is a summary of an agent for listing
type AgentInfo struct {
	Name        string         `json:"name"`
	Path        string         `json:"path"`
	Description string         `json:"description"`
	IsBuiltin   bool           `json:"is_builtin"`
	Functions   []FunctionInfo `json:"functions,omitempty"`
}

// FunctionInfo is a summary of a function for display
type FunctionInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Safe        bool   `json:"safe"`
}

// HistoryInfo is a summary of a conversation history entry
type HistoryInfo struct {
	Index          int    `json:"index"`
	Agent          string `json:"agent"`
	Query          string `json:"query"`
	Timestamp      string `json:"timestamp"`
	FileName       string `json:"filename"`
	ConversationID string `json:"conversation_id"`
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Local only, no auth needed
	},
}

// webSession tracks the state for a single WebSocket chat session
type webSession struct {
	conn       *websocket.Conn
	app        *Application
	mu         sync.Mutex
	approvalCh chan confirmResponse
}

func (s *webSession) sendJSON(msg WSMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.conn.WriteJSON(msg)
}

// runServeMode starts the HTTP/WebSocket server
func runServeMode(opts *CLIOptions) error {
	mux := http.NewServeMux()

	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		handleWebSocket(w, r, opts)
	})

	// API endpoints
	mux.HandleFunc("/api/agents", handleListAgents)
	mux.HandleFunc("/api/agents/", handleGetAgent)
	mux.HandleFunc("/api/history", handleListHistory)
	mux.HandleFunc("/api/history/", handleGetHistory)

	// Serve embedded static files
	mux.Handle("/", http.FileServer(http.FS(webFS)))

	addr := fmt.Sprintf("127.0.0.1:%d", opts.ServePort)
	fmt.Fprintf(os.Stderr, "esa web server listening on http://%s\n", addr)

	return http.ListenAndServe(addr, mux)
}

// agentToFunctions converts agent functions to FunctionInfo list
func agentToFunctions(agent Agent) []FunctionInfo {
	var fns []FunctionInfo
	for _, fc := range agent.Functions {
		fns = append(fns, FunctionInfo{
			Name:        fc.Name,
			Description: fc.Description,
			Safe:        fc.Safe,
		})
	}
	return fns
}

// handleListAgents returns a JSON list of available agents
func handleListAgents(w http.ResponseWriter, r *http.Request) {
	var agents []AgentInfo

	// Built-in agents
	for name, tomlContent := range builtinAgents {
		var agent Agent
		if _, err := toml.Decode(tomlContent, &agent); err != nil {
			continue
		}
		agents = append(agents, AgentInfo{
			Name:        name,
			Path:        "builtin:" + name,
			Description: agent.Description,
			IsBuiltin:   true,
			Functions:   agentToFunctions(agent),
		})
	}

	// User agents
	userAgents, userNames, _ := getUserAgents(false)
	for i, agent := range userAgents {
		agents = append(agents, AgentInfo{
			Name:        userNames[i],
			Path:        expandHomePath(fmt.Sprintf("%s/%s.toml", DefaultAgentsDir, userNames[i])),
			Description: agent.Description,
			IsBuiltin:   false,
			Functions:   agentToFunctions(agent),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(agents)
}

// handleGetAgent returns detailed info for a single agent
func handleGetAgent(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/api/agents/")
	if name == "" {
		http.Error(w, "agent name required", http.StatusBadRequest)
		return
	}

	_, agentPath := ParseAgentString("+" + name)
	agent, err := loadConfiguration(&CLIOptions{AgentName: name, AgentPath: agentPath})
	if err != nil {
		http.Error(w, fmt.Sprintf("agent not found: %v", err), http.StatusNotFound)
		return
	}

	info := AgentInfo{
		Name:        agent.Name,
		Path:        agentPath,
		Description: agent.Description,
		IsBuiltin:   strings.HasPrefix(agentPath, "builtin:"),
		Functions:   agentToFunctions(agent),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(info)
}

// handleListHistory returns a JSON list of conversation history
func handleListHistory(w http.ResponseWriter, r *http.Request) {
	sortedFiles, _, err := getSortedHistoryFiles()
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]HistoryInfo{})
		return
	}

	var histories []HistoryInfo
	cacheDir, _ := setupCacheDir()

	for i, fileName := range sortedFiles {
		parts := strings.SplitN(strings.TrimSuffix(fileName, ".json"), "-", 5)
		agentName := "unknown"
		timestampStr := "unknown"
		if len(parts) == 5 {
			agentName = parts[3]
			timestampStr = parts[4]
		}

		// Get first user query
		var query string
		historyFilePath := fmt.Sprintf("%s/%s", cacheDir, fileName)
		if historyData, err := os.ReadFile(historyFilePath); err == nil {
			var history ConversationHistory
			if err := json.Unmarshal(historyData, &history); err == nil {
				prevMessage := ""
				for _, msg := range history.Messages {
					if msg.Role == openai.ChatMessageRoleAssistant {
						query = strings.ReplaceAll(prevMessage, "\n", " ")
						if len(query) > 80 {
							query = query[:77] + "..."
						}
						break
					}
					prevMessage = msg.Content
				}
			}
		}

		conversationID := ""
		if len(parts) == 5 {
			conversationID = parts[0]
		}

		histories = append(histories, HistoryInfo{
			Index:          i + 1,
			Agent:          agentName,
			Query:          query,
			Timestamp:      timestampStr,
			FileName:       fileName,
			ConversationID: conversationID,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(histories)
}

// handleGetHistory returns the messages from a specific history file
func handleGetHistory(w http.ResponseWriter, r *http.Request) {
	conversation := strings.TrimPrefix(r.URL.Path, "/api/history/")
	if conversation == "" {
		http.Error(w, "conversation ID required", http.StatusBadRequest)
		return
	}

	_, history, ok := readHistoryFile(conversation)
	if !ok {
		http.Error(w, "history not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(history)
}

// handleWebSocket handles a WebSocket connection for chat
func handleWebSocket(w http.ResponseWriter, r *http.Request, baseOpts *CLIOptions) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}
	defer conn.Close()

	session := &webSession{
		conn:       conn,
		approvalCh: make(chan confirmResponse, 1),
	}

	for {
		var msg WSMessage
		if err := conn.ReadJSON(&msg); err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				log.Printf("WebSocket read error: %v", err)
			}
			return
		}

		switch msg.Type {
		case wsMsgMessage:
			go session.handleChatMessage(msg, baseOpts)
		case wsMsgContinue:
			go session.handleContinueChat(msg, baseOpts)
		case wsMsgApproval:
			session.approvalCh <- confirmResponse{
				approved: msg.Approved,
				message:  msg.Message,
			}
		}
	}
}

// handleContinueChat continues an existing conversation from history
func (s *webSession) handleContinueChat(msg WSMessage, baseOpts *CLIOptions) {
	conversationID := msg.ID
	if conversationID == "" {
		s.sendJSON(WSMessage{Type: wsMsgError, Content: "No conversation ID provided"})
		return
	}

	opts := &CLIOptions{
		Model:        msg.Model,
		ConfigPath:   baseOpts.ConfigPath,
		AskLevel:     "unsafe",
		HideProgress: true,
		ContinueChat: true,
		Conversation: conversationID,
	}

	// Parse agent from message
	agentStr := msg.Agent
	if agentStr == "" {
		agentStr = "+default"
	}
	agentName, agentPath := ParseAgentString(agentStr)
	opts.AgentName = agentName
	opts.AgentPath = agentPath
	if opts.AgentPath == "" {
		opts.AgentPath = DefaultAgentPath
	}

	app, err := NewApplication(opts)
	if err != nil {
		s.sendJSON(WSMessage{Type: wsMsgError, Content: fmt.Sprintf("Failed to initialize: %v", err)})
		return
	}
	s.app = app

	cleanup, err := app.initializeRuntime()
	if err != nil {
		s.sendJSON(WSMessage{Type: wsMsgError, Content: fmt.Sprintf("Failed to initialize runtime: %v", err)})
		return
	}
	defer cleanup()

	// Add the new user message
	app.messages = append(app.messages, openai.ChatCompletionMessage{
		Role:    "user",
		Content: msg.Content,
	})

	s.runWebConversationLoop(app, *opts)
}

// handleChatMessage processes a new chat message from the web client
func (s *webSession) handleChatMessage(msg WSMessage, baseOpts *CLIOptions) {
	// Build CLI options for this session
	opts := &CLIOptions{
		AgentPath:    "",
		Model:        msg.Model,
		ConfigPath:   baseOpts.ConfigPath,
		AskLevel:     "unsafe", // Web uses unsafe level, approval handled in UI
		HideProgress: true,
	}

	// Parse agent from message
	agentStr := msg.Agent
	if agentStr == "" {
		agentStr = "+default"
	}
	agentName, agentPath := ParseAgentString(agentStr)
	opts.AgentName = agentName
	opts.AgentPath = agentPath
	if opts.AgentPath == "" {
		opts.AgentPath = DefaultAgentPath
	}

	// Create application for this session
	app, err := NewApplication(opts)
	if err != nil {
		s.sendJSON(WSMessage{Type: wsMsgError, Content: fmt.Sprintf("Failed to initialize: %v", err)})
		return
	}
	s.app = app

	// Initialize runtime (MCP servers, system prompt)
	cleanup, err := app.initializeRuntime()
	if err != nil {
		s.sendJSON(WSMessage{Type: wsMsgError, Content: fmt.Sprintf("Failed to initialize runtime: %v", err)})
		return
	}
	defer cleanup()

	// Add user message
	app.messages = append(app.messages, openai.ChatCompletionMessage{
		Role:    "user",
		Content: msg.Content,
	})

	// Run conversation loop
	s.runWebConversationLoop(app, *opts)
}

// runWebConversationLoop is the web-adapted version of runConversationLoop
func (s *webSession) runWebConversationLoop(app *Application, opts CLIOptions) {
	openAITools := convertFunctionsToTools(app.agent.Functions)
	mcpTools := app.mcpManager.GetAllTools()
	openAITools = append(openAITools, mcpTools...)

	for {
		stream, err := app.createChatCompletionWithRetry(openAITools)
		if err != nil {
			s.sendJSON(WSMessage{Type: wsMsgError, Content: fmt.Sprintf("LLM error: %v", err)})
			return
		}

		assistantMsg := s.handleWebStreamResponse(stream)
		app.messages = append(app.messages, assistantMsg)
		app.saveConversationHistory()

		if len(assistantMsg.ToolCalls) == 0 {
			s.sendJSON(WSMessage{Type: wsMsgDone})
			return
		}

		s.handleWebToolCalls(app, assistantMsg.ToolCalls, opts)
	}
}

// handleWebStreamResponse streams LLM tokens over WebSocket
func (s *webSession) handleWebStreamResponse(stream *openai.ChatCompletionStream) openai.ChatCompletionMessage {
	defer stream.Close()

	var assistantMsg openai.ChatCompletionMessage
	var fullContent strings.Builder

	for {
		response, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			s.sendJSON(WSMessage{Type: wsMsgError, Content: fmt.Sprintf("Stream error: %v", err)})
			break
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
			content := response.Choices[0].Delta.Content
			if content != "" {
				fullContent.WriteString(content)
				s.sendJSON(WSMessage{Type: wsMsgToken, Content: content})
			}
		}
	}

	assistantMsg.Role = "assistant"
	assistantMsg.Content = fullContent.String()
	return assistantMsg
}

// handleWebToolCalls processes tool calls, sending approval requests over WebSocket
func (s *webSession) handleWebToolCalls(app *Application, toolCalls []openai.ToolCall, opts CLIOptions) {
	for _, toolCall := range toolCalls {
		if toolCall.Type != "function" || toolCall.Function.Name == "" {
			continue
		}

		// MCP tool calls
		if strings.HasPrefix(toolCall.Function.Name, "mcp_") {
			s.handleWebMCPToolCall(app, toolCall, opts)
			continue
		}

		// Find matching function
		var matchedFunc FunctionConfig
		for _, fc := range app.agent.Functions {
			if fc.Name == toolCall.Function.Name {
				matchedFunc = fc
				break
			}
		}

		if matchedFunc.Name == "" {
			app.appendToolError(toolCall, fmt.Errorf("no matching function found: %s", toolCall.Function.Name))
			s.sendJSON(WSMessage{
				Type:   wsMsgToolResult,
				ID:     toolCall.ID,
				Name:   toolCall.Function.Name,
				Output: fmt.Sprintf("Error: no matching function found: %s", toolCall.Function.Name),
			})
			continue
		}

		// Parse args and prepare command
		parsedArgs, err := parseAndValidateArgs(matchedFunc, toolCall.Function.Arguments)
		if err != nil {
			app.appendToolError(toolCall, err)
			s.sendJSON(WSMessage{
				Type:   wsMsgToolResult,
				ID:     toolCall.ID,
				Name:   matchedFunc.Name,
				Output: fmt.Sprintf("Error: %v", err),
			})
			continue
		}

		command, err := prepareCommand(matchedFunc, parsedArgs)
		if err != nil {
			app.appendToolError(toolCall, err)
			s.sendJSON(WSMessage{
				Type:   wsMsgToolResult,
				ID:     toolCall.ID,
				Name:   matchedFunc.Name,
				Output: fmt.Sprintf("Error: %v", err),
			})
			continue
		}

		isSafe := matchedFunc.Safe
		askLevel := app.getEffectiveAskLevel()
		requiresApproval := needsConfirmation(askLevel, isSafe)

		// Send tool call notification to client
		s.sendJSON(WSMessage{
			Type:    wsMsgToolCall,
			ID:      toolCall.ID,
			Name:    matchedFunc.Name,
			Command: command,
			Safe:    !requiresApproval,
			Args:    toolCall.Function.Arguments,
		})

		// Only wait for approval if the function requires it
		if requiresApproval {
			approval := <-s.approvalCh
			if !approval.approved {
				result := "Command execution cancelled by user."
				if approval.message != "" {
					result = fmt.Sprintf("Message from user: %s", approval.message)
				}
				content := fmt.Sprintf("Command: %s\n\nOutput: \n%s", command, result)
				app.messages = append(app.messages, openai.ChatCompletionMessage{
					Role:       "tool",
					Name:       toolCall.Function.Name,
					Content:    content,
					ToolCallID: toolCall.ID,
				})
				s.sendJSON(WSMessage{
					Type:   wsMsgToolResult,
					ID:     toolCall.ID,
					Name:   matchedFunc.Name,
					Output: result,
				})
				continue
			}
		}

		// Execute the command
		expandedCmd := expandHomePath(command)
		provider, model, _ := app.parseModel()
		os.Setenv("ESA_MODEL", fmt.Sprintf("%s/%s", provider, model))

		output, stdinContent, cmdErr := executeShellCommand(expandedCmd, matchedFunc, parsedArgs)
		result := strings.TrimSpace(string(output))
		_ = stdinContent

		if cmdErr != nil {
			app.appendToolError(toolCall, cmdErr)
			s.sendJSON(WSMessage{
				Type:   wsMsgToolResult,
				ID:     toolCall.ID,
				Name:   matchedFunc.Name,
				Output: fmt.Sprintf("Error: %v", cmdErr),
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
		app.saveConversationHistory()

		s.sendJSON(WSMessage{
			Type:   wsMsgToolResult,
			ID:     toolCall.ID,
			Name:   matchedFunc.Name,
			Output: result,
		})
	}
}

// handleWebMCPToolCall handles MCP tool calls via WebSocket
func (s *webSession) handleWebMCPToolCall(app *Application, toolCall openai.ToolCall, opts CLIOptions) {
	var arguments any
	if toolCall.Function.Arguments != "" {
		if err := json.Unmarshal([]byte(toolCall.Function.Arguments), &arguments); err != nil {
			app.appendToolError(toolCall, fmt.Errorf("failed to parse arguments: %v", err))
			s.sendJSON(WSMessage{
				Type:   wsMsgToolResult,
				ID:     toolCall.ID,
				Name:   toolCall.Function.Name,
				Output: fmt.Sprintf("Error parsing arguments: %v", err),
			})
			return
		}
	}

	argsDisplay := formatArgsForDisplay(arguments)
	displayCommand := fmt.Sprintf("%s(%s)", toolCall.Function.Name, argsDisplay)

	// Always ask for approval on web for MCP tools
	s.sendJSON(WSMessage{
		Type:    wsMsgToolCall,
		ID:      toolCall.ID,
		Name:    toolCall.Function.Name,
		Command: displayCommand,
		Safe:    false,
		Args:    toolCall.Function.Arguments,
	})

	approval := <-s.approvalCh
	if !approval.approved {
		result := "Tool execution cancelled by user."
		if approval.message != "" {
			result = fmt.Sprintf("Message from user: %s", approval.message)
		}
		app.messages = append(app.messages, openai.ChatCompletionMessage{
			Role:       "tool",
			Name:       toolCall.Function.Name,
			Content:    result,
			ToolCallID: toolCall.ID,
		})
		s.sendJSON(WSMessage{
			Type:   wsMsgToolResult,
			ID:     toolCall.ID,
			Name:   toolCall.Function.Name,
			Output: result,
		})
		return
	}

	// Call with askLevel "none" since we already handled approval in the web layer
	result, err := app.mcpManager.CallTool(toolCall.Function.Name, arguments, "none")
	if err != nil {
		app.appendToolError(toolCall, err)
		s.sendJSON(WSMessage{
			Type:   wsMsgToolResult,
			ID:     toolCall.ID,
			Name:   toolCall.Function.Name,
			Output: fmt.Sprintf("Error: %v", err),
		})
		return
	}

	app.messages = append(app.messages, openai.ChatCompletionMessage{
		Role:       "tool",
		Name:       toolCall.Function.Name,
		Content:    result,
		ToolCallID: toolCall.ID,
	})
	app.saveConversationHistory()

	s.sendJSON(WSMessage{
		Type:   wsMsgToolResult,
		ID:     toolCall.ID,
		Name:   toolCall.Function.Name,
		Output: result,
	})
}
