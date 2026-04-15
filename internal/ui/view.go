package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func (m Model) View() string {
	border := textareaBorderNormal
	if m.flashingReject {
		border = textareaBorderReject
	}
	return lipgloss.JoinVertical(lipgloss.Left,
		m.viewport.View(),
		border.Render(m.textarea.View()),
	)
}

func (m *Model) rebuildViewport() {
	var sb strings.Builder
	for i, e := range m.history {
		if i > 0 {
			sb.WriteString("\n")
		}
		switch e.kind {
		case entryUser:
			sb.WriteString(userCiteStyle.Render(e.raw))
		case entryAssistant:
			// Already glamour-rendered once the turn completes; raw text while streaming.
			sb.WriteString(e.raw)
		case entryToolCall:
			sb.WriteString(toolCallStyle.Render(e.raw))
		case entryToolResult:
			sb.WriteString(toolResultStyle.Render(e.raw))
		case entryStats:
			sb.WriteString(statsStyle.Render(e.raw))
		case entryRetry:
			sb.WriteString(retryStyle.Render(e.raw))
		case entryError:
			sb.WriteString(errorStyle.Render(e.raw))
		}
	}
	m.viewport.SetContent(sb.String())
}
