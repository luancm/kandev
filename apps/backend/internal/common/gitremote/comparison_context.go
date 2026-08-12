package gitremote

import (
	"errors"
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/common/securityutil"
)

// ComparisonContextUpdate describes how an observation changes the context
// currently applied to one workspace. Replace carries a complete target;
// clear removes an explicitly stale target.
type ComparisonContextUpdate string

const (
	ComparisonContextReplace ComparisonContextUpdate = "replace"
	ComparisonContextClear   ComparisonContextUpdate = "clear"
)

// ComparisonContext is the credential-free value transported from the
// backend to an agentctl runtime. ContextGeneration is opaque and is only
// useful for rejecting stale observations; it has no provider semantics.
type ComparisonContext struct {
	ContextGeneration string                  `json:"context_generation,omitempty"`
	Target            *RemoteRefIdentity      `json:"target,omitempty"`
	StoredBaseCommit  string                  `json:"stored_base_commit,omitempty"`
	Update            ComparisonContextUpdate `json:"update"`
}

// Validate enforces the transport boundary. In particular, repository
// identities and refs may not contain URL credentials or shell-shaped input.
func (c ComparisonContext) Validate() error {
	switch c.Update {
	case ComparisonContextReplace:
		if c.Target == nil {
			return errors.New("comparison context replace requires a target")
		}
		if err := c.Target.Validate(); err != nil {
			return fmt.Errorf("comparison context target: %w", err)
		}
	case ComparisonContextClear:
		if c.Target != nil {
			return errors.New("comparison context clear must not carry a target")
		}
		if c.StoredBaseCommit != "" {
			return errors.New("comparison context clear must not carry a base commit")
		}
	default:
		return fmt.Errorf("unsupported comparison context update %q", c.Update)
	}
	if c.ContextGeneration != "" {
		if len(c.ContextGeneration) > 256 || strings.TrimSpace(c.ContextGeneration) != c.ContextGeneration || strings.ContainsAny(c.ContextGeneration, "\r\n\t") {
			return errors.New("comparison context generation is invalid")
		}
	}
	if c.StoredBaseCommit != "" && !securityutil.LooksLikeCommitSHA(c.StoredBaseCommit) {
		return errors.New("comparison context stored base commit is invalid")
	}
	return nil
}

// Clone returns an independent context suitable for storing in a mutable
// configuration map.
func (c ComparisonContext) Clone() ComparisonContext {
	if c.Target == nil {
		return c
	}
	clone := *c.Target
	clone.Repository = c.Target.Repository
	c.Target = &clone
	return c
}

// NewComparisonContext constructs a validated replacement observation.
func NewComparisonContext(target RemoteRefIdentity, storedBaseCommit, generation string) (ComparisonContext, error) {
	context := ComparisonContext{
		ContextGeneration: generation,
		Target:            &target,
		StoredBaseCommit:  storedBaseCommit,
		Update:            ComparisonContextReplace,
	}
	if err := context.Validate(); err != nil {
		return ComparisonContext{}, err
	}
	return context, nil
}

// ClearComparisonContext constructs an explicit clear observation.
func ClearComparisonContext(generation string) ComparisonContext {
	return ComparisonContext{ContextGeneration: generation, Update: ComparisonContextClear}
}

// LinkedChange is the provider-neutral portion of a linked PR/MR needed to
// select a comparison target. Missing pointers intentionally represent an
// incompletely hydrated provider row and must fail closed when it is the
// exact candidate for the current action head.
type LinkedChange struct {
	Source           *RemoteRefIdentity
	Base             *RemoteRefIdentity
	StoredBaseCommit string
}

// ComparisonContextInput contains the server-side evidence used to choose a
// context. The action head is executor-local truth; the other values are
// provider/task observations.
type ComparisonContextInput struct {
	ActionHead                   *RemoteRefIdentity
	LinkedChanges                []LinkedChange
	RemoteContributionTarget     *RemoteRefIdentity
	RemoteContributionBaseCommit string
	AttachedRepository           *RemoteRepositoryIdentity
	SelectedBase                 string
	ContextGeneration            string
}

// ComparisonContextSelection is the result of the fail-closed precedence
// rules. A nil Context means the caller must retain its previous observation.
type ComparisonContextSelection struct {
	Context ComparisonContext
	State   ResolutionState
	Reason  string
}

// SelectComparisonContext applies the only supported selection precedence:
// an exact, unique linked change; a validated contribution target when no
// linked change exists; and finally the attached repository plus selected
// base. It never falls back after linked evidence exists but is incomplete,
// absent, or ambiguous.
func SelectComparisonContext(input ComparisonContextInput) ComparisonContextSelection {
	if len(input.LinkedChanges) > 0 {
		if input.ActionHead == nil {
			return unresolvedComparisonContext("action head is unavailable")
		}
		matches := make([]LinkedChange, 0, len(input.LinkedChanges))
		for _, change := range input.LinkedChanges {
			if change.Source != nil && change.Source.Equal(*input.ActionHead) {
				matches = append(matches, change)
			}
		}
		if len(matches) == 0 {
			return unresolvedComparisonContext("no linked change matches the exact action head")
		}
		if len(matches) > 1 {
			return ComparisonContextSelection{State: ResolutionAmbiguous, Reason: "multiple linked changes match the exact action head"}
		}
		match := matches[0]
		if match.Base == nil {
			return unresolvedComparisonContext("exact linked change is missing its base identity")
		}
		context, err := NewComparisonContext(*match.Base, match.StoredBaseCommit, input.ContextGeneration)
		if err != nil {
			return unresolvedComparisonContext("exact linked change is incomplete: " + err.Error())
		}
		return ComparisonContextSelection{Context: context, State: ResolutionResolved, Reason: "exact linked change"}
	}

	if input.RemoteContributionTarget != nil {
		context, err := NewComparisonContext(*input.RemoteContributionTarget, input.RemoteContributionBaseCommit, input.ContextGeneration)
		if err != nil {
			return unresolvedComparisonContext("remote contribution target is invalid: " + err.Error())
		}
		return ComparisonContextSelection{Context: context, State: ResolutionResolved, Reason: "validated remote contribution target"}
	}

	if input.AttachedRepository == nil {
		return unresolvedComparisonContext("attached repository is unavailable")
	}
	target := RemoteRefIdentity{Repository: *input.AttachedRepository, Ref: input.SelectedBase}
	context, err := NewComparisonContext(target, "", input.ContextGeneration)
	if err != nil {
		return unresolvedComparisonContext("attached repository target is invalid: " + err.Error())
	}
	return ComparisonContextSelection{Context: context, State: ResolutionResolved, Reason: "attached repository and selected base"}
}

func unresolvedComparisonContext(reason string) ComparisonContextSelection {
	return ComparisonContextSelection{State: ResolutionUnresolved, Reason: reason}
}
