# ESA Coding Guidelines

ESA is a single-package Go CLI tool (`package main`) for personalized AI agents.
All source files live at the repository root -- there are no sub-packages.
Requires Go 1.23.6+ (toolchain go1.24.3 per `go.mod`).

## Build & Test Commands

```bash
# Build
go build

# Run all tests
go test -v ./...

# Run a single test function
go test -v -run TestFunctionName

# Run a specific subtest
go test -v -run TestValidateAgent/valid_agent

# Run multiple tests by regex
go test -v -run "TestAgent|TestParseModel"

# Install dependencies
go mod download

# Lint (standard Go tools only, no golangci-lint configured)
gofmt -l .
go vet ./...

# Run the binary
./esa "your prompt"
./esa +agent_name "your prompt"
./esa --serve --port 8080    # web server mode
./esa --repl                 # interactive REPL
./esa --debug "prompt"       # debug LLM requests/responses
./esa --show-commands "prompt"
./esa --show-tool-calls "prompt"
./esa -c "follow up"         # continue last conversation
./esa -C session-id "prompt" # continue specific conversation
./esa -r                     # retry last conversation
```

CI runs `go test -v ./...` on push/PR to master. No race detection or coverage in CI.

## Project Structure

| File | Purpose |
|------|---------|
| `main.go` | Entry point -- creates and executes Cobra root command |
| `cli.go` | CLI flags (`CLIOptions` struct), Cobra command setup |
| `agent.go` | Agent TOML loading, validation, struct definitions |
| `agent_util.go` | Agent string parsing (`+name`, paths, builtins) |
| `application.go` | Core orchestrator: conversation flow, LLM streaming, tool execution |
| `llm.go` | LLM provider abstraction (`LLMClient`, `LLMStream` interfaces) and OpenAI wrapper |
| `anthropic.go` | Native Anthropic Messages API client with SSE streaming |
| `client.go` | LLM client instantiation: routes to Anthropic or OpenAI-compatible providers |
| `config.go` | Global config at `~/.config/esa/config.toml` |
| `function.go` | Function execution: JSON arg parsing, template substitution, shell exec |
| `print.go` | Output formatting (text, markdown, JSON, HTML) |
| `repl.go` | Interactive REPL with `/help`, `/model`, `/agent`, `/editor`, `/config` commands |
| `server.go` | HTTP + WebSocket web server with embedded UI |
| `stats.go` | Usage statistics collection and display |
| `utils.go` | Shared utilities (path expansion, history files, providers) |
| `builtins.go` | `//go:embed` for builtin agent TOML files |
| `web_embed.go` | `//go:embed` for web UI assets |
| `builtins/` | Embedded agent configs: `default.toml`, `new.toml`, `auto.toml` |
| `examples/` | Sample agent TOML configurations |
| `docs/` | Detailed documentation (agent creation) |
| `web/` | Embedded web interface (HTML/JS/CSS) |

## Code Style

### Formatting
- **Tabs for indentation** (standard `gofmt`)
- No explicit line length limit
- Run `gofmt` before committing

### Imports
Two groups separated by a blank line -- standard library first, then third-party:
```go
import (
    "fmt"
    "os"
    "strings"

    "github.com/fatih/color"
    "github.com/spf13/cobra"
)
```
No third group needed since everything is `package main`.

### Naming Conventions
- **Exported constants**: PascalCase with descriptive prefixes (`DefaultConfigPath`, `DefaultAgentsDir`)
- **Unexported constants**: camelCase (`historyTimeFormat`, `defaultModel`, `maxRetryCount`)
- **Error message constants**: camelCase with `err` prefix (`errFailedToLoadConfig`)
- **Types/Structs**: PascalCase nouns; config types end with `Config` (`FunctionConfig`, `ProviderConfig`), info types with `Info`, stats types with `Stats`
- **Unexported types**: camelCase (`providerInfo`, `webSession`)
- **Exported functions**: PascalCase; constructors use `New` prefix (`NewApplication`, `NewAgent`)
- **Unexported functions**: camelCase; use prefixes: `handle` for handlers, `print` for output, `setup` for initialization, `is`/`needs` for boolean checks
- **Method receivers**: Short names -- `app`, `c`, `m`, `s`, `sc`
- **Struct fields**: PascalCase for serialized fields with `toml:"name"` or `json:"name"` tags; camelCase for internal fields (`baseURL`, `apiKeyEnvar`)
- **Files**: lowercase, underscores for multi-word (`agent_util.go`)

### Error Handling
- Return `error` as the last return value
- Check errors immediately, return early:
  ```go
  if err != nil {
      return nil, fmt.Errorf("failed to load agent %s: %w", name, err)
  }
  ```
- Wrap errors with `fmt.Errorf("context: %w", err)` using `%w` verb
- Define error message constants for repeated strings:
  ```go
  const errFailedToLoadConfig = "failed to load global config"
  ```
- Custom error types for structured errors (`CacheError` with `Operation`, `Path`, `Err` fields)
- `log.Fatalf` for unrecoverable runtime errors; `os.Exit(1)` only in `main.go`
- Silent `continue` acceptable in non-critical loops (stats processing, file iteration)

### Constructors and Cleanup
- Constructors return pointers: `func NewApplication(opts *CLIOptions) (*Application, error)`
- Resource-allocating functions return cleanup funcs:
  ```go
  func (app *Application) initializeRuntime() (cleanup func(), err error) { ... }
  // caller: defer cleanup()
  ```

### Method Receivers
- All struct methods use pointer receivers (no value receivers)
- Mutex-protected structs use `Internal` suffix for lock-holder methods:
  ```go
  func (c *Client) Stop() error { c.mu.Lock(); defer c.mu.Unlock(); c.stopInternal() }
  func (c *Client) stopInternal() { ... }
  ```

### Comments
- Exported items: doc comments starting with the name (`// LoadConfig loads the configuration...`)
- Inline comments for non-obvious logic
- Section comments in long functions: `// --- Print Header ---`
- `TODO:` and `FIXME:` with context for known issues

## Testing Conventions

All tests are in `package main` at root. No test sub-packages, no `testdata/` directory.

### Patterns
- **Table-driven tests** with `t.Run()` subtests (strongly preferred):
  ```go
  tests := []struct {
      name    string
      input   string
      want    string
      wantErr bool
  }{ ... }
  for _, tt := range tests {
      t.Run(tt.name, func(t *testing.T) { ... })
  }
  ```
- Test slice always named `tests`, iteration variable always `tt`
- First struct field is always `name` for subtest identification
- Expected values use `want` prefix (`wantProvider`, `wantErr`, `wantIndex`)
- **No assertion library** -- use `t.Errorf` for failures, `t.Fatalf` for setup errors:
  ```go
  if got != tt.want {
      t.Errorf("funcName() = %v, want %v", got, tt.want)
  }
  ```

### Test Setup
- `t.TempDir()` for temporary directories (preferred over manual `os.MkdirTemp`)
- `t.Setenv()` for environment variables (auto-cleanup)
- Save/restore global state with `defer` when modifying package-level vars
- All test data is inline (TOML strings, struct literals) -- no fixture files

## Key Dependencies

| Package | Purpose |
|---------|---------|
| `github.com/spf13/cobra` | CLI framework |
| `github.com/BurntSushi/toml` | TOML config parsing |
| `github.com/sashabaranov/go-openai` | OpenAI-compatible LLM client |
| `github.com/charmbracelet/glamour` | Terminal markdown rendering |
| `github.com/fatih/color` | Terminal color output |
| `github.com/gorilla/websocket` | WebSocket for web server |
| `golang.org/x/term` | Terminal raw mode input |

## Adding CLI Features

1. Add field to `CLIOptions` struct in `cli.go`
2. Register the flag in `createRootCommand()`
3. Handle in the main `RunE` function
4. Write table-driven tests following existing patterns
