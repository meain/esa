# ESA

The project lets you specify functions in a TOML file which is sent to
LLM APIs(currently only OpenAI) as available functions. The response
is then parsed and calls the associated command which could be a shell
script or any executable.

## Features

- Define custom functions with parameters in a TOML file.
- Integrate with OpenAI's API to process commands.
- Execute system commands based on the function definitions.

## Configuration

The configuration file is located at `~/.config/esa/config.toml`. You
can use the provided `config-example.toml` as a template.

## Usage

To use the application, run the following command in your terminal:

``` bash
esa "<commands>"
```

With the provided example config you could execute things like:

``` bash
esa "set an alarm for 10:30am"
esa "open golang playground" # works if the llm knows about it
esa "reduce brightness"
```

> You can probably combine it with whisper to have a voice assistant
