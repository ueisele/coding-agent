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

- [Go](https://go.dev/) 1.24+
- An [Anthropic API key](https://console.anthropic.com/settings/keys)

## Usage

```sh
export ANTHROPIC_API_KEY="your-key-here"
go run main.go
```

Then just talk to it:

```
You: What Go version does this project use?
Claude: [reads go.mod] This project uses Go 1.24.1.
```

## How It Works

The entire agent is under 400 lines of Go. The core loop is:

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