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
	if len(m.history) == 0 {
		m.viewport.SetContent(m.welcomeScreen())
		return
	}
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

const welcomeLogo = `                 .
               .:*:.
             .:+***+:.
           .-=*#####*=-.
         .-=+*#######*+=-.
       .-==+*#########*+==-.
     .:-==++*#########*++==-:.
       .-==+*#########*+==-.
         .-=+*#######*+=-.
           .-=*#####*=-.
             .:+***+:.
               .:*:.
                 .`

func (m Model) welcomeScreen() string {
	logo := welcomeLogoStyle.Render(welcomeLogo)
	title := welcomeTitleStyle.Render("coding-agent")
	subtitle := welcomeSubtitleStyle.Render("a minimal code-editing agent in Go")
	tagline := welcomeTaglineStyle.Render("based on ampcode.com/notes/how-to-build-an-agent")

	tools := welcomeToolsStyle.Render("⚙ read_file   list_files   edit_file")

	keyRow := func(key, desc string) string {
		return welcomeKeyStyle.Render(key) + "  " + welcomeKeyDescStyle.Render(desc)
	}
	keys := lipgloss.JoinVertical(lipgloss.Left,
		keyRow("Enter            ", "send message"),
		keyRow("Alt+Enter / Ctrl+J", "insert newline"),
		keyRow("Ctrl+C / Ctrl+D  ", "quit"),
	)

	rightBlock := lipgloss.JoinVertical(lipgloss.Left,
		title,
		"",
		subtitle,
		tagline,
		"",
		tools,
		"",
		keys,
	)
	content := lipgloss.JoinHorizontal(lipgloss.Center,
		logo,
		"    ",
		rightBlock,
	)

	w := m.viewport.Width
	h := m.viewport.Height
	if w <= 0 || h <= 0 {
		return content
	}
	return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, content)
}
