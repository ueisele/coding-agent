package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/packages/ssestream"
)

// ErrAgentBusy is returned via an ErrorEvent when Submit is called while a
// turn is already in flight.
var ErrAgentBusy = errors.New("agent: turn already in flight")

// ErrTurnAborted is returned via an ErrorEvent when a turn is cancelled
// (via Stop, a deadline on the turn context, or cancellation of the parent
// context). The underlying cause from the context is joined via fmt.Errorf
// so that both errors.Is(err, ErrTurnAborted) and
// errors.Is(err, context.Canceled) work on the wrapped error.
var ErrTurnAborted = errors.New("agent: turn aborted")

type Agent struct {
	client      *anthropic.Client
	tools       []ToolDefinition
	model       anthropic.Model
	turnTimeout time.Duration // zero means no timeout

	busy         atomic.Bool
	activeCancel atomic.Pointer[context.CancelFunc]
	conversation []anthropic.MessageParam
}

// Option configures an Agent at construction time.
type Option func(*Agent)

// WithTurnTimeout sets a maximum duration for a single turn. When the
// timeout expires, the active turn is cancelled exactly as if Stop had
// been called, and an ErrorEvent wrapping context.DeadlineExceeded is
// emitted. A zero or negative duration disables the timeout.
func WithTurnTimeout(d time.Duration) Option {
	return func(a *Agent) { a.turnTimeout = d }
}

// WithModel overrides the default model.
func WithModel(m anthropic.Model) Option {
	return func(a *Agent) { a.model = m }
}

func New(client *anthropic.Client, tools []ToolDefinition, opts ...Option) *Agent {
	a := &Agent{
		client: client,
		tools:  tools,
		model:  anthropic.ModelClaudeHaiku4_5,
	}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

// Submit starts a new turn with the given user input. On success it returns
// an event channel that closes when the turn (including any tool-use loops)
// is complete, and a nil error.
//
// Only one turn may be in flight at a time. If Submit is called while
// another turn is active it returns (nil, ErrAgentBusy) synchronously —
// nothing is appended to the conversation, no goroutine is started, and no
// events flow. The caller should treat this as "submission rejected, try
// again later" rather than as a turn error.
func (a *Agent) Submit(parent context.Context, userInput string) (<-chan Event, error) {
	if !a.busy.CompareAndSwap(false, true) {
		return nil, ErrAgentBusy
	}

	// Derive a turn-scoped context so Stop can cancel just this turn
	// without affecting the caller's parent context. If a turn timeout is
	// configured, layer it on top — the first of Stop, timeout, or parent
	// cancel wins.
	var turnCtx context.Context
	var cancel context.CancelFunc
	if a.turnTimeout > 0 {
		turnCtx, cancel = context.WithTimeout(parent, a.turnTimeout)
	} else {
		turnCtx, cancel = context.WithCancel(parent)
	}
	a.activeCancel.Store(&cancel)

	out := make(chan Event, 128)
	go func() {
		defer close(out)
		defer a.busy.Store(false)
		defer a.activeCancel.Store(nil)
		defer cancel()

		a.conversation = append(a.conversation,
			anthropic.NewUserMessage(anthropic.NewTextBlock(userInput)))
		// runTurn owns its own conversation rollback: it preserves completed
		// rounds and only discards the in-flight round on failure. See the
		// runTurn doc for the exact semantics.
		if !a.runTurn(turnCtx, out) {
			// If the turn aborted via context cancellation (Stop, deadline,
			// or parent cancellation), make it visible in the UI.
			// Non-blocking so we don't stall closing the channel in the
			// pathological case where the consumer has stopped reading.
			if turnCtx.Err() != nil {
				select {
				case out <- ErrorEvent{Err: fmt.Errorf("%w: %w", ErrTurnAborted, turnCtx.Err())}:
				default:
				}
			}
		}
	}()
	return out, nil
}

// Stop cancels the currently active turn, if any. Safe to call concurrently
// and when no turn is in flight — it is a no-op in that case. After Stop, the
// active turn's event channel will close (via ctx cancellation propagating
// through runLoop) and the conversation will be rolled back.
func (a *Agent) Stop() {
	if p := a.activeCancel.Load(); p != nil {
		(*p)()
	}
}

// runTurn runs the inference rounds that make up one turn. Each round is a
// single streaming request to Claude plus any tool executions its response
// demands; the turn ends when a round returns no tool_use blocks.
//
// Returns true on clean completion, false if the turn aborted (context
// cancel, transient error budget exhausted, etc.).
//
// Rollback semantics: on failure, the conversation is truncated to the
// last round boundary — i.e., the end of the most recently completed round,
// or the state before the user message if the first round itself failed.
// Completed rounds (and their already-executed tool side effects) are
// preserved because their tool_use / tool_result pairs form a valid,
// self-contained prefix the next turn can continue from.
func (a *Agent) runTurn(ctx context.Context, out chan<- Event) bool {
	send := func(e Event) bool {
		select {
		case out <- e:
			return true
		case <-ctx.Done():
			return false
		}
	}

	// safeLen always points to a conversation state where every tool_use
	// has a matching tool_result. On failure we roll back to this length.
	//
	// Start of turn: the user message has just been appended by Submit. We
	// emit an initial RoundCommitted so the UI can advance its own rollback
	// boundary to include the user entry. This is the contract point that
	// keeps the two sides in sync — the UI does not assume anything about
	// what the agent has appended; it reacts to this event.
	safeLen := len(a.conversation)
	if !send(RoundCommitted{}) {
		a.conversation = a.conversation[:safeLen-1]
		return false
	}

	for {
		if ctx.Err() != nil {
			a.conversation = a.conversation[:safeLen]
			return false
		}

		message, elapsed, ok := a.runRound(ctx, send)
		if !ok {
			a.conversation = a.conversation[:safeLen]
			return false
		}

		a.conversation = append(a.conversation, message.ToParam())

		if !send(Stats{
			InputTokens:  message.Usage.InputTokens,
			OutputTokens: message.Usage.OutputTokens,
			Elapsed:      elapsed,
		}) {
			a.conversation = a.conversation[:safeLen]
			return false
		}

		toolResults := []anthropic.ContentBlockParamUnion{}
		for _, content := range message.Content {
			if content.Type != "tool_use" {
				continue
			}
			if !send(ToolCallStart{Name: content.Name, Input: content.Input}) {
				a.conversation = a.conversation[:safeLen]
				return false
			}
			output, isErr := a.executeTool(content.Name, content.Input)
			if !send(ToolCallEnd{Name: content.Name, Output: output, IsError: isErr}) {
				a.conversation = a.conversation[:safeLen]
				return false
			}
			toolResults = append(toolResults, anthropic.NewToolResultBlock(content.ID, output, isErr))
		}

		if len(toolResults) == 0 {
			return true
		}
		a.conversation = append(a.conversation, anthropic.NewUserMessage(toolResults...))

		// This round is now complete: assistant message is present and every
		// tool_use it contained has a matching tool_result. Update safeLen and
		// tell the UI the same thing so its history mirrors our rollback
		// boundary.
		safeLen = len(a.conversation)
		if !send(RoundCommitted{}) {
			a.conversation = a.conversation[:safeLen]
			return false
		}
	}
}

// Retry parameters for streamWithRetry. Start at 1s, multiply by 1.5 per
// attempt, cap each individual wait at 10s, and give up after one minute
// of cumulative backoff.
const (
	retryInitialBackoff = time.Second
	retryMultiplier     = 1.5
	retryMaxBackoff     = 10 * time.Second
	retryBudget         = time.Minute
)

// runRound runs one inference round with streaming and transparent retry
// on transient errors (rate limit, overloaded, 5xx) using exponential
// backoff. A round is one streaming request to Claude producing one
// assistant message.
//
// On retry, any partial TextDeltas emitted during the failed attempt are
// invalidated — the UI is expected to drop the in-progress assistant entry
// when it receives the Retrying event, so the new attempt starts a fresh
// block.
func (a *Agent) runRound(ctx context.Context, send func(Event) bool) (*anthropic.Message, time.Duration, bool) {
	backoff := retryInitialBackoff
	var retryStart time.Time
	attempt := 0

	for {
		attempt++
		start := time.Now()
		stream := a.startInference(ctx)

		message := anthropic.Message{}
		var streamErr error

		for stream.Next() {
			event := stream.Current()
			if err := message.Accumulate(event); err != nil {
				streamErr = err
				break
			}
			if event.Type == "content_block_delta" && event.Delta.Type == "text_delta" {
				if !send(TextDelta{Text: event.Delta.Text}) {
					return nil, 0, false
				}
			}
		}
		if streamErr == nil {
			streamErr = stream.Err()
		}

		if streamErr == nil {
			return &message, time.Since(start), true
		}

		if !isTransient(streamErr) {
			send(ErrorEvent{Err: streamErr})
			return nil, 0, false
		}

		// Start the retry-budget clock on the first retryable failure.
		if retryStart.IsZero() {
			retryStart = time.Now()
		}
		if time.Since(retryStart)+backoff > retryBudget {
			send(ErrorEvent{Err: fmt.Errorf("retry budget exhausted after %d attempts: %w", attempt, streamErr)})
			return nil, 0, false
		}

		if !send(Retrying{Attempt: attempt, Wait: backoff, Err: streamErr}) {
			return nil, 0, false
		}

		select {
		case <-ctx.Done():
			return nil, 0, false
		case <-time.After(backoff):
		}

		backoff = time.Duration(float64(backoff) * retryMultiplier)
		if backoff > retryMaxBackoff {
			backoff = retryMaxBackoff
		}
	}
}

// HumanizeError returns a short human-readable description of err. For
// Anthropic API errors it extracts the message and error type from the
// response JSON; for anything else it falls back to err.Error(). Lives in
// the agent package so the UI layer doesn't need to import anthropic.
func HumanizeError(err error) string {
	if err == nil {
		return ""
	}
	var ae *anthropic.Error
	if errors.As(err, &ae) {
		var body struct {
			Error struct {
				Type    string `json:"type"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if jsonErr := json.Unmarshal([]byte(ae.RawJSON()), &body); jsonErr == nil && body.Error.Message != "" {
			return fmt.Sprintf("%s (%s, HTTP %d)", body.Error.Message, body.Error.Type, ae.StatusCode)
		}
		return fmt.Sprintf("API error (HTTP %d)", ae.StatusCode)
	}
	return err.Error()
}

// isTransient reports whether an inference error should trigger a retry.
// Transient errors cover server-side rate limit / overloaded / 5xx responses
// and network errors. Context cancellation is never transient.
func isTransient(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var ae *anthropic.Error
	if errors.As(err, &ae) {
		switch ae.StatusCode {
		case 408, 425, 429, 500, 502, 503, 504, 529:
			return true
		}
		return false
	}
	// Non-API errors are typically network issues — retry by default.
	return true
}

func (a *Agent) startInference(ctx context.Context) *ssestream.Stream[anthropic.MessageStreamEventUnion] {
	anthropicTools := []anthropic.ToolUnionParam{}
	for _, tool := range a.tools {
		anthropicTools = append(anthropicTools, anthropic.ToolUnionParam{
			OfTool: &anthropic.ToolParam{
				Name:        tool.Name,
				Description: anthropic.String(tool.Description),
				InputSchema: tool.InputSchema,
			},
		})
	}

	return a.client.Messages.NewStreaming(ctx, anthropic.MessageNewParams{
		Model:     a.model,
		MaxTokens: 1024,
		Messages:  a.conversation,
		Tools:     anthropicTools,
	})
}

func (a *Agent) executeTool(name string, input json.RawMessage) (string, bool) {
	for _, tool := range a.tools {
		if tool.Name == name {
			out, err := tool.Function(input)
			if err != nil {
				return err.Error(), true
			}
			return out, false
		}
	}
	return "tool not found: " + name, true
}
