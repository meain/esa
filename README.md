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

## Configuration

The configuration file is located at `~/.config/esa/config.toml`. You
can use the provided `config-example.toml` as a template.

## Usage

To use the application, run the following command in your terminal:

```bash
esa "<commands>"
```

The capabilities of your assistant are easily extendable by adding
more functions to the config file.

With the provided example config you could execute things like:

```bash
esa "what is a harpoon" # answer basic questions
esa "who is esa?" # as about itself
esa "set an alarm for 10:30am"
esa "sen alarm for 1 hour from now"
esa "open golang playground" # works if the llm knows about it
esa "reduce brightness"
esa "increase brightness if it is after 2PM" # is it pointless, yes but it works
```

> You can probably combine it with whisper to have a voice assistant

## Notes

You can connect it to whisper and a voice model to make it a voice assistant.

```bash
,transcribe-audio | xargs -I{} esa "{}" | say
```

_`,transcribe-audio` is a small script that I have that uses whisper. You can find it [here](https://github.com/meain/dotfiles/blob/master/scripts/.local/bin/random/%2Ctranscribe-audio)._
