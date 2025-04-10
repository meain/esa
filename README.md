# ESA

<img src="https://github.com/user-attachments/assets/5c2915ab-4a8e-4b49-b3b6-394d5644dac2" alt="Mascot" width="300" align="right"/>

ESA is an AI-powered command-line tool that lets you control your system using natural language. It translates your plain English commands into system actions by connecting Large Language Models (LLMs) with your custom-defined functions.

## Features

- Natural language interface to system commands
- Support for multiple LLM providers (OpenAI, Groq, local models, etc.)
- Custom function definitions using TOML configuration
- Conversation history and continuation
- Built-in safety controls and command confirmation levels
- Extensible agent system for specialized tasks

## Quick Start

1. Install ESA (requires Go 1.21+):

   ```bash
   go install github.com/meain/esa@latest
   ```

2. Set up your OpenAI API key:

   ```bash
   export OPENAI_API_KEY="your-key-here"
   ```

3. Try your first command:
   ```bash
   esa what time is it
   esa will it rain today
   ```

> The source for the default agent can be found [here](https://github.com/meain/esa/blob/master/builtins/default.toml)

## Basic Usage

ESA uses configuration files in `~/.config/esa/agents/` to define what commands it can execute. The default agent (`default.toml`) provides basic capabilities, and you can create specialized agents for specific tasks.

To use the application with the default agent, run:

```bash
esa [--debug] [--agent <path>] [--ask <level>] "<command>"
```

To list all available agents:

```bash
esa list-agents
```

To see details about a specific agent:

```bash
esa show-agent +<agent-name>
```

To list saved conversation histories:

```bash
esa list-history
```

To view a specific conversation history (e.g., the 3rd most recent):

```bash
esa [--output <format>] show-history 3 # format can be text, markdown, or json
```

> NOTE: `--output` flag should come before show-history

You can use different agents by using the `+` syntax followed by the agent name:

```bash
esa +jira "list all open issues"     # Uses ~/.config/esa/agents/jira.toml
esa +k8s "show pod status"           # Uses ~/.config/esa/agents/k8s.toml
esa +commit "summarize changes"      # Uses ~/.config/esa/agents/commit.toml
```

Each agent is defined by its own TOML configuration file in `~/.config/esa/agents`. The agent name corresponds to the filename (without the .toml extension). You can create your own agents by defining custom TOML configuration files in this directory. Several example agent configurations are included in the repository under `examples/`.

You can study these examples to learn how to structure your own agents, or copy and modify them for your needs. Each example demonstrates different patterns like:

- Safe vs unsafe command handling
- Parameter validation
- Input/output processing
- Tool integration (git, kubectl, etc)
- Complex workflows

You can also create a new agent configuration using the `+new` syntax:

```bash
esa +new "Create a coding assistant with read_file and list_files functions"
```

It will output a agent config file which you can use for a coding assistant agent.

The available flags are:

- `--debug`: Enables debug mode, printing additional information about the assistant's response and function execution.
- `--agent <path>`: Specifies the path to the agent configuration file. Defaults to `~/.config/esa/agents/default.toml`.
- `--ask <level>`: Specifies the confirmation level for command execution. Options are `none`, `unsafe`, and `all`. Default is `none`.
- `-c, --continue`: Continue the last conversation with the assistant.
- `-r, --retry [<new text>]`: Retry the last user message optionally replacing the last user message.
- `--output <format>`: Specifies the output format for `show-history`. Options are `text`, `markdown`, `json`. Default is `text`.

## Configuration

### Configuration

The configuration file at `~/.config/esa/config.toml` allows you to define global settings, model aliases, and additional providers.

### Global Config Structure

```toml
[settings]
show_commands = true      # Always show executed commands
default_model = "openai/gpt-4o-mini" # Default model to use (can be overridden by --model/-m)

[model_aliases]
# Define model aliases for easier reference
gemini = "openrouter/google/gemini-2.5-pro-exp-03-25:free"
4o = "openai/gpt-4o"
mini = "openai/gpt-4o-mini"

[providers]
# Configure additional providers that follow the OpenAI API specification
[providers.custom]
base_url = "https://api.custom-llm.com/v1"    # API endpoint that follows OpenAI spec
api_key_env = "CUSTOM_API_KEY"                # Environment variable for API key
```

When specifying models, you can use either:

- The provider/model format (e.g., "openai/gpt-4")
- A defined alias (e.g., "4o" for "openai/gpt-4o")

Built-in providers:

- `openai`: OpenAI models (gpt-4, gpt-3.5-turbo, etc)
- `openrouter`: Access to various models through OpenRouter
- `groq`: Groq's hosted models
- `github`: GitHub's models through Azure inference
- `ollama`: Local models through Ollama (http://localhost:11434)
- Custom providers can be added via config

The model can be specified in three ways (in order of precedence):

1. Command line flag: `--model/-m`
2. Default model in config: `settings.default_model`
3. Built-in default: "openai/gpt-4o-mini"

For each provider, the system will automatically use the appropriate API key from the environment:

- OpenAI: OPENAI_API_KEY
- Azure: AZURE_OPENAI_API_KEY
- OpenRouter: OPENROUTER_API_KEY
- Groq: GROQ_API_KEY
- GitHub: GITHUB_MODELS_API_KEY
- Custom providers: As specified in their config

To use a custom provider:

1. Add the provider configuration in config.toml
2. Set the appropriate API key in your environment
3. Use the provider with either the full name or an alias

Example using a custom provider:

```bash
# In ~/.config/esa/config.toml
[providers.localai]
base_url = "http://localhost:8080/v1"
api_key_env = "LOCALAI_API_KEY"

# In shell
export LOCALAI_API_KEY="your-key"
esa --model "localai/llama2" "your command"
```

### Confirmation Levels

The `--ask` flag allows you to specify the level of confirmation required before executing commands. The available options are:

- `none`: No confirmation is required.
- `unsafe`: Confirmation is required for commands marked as non-safe.
- `all`: Confirmation is required for all commands.

### Agent Configuration File Structure

The default agent configuration file is located at `~/.config/esa/agents/default.toml`. It is a TOML file that defines the functions available to the assistant and its behavior. Here's the detailed structure:

#### System Prompt

```toml
system_prompt = """
You are a helpful assistant. Define your role, behavior, and any specific instructions here.
Keep your responses short and to the point.
"""
```

#### Function Definitions

Functions are defined as an array of TOML tables. Each function includes:

```toml
[[functions]]
name = "function_name"              # Name of the function
description = "function details"    # Description for the LLM
command = "command {{param}}"       # Command template with parameter placeholders
safe = true                        # Whether the function is considered safe (optional)

[[functions.parameters]]
name = "param"                     # Parameter name
type = "string"                    # Parameter type (string, number, boolean)
description = "param details"      # Parameter description
required = true                    # Whether the parameter is required
```

Key components:

- `name`: The name of the function that the LLM will call
- `description`: Detailed description helping the LLM understand when to use the function
- `command`: The actual command to execute, using `{{param}}` syntax for parameter substitution
- `safe`: Boolean flag indicating if the function is safe to execute without confirmation
- `parameters`: Array of parameter definitions that the function accepts
  - `name`: Parameter name used in command templating
  - `type`: Parameter data type
  - `description`: Parameter description for the LLM
  - `required`: Whether the parameter must be provided

Example: A function to read a file's contents:

```toml
[[functions]]
name = "read_file"
description = "Read the content of a file"
command = "cat '{{file}}'"
safe = true

[[functions.parameters]]
name = "file"
type = "string"
description = "Path to the file"
required = true
```

Other configuration options:

- `ask`: The confirmation level for command execution (none/unsafe/all)
- `system_prompt`: The main prompt that defines the assistant's behavior

#### Safe Property

The `safe` property in the function configuration determines whether a command is considered safe or potentially unsafe. If `safe` is set to `true`, the command will be executed without confirmation when the `--ask` level is set to `unsafe`. If `safe` is set to `false` or not specified, confirmation will be required.

The capabilities of your assistant are easily extendable by adding more functions to the agent config file.

With the provided example agent config you could execute things like:

```bash
esa "what is a harpoon" # answer basic questions
esa "who is esa?" # as about itself
esa "set an alarm for 10:30am"
esa "sen alarm for 1 hour from now"
esa "open golang playground" # works if the llm knows about it
esa "reduce brightness"
esa "increase brightness if it is after 2PM" # is it pointless, yes but it works
esa "send an email to user@provider.com reminding to take an umbrella if it will rain tomorrow" # something complex
```

For more complex tasks, it is advisable to use larger models like
`gpt-4o`, while `gpt-4o-mini` is sufficient for simpler tasks. Please
note that function calling may not perform reliably with smaller local
models, such as the 8b version of llama3.2.

```bash
cat main.go |
  esa 'Summarize the provided code and send an email to mail@meain.io. Send the email only if it will not rain tonight. Also send a notification after that.'
```

_You can find examples of the functions in the `functions` folder._

> CAUTION: Be careful with the functions you add. If you let it
> overwrite files or run commands with, it could be dangerous. Just
> because you can do something doesn't mean you should.

#### Example: Coding Assistant

Here's an example of how to configure a coding assistant that can answer queries about your code:

```toml
system_prompt = "You are a helpful assistant. Keep your responses short and to the point."

[[functions]]
name = "list_files"
description = "List files in a directory"
command = "ls '{{path}}'"

[[functions.parameters]]
name = "path"
type = "string"
description = "Path to the directory"
required = true

[[functions]]
name = "read_file"
description = "Read the content of a file"
command = "cat '{{file}}'"

[[functions.parameters]]
name = "file"
type = "string"
description = "Path to the file"
required = true

[[functions]]
name = "tree"
description = "Show the tree structure of a directory"
command = "tree '{{path}}'"

[[functions.parameters]]
name = "path"
type = "string"
description = "Path to the directory"
required = true
```

With this configuration, you can ask questions like:

```bash
esa "list files in the current directory"
esa "show me the content of main.go"
esa "show me the tree structure of the functions directory"
```

## Notes

You can connect it to whisper and a voice model to make it a voice assistant.

```bash
,transcribe-audio | xargs -I{} esa "{}" | say
```

_`,transcribe-audio` is a small script that I have that uses whisper. You can find it [here](https://github.com/meain/dotfiles/blob/master/scripts/.local/bin/random/%2Ctranscribe-audio)._
