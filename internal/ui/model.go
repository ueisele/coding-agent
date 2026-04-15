package ui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"

	"github.com/ueisele/coding-agent/internal/agent"
)

const rejectFlashDuration = 300 * time.Millisecond

// Deps is the set of callbacks the UI needs from its environment.
// Main constructs these from an *agent.Agent and a context; the UI
// itself stays free of concurrency primitives and domain types other
// than agent.Event for event matching.
type Deps struct {
	// Submit kicks off a new turn with the given user text. On success the
	// returned channel emits events until the turn is complete and then
	// closes. On synchronous rejection (e.g. the agent is still busy with
	// a prior turn) it returns (nil, error) — the caller should not treat
	// this as a turn failure, just as "not accepted, try again later".
	Submit func(text string) (<-chan agent.Event, error)
	// Stop, SetModel, Reset will be added here as commands are wired up.
}

type entryKind int

const (
	entryUser entryKind = iota
	entryAssistant
	entryToolCall
	entryToolResult
	entryStats
	entryRetry
	entryError
)

type entry struct {
	kind     entryKind
	raw      string // source text (for assistant: accumulates during streaming)
	rendered bool   // for assistant: true once glamour has processed it
}

// Bubble Tea messages

type eventMsg struct{ e agent.Event }
type turnDoneMsg struct{}
type rejectFlashClearMsg struct{ seq int }

type Model struct {
	deps     Deps
	textarea textarea.Model
	viewport viewport.Model
	renderer *glamour.TermRenderer

	history   []entry
	streaming bool
	events    <-chan agent.Event

	// committedLen mirrors the agent's safeLen: the length of history that
	// corresponds to a conversation state the agent will preserve. On a
	// Retrying or ErrorEvent the UI truncates history to committedLen so
	// the display stays in sync with what the agent kept in conversation.
	// It is advanced only by RoundCommitted events from the agent — the UI
	// never guesses at the agent's internal state.
	committedLen int

	// rejectFlashSeq is a monotonic counter used to invalidate stale
	// rejectFlashClearMsg ticks when repeated rejections overlap.
	rejectFlashSeq int
	// flashingReject drives the textarea's outer border color in View().
	flashingReject bool

	width, height int
}

const (
	textareaHeight = 3
	textareaChrome = 2 // border + padding around the textarea widget
)

func New(deps Deps) Model {
	ta := textarea.New()
	ta.Placeholder = "Ask something…   (Alt+Enter for newline)"
	ta.Prompt = "> "
	ta.CharLimit = 8192
	ta.SetHeight(textareaHeight)
	ta.ShowLineNumbers = false
	ta.Focus()

	// Rebind InsertNewline away from plain Enter so our Update handler can
	// use Enter for submit. Alt+Enter is the primary newline key; Ctrl+J
	// (literal linefeed) is a terminal-friendly fallback.
	ta.KeyMap.InsertNewline = key.NewBinding(
		key.WithKeys("alt+enter", "ctrl+j"),
		key.WithHelp("alt+enter", "insert newline"),
	)

	// Disable the textarea's own border — we wrap its View() output in our
	// own lipgloss border at render time. This avoids a pointer hazard
	// inside textarea.Model where its unexported style pointer leaks
	// between value copies of the enclosing struct.
	noBorder := lipgloss.NewStyle()
	ta.FocusedStyle.Base = noBorder
	ta.BlurredStyle.Base = noBorder

	vp := viewport.New(80, 20)

	r, _ := glamour.NewTermRenderer(
		glamour.WithStandardStyle("dark"),
		glamour.WithWordWrap(78),
	)

	return Model{
		deps:     deps,
		textarea: ta,
		viewport: vp,
		renderer: r,
	}
}

func (m Model) Init() tea.Cmd {
	return textarea.Blink
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		wasAtBottom := m.viewport.AtBottom()
		m.width = msg.Width
		m.height = msg.Height
		m.textarea.SetWidth(msg.Width - textareaChrome)
		vpHeight := msg.Height - textareaHeight - textareaChrome
		if vpHeight < 3 {
			vpHeight = 3
		}
		m.viewport.Width = msg.Width
		m.viewport.Height = vpHeight
		wrap := msg.Width - 4
		if wrap < 20 {
			wrap = 20
		}
		if r, err := glamour.NewTermRenderer(
			glamour.WithStandardStyle("dark"),
			glamour.WithWordWrap(wrap),
		); err == nil {
			m.renderer = r
		}
		m.rebuildViewport()
		if wasAtBottom {
			m.viewport.GotoBottom()
		}

	case tea.KeyMsg:
		if msg.Type == tea.KeyCtrlC {
			return m, tea.Quit
		}
		if msg.Type == tea.KeyCtrlD && m.textarea.Value() == "" {
			return m, tea.Quit
		}
		// The agent is the single source of truth for "can I start a new
		// turn right now?" — if it's already running one, Submit returns
		// ErrAgentBusy synchronously and we flash. We deliberately do not
		// gate on m.streaming here; that field is presentation state only.
		if msg.Type == tea.KeyEnter && !msg.Alt {
			userText := strings.TrimSpace(m.textarea.Value())
			if userText == "" {
				break
			}
			events, err := m.deps.Submit(userText)
			if err != nil {
				// Synchronous rejection (ErrAgentBusy). Don't touch history
				// or the textarea — just flash the border red so the user
				// notices the message wasn't sent, and they can retry.
				// Call flashReject first so its pointer-receiver mutation
				// lands on m before we return (Go evaluates return values
				// left-to-right, which would otherwise snapshot m before
				// the mutation).
				cmd := m.flashReject()
				return m, cmd
			}
			m.textarea.Reset()
			m.history = append(m.history, entry{kind: entryUser, raw: userText})
			// Pessimistic rollback boundary: the user entry is not committed
			// yet. The agent will emit an initial RoundCommitted at the
			// start of its turn which advances committedLen to include it.
			m.committedLen = len(m.history) - 1
			m.streaming = true
			m.events = events
			m.rebuildViewport()
			m.viewport.GotoBottom()
			return m, waitForEvent(m.events)
		}

	case eventMsg:
		wasAtBottom := m.viewport.AtBottom()
		m = m.handleEvent(msg.e)
		m.rebuildViewport()
		if wasAtBottom {
			m.viewport.GotoBottom()
		}
		cmds = append(cmds, waitForEvent(m.events))

	case rejectFlashClearMsg:
		// Stale ticks (e.g. older rejections) are invalidated here so a
		// rapid series of rejections doesn't leave the border desynced.
		if msg.seq == m.rejectFlashSeq {
			m.flashingReject = false
		}

	case turnDoneMsg:
		wasAtBottom := m.viewport.AtBottom()
		m.streaming = false
		m = m.finalizeAssistant()
		m.rebuildViewport()
		if wasAtBottom {
			m.viewport.GotoBottom()
		}
	}

	var cmd tea.Cmd
	m.textarea, cmd = m.textarea.Update(msg)
	cmds = append(cmds, cmd)
	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m Model) handleEvent(e agent.Event) Model {
	switch v := e.(type) {
	case agent.TextDelta:
		// Continue the current assistant entry if the last entry is an
		// unrendered assistant block; otherwise start a new one.
		if n := len(m.history); n > 0 &&
			m.history[n-1].kind == entryAssistant &&
			!m.history[n-1].rendered {
			m.history[n-1].raw += v.Text
		} else {
			m.history = append(m.history, entry{kind: entryAssistant, raw: v.Text})
		}
	case agent.ToolCallStart:
		// The preceding assistant text block (if any) is now complete.
		// Finalize it through glamour before we move on.
		m = m.finalizeAssistant()
		m.history = append(m.history, entry{
			kind: entryToolCall,
			raw:  fmt.Sprintf("⚙ %s %s", v.Name, formatToolArgs(v.Input)),
		})
	case agent.ToolCallEnd:
		summary := summarize(v.Output, v.IsError)
		m.history = append(m.history, entry{kind: entryToolResult, raw: summary})
	case agent.Stats:
		var tps float64
		if s := v.Elapsed.Seconds(); s > 0 {
			tps = float64(v.OutputTokens) / s
		}
		m.history = append(m.history, entry{
			kind: entryStats,
			raw: fmt.Sprintf("[in=%d out=%d · %.1fs · %.0f tok/s]",
				v.InputTokens, v.OutputTokens, v.Elapsed.Seconds(), tps),
		})
	case agent.RoundCommitted:
		// Mirror the agent: everything up to here is safe from rollback.
		m.committedLen = len(m.history)
	case agent.Retrying:
		// Drop anything accumulated since the last committed round — matches
		// the agent's conversation rollback exactly.
		m.history = m.history[:m.committedLen]
		m.history = append(m.history, entry{
			kind: entryRetry,
			raw: fmt.Sprintf("retry: attempt %d failed, waiting %.1fs — %s",
				v.Attempt, v.Wait.Seconds(), agent.HumanizeError(v.Err)),
		})
	case agent.ErrorEvent:
		// Same reasoning: drop anything the agent rolled back, then show
		// the error in its place.
		m.history = m.history[:m.committedLen]
		m.history = append(m.history, entry{kind: entryError, raw: agent.HumanizeError(v.Err)})
	}
	return m
}

// finalizeAssistant finds the *most recent* assistant entry and renders it
// through glamour if it hasn't been rendered yet. Called at every tool-call
// boundary (so text-then-tool-then-text turns render each segment as soon as
// it's complete) and once more at end-of-turn for the trailing segment.
//
// The scan stops at the first assistant entry it encounters; it never reaches
// past a rendered assistant to touch an older one.
func (m Model) finalizeAssistant() Model {
	for i := len(m.history) - 1; i >= 0; i-- {
		if m.history[i].kind != entryAssistant {
			continue
		}
		if m.history[i].rendered {
			return m
		}
		if out, err := m.renderer.Render(m.history[i].raw); err == nil {
			m.history[i].raw = strings.TrimRight(out, "\n")
			m.history[i].rendered = true
		} else {
			m.history = append(m.history, entry{
				kind: entryError,
				raw:  "glamour render failed: " + err.Error(),
			})
		}
		return m
	}
	return m
}

// flashReject flips the textarea's outer border to the reject color and
// schedules a tick that will restore it after rejectFlashDuration. The
// sequence counter is bumped on every call so pending clears from earlier
// rejections no-op when they fire. The actual border color is chosen by
// View() based on m.flashingReject — no textarea internal state is touched.
func (m *Model) flashReject() tea.Cmd {
	m.rejectFlashSeq++
	m.flashingReject = true
	seq := m.rejectFlashSeq
	return tea.Tick(rejectFlashDuration, func(time.Time) tea.Msg {
		return rejectFlashClearMsg{seq: seq}
	})
}

func waitForEvent(ch <-chan agent.Event) tea.Cmd {
	return func() tea.Msg {
		e, ok := <-ch
		if !ok {
			return turnDoneMsg{}
		}
		return eventMsg{e}
	}
}

func formatToolArgs(b []byte) string {
	s := strings.TrimSpace(string(b))
	if s == "" || s == "{}" {
		return ""
	}
	var buf bytes.Buffer
	if err := json.Compact(&buf, b); err != nil {
		return s
	}
	return buf.String()
}

func summarize(output string, isError bool) string {
	output = strings.TrimRight(output, "\n")
	if isError {
		return "  ↳ error: " + firstLine(output)
	}
	lines := strings.Count(output, "\n") + 1
	first := firstLine(output)
	if lines == 1 && len(first) <= 80 {
		return "  ↳ " + first
	}
	return fmt.Sprintf("  ↳ %s  (%d lines)", truncate(first, 60), lines)
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
