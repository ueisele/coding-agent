package agent

import (
	"encoding/json"
	"time"
)

// Event is a domain event emitted by the agent during a turn.
// The UI layer consumes these without knowing anything about the Anthropic API.
type Event interface{ isEvent() }

type TextDelta struct {
	Text string
}

type ToolCallStart struct {
	Name  string
	Input json.RawMessage
}

type ToolCallEnd struct {
	Name    string
	Output  string
	IsError bool
}

// Stats is emitted once per inference call (not per turn — a single turn may
// contain multiple inference calls when tools are used).
type Stats struct {
	InputTokens  int64
	OutputTokens int64
	Elapsed      time.Duration
}

// RoundCommitted is emitted after a round finishes successfully and its
// tool_use / tool_result blocks are safely in the conversation. It marks a
// rollback boundary: if a later round fails, the agent rolls its conversation
// back to the length at this point, and the UI should likewise treat its
// history up to this marker as preserved. Mirrors the agent's internal
// safeLen advance.
type RoundCommitted struct{}

type ErrorEvent struct {
	Err error
}

// Retrying is emitted when a retryable error occurred and the agent is
// about to wait Wait and try the same inference again. Attempt is the
// 1-indexed number of the attempt that just failed.
type Retrying struct {
	Attempt int
	Wait    time.Duration
	Err     error
}

func (TextDelta) isEvent()      {}
func (ToolCallStart) isEvent()  {}
func (ToolCallEnd) isEvent()    {}
func (Stats) isEvent()          {}
func (RoundCommitted) isEvent() {}
func (ErrorEvent) isEvent()     {}
func (Retrying) isEvent()       {}
