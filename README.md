# ESA

<img src="https://github.com/user-attachments/assets/5c2915ab-4a8e-4b49-b3b6-394d5644dac2" alt="Mascot" width="300" align="right"/>

**ESA** is an AI-powered command-line tool that lets you create powerful personalized small agents. By connecting Large Language Models (LLMs) with shell scripts as functions, ESA lets you control your system, automate tasks, and query information using plain English commands.

## ✨ Features

- **Natural Language Interface**: Execute system commands using conversational language
- **Multi-Provider LLM Support**: Works with OpenAI, Groq, Ollama, OpenRouter, GitHub Models, and custom providers
- **Extensible Agent System**: Create specialized agents for different domains (DevOps, Git, coding, etc.)
- **Function-Based Architecture**: Define custom commands via TOML configuration files
- **Conversation History**: Continue and retry conversations with full context preservation
- **Safety Controls**: Built-in confirmation levels and safe/unsafe command classification
- **Flexible Output**: Support for text, markdown, and JSON output formats

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
esa "what time is it"
esa "what files are in the current directory"
esa "calculate 15% tip on $47.50"

# More complex tasks
esa "will it rain today"
esa "set an alarm for 2 hours from now"

# Use esa's investigations to learn
esa --show-history 7 | esa "convert this interaction into a doc"
```

The operations you can do and questions you can ask depends on the agent config. While the real power of ESA comes from the fact that you can create custom agents, the default builtin agent can do some basic stuff.

> 💡 **Tip**: The default agent provides basic system functions. See the [Agent Creation Guide](./docs/agents.md) to create specialized agents.

### Using Specialized Agents

ESA becomes powerful when you use specialized agents. The following are things you can do with the example agents provided in `./example`. Use the `+agent-name` syntax:

```bash
# Kubernetes operations
esa +k8s "what is the secret value that starts with AUD_"
esa +k8s "how to get latest cronjob pod logs"

# JIRA integration
esa +jira "list all open issues assigned to me"
esa +jira "pending issues related to authentication"

# Git operations with the commit agent
git diff --staged | esa +commit
```

### Conversation Features

```bash
# Continue the last conversation
esa -c "and what about yesterday's weather"

# Retry the last command with modifications
esa -r "make it more detailed"

# View conversation history
esa --list-history
esa --show-history 3
esa --show-history 1 --output json
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

## 🎯 Available Example Agents

ESA includes several example agents you can use or customize:

| Agent | Purpose | Example Usage |
|-------|---------|---------------|
| **default** | Basic system operations | `esa "what time is it"` |
| **commit** | Git commit message generation | `esa +commit "create commit message"` |
| **k8s** | Kubernetes cluster operations | `esa +k8s "show pod status"` |
| **jira** | JIRA issue management | `esa +jira "list open issues"` |
| **web** | Web development tasks | `esa +web "what is an agent?"` |

See the [`examples/`](examples/) directory for more agent configurations.

## 🔧 Command-Line Options

```bash
# Core options
--model, -m <model>      # Specify model (e.g., "openai/gpt-4")
--config <path>          # Path to config file
--debug                  # Enable debug output
--ask <level>            # Confirmation level: none/unsafe/all

# Conversation management
-c, --continue           # Continue last conversation
-r, --retry              # Retry last command (optionally with new text)

# Output and display
--show-commands          # Show executed commands
--hide-progress          # Disable progress indicators
--output <format>        # Output format for --show-history: text/markdown/json

# Information commands (require arguments)
--list-agents            # Show all available agents
--list-history           # Show conversation history
--show-history <index>   # Display specific conversation (e.g., --show-history 1)
--show-agent <agent>     # Show agent details (e.g., --show-agent +coder)
```

### Examples

```bash
# Basic usage
esa "What is the weather like?"
esa +coder "How do I write a function in Go?"

# Agent and history management
esa --list-agents
esa --show-agent +coder
esa --show-agent ~/.config/esa/agents/custom.toml
esa --list-history
esa --show-history 1
esa --show-history 1 --output json

# Conversation flow
esa --continue "tell me more about that"
esa --retry "make it shorter"
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

| Provider | Models | API Key Environment |
|----------|--------|-------------------|
| **OpenAI** | GPT-4, GPT-3.5, etc. | `OPENAI_API_KEY` |
| **Groq** | Llama, Mixtral models | `GROQ_API_KEY` |
| **OpenRouter** | Various models | `OPENROUTER_API_KEY` |
| **GitHub** | Azure-hosted models | `GITHUB_MODELS_API_KEY` |
| **Ollama** | Local models | `OLLAMA_API_KEY` (optional) |
| **Custom** | OpenAI-compatible APIs | Configurable |

## 📚 Custom Agents

ESA's power comes from custom agents. See the [Agent Creation Guide](./docs/agents.md) for detailed instructions on:

- Writing agent configuration files
- Defining custom functions
- Parameter handling and validation
- Advanced templating features
- Best practices and examples