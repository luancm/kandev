package models

import (
	"context"
	"errors"
	"strings"
)

// ReservedLaunchIdentity pins a server-owned launch before provider admission.
type ReservedLaunchIdentity struct {
	TaskID       string
	SessionID    string
	TurnID       string
	SubmissionID string
}

type reservedLaunchIdentityKey struct{}

func WithReservedLaunchIdentity(ctx context.Context, identity ReservedLaunchIdentity) context.Context {
	return context.WithValue(ctx, reservedLaunchIdentityKey{}, identity)
}

func ReservedLaunchIdentityFromContext(ctx context.Context) (ReservedLaunchIdentity, bool) {
	identity, ok := ctx.Value(reservedLaunchIdentityKey{}).(ReservedLaunchIdentity)
	return identity, ok && identity.TaskID != "" && identity.SessionID != "" && identity.TurnID != "" && identity.SubmissionID != ""
}

type explicitManualCreationKey struct{}

// WithExplicitManualCreation carries server-owned request origin to persistence.
func WithExplicitManualCreation(ctx context.Context, taskID string) context.Context {
	return context.WithValue(ctx, explicitManualCreationKey{}, taskID)
}

func ExplicitManualCreation(ctx context.Context, taskID string) bool {
	manualTaskID, _ := ctx.Value(explicitManualCreationKey{}).(string)
	return taskID != "" && manualTaskID == taskID
}

// ReadReservedLaunchIdentity distinguishes an absent context from a malformed
// reservation. The compatibility getter intentionally omits that distinction.
func ReadReservedLaunchIdentity(ctx context.Context) (ReservedLaunchIdentity, bool, error) {
	raw := ctx.Value(reservedLaunchIdentityKey{})
	if raw == nil {
		return ReservedLaunchIdentity{}, false, nil
	}
	identity, ok := raw.(ReservedLaunchIdentity)
	if !ok || !completeReservedLaunchIdentity(identity.TaskID, identity.SessionID, identity.TurnID, identity.SubmissionID) {
		return identity, true, errors.New("reserved launch context is incomplete")
	}
	return identity, true, nil
}

func completeReservedLaunchIdentity(values ...string) bool {
	for _, value := range values {
		if value == "" || strings.TrimSpace(value) != value {
			return false
		}
	}
	return true
}
