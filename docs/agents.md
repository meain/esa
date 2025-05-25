# Agent Creation Guide

This guide provides comprehensive instructions for creating custom ESA agents. Agents are TOML configuration files that define specialized AI assistants with custom functions and behaviors.

## 📋 Table of Contents

- [Overview](#overview)
- [Agent Structure](#agent-structure)
- [Basic Agent Example](#basic-agent-example)
- [Configuration Reference](#configuration-reference)
- [Function Definition](#function-definition)
- [Parameter Handling](#parameter-handling)
- [Advanced Features](#advanced-features)
- [Best Practices](#best-practices)
- [Example Agents](#example-agents)
- [Debugging and Testing](#debugging-and-testing)

## Overview

ESA agents are defined in TOML files that specify:
- **System Prompt**: Instructions that guide the AI's behavior
- **Functions**: Command-line tools the agent can execute
- **Parameters**: Input validation and formatting
- **Safety Settings**: Confirmation levels and command classification

Agents are stored in `~/.config/esa/agents/` and can be invoked using the `+agent-name` syntax.

## Agent Structure

Every agent configuration follows this basic structure:

```toml
# Optional: Agent metadata
name = "Agent Name"
description = "Brief description of what this agent does"
default_model = "provider/model-name"

# Core configuration
system_prompt = """Instructions for the AI assistant"""
initial_message = "Default message when no input provided" # optional
ask = "unsafe"  # Confirmation level(global): none, unsafe, all

# Function definitions
[[functions]]
name = "function_name"
description = "What this function does"
command = "shell command with {{parameters}}"
safe = true
# ... additional function properties

[[functions.parameters]]
name = "param_name"
type = "string"
description = "Parameter description"
required = true
```

## Basic Agent Example

Here's a simple file management agent:

```toml
name = "File Manager"
description = "Basic file operations assistant"
default_model = "openai/gpt-4o-mini"

system_prompt = """
You are a file management assistant. Help users with basic file operations
like listing, reading, and organizing files safely.

Keep responses concise and always confirm before performing destructive operations.
"""

ask = "unsafe"

[[functions]]
name = "list_files"
description = "List files in a directory"
command = "ls -la {{path}}"
safe = true

[[functions.parameters]]
name = "path"
type = "string"
description = "Directory path to list (defaults to current directory)"
required = false

[[functions]]
name = "read_file"
description = "Display the contents of a file"
command = "cat {{filename}}"
safe = true

[[functions.parameters]]
name = "filename"
type = "string"
description = "Path to the file to read"
required = true

[[functions]]
name = "create_directory"
description = "Create a new directory"
command = "mkdir -p {{dirname}}"
safe = false

[[functions.parameters]]
name = "dirname"
type = "string"
description = "Name of the directory to create"
required = true
```

Save this as `~/.config/esa/agents/files.toml` and use it with:

```bash
esa +files "is the current directory a go project?"
esa +files "based on README.md how can I use this project"
```

## Configuration Reference

### Agent Properties

| Property | Type | Required | Description |
|----------|------|----------|-------------|
| `name` | string | No | Human-readable agent name |
| `description` | string | No | Brief description for `list-agents` |
| `system_prompt` | string | Yes | Core instructions for the AI |
| `initial_message` | string | No | Default message when no input provided |
| `ask` | string | No | Confirmation level: `none`, `unsafe`, `all` |
| `default_model` | string | No | Preferred model for this agent (e.g., `openai/gpt-4o-mini`) |

### Model Selection Hierarchy

ESA uses the following priority order to determine which model to use:

1. **CLI Model Flag** (`--model` or `-m`) - Highest priority
2. **Agent Default Model** (`agent.toml` → `default_model`)
3. **Global Config Default** (`config.toml` → `settings.default_model`)
4. **Built-in Fallback** (`openai/gpt-4o-mini`) - Lowest priority

This hierarchy allows you to:
- Set reasonable defaults for specific agents
- Override globally via configuration
- Override per-command via CLI flags

**Example Usage:**
```bash
# Uses agent's default model
esa +code-analysis "review this function"

# Overrides with specific model
esa --model "openai/gpt-4" +code-analysis "review this function"
```

### System Prompt Guidelines

The system prompt is crucial for agent behavior. It should:
- Clearly define the agent's role and purpose
- Provide specific instructions for handling tasks
- Include any domain-specific knowledge or constraints
- Mention response style preferences (concise, detailed, etc.)

> **TIP:** While not strictly necessary, providing output examples in XML tags are greatly beneficial to explalin how to format the output to the LLM

**Template Variables in System Prompt:**

You can run bash command to generate output that will be templated out in the system prompt by doing `{{$<command>}}`. Here are a few examples:

- `{{$date '+%Y-%m-%d %A'}}` - Current date
- `{{$uname}}` - Operating system info
- `{{$pwd}}` - Current working directory
- `{{$whoami}}` - Current user
- `{{$jira me}}` - Get current Jira user

**Example with Input/Output Examples:**

The most effective system prompts include input/output examples in XML tags to guide the LLM's behavior:

```toml
system_prompt = """
You are a Kubernetes assistant helping manage clusters and workloads. 
Working in {{$pwd}} on {{$date '+%A, %B %d, %Y'}}.
Current context: {{$kubectl config current-context 2>/dev/null || echo 'Not configured'}}

When users ask about Kubernetes operations, analyze their request and use the appropriate functions.
Always provide clear explanations of what commands will do before executing them.

<examples>
<example>
<input>show me running pods in the default namespace</input>
*calls list_pods function with namespace="default"*
</output>
Here are the running pods in the default namespace:
- web-app-1234 (Running, Ready 1/1)
- api-service-5678 (Running, Ready 2/2)
- database-9012 (Running, Ready 1/1)

All pods appear healthy and ready to serve traffic.</output>
</example>

<example>
<input>what's wrong with my failing pod?</input>
<output>
The pod is in CrashLoopBackOff state. Looking at the logs, I can see:
- Error: Connection refused to database on port 5432
- The application is failing to connect to the database

This suggests a connectivity issue. Check:
1. Database service is running
2. Network policies allow communication
3. Connection string is correct</output>
</example>
</examples>

Keep responses concise but informative. Always explain what actions you're taking and why.
"""
```

### Confirmation Levels

| Level | Behavior |
|-------|----------|
| `none` | No confirmation required (default) |
| `unsafe` | Confirm commands marked as `safe = false` |
| `all` | Confirm every command execution |

## Function Definition

Functions are the core capability of agents. Each function maps to a shell command or script with parameters.

### Basic Function Structure

```toml
[[functions]]
name = "function_name"
description = "Clear description of what this function does"
command = "shell command with {{param}} placeholders"
safe = true
```

### Function Properties

| Property | Type | Required | Default | Description |
|----------|------|----------|---------|-------------|
| `name` | string | Yes | - | Unique function identifier |
| `description` | string | Yes | - | Detailed function description |
| `command` | string | Yes | - | Shell command template |
| `safe` | boolean | No | `false` | Whether command is safe to run |
| `stdin` | string | No | - | Input to pass to command's stdin |
| `output` | string | No | - | Show output to user during execution|
| `pwd` | string | No | - | Working directory for command |
| `timeout` | integer | No | 30 | Command timeout in seconds |

### Command Templates

Commands use `{{parameter}}` placeholders that are replaced with user-provided values:

```toml
command = "grep '{{pattern}}' {{file}} {{flags}}"
```

**Special Shell Blocks:**
- `{{$command}}` - Execute shell command and insert output
- `{{#prompt}}` - Prompt user for input

Examples:
```toml
# Get current date in command
command = "echo 'Today is {{$date}}'"

# Use environment variable
command = "curl -H 'Authorization: Bearer {{$GITHUB_TOKEN}}' {{url}}"

# Prompt user for input
command = "git commit -m '{{#Enter commit message:}}'"
```

## Parameter Handling

Parameters define inputs for your functions with validation and formatting.

### Parameter Structure

```toml
[[functions.parameters]]
name = "param_name"
type = "string"
description = "Parameter description"
required = true
format = "format_string"
options = ["option1", "option2"]
```

### Parameter Properties

| Property | Type | Required | Description |
|----------|------|----------|-------------|
| `name` | string | Yes | Parameter name for `{{name}}` placeholder |
| `type` | string | Yes | Data type: `string`, `number`, `boolean` |
| `description` | string | Yes | Clear description for the AI |
| `required` | boolean | No | Whether parameter is required |
| `format` | string | No | Format string for parameter substitution |
| `options` | array | No | Allowed values (creates enum) |

### Parameter Types

**String Parameters:**
```toml
[[functions.parameters]]
name = "filename"
type = "string"
description = "Path to the file"
required = true
```

**Boolean Parameters:**
```toml
[[functions.parameters]]
name = "verbose"
type = "boolean"
description = "Enable verbose output"
required = false
format = "-v"  # Only added if true
```

**Enum Parameters:**
```toml
[[functions.parameters]]
name = "log_level"
type = "string"
description = "Logging level"
options = ["debug", "info", "warn", "error"]
required = true
```

**Number Parameters:**
```toml
[[functions.parameters]]
name = "port"
type = "number"
description = "Port number"
required = true
```

### Parameter Formatting

The `format` property provides powerful control over how parameters are inserted into commands. ESA supports several formatting patterns:

#### Basic String Formatting

```toml
[[functions.parameters]]
name = "filename"
type = "string"
format = "--file=%s"  # Results in: --file=myfile.txt
```

#### Boolean Parameters

Boolean parameters have special handling - they only appear in the command when `true`:

```toml
[[functions.parameters]]
name = "verbose"
type = "boolean" 
format = "-v"  # Only appears if verbose=true

# Usage in command: ls {{verbose}} {{path}}
# If verbose=true:  ls -v /path
# If verbose=false: ls /path
```

#### Flag-Style Parameters

For parameters that should appear as flags with values:

```toml
[[functions.parameters]]
name = "output_format"
type = "string"
format = "--output %s"  # Results in: --output json

# Alternative formats:
format = "--output=%s"   # Results in: --output=json
format = "-o %s"         # Results in: -o json
```

#### Optional Parameters with Defaults

When parameters are not required, they're simply omitted from the command:

```toml
[[functions.parameters]]
name = "port"
type = "number"
required = false
format = "--port %d"

# If port provided: command --port 8080
# If port omitted:  command
```

#### Complex Formatting Examples

```toml
# Conditional file inclusion
[[functions.parameters]]
name = "config_file"
type = "string"
required = false
format = "--config=%s"

# Multiple format styles
[[functions.parameters]]
name = "log_level"
type = "string"
options = ["debug", "info", "warn", "error"]
format = "--log-level=%s"

# Boolean with custom flag
[[functions.parameters]]
name = "force"
type = "boolean"
format = "--force"  # Only appears when true

# Numeric with validation
[[functions.parameters]]
name = "timeout"
type = "number"
format = "--timeout=%d"
```

#### Format Pattern Summary

| Pattern | Description | Example |
|---------|-------------|---------|
| `"--flag %s"` | Space-separated flag and value | `--output json` |
| `"--flag=%s"` | Equals-separated flag and value | `--output=json` |
| `"-f"` | Simple flag (boolean only) | `-f` (if true) |
| `"%s"` | Raw value substitution | `myfile.txt` |
| No format | Direct parameter replacement | `{{param}}` → `value` |

> **Note**: The `%s`, `%d`, `%f` format specifiers follow Go's fmt package conventions for string, integer, and float formatting respectively.

## Advanced Features

### Working Directory Control

Set the working directory for specific functions. This is useful when commands need to run in a specific location:

```toml
[[functions]]
name = "git_status"
command = "git status"
pwd = "{{repo_path}}"

[[functions.parameters]]
name = "repo_path"
type = "string"
description = "Path to git repository"
required = true
```

**Advanced pwd usage:**
```toml
# Use environment variables
pwd = "$HOME/projects/{{project_name}}"

# Relative paths work too
pwd = "./subdir/{{folder}}"

# Multiple parameter substitution
pwd = "{{base_path}}/{{project}}/{{branch}}"
```

### Stdin Input

Pass data to command's standard input with full parameter and shell substitution support:

```toml
[[functions]]
name = "format_json"
command = "jq '.'"
stdin = "{{json_data}}"

[[functions.parameters]]
name = "json_data"
type = "string"
description = "JSON data to format"
required = true
```

**Advanced stdin examples:**
```toml
# Multi-line stdin with parameter substitution
[[functions]]
name = "send_email"
command = "sendmail {{recipient}}"
stdin = """Subject: {{subject}}
From: {{sender}}

{{message_body}}
"""

# Stdin with shell command execution
[[functions]]
name = "process_with_context"
command = "grep '{{pattern}}'"
stdin = "{{$cat /path/to/file}}"  # Execute shell command for stdin

# Template-heavy stdin for config generation
[[functions]]
name = "generate_config"
command = "tee {{config_name}}"
stdin = """
[server]
host = "{{host}}"
port = {{port}}
debug = {{debug}}

[database]
url = "{{db_url}}"
"""
```

### Command Timeouts

Control execution time limits for different types of operations:

```toml
# Quick operations (default: 60 seconds)
[[functions]]
name = "list_files"
command = "ls -la"
# timeout defaults to 60

# Long-running operations
[[functions]]
name = "large_download"
command = "wget {{url}}"
timeout = 300  # 5 minutes

# Very long operations
[[functions]]
name = "backup_database"
command = "pg_dump {{database}} > backup.sql"
timeout = 3600  # 1 hour
```

### Shell Command Blocks in System Prompts and Commands

ESA supports dynamic content generation using shell command blocks:

#### In System Prompts

```toml
system_prompt = """
You are a Git assistant working in {{$pwd}} on {{$date '+%A, %B %d, %Y'}}.
Current branch: {{$git branch --show-current 2>/dev/null || echo 'Not a git repo'}}
Git status: {{$git status --porcelain | wc -l}} files changed
Current user: {{$whoami}}
System: {{$uname -s}}

Help with git operations while maintaining repository safety.
"""
```

#### In Function Commands

```toml
[[functions]]
name = "create_timestamped_file"
command = "touch {{filename}}_{{$date '+%Y%m%d_%H%M%S'}}.{{extension}}"

[[functions]]
name = "backup_with_user"
command = "cp {{file}} {{file}}.bak.{{$whoami}}.{{$date '+%Y%m%d'}}"
```

### User Input Prompting

Use the `{{#prompt}}` syntax to interactively collect input during command execution:
```toml
[[functions]]
name = "create_issue"
command = "gh issue create --title '{{title}}' --body '{{#Enter issue description (end with empty line):}}'"
```

### Output Specifications and User Communication

The `output` field documents expected output and can be used for user communication:

```toml
[[function]]
name = "ask_user_question"
description = "Ask a question to the user. Use this instead of directly asking the user a question."
output = "{{query}}"
command = "read answer && echo $answer"
safe = true

[[functions.parameters]]
name = "query"
type = "string"
description = "Question to ask the user"
required = true
```

### Environment Variable Integration

ESA supports environment variable expansion in commands:

```toml
[[functions]]
name = "deploy_to_server"
command = "scp {{file}} $DEPLOY_USER@$DEPLOY_HOST:{{remote_path}}"
safe = false

[[functions]]
name = "query_database"
command = "psql $DATABASE_URL -c '{{query}}'"
safe = true
```

### Error Handling and Fallbacks

Build robust functions with error handling:

```toml
[[functions]]
name = "smart_git_status"
command = "jj status 2>/dev/null || git status 2>/dev/null || echo 'Not a version-controlled directory'"
safe = true

[[functions]]
name = "safe_package_install"
command = "npm install {{package}} || yarn add {{package}} || echo 'No package manager found'"
safe = false

[[functions]]
name = "cross_platform_open"
command = "open {{file}} 2>/dev/null || xdg-open {{file}} 2>/dev/null || start {{file}} 2>/dev/null || echo 'Cannot open file on this platform'"
safe = true
```

### Performance Optimization

Tips for efficient function design:

```toml
# Use specific commands instead of general ones
[[functions]]
name = "count_files"
command = "find {{directory}} -type f | wc -l"  # Better than ls | wc
safe = true

# Limit output for large datasets
[[functions]]
name = "recent_logs"
command = "tail -n {{lines}} {{logfile}}"  # Better than cat for large files
safe = true

[[functions.parameters]]
name = "lines"
type = "number"
required = false
format = "%d"

# Use grep for filtering instead of processing all data
[[functions]]
name = "search_logs"
command = "grep '{{pattern}}' {{logfile}} | head -20"
safe = true
```

## Best Practices

### 1. Safety First

- Mark read-only operations as `safe = true`
- Mark destructive operations as `safe = false`
- Use appropriate `ask` levels for your use case
- Validate parameters thoroughly

```toml
# Safe operation
[[functions]]
name = "list_files"
command = "ls {{path}}"
safe = true

# Unsafe operation
[[functions]]
name = "delete_file"
command = "rm {{file}}"
safe = false
```

### 2. Clear Descriptions

Write detailed descriptions that help the AI understand when and how to use functions:

```toml
[[functions]]
name = "git_commit"
description = "Create a git commit with the specified message. Use this after staging changes with git add."
command = "git commit -m '{{message}}'"
```

### 3. Parameter Validation

Use `options` for enum-like parameters and clear `description` fields:

```toml
[[functions.parameters]]
name = "priority"
type = "string" 
description = "Issue priority level"
options = ["low", "medium", "high", "critical"]
required = true
```

### 4. Defensive Programming

Handle edge cases and provide fallbacks:

```toml
# Use || for command fallbacks
command = "jj status || git status"

# Redirect errors to avoid noise
command = "command 2>/dev/null || echo 'Command failed'"
```

### 5. Consistent Naming

Use clear, consistent naming conventions:

- Functions: `verb_noun` (e.g., `list_files`, `create_branch`)
- Parameters: `snake_case` (e.g., `file_path`, `commit_message`)
- Agents: `descriptive-name` (e.g., `git-ops`, `k8s-admin`)

### 6. Documentation

Include helpful comments in your agent files:

```toml
# This agent helps with Kubernetes cluster management
# Requires kubectl to be installed and configured

system_prompt = """..."""

# Core cluster information
[[functions]]
name = "get_nodes"
# ... function definition
```

## Example Agents

### Git Operations Agent

```toml
name = "Git Assistant"
description = "Git repository management and operations"

system_prompt = """
You are a Git operations assistant. Help users with git commands,
repository management, and version control workflows.

Always check repository status before performing operations.
Provide clear explanations of what each command does.
"""

ask = "unsafe"

[[functions]]
name = "git_status"
description = "Show the working tree status"
command = "git status --porcelain"
safe = true

[[functions]]
name = "git_log"
description = "Show commit history"
command = "git log --oneline -n {{count}}"
safe = true

[[functions.parameters]]
name = "count"
type = "number"
description = "Number of commits to show"
required = false

[[functions]]
name = "git_add"
description = "Add files to staging area"
command = "git add {{files}}"
safe = false

[[functions.parameters]]
name = "files"
type = "string"
description = "Files to add (use . for all files)"
required = true

[[functions]]
name = "git_commit"
description = "Create a commit with message"
command = "git commit -m '{{message}}'"
safe = false

[[functions.parameters]]
name = "message"
type = "string"
description = "Commit message"
required = true
```

### System Monitor Agent

```toml
name = "System Monitor"
description = "System resource monitoring and information"

system_prompt = """
You are a system monitoring assistant. Provide information about
system resources, processes, and performance metrics.

Present information in a clear, organized format.
"""

[[functions]]
name = "cpu_usage"
description = "Show CPU usage information"
command = "top -l 1 | grep 'CPU usage'"
safe = true

[[functions]]
name = "memory_usage"
description = "Show memory usage statistics"
command = "vm_stat"
safe = true

[[functions]]
name = "disk_usage"
description = "Show disk space usage"
command = "df -h {{path}}"
safe = true

[[functions.parameters]]
name = "path"
type = "string"
description = "Path to check (defaults to root filesystem)"
required = false

[[functions]]
name = "list_processes"
description = "List running processes"
command = "ps aux | head -{{lines}}"
safe = true

[[functions.parameters]]
name = "lines"
type = "number"
description = "Number of processes to show"
required = false
```

## Debugging and Testing

### 1. Debug Mode

Use debug mode to see what's happening:

```bash
esa --debug +myagent "test command"
```

This shows:
- System prompt processing
- Function calls and parameters
- Command execution details
- Error messages

### 2. Show Commands

See the actual commands being executed:

```bash
esa --show-commands +myagent "test command"
```

### 3. Agent Validation

Check your agent configuration:

```bash
# View agent details
esa show-agent +myagent

# List all agents
esa list-agents
```

### 4. Testing Functions

Test individual functions by creating simple requests:

```bash
esa +myagent "use the list_files function to show current directory"
```

### 5. Common Issues

**Parameter Not Found:**
- Check parameter names match exactly between function command and parameter definition
- Ensure required parameters are marked correctly

**Command Failures:**
- Test commands manually in terminal first
- Check file paths and permissions
- Verify required tools are installed

**AI Not Using Functions:**
- Make function descriptions more specific
- Ensure system prompt mentions when to use functions
- Check that function names are descriptive

### 6. Iterative Development

1. Start with simple functions
2. Test each function individually
3. Add complexity gradually
4. Use debug mode to troubleshoot
5. Refine system prompt based on behavior

## Agent Storage and Organization

### Default Locations

- **User agents**: `~/.config/esa/agents/`
- **Built-in agents**: Embedded in ESA binary
- **Example agents**: `examples/` directory in source

### File Organization

```
~/.config/esa/
├── config.toml          # Global configuration
└── agents/
    ├── git-ops.toml     # Git operations
    ├── k8s-admin.toml   # Kubernetes admin
    ├── dev-tools.toml   # Development utilities
    └── personal.toml    # Personal tasks
```

### Sharing Agents

Agents are portable TOML files that can be easily shared:

```bash
# Share an agent
cp ~/.config/esa/agents/myagent.toml ~/shared/

# Install a shared agent
cp ~/shared/myagent.toml ~/.config/esa/agents/
```

## Conclusion

ESA agents provide a powerful way to create specialized AI assistants for any domain. Start with simple agents and gradually add complexity as you learn the system.

Key points to remember:
- Focus on clear, descriptive system prompts
- Mark functions as safe/unsafe appropriately
- Use parameter validation to prevent errors
- Test thoroughly with debug mode
- Follow consistent naming conventions

For more examples, check the [`examples/`](examples/) directory and the [built-in agents](builtins/).

**Happy agent building!** 🚀
