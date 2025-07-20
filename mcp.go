package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"

	"github.com/sashabaranov/go-openai"
)

// MCPClient represents a client connection to an MCP server
type MCPClient struct {
	name           string
	config         MCPServerConfig
	cmd            *exec.Cmd
	stdin          io.WriteCloser
	stdout         io.ReadCloser
	stderr         io.ReadCloser
	tools          []openai.Tool
	functionSafety map[string]bool // Maps function name to safety status
	mu             sync.Mutex
	running        bool
}

// MCPRequest represents a JSON-RPC 2.0 request
type MCPRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

// MCPResponse represents a JSON-RPC 2.0 response
type MCPResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Result  interface{} `json:"result,omitempty"`
	Error   *MCPError   `json:"error,omitempty"`
}

// MCPError represents a JSON-RPC 2.0 error
type MCPError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// MCPTool represents a tool/function available from the MCP server
type MCPTool struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema interface{} `json:"inputSchema"`
}

// MCPToolsListResult represents the result of listing tools
type MCPToolsListResult struct {
	Tools []MCPTool `json:"tools"`
}

// MCPCallToolParams represents parameters for calling a tool
type MCPCallToolParams struct {
	Name      string      `json:"name"`
	Arguments interface{} `json:"arguments,omitempty"`
}

// MCPCallToolResult represents the result of calling a tool
type MCPCallToolResult struct {
	Content []MCPContent `json:"content"`
	IsError bool         `json:"isError,omitempty"`
}

// MCPContent represents content in MCP protocol
type MCPContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// NewMCPClient creates a new MCP client
func NewMCPClient(name string, config MCPServerConfig) *MCPClient {
	return &MCPClient{
		name:           name,
		config:         config,
		functionSafety: make(map[string]bool),
	}
}

// Start starts the MCP server process and initializes the connection
func (c *MCPClient) Start(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.running {
		return nil
	}

	// Create command
	c.cmd = exec.CommandContext(ctx, c.config.Command, c.config.Args...)

	// Set up pipes
	stdin, err := c.cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdin pipe: %w", err)
	}
	c.stdin = stdin

	stdout, err := c.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}
	c.stdout = stdout

	stderr, err := c.cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to create stderr pipe: %w", err)
	}
	c.stderr = stderr

	// Start the process
	if err := c.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start MCP server %s: %w", c.name, err)
	}

	c.running = true

	// Initialize the MCP connection
	if err := c.initialize(); err != nil {
		c.stopInternal() // Use internal stop method to avoid deadlock
		return fmt.Errorf("failed to initialize MCP server %s: %w", c.name, err)
	}

	// Load available tools
	if err := c.loadTools(); err != nil {
		c.stopInternal() // Use internal stop method to avoid deadlock
		return fmt.Errorf("failed to load tools from MCP server %s: %w", c.name, err)
	}

	return nil
}

// Stop stops the MCP server process
func (c *MCPClient) Stop() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.stopInternal()
	return nil
}

// stopInternal stops the MCP server process without acquiring the mutex (caller must hold lock)
func (c *MCPClient) stopInternal() {
	if !c.running {
		return
	}

	c.running = false

	// Close pipes
	if c.stdin != nil {
		c.stdin.Close()
	}
	if c.stdout != nil {
		c.stdout.Close()
	}
	if c.stderr != nil {
		c.stderr.Close()
	}

	// Terminate process
	if c.cmd != nil && c.cmd.Process != nil {
		c.cmd.Process.Kill()
		c.cmd.Wait()
	}
}

// initialize sends the initialization request to the MCP server
func (c *MCPClient) initialize() error {
	initRequest := MCPRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "initialize",
		Params: map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities": map[string]interface{}{
				"tools": map[string]interface{}{},
			},
			"clientInfo": map[string]interface{}{
				"name":    "esa",
				"version": "1.0.0",
			},
		},
	}

	response, err := c.sendRequest(initRequest)
	if err != nil {
		return err
	}

	if response.Error != nil {
		return fmt.Errorf("initialization failed: %s", response.Error.Message)
	}

	// Send initialized notification
	initNotification := MCPRequest{
		JSONRPC: "2.0",
		Method:  "notifications/initialized",
	}

	return c.sendNotification(initNotification)
}

// loadTools loads the available tools from the MCP server
func (c *MCPClient) loadTools() error {
	request := MCPRequest{
		JSONRPC: "2.0",
		ID:      2,
		Method:  "tools/list",
	}

	response, err := c.sendRequest(request)
	if err != nil {
		return err
	}

	if response.Error != nil {
		return fmt.Errorf("failed to list tools: %s", response.Error.Message)
	}

	// Parse the tools list result
	resultBytes, err := json.Marshal(response.Result)
	if err != nil {
		return fmt.Errorf("failed to marshal tools result: %w", err)
	}

	var toolsResult MCPToolsListResult
	if err := json.Unmarshal(resultBytes, &toolsResult); err != nil {
		return fmt.Errorf("failed to unmarshal tools result: %w", err)
	}

	// Create sets for quick lookup
	allowedFunctions := make(map[string]bool)
	safeFunctions := make(map[string]bool)

	// If allowed_functions is specified, only allow those functions
	if len(c.config.AllowedFunctions) > 0 {
		for _, funcName := range c.config.AllowedFunctions {
			allowedFunctions[funcName] = true
		}
	}

	// Build safe functions set
	for _, funcName := range c.config.SafeFunctions {
		safeFunctions[funcName] = true
	}

	// Convert MCP tools to OpenAI tools
	c.tools = make([]openai.Tool, 0, len(toolsResult.Tools))
	for _, mcpTool := range toolsResult.Tools {
		// Skip if not in allowed functions list (when list is specified)
		if len(c.config.AllowedFunctions) > 0 && !allowedFunctions[mcpTool.Name] {
			continue
		}

		// Determine safety: check safe_functions first, then fall back to server-level safe setting
		isSafe := c.config.Safe // Default to server-level setting
		if _, exists := safeFunctions[mcpTool.Name]; exists {
			isSafe = true // Function is explicitly marked as safe
		}

		// Store function safety information
		c.functionSafety[mcpTool.Name] = isSafe

		openaiTool := openai.Tool{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        fmt.Sprintf("mcp_%s_%s", c.name, mcpTool.Name),
				Description: mcpTool.Description,
				Parameters:  mcpTool.InputSchema,
			},
		}
		c.tools = append(c.tools, openaiTool)
	}

	return nil
}

// GetTools returns the available tools from this MCP server
func (c *MCPClient) GetTools() []openai.Tool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.tools
}

// CallTool calls a tool on the MCP server
func (c *MCPClient) CallTool(toolName string, arguments interface{}, askLevel string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.running {
		return "", fmt.Errorf("MCP server %s is not running", c.name)
	}

	// Remove the mcp_<servername>_ prefix from the tool name
	prefix := fmt.Sprintf("mcp_%s_", c.name)
	if !strings.HasPrefix(toolName, prefix) {
		return "", fmt.Errorf("invalid tool name format: %s", toolName)
	}
	actualToolName := strings.TrimPrefix(toolName, prefix)

	// Get the safety status for this specific function
	isSafe, exists := c.functionSafety[actualToolName]
	if !exists {
		// If we don't have safety info, fall back to server-level setting
		isSafe = c.config.Safe
	}

	// Check if confirmation is needed
	if needsConfirmation(askLevel, isSafe) {
		// Format arguments for display
		var argsDisplay string
		if arguments != nil {
			if argsJSON, err := json.Marshal(arguments); err == nil {
				argsDisplay = string(argsJSON)
			} else {
				argsDisplay = fmt.Sprintf("%v", arguments)
			}
		} else {
			argsDisplay = "{}"
		}

		response := confirm(fmt.Sprintf("Call %s:%s(%s)?", c.name, actualToolName, argsDisplay))
		if !response.approved {
			if response.message != "" {
				return fmt.Sprintf("Message from user: %s", response.message), nil
			}
			return "MCP tool execution cancelled by user.", nil
		}
	}

	request := MCPRequest{
		JSONRPC: "2.0",
		ID:      3,
		Method:  "tools/call",
		Params: MCPCallToolParams{
			Name:      actualToolName,
			Arguments: arguments,
		},
	}

	response, err := c.sendRequest(request)
	if err != nil {
		return "", err
	}

	if response.Error != nil {
		return "", fmt.Errorf("tool call failed: %s", response.Error.Message)
	}

	// Parse the tool call result
	resultBytes, err := json.Marshal(response.Result)
	if err != nil {
		return "", fmt.Errorf("failed to marshal tool result: %w", err)
	}

	var toolResult MCPCallToolResult
	if err := json.Unmarshal(resultBytes, &toolResult); err != nil {
		return "", fmt.Errorf("failed to unmarshal tool result: %w", err)
	}

	// Combine all content text
	var result strings.Builder
	for i, content := range toolResult.Content {
		if i > 0 {
			result.WriteString("\n")
		}
		result.WriteString(content.Text)
	}

	if toolResult.IsError {
		return "", fmt.Errorf("Tool execution error\n" + result.String())
	}

	resultStr := result.String()
	// Ensure we don't return empty results which could cause API issues
	if resultStr == "" {
		resultStr = "(No output)"
	}

	return resultStr, nil
}

// sendRequest sends a JSON-RPC request and waits for response
func (c *MCPClient) sendRequest(request MCPRequest) (*MCPResponse, error) {
	// Marshal request
	requestBytes, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Send request
	if _, err := c.stdin.Write(append(requestBytes, '\n')); err != nil {
		return nil, fmt.Errorf("failed to write request: %w", err)
	}

	// Read response
	scanner := bufio.NewScanner(c.stdout)
	// Increase buffer size to handle large responses (default is 64KB, set to 10MB)
	const maxTokenSize = 10 * 1024 * 1024 // 10MB
	buffer := make([]byte, maxTokenSize)
	scanner.Buffer(buffer, maxTokenSize)

	if !scanner.Scan() {
		// Check for scanner error
		if err := scanner.Err(); err != nil {
			return nil, fmt.Errorf("failed to read response (scanner error): %w", err)
		}

		// Check stderr for any error messages from the MCP server
		stderrBytes := make([]byte, 1024)
		if n, err := c.stderr.Read(stderrBytes); err == nil && n > 0 {
			return nil, fmt.Errorf("failed to read response, stderr from MCP server: %s", string(stderrBytes[:n]))
		}

		return nil, fmt.Errorf("failed to read response (no data from MCP server)")
	}

	responseBytes := scanner.Bytes()

	var response MCPResponse
	if err := json.Unmarshal(responseBytes, &response); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w, raw response: %s", err, string(responseBytes))
	}

	return &response, nil
}

// sendNotification sends a JSON-RPC notification (no response expected)
func (c *MCPClient) sendNotification(notification MCPRequest) error {
	// Marshal notification
	notificationBytes, err := json.Marshal(notification)
	if err != nil {
		return fmt.Errorf("failed to marshal notification: %w", err)
	}

	// Send notification
	if _, err := c.stdin.Write(append(notificationBytes, '\n')); err != nil {
		return fmt.Errorf("failed to write notification: %w", err)
	}

	return nil
}

// MCPManager manages multiple MCP clients
type MCPManager struct {
	clients map[string]*MCPClient
	mu      sync.RWMutex
}

// NewMCPManager creates a new MCP manager
func NewMCPManager() *MCPManager {
	return &MCPManager{
		clients: make(map[string]*MCPClient),
	}
}

// StartServers starts all MCP servers from the agent configuration
func (m *MCPManager) StartServers(ctx context.Context, mcpServers map[string]MCPServerConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for name, config := range mcpServers {
		client := NewMCPClient(name, config)
		if err := client.Start(ctx); err != nil {
			// Stop any already started clients
			m.stopAllClients()
			return fmt.Errorf("failed to start MCP server %s: %w", name, err)
		}
		m.clients[name] = client
	}

	return nil
}

// StopAllServers stops all MCP servers
func (m *MCPManager) StopAllServers() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopAllClients()
}

// stopAllClients stops all clients (caller must hold lock)
func (m *MCPManager) stopAllClients() {
	for name, client := range m.clients {
		if err := client.Stop(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to stop MCP server %s: %v\n", name, err)
		}
	}
	m.clients = make(map[string]*MCPClient)
}

// GetAllTools returns all tools from all MCP servers
func (m *MCPManager) GetAllTools() []openai.Tool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var allTools []openai.Tool
	for _, client := range m.clients {
		allTools = append(allTools, client.GetTools()...)
	}
	return allTools
}

// CallTool calls a tool on the appropriate MCP server
func (m *MCPManager) CallTool(toolName string, arguments interface{}, askLevel string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Find the client that owns this tool
	for serverName, client := range m.clients {
		prefix := fmt.Sprintf("mcp_%s_", serverName)
		if strings.HasPrefix(toolName, prefix) {
			return client.CallTool(toolName, arguments, askLevel)
		}
	}

	return "", fmt.Errorf("no MCP server found for tool: %s", toolName)
}
