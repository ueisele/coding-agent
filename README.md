# coding-agent

A minimal code-editing agent in Go, built to understand how coding agents like [Claude Code](https://docs.anthropic.com/en/docs/agents-and-tools/claude-code/overview), [Amp](https://ampcode.com/), and others work under the hood.

Based on Thorsten Ball's tutorial [How to Build an Agent](https://ampcode.com/notes/how-to-build-an-agent).

## What It Does

An LLM (Claude) sits in a loop with access to three tools:

- **read_file** — read the contents of a file
- **list_files** — list files and directories
- **edit_file** — create or modify files via string replacement

The agent decides on its own when and how to use these tools to accomplish a task. There is no hardcoded logic telling it "if the user mentions a file, read it." The LLM figures that out from context.

## Prerequisites

- [Go](https://go.dev/) 1.26.2
- An [Anthropic API key](https://console.anthropic.com/settings/keys)

## Usage

```sh
export ANTHROPIC_API_KEY="your-key-here"
go run .
```

Then just talk to it:

```
You: What Go version does this project use?
Claude: [reads go.mod] This project uses Go 1.26.2.
```

The UI runs as a [Bubble Tea](https://github.com/charmbracelet/bubbletea) TUI with a bordered input box at the bottom, streaming output, and markdown rendering via [Glamour](https://github.com/charmbracelet/glamour). Ctrl+C or Ctrl+D (on empty input) to quit.

## How It Works

The core loop is:

1. Read user input
2. Send conversation to Claude (including tool definitions)
3. If Claude responds with text → print it, go to 1
4. If Claude responds with a tool call → execute the tool, send the result back, go to 3

That's it. Everything else is boilerplate and type wiring.

## Extending It

Natural next steps beyond the tutorial:

- Add a `bash` tool so the agent can run commands and verify its own work
- Add confirmation prompts before destructive file operations
- Implement context window management (summarization, truncation)
- Experiment with different models and system prompts

## License

MIT