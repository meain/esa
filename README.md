# ESA

![Screencast GIF](https://github.com/user-attachments/assets/99abe3a1-c620-4909-a503-22b4d70a5cac)

<img src="https://github.com/user-attachments/assets/5c2915ab-4a8e-4b49-b3b6-394d5644dac2" alt="Mascot" width="300" align="right"/>

**ESA** is an AI-powered command-line tool that lets you create powerful personalized small agents. By connecting Large Language Models (LLMs) with shell scripts as functions, ESA lets you control your system, automate tasks, and query information using plain English commands.

## ✨ Features

- **Natural Language Interface**: Execute system commands using conversational language
- **Multi-Provider LLM Support**: Works with OpenAI, Groq, Ollama, OpenRouter, GitHub Models, and custom providers
- **Extensible Agent System**: Create specialized agents for different domains (DevOps, Git, coding, etc.)
- **Function-Based Architecture**: Define custom commands via TOML configuration files
- **MCP Server Integration**: Connect with Model Context Protocol servers for enhanced capabilities
- **Conversation History**: Continue and retry conversations with full context preservation
- **Safety Controls**: Built-in confirmation levels and safe/unsafe command classification
- **Flexible Output**: Support for text, markdown, and JSON output formats

| <video src="https://github.com/user-attachments/assets/cda852f3-edc2-4612-9920-4c53dc76a9a8"></video> |
| ----------------------------------------------------------------------------------------------------- |
| **Tree-Sitter agent**                                                                                 |

| <video src="https://github.com/user-attachments/assets/ce584248-c50d-456d-aa35-4a080790c2a4"></video> |
| ----------------------------------------------------------------------------------------------------- |
| **Coder agent**                                                                                       |

> See [meain/esa#4](https://github.com/meain/esa/issues/4) for more demos

## 🚀 Quick Start

### 1. Installation

**Option A: Using Go**

```bash
go install github.com/meain/esa@latest
```

**Option B: Clone and Build**

```bash
git clone https://github.com/meain/esa.git
cd esa
go build -o esa
```

### 2. Setup API Key

ESA works with multiple LLM providers. Set up at least one:

```bash
# OpenAI
export OPENAI_API_KEY="your-openai-key"

# Or use other providers
export GROQ_API_KEY="your-groq-key"
export OLLAMA_API_KEY=""  # Leave empty for local Ollama
```

### 3. Try Your First Commands

```bash
# Get help and see all available options
esa --help

# Basic queries
esa what time is it
esa what files are in the current directory
esa "calculate 15% tip on $47.50"

# More complex tasks
esa will it rain today
esa set an alarm for 2 hours from now

# Use esa's investigations to learn
esa --show-history 7 | esa convert this interaction into a doc
```

The operations you can do and questions you can ask depends on the agent config. While the real power of ESA comes from the fact that you can create custom agents, the default builtin agent can do some basic stuff.

> 💡 **Tip**: The default agent provides basic system functions. See the [Agent Creation Guide](./docs/agents.md) to create specialized agents.

### Built-in Agents

ESA comes with several built-in agents that are always available:

| Agent        | Description                                                   | Usage                          |
| ------------ | ------------------------------------------------------------- | ------------------------------ |
| **+default** | Basic system operations like file management and calculations | `esa +default what time is it` |
| **+new**     | Creates new custom agents                                     | `esa +new create a git agent`  |
| **+auto**    | Automatically selects the right agent based on your query     | `esa +auto analyze this code`  |

> 💡 **Tip**: You can override built-in agents by creating your own agent with the same name in `~/.config/esa/agents/`. When a name conflict occurs, your custom agent will be used instead of the built-in one.

### Using Specialized Agents

ESA becomes powerful when you use specialized agents. The following are things you can do with the example agents provided in `./example`. Use the `+agent-name` syntax:

```bash
# Kubernetes operations
esa +k8s what is the secret value that starts with AUD_
esa +k8s how to get latest cronjob pod logs

# JIRA integration
esa +jira list all open issues assigned to me
esa +jira pending issues related to authentication

# Git operations with the commit agent
git diff --staged | esa +commit
```

### Conversation Features

```bash
# Continue the last conversation
esa -c "and what about yesterday's weather"

# Retry the last command with modifications
esa -r make it more detailed

# View conversation history
esa --list-history
esa --show-history 3
esa --show-history 1 --output json

# Show last output of a previous interaction
esa --show-output 1

# Display agent and model statistics
esa --show-stats
```

### REPL Mode (Interactive Sessions)

ESA supports REPL (Read-Eval-Print Loop) mode for interactive conversations. This is perfect for extended sessions where you want to have back-and-forth conversations with your AI assistant.

```bash
# Start REPL mode
esa --repl

# Start REPL with an initial query
esa --repl what time is it

# Start REPL with a specific agent
esa --repl +k8s show me all pods
```

#### REPL Commands

Once in REPL mode, you can use special commands:

```bash
# Get help
you> /help

# Exit the session
you> /exit
you> /quit

# Show current configuration
you> /config

# View or change the model
you> /model                    # Show current model
you> /model openai/gpt-4o     # Switch to a different model
you> /model mini              # Use a model alias
```

#### REPL Features

- **Persistent Context**: The conversation continues across multiple inputs
- **Agent Selection**: Use `+agent` syntax in your initial query or when starting REPL
- **Model Switching**: Change models mid-conversation with `/model`
- **Configuration Display**: View current settings with `/config`
- **Multi-line Input**: Press enter twice to send your message
- **History Preservation**: All REPL conversations are saved and can be viewed later

#### Example REPL Session

```bash
$ esa --repl "+k8s"
[REPL] Starting interactive mode
- '/exit' or '/quit' to end the session
- '/help' for available commands
- Press enter twice to send your message.

you> show me all pods in the default namespace

esa> Here are all the pods in the default namespace:
[... pod listing ...]

you> what about in the kube-system namespace

esa> Here are the pods in the kube-system namespace:
[... pod listing ...]

you> /model openai/gpt-4o
[REPL] Model updated to: openai/gpt-4o

you> can you explain what each of these pods does

esa> [... detailed explanation ...]

you> /exit
[REPL] Goodbye!
```

### Working with Different Models

```bash
# Use a specific model
esa --model "openai/o3" "complex reasoning task"
esa --model "groq/llama3-70b" "quick question"

# Use model aliases (defined in config)
esa --model "mini" "your command"
```

## 🛠️ Configuration

### Global Configuration

Create `~/.config/esa/config.toml` for global settings:

```toml
[settings]
show_commands = true                     # Show executed commands
default_model = "openai/gpt-4o-mini"    # Default model

[model_aliases]
# Create shortcuts for frequently used models
4o = "openai/gpt-4o"
mini = "openai/gpt-4o-mini"
groq = "groq/llama3-70b-8192"
local = "ollama/llama3.2"

[providers.localai]
# Add custom OpenAI-compatible providers
base_url = "http://localhost:8080/v1"
api_key_env = "LOCALAI_API_KEY"
```

### Agent Management

```bash
# List available agents
esa --list-agents

# View agent details and available functions
esa --show-agent +k8s
esa --show-agent +commit
esa --show-agent ~/.config/esa/agents/custom.toml

# Agents are stored in ~/.config/esa/agents/
# Each agent is a .toml file defining its capabilities
```

## 🎯 Available Agents

### Built-in Agents

ESA includes several built-in agents that are always available:

| Agent        | Purpose                                                         | Example Usage                    |
| ------------ | --------------------------------------------------------------- | -------------------------------- |
| **+default** | Basic system operations (files, calculations, weather)          | `esa +default "what time is it"` |
| **+new**     | Creates new custom agent configurations                         | `esa +new "create a git agent"`  |
| **+auto**    | Automatically selects the appropriate agent based on your query | `esa +auto "analyze this code"`  |

> 💡 **Note**: You can override built-in agents by creating your own agent with the same name in `~/.config/esa/agents/`. Your custom agent will take precedence over the built-in one.

### Example Agents

ESA also includes several example agents you can use or customize:

| Agent      | Purpose                       | Example Usage                         |
| ---------- | ----------------------------- | ------------------------------------- |
| **commit** | Git commit message generation | `esa +commit "create commit message"` |
| **k8s**    | Kubernetes cluster operations | `esa +k8s "show pod status"`          |
| **jira**   | JIRA issue management         | `esa +jira "list open issues"`        |
| **web**    | Web development tasks         | `esa +web "what is an agent?"`        |

Each agent can specify a preferred model optimized for its tasks. For
example, lightweight agents might use `gpt-4.1-nano` for quick
responses, while complex analysis agents might use `o3` for better
reasoning.

See the [`examples/`](examples/) directory for more agent configurations.

## 🔧 Command-Line Options

```bash
# Core options
--model, -m <model>      # Specify model (e.g., "openai/gpt-4")
--agent <path>           # Path to agent config file
--config <path>          # Path to config file
--debug                  # Enable debug output
--ask <level>            # Confirmation level: none/unsafe/all
--repl                   # Start interactive REPL mode

# Conversation management
-c, --continue           # Continue last conversation
-r, --retry              # Retry last command (optionally with new text)

# Output and display
--show-commands          # Show executed commands
--show-tool-calls        # Show LLM tool call requests and responses
--hide-progress          # Disable progress indicators
--output <format>        # Output format for --show-history: text/markdown/json

# Information commands
--list-agents            # Show all available agents
--list-history           # Show conversation history
--show-history <index>   # Display specific conversation (e.g., --show-history 1)
--show-output <index>    # Display only last output from conversation (e.g., --show-output 1)
--show-agent <agent>     # Show agent details (e.g., --show-agent +coder)
--show-stats             # Display agent and model statistics
--pretty, -p             # Pretty print markdown output (disables streaming)
```

### Examples

```bash
# Basic usage
esa "What is the weather like?"
esa +coder "How do I write a function in Go?"
esa --agent ~/.config/esa/agents/custom.toml "analyze this code"

# Agent and history management
esa --list-agents
esa --show-agent +coder
esa --show-agent ~/.config/esa/agents/custom.toml
esa --list-history
esa --show-history 1
esa --show-history 1 --output json
esa --show-output 1
esa --show-output 1 --pretty   # Pretty print markdown output

# Conversation flow
esa --continue "tell me more about that"
esa --retry "make it shorter"

# REPL mode
esa --repl                                    # Start interactive mode
esa --repl "what time is it"                  # Start with initial query
esa --repl "+k8s show me all pods"            # Start with specific agent

# Display options
esa --show-commands "list files"              # Show command executions
esa --show-tool-calls "read README.md"        # Show tool calls and results
```

## 📋 Safety and Security

ESA includes several safety mechanisms:

### Confirmation Levels

- **`--ask none`** (default): No confirmation required
- **`--ask unsafe`**: Confirm potentially dangerous commands
- **`--ask all`**: Confirm every command execution

### Function Safety Classification

Functions in agent configurations can be marked as:

- **`safe = true`**: Commands that only read data or perform safe operations
- **`safe = false`**: Commands that modify system state or could be dangerous

### Example: Safe vs Unsafe

```toml
[[functions]]
name = "list_files"
command = "ls {{path}}"
safe = true              # Reading directory contents is safe

[[functions]]
name = "delete_file"
command = "rm {{file}}"
safe = false             # File deletion requires confirmation
```

## 🌐 Supported LLM Providers

| Provider       | Models                 | API Key Environment         |
| -------------- | ---------------------- | --------------------------- |
| **OpenAI**     | GPT-4, GPT-3.5, etc.   | `OPENAI_API_KEY`            |
| **Groq**       | Llama, Mixtral models  | `GROQ_API_KEY`              |
| **OpenRouter** | Various models         | `OPENROUTER_API_KEY`        |
| **GitHub**     | Azure-hosted models    | `GITHUB_MODELS_API_KEY`     |
| **Ollama**     | Local models           | `OLLAMA_API_KEY` (optional) |
| **Custom**     | OpenAI-compatible APIs | Configurable                |

## 🔌 MCP Server Support

ESA supports [Model Context Protocol (MCP)](https://github.com/modelcontextprotocol/spec) servers, allowing you to integrate with external tools and services that implement the MCP specification.

### What is MCP?

MCP is a protocol that allows AI assistants to securely connect with external data sources and tools. MCP servers can provide:

- **File system access** (read/write files, directory operations)
- **Database connectivity** (query databases, execute operations)
- **Web services** (fetch URLs, API integrations)
- **Custom tools** (domain-specific functionality)

### Configuration

Add MCP servers to your agent configuration alongside regular functions:

```toml
name = "Filesystem Agent"
description = "Agent with file system access via MCP"

# MCP Servers with security and function filtering
[mcp_servers.filesystem]
command = "npx"
args = [
    "-y", "@modelcontextprotocol/server-filesystem",
    "/Users/username/Documents"
]
safe = false  # File operations are potentially unsafe
safe_functions = ["read_file", "list_directory"]  # These specific functions are considered safe
allowed_functions = ["read_file", "write_file", "list_directory"]  # Only expose these functions to the LLM

[mcp_servers.database]
command = "uvx"
args = ["mcp-server-postgres", "postgresql://localhost/mydb"]
safe = false  # Database operations are unsafe by default
safe_functions = ["select", "show"]  # Only SELECT and SHOW queries are safe

# Regular functions work alongside MCP servers
[[functions]]
name = "list_files"
description = "List files using shell command"
command = "ls -la {path}"
safe = true
```

### Security and Function Control

MCP servers support the same security model as regular functions:

- **Server-level Safety**: Set `safe = true/false` to mark all functions from a server as safe by default
- **Function-level Safety**: Use `safe_functions = ["func1", "func2"]` to override safety for specific functions
- **Function Filtering**: Use `allowed_functions = ["func1", "func2"]` to limit which functions are exposed to the LLM

### Command and Tool Call Display

Use `--show-commands` to see MCP tool executions:

```bash
esa --show-commands +filesystem "list files in current directory"
# Shows: # filesystem:list_directory({"path": "."})
```

Use `--show-tool-calls` to see the raw tool call requests and responses:

```bash
esa --show-tool-calls +filesystem "read the first 10 lines of README.md"
# Shows detailed JSON of tool call request and response
```

### Usage

MCP tools are automatically discovered and integrated with your agent:

```bash
# MCP tools are prefixed with 'mcp_{server_name}_{tool_name}'
esa +filesystem "read the contents of config.json"
esa +database "show me all users in the database"

# View available MCP tools and their security settings
esa --show-agent examples/mcp.toml

# Use with confirmation and tool visibility
esa --ask unsafe --show-commands +filesystem "write a file"  # See command execution
esa --ask unsafe --show-tool-calls +filesystem "write a file"  # See command and output
```

### Benefits

- **Security**: MCP servers run in isolation with defined permissions and granular safety controls
- **Extensibility**: Easy integration with existing MCP-compatible tools
- **Flexibility**: Combine MCP tools with regular shell functions
- **Standardization**: Use any MCP server implementation
- **Function Control**: Filter and control which MCP functions are available to the LLM
- **Command Visibility**: Full transparency with `--show-commands` flag support

See [`examples/mcp.toml`](examples/mcp.toml) for a complete example.

## 📚 Custom Agents

ESA's power comes from custom agents. See the [Agent Creation Guide](./docs/agents.md) for detailed instructions on:

- Writing agent configuration files
- Defining custom functions
- Parameter handling and validation
- Advanced templating features
- Best practices and examples

## FAQ

<details>
<summary>What all agents do I have?</summary>
I have quite a few personal agents. The ones that I can make public are available in my [dotfiles](https://github.com/meain/dotfiles/tree/master/esa/.config/esa/agents).
</details>

<details>
<summary>How to setup GitHub Copilot</summary>
1. The easiest way to get the Copilot token is to sign in to Copilot from any JetBrains IDE (PyCharm, GoLand, etc).

2. After authentication, locate the configuration file:

   - Linux/macOS: `~/.config/github-copilot/apps.json`
   - Windows: `~\AppData\Local\github-copilot\apps.json`

3. Copy the `oauth_token` value from this file.

4. Set the token as your `COPILOT_API_KEY`:

   ```bash
   export COPILOT_API_KEY=your_oauth_token_here
   ```

Important Note: Tokens created by the Neovim copilot.lua plugin (old `hosts.json`) sometimes lack the needed scopes. If you see "access to this endpoint is forbidden", regenerate the token with a JetBrains IDE.

</details>.
