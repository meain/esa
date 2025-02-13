# ESA

<img src="https://github.com/user-attachments/assets/5c2915ab-4a8e-4b49-b3b6-394d5644dac2" alt="Mascot" width="300" align="right"/>

The project lets you specify functions in a TOML file which is sent to
LLM APIs(currently only OpenAI) as available functions. The response
is then parsed and calls the associated command which could be a shell
script or any executable.

## Features

- Define custom functions with parameters in a TOML file.
- Integrate with OpenAI's API to process commands.
- Execute system commands based on the function definitions.

## Usage

To use the application, run the following command in your terminal:

```bash
esa [--debug] [--config <path>] [--ask <level>] "<command>"
```

You can also specify a different config file by using the `+` syntax:

```bash
esa +jira "list all open issues"
```

This will use the config file located at `~/.config/esa/jira.toml`.

You can create a new agent configuration using the `+new` syntax:

```bash
esa +new "Create a coding assistant with read_file and list_files functions"
```

It will output a config file which you can use for a coding assistant agent.

The available flags are:
- `--debug`: Enables debug mode, printing additional information about the assistant's response and function execution.
- `--config <path>`: Specifies the path to the configuration file. Defaults to `~/.config/esa/config.toml`.
- `--ask <level>`: Specifies the confirmation level for command execution. Options are `none`, `unsafe`, and `all`. Default is `none`.
- `-c, --continue`: Continue the last conversation with the assistant. The conversation history is stored per configuration file.

## Configuration

### Confirmation Levels

The `--ask` flag allows you to specify the level of confirmation required before executing commands. The available options are:

- `none`: No confirmation is required.
- `unsafe`: Confirmation is required for commands marked as non-safe.
- `all`: Confirmation is required for all commands.

### Safe Property

The `safe` property in the function configuration determines whether a command is considered safe or potentially unsafe. If `safe` is set to `true`, the command will be executed without confirmation when the `--ask` level is set to `unsafe`. If `safe` is set to `false` or not specified, confirmation will be required.

The capabilities of your assistant are easily extendable by adding more functions to the config file.

With the provided example config you could execute things like:

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

``` bash
cat main.go |
  esa 'Summarize the provided code and send an email to mail@meain.io. Send the email only if it will not rain tonight. Also send a notification after that.'
```

_You can find examples of the functions in the `functions` folder._

> CAUTION: Be careful with the functions you add. If you let it
> overwrite files or run commands with, it could be dangerous. Just
> because you can do something doesn't mean you should.

### Configuration File

The configuration file is located at `~/.config/esa/config.toml`.  It
is a TOML file that defines the functions available to the
assistant. It includes the following:

- `functions`: An array of function definitions. Each function has:
  - `name`: The name of the function.
  - `description`: A description of the function.
  - `command`: The command to execute when the function is called.
  - `parameters`: An array of parameter definitions. Each parameter has:
    - `name`: The name of the parameter.
    - `type`: The type of the parameter (e.g., string).
    - `description`: A description of the parameter.
    - `required`: A boolean indicating if the parameter is required.
- `ask`: The confirmation level for command execution.
- `system_prompt`: The system prompt for the assistant.

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
