# MCP Server Integration Guide

ESA supports Model Context Protocol (MCP) servers, allowing you to extend your agents with sophisticated external tools and services.

## What are MCP Servers?

MCP (Model Context Protocol) servers are external processes that provide tools and capabilities to AI assistants. They can offer:

- File system operations
- Database access
- API integrations
- Custom business logic
- External service connections

## Configuration

MCP servers are configured at the agent level alongside regular functions. Add them to your agent TOML file:

```toml
# Agent configuration
name = "My Agent"
description = "Agent with MCP server support"
system_prompt = "..."

# MCP Servers configuration
[mcp_servers.filesystem]
command = "docker"
args = [
    "run",
    "-i",
    "--rm",
    "--mount", "type=bind,src=/path/to/local/dir,dst=/projects/dir",
    "mcp/filesystem",
    "/projects"
]

[mcp_servers.database]
command = "mcp-database-server"
args = ["--connection", "postgresql://localhost/mydb"]

# Regular functions can coexist with MCP servers
[[functions]]
name = "regular_function"
description = "A regular shell command function"
command = "echo {{message}}"
safe = true

[[functions.parameters]]
name = "message"
type = "string"
required = true
```

## MCP Server Configuration

Each MCP server requires:

- **Name**: A unique identifier for the server
- **Command**: The executable or docker command to start the server
- **Args**: Arguments passed to the command
- **Safe** (optional): Whether tools from this server are considered safe by default (defaults to `false`)
- **Safe Functions** (optional): List of specific functions that are safe, overriding the server-level safe setting
- **Allowed Functions** (optional): List of functions to expose to the LLM (if empty, all functions are allowed)

### Security and Function Control

ESA provides granular control over MCP server security and function exposure:

- **Server-level Safety**: Set `safe = true` to mark all functions from a server as safe by default
- **Function-level Safety**: Use `safe_functions = ["func1", "func2"]` to override safety for specific functions  
- **Function Filtering**: Use `allowed_functions = ["func1", "func2"]` to limit which functions are exposed to the LLM

Safety affects confirmation behavior based on the global `--ask` level:
- `--ask none`: No confirmation required
- `--ask unsafe`: Confirm functions marked as `safe = false`
- `--ask all`: Confirm every function execution

### Example Configurations

#### Filesystem Server (Docker)
```toml
[mcp_servers.filesystem]
command = "docker"
args = [
    "run",
    "-i",
    "--rm",
    "--mount", "type=bind,src=/Users/username/projects,dst=/projects",
    "mcp/filesystem",
    "/projects"
]
safe = false  # File operations are potentially unsafe
safe_functions = ["read_file", "list_directory"]  # Read operations are safe
allowed_functions = ["read_file", "write_file", "list_directory"]  # Limit available functions
```

#### Database Server
```toml
[mcp_servers.database]
command = "mcp-database-server"
args = [
    "--host", "localhost",
    "--port", "5432",
    "--database", "myapp"
]
safe = false  # Database operations are unsafe by default
safe_functions = ["select", "show"]  # Only SELECT and SHOW queries are safe
allowed_functions = ["select", "insert", "update", "show"]  # Limit available operations
```

#### Web Fetch Server (All Functions Safe)
```toml
[mcp_servers.fetch]
command = "uvx"
args = ["mcp-server-fetch"]
safe = true  # All web fetch operations are considered safe
# No allowed_functions specified, so all functions are available
```

#### Custom Python MCP Server
```toml
[mcp_servers.custom]
command = "python"
args = [
    "/path/to/my-mcp-server.py",
    "--config", "/path/to/config.json"
]
safe = false
safe_functions = ["get_info", "list_items"]  # Only these functions are safe
allowed_functions = ["get_info", "list_items", "create_item"]  # Only expose these functions
```

## How It Works

1. **Server Startup**: When you run an agent with MCP servers, ESA automatically starts the configured servers
2. **Tool Discovery**: ESA queries each server for available tools using the MCP protocol
3. **Tool Integration**: MCP tools are added to the agent alongside regular functions
4. **Tool Execution**: When the AI calls an MCP tool, ESA forwards the request to the appropriate server
5. **Response Handling**: Results from MCP servers are returned to the AI like regular function outputs

## Tool Naming

MCP tools are automatically prefixed with `mcp_<server_name>_` to avoid conflicts:

- Server "filesystem" tool "read_file" becomes `mcp_filesystem_read_file`
- Server "database" tool "query" becomes `mcp_database_query`

## Debugging

Use debug mode to see MCP server activity:

```bash
go run ./... --agent examples/filesystem.toml --debug "list files in current directory"
```

This shows:
- MCP server startup
- Tool discovery
- Tool execution details
- Error messages

## Error Handling

- If an MCP server fails to start, the agent will not start
- If a server becomes unavailable during execution, tool calls will fail gracefully
- All servers are automatically stopped when the agent exits

## Security Considerations

- MCP servers run as separate processes with their own permissions
- Use Docker containers for better isolation
- Only mount necessary directories in filesystem servers
- Validate server binaries and configurations
- Consider network access restrictions for servers that make external connections

### Safety Classification and Confirmation

MCP servers and their individual functions can be classified for safety to help with confirmation decisions:

- **Server-level Safety**: `safe = true/false` sets the default safety for all functions from the server
- **Function-level Safety**: `safe_functions = ["func1", "func2"]` overrides server-level setting for specific functions
- **Function Filtering**: `allowed_functions = ["func1", "func2"]` limits which functions are exposed to the LLM

Confirmation behavior is controlled by the global `--ask` flag combined with function safety:

- **`--ask none`**: No confirmation required (default)
- **`--ask unsafe`**: Confirm functions marked as `safe = false`
- **`--ask all`**: Confirm every function execution

### Command Display

Use the `--show-commands` flag to see MCP tool executions:

```bash
esa --show-commands +mcp-agent "list files in current directory"
```

This will display:
```
MCP filesystem: list_directory({"path": "."})
```

## Available MCP Servers

Popular MCP servers include:

- **Filesystem**: File operations (reading, writing, listing)
- **Database**: SQL query execution
- **Git**: Version control operations
- **Browser**: Web automation
- **Kubernetes**: Cluster management
- **Custom**: Build your own using MCP libraries

## Creating Custom MCP Servers

You can create custom MCP servers using available libraries:

- **Python**: `mcp` package
- **TypeScript**: `@modelcontextprotocol/sdk`
- **Other languages**: Implement JSON-RPC over stdio

Example minimal Python MCP server:
```python
from mcp import Server, Tool
import sys

server = Server("my-custom-server")

@server.tool("hello")
def hello_tool(name: str) -> str:
    return f"Hello, {name}!"

if __name__ == "__main__":
    server.run()
```

## Best Practices

1. **Start Simple**: Begin with existing MCP servers before building custom ones
2. **Use Docker**: Container-based servers provide better isolation
3. **Test Standalone**: Verify MCP servers work independently before integrating
4. **Monitor Resources**: MCP servers consume additional system resources
5. **Handle Failures**: Design agents to work even if some MCP servers are unavailable
6. **Documentation**: Document required MCP servers and their setup in your agent files

## Examples

See the `examples/` directory for working agent configurations:

- `examples/mcp.toml`: Filesystem and fetch operations with MCP

For more information about the Model Context Protocol, visit: https://modelcontextprotocol.io
