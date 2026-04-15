package ui

import "github.com/charmbracelet/lipgloss"

var (
	userCiteStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("179")).
			Italic(true).
			BorderStyle(lipgloss.Border{Left: "│"}).
			BorderLeft(true).
			BorderForeground(lipgloss.Color("179")).
			PaddingLeft(1)

	toolCallStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("82")).
			Bold(true)

	toolResultStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240"))

	statsStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240"))

	retryStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("214")). // amber
			Italic(true)

	textareaBorderNormal = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("244")).
				Padding(0, 1)

	textareaBorderReject = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("196")).
				Padding(0, 1)

	welcomeLogoStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("179"))

	welcomeTitleStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("179")).
				Bold(true)

	welcomeSubtitleStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("252"))

	welcomeTaglineStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("240")).
				Italic(true)

	welcomeToolsStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("82")).
				Bold(true)

	welcomeKeyStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("179")).
			Bold(true)

	welcomeKeyDescStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("244"))

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
			Bold(true)
)
