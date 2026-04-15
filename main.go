package main

import (
	"context"
	"fmt"
	"os"

	"github.com/anthropics/anthropic-sdk-go"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/ueisele/coding-agent/internal/agent"
	"github.com/ueisele/coding-agent/internal/ui"
)

func main() {
	if os.Getenv("ANTHROPIC_API_KEY") == "" {
		fmt.Fprintln(os.Stderr, "ANTHROPIC_API_KEY is not set")
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client := anthropic.NewClient()

	tools := []agent.ToolDefinition{
		agent.ReadFileDefinition,
		agent.ListFilesDefinition,
		agent.EditFileDefinition,
	}
	a := agent.New(&client, tools)

	deps := ui.Deps{
		Submit: func(text string) (<-chan agent.Event, error) {
			return a.Submit(ctx, text)
		},
	}

	p := tea.NewProgram(
		ui.New(deps),
		tea.WithAltScreen(),
	)
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
