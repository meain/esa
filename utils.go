package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/fatih/color"
	"golang.org/x/term"
)

// expandHomePath expands the ~ character in a path to the user's home directory
func expandHomePath(path string) string {
	if strings.HasPrefix(path, "~") {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return path // Return original path if we can't get home dir
		}
		return filepath.Join(homeDir, path[1:])
	}
	return path
}

// CacheError represents errors related to cache directory operations
type CacheError struct {
	Operation string
	Path      string
	Err       error
}

func (e *CacheError) Error() string {
	return fmt.Sprintf("cache %s failed for path '%s': %v", e.Operation, e.Path, e.Err)
}

// FileError represents errors related to file operations
type FileError struct {
	Operation string
	Path      string
	Err       error
}

func (e *FileError) Error() string {
	return fmt.Sprintf("file %s failed for path '%s': %v", e.Operation, e.Path, e.Err)
}

// wrapCacheError wraps an error with cache context
func wrapCacheError(operation, path string, err error) error {
	if err == nil {
		return nil
	}
	return &CacheError{Operation: operation, Path: path, Err: err}
}

// wrapFileError wraps an error with file context
func wrapFileError(operation, path string, err error) error {
	if err == nil {
		return nil
	}
	return &FileError{Operation: operation, Path: path, Err: err}
}

// confirmResponse represents the response type from a confirmation prompt
type confirmResponse struct {
	approved bool
	message  string
}

// confirm prompts the user for confirmation with yes/no/message options
func confirm(prompt string) confirmResponse {
	cyan := color.New(color.FgCyan).SprintFunc()
	fmt.Fprintf(os.Stderr, "%s %s (m/y/N): ", cyan("[?]"), prompt)

	oldState, _ := term.MakeRaw(int(os.Stdin.Fd()))

	reader := bufio.NewReader(os.Stdin)
	char, err := reader.ReadByte()
	if err != nil {
		return confirmResponse{approved: false, message: ""}
	}

	response := strings.ToLower(string(char))
	fmt.Fprintf(os.Stderr, "%s\n\r", response)

	term.Restore(int(os.Stdin.Fd()), oldState)

	if response == "m" {
		fmt.Fprintf(os.Stderr, "%s Enter message: ", cyan("[?]"))
		message, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		message = strings.TrimSuffix(message, "\n")
		return confirmResponse{approved: false, message: message}
	}

	return confirmResponse{approved: response == "y", message: ""}
}

// setupCacheDir ensures the cache directory exists and returns its path.
func setupCacheDir() (string, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", wrapCacheError("get user cache directory", "", err)
	}
	esaDir := filepath.Join(cacheDir, "esa")
	if err := os.MkdirAll(esaDir, 0755); err != nil {
		return "", wrapCacheError("create directory", esaDir, err)
	}
	return esaDir, nil
}

// setupCacheDirWithFallback ensures the cache directory exists and handles errors gracefully
func setupCacheDirWithFallback() string {
	cacheDir, err := setupCacheDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not setup cache directory: %v\n", err)
		// Fallback to temp directory
		return filepath.Join(os.TempDir(), "esa")
	}
	return cacheDir
}

func createNewHistoryFile(cacheDir string, agentName string, conversation string) string {
	if agentName == "" {
		agentName = "default"
	}
	timestamp := time.Now().Format(historyTimeFormat)

	if _, ok := getConversationIndex(conversation); ok {
		return filepath.Join(cacheDir, fmt.Sprintf("---%s-%s.json", agentName, timestamp))
	}

	return filepath.Join(cacheDir, fmt.Sprintf("%s---%s-%s.json", conversation, agentName, timestamp))
}

func getConversationIndex(conversation string) (int, bool) {
	val, err := strconv.Atoi(conversation)
	if err != nil || val < 0 {
		return 0, false
	}

	return val - 1, true
}

func findHistoryFile(cacheDir string, conversation string) (string, error) {
	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		return "", err
	}

	type fileEntry struct {
		name    string
		modTime time.Time
	}

	index, isIndex := getConversationIndex(conversation)

	var files []fileEntry

	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".json") {
			info, err := entry.Info()
			if err != nil {
				continue
			}
			files = append(files, fileEntry{
				name:    entry.Name(),
				modTime: info.ModTime(),
			})
		}
	}

	if len(files) == 0 {
		return "", fmt.Errorf("no history files found")
	}

	if isIndex {
		sort.Slice(files, func(i, j int) bool {
			return files[i].modTime.After(files[j].modTime)
		})

		if index < 0 || index >= len(files) {
			return "", fmt.Errorf("history file index %d out of range (0-%d)", index, len(files)-1)
		}

		return filepath.Join(cacheDir, files[index].name), nil
	} else {
		// Read the conversation ID from the json file and return file with that id
		for _, file := range files {
			if strings.HasPrefix(file.name, conversation+"---") {
				return filepath.Join(cacheDir, file.name), nil
			}
		}

		return "", fmt.Errorf("no history file found for conversation %s", conversation)
	}
}

func getHistoryFilePath(cacheDir string, opts *CLIOptions) (string, bool) {
	if !opts.ContinueChat && !opts.RetryChat {
		cacheDir = setupCacheDirWithFallback()
		return createNewHistoryFile(cacheDir, opts.AgentName, opts.Conversation), false
	}

	if filePath, err := findHistoryFile(cacheDir, opts.Conversation); err == nil {
		return filePath, true
	}

	cacheDir = setupCacheDirWithFallback()
	return createNewHistoryFile(cacheDir, opts.AgentName, opts.Conversation), false
}

// Read stdin if exists. Used to detect if input is piped and read it if so
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

func readUserInput(prompt string, multiline bool) (string, error) {
	reader := bufio.NewReader(os.Stdin)
	if prompt != "" {
		color.New(color.FgBlue).Fprint(os.Stderr, prompt)
		color.New(color.FgHiWhite, color.Italic).Fprint(os.Stderr, " (ctrl+d on empty line to complete)\n")
	}

	var result strings.Builder

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err.Error() == "EOF" {
				// Ctrl+D pressed
				break
			}
			return "", err
		}

		// Return if we just want a single line
		if !multiline {
			return line, nil
		}

		// Remove the trailing newline and add to result
		line = strings.TrimSuffix(line, "\n")

		if result.Len() > 0 {
			result.WriteByte('\n')
		}
		result.WriteString(line)

		// Check if line is empty and we got EOF (Ctrl+D)
		if line == "" {
			nextByte, err := reader.ReadByte()
			if err != nil && err.Error() == "EOF" {
				break
			}
			if err == nil {
				// Put the byte back by creating a new reader with it
				result.WriteByte('\n')
				remaining, _ := reader.ReadString('\n')
				result.WriteString(string(nextByte) + remaining)
			}
		}
	}

	return result.String(), nil
}

// getSortedHistoryFiles retrieves and sorts history files by modification time.
func getSortedHistoryFiles() ([]string, map[string]os.FileInfo, error) {
	cacheDir, err := setupCacheDir()
	if err != nil {
		return nil, nil, err
	}

	// Check if the directory exists
	if _, err := os.Stat(cacheDir); os.IsNotExist(err) {
		return nil, nil, wrapCacheError("access", cacheDir, fmt.Errorf("directory does not exist"))
	}

	// Read all .json files in the directory
	files, err := os.ReadDir(cacheDir)
	if err != nil {
		return nil, nil, wrapCacheError("read", cacheDir, err)
	}

	historyItems := make(map[string]os.FileInfo) // Store file info to sort by mod time later

	for _, file := range files {
		if !file.IsDir() && strings.HasSuffix(file.Name(), ".json") {
			info, err := file.Info()
			if err != nil {
				continue // Skip files we can't get info for
			}
			historyItems[file.Name()] = info
		}
	}

	if len(historyItems) == 0 {
		return nil, nil, wrapCacheError("find history files", cacheDir, fmt.Errorf("no history files found"))
	}

	// Sort files by modification time (most recent first)
	sortedFiles := make([]string, 0, len(historyItems))
	for name := range historyItems {
		sortedFiles = append(sortedFiles, name)
	}
	// Custom sort function
	sort.Slice(sortedFiles, func(i, j int) bool {
		return historyItems[sortedFiles[i]].ModTime().After(historyItems[sortedFiles[j]].ModTime())
	})

	return sortedFiles, historyItems, nil
}

func parseModel(modelStr string, agent Agent, config *Config) (provider string, model string, info providerInfo) {
	if modelStr == "" {
		if agent.DefaultModel != "" {
			modelStr = agent.DefaultModel
		} else if config.Settings.DefaultModel != "" {
			modelStr = config.Settings.DefaultModel
		} else {
			// Fallback to default model if nothing is specified
			modelStr = defaultModel
		}
	}

	// Check if the model string is an alias
	if config != nil {
		if aliasedModel, ok := config.ModelAliases[modelStr]; ok {
			modelStr = aliasedModel
		}
	}

	parts := strings.SplitN(modelStr, "/", 2)
	if len(parts) != 2 {
		log.Fatalf("invalid model format %q - must be provider/model", modelStr)
	}

	provider = parts[0]
	model = parts[1]

	// Start with default provider info
	switch provider {
	case "openai":
		info = providerInfo{
			baseURL:     "https://api.openai.com/v1",
			apiKeyEnvar: "OPENAI_API_KEY",
		}
	case "ollama":
		host := os.Getenv("OLLAMA_HOST")
		if host == "" {
			host = "http://localhost:11434"
		}

		if !strings.HasPrefix(host, "http://") && !strings.HasPrefix(host, "https://") {
			host = "http://" + host
		}

		if !strings.HasSuffix(host, "/v1") {
			host = strings.TrimSuffix(host, "/") + "/v1"
		}

		info = providerInfo{
			baseURL:          host,
			apiKeyEnvar:      "OLLAMA_API_KEY",
			apiKeyCanBeEmpty: true, // Ollama does not require an API key by default
		}
	case "openrouter":
		info = providerInfo{
			baseURL:     "https://openrouter.ai/api/v1",
			apiKeyEnvar: "OPENROUTER_API_KEY",
		}
	case "groq":
		info = providerInfo{
			baseURL:     "https://api.groq.com/openai/v1",
			apiKeyEnvar: "GROQ_API_KEY",
		}
	case "github":
		info = providerInfo{
			baseURL:     "https://models.inference.ai.azure.com",
			apiKeyEnvar: "GITHUB_MODELS_API_KEY",
		}
	case "copilot":
		info = providerInfo{
			baseURL:     "https://api.githubcopilot.com",
			apiKeyEnvar: "COPILOT_API_KEY",
			additionalHeaders: map[string]string{
				"Content-Type":           "application/json",
				"Copilot-Integration-Id": "vscode-chat",
			},
		}
	}

	// Override with config if present
	if config != nil {
		if providerCfg, ok := config.Providers[provider]; ok {
			// Only override non-empty values
			if providerCfg.BaseURL != "" {
				info.baseURL = providerCfg.BaseURL
			}

			if providerCfg.APIKeyEnvar != "" {
				info.apiKeyEnvar = providerCfg.APIKeyEnvar
			}

			if len(providerCfg.AdditionalHeaders) > 0 {
				if info.additionalHeaders == nil {
					info.additionalHeaders = make(map[string]string)
				}

				for key, value := range providerCfg.AdditionalHeaders {
					info.additionalHeaders[key] = value
				}
			}
		}
	}

	return provider, model, info
}
