// Package move contains the transport-neutral contract for a one-shot
// workflow-step move. It deliberately has no task, persistence, or
// orchestrator dependencies so every move entry point can share the same
// normalization and validation rules.
package move

import "strings"

// EntryOptions are one-shot options applied when a task enters a workflow
// step. They do not modify the target step's durable configuration.
//
// The pointer to EntryOptions is the optional nested entry_options value on a
// move request. NormalizeEntryOptions returns nil when the value is empty.
type EntryOptions struct {
	ResetContext   bool   `json:"reset_context,omitempty"`
	Instructions   string `json:"instructions,omitempty"`
	AgentProfileID string `json:"agent_profile_id,omitempty"`
	Model          string `json:"model,omitempty"`
}

// StepEntryOptions is the explicit workflow-step vocabulary for the nested
// entry_options value. It is an alias so transports do not need duplicate
// shapes while callers can use the name that matches their boundary.
type StepEntryOptions = EntryOptions

// Options is a short vocabulary alias for callers that refer to the nested
// value as move options.
type Options = EntryOptions

// MoveChange describes whether a move changes the workflow step or only its
// position metadata. Entry options are meaningful only for Step changes.
type MoveChange string

const (
	MoveChangeNone         MoveChange = "none"
	MoveChangePositionOnly MoveChange = "position_only"
	MoveChangeStep         MoveChange = "step"
)

// Disposition identifies whether the move was applied immediately or retained
// for application after the current agent turn.
type Disposition string

const (
	DispositionCommitted Disposition = "committed"
	DispositionDeferred  Disposition = "deferred"
)

// MoveResult is the transport-neutral result of accepting a move. The
// EntryOptions value is normalized and copied so callers cannot mutate the
// accepted contract through their input pointer.
type MoveResult struct {
	Disposition  Disposition   `json:"disposition"`
	EntryOptions *EntryOptions `json:"entry_options,omitempty"`
}

// Result is a concise alias for MoveResult.
type Result = MoveResult

// NewCommittedResult creates the result for a move applied immediately.
func NewCommittedResult(options *EntryOptions) MoveResult {
	return MoveResult{Disposition: DispositionCommitted, EntryOptions: normalizedCopy(options)}
}

// NewDeferredResult creates the result for a move retained until the active
// turn finishes.
func NewDeferredResult(options *EntryOptions) MoveResult {
	return MoveResult{Disposition: DispositionDeferred, EntryOptions: normalizedCopy(options)}
}

// Committed reports whether this result was applied immediately.
func (r MoveResult) Committed() bool {
	return r.Disposition == DispositionCommitted
}

// Deferred reports whether this result was retained for later application.
func (r MoveResult) Deferred() bool {
	return r.Disposition == DispositionDeferred
}

// HasOverrides reports whether options contain at least one effective value.
// Whitespace-only strings are treated as absent, matching normalization.
func HasOverrides(options *EntryOptions) bool {
	return normalizedCopy(options) != nil
}

// HasOverrides reports whether this value contains at least one effective
// entry option.
func (o *EntryOptions) HasOverrides() bool {
	return HasOverrides(o)
}

func normalizedCopy(options *EntryOptions) *EntryOptions {
	if options == nil {
		return nil
	}
	copy := *options
	copy.Instructions = strings.TrimSpace(copy.Instructions)
	copy.AgentProfileID = strings.TrimSpace(copy.AgentProfileID)
	copy.Model = strings.TrimSpace(copy.Model)
	if copy == (EntryOptions{}) {
		return nil
	}
	return &copy
}
