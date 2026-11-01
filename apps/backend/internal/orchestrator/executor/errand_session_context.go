package executor

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/task/models"
)

type errandSessionKey struct{}

// WithErrandSession marks the session prepared under ctx as an errand of the
// given parent session. The role is set when the row is inserted and never
// changed afterwards.
func WithErrandSession(ctx context.Context, parentSessionID string) context.Context {
	return context.WithValue(ctx, errandSessionKey{}, parentSessionID)
}

func errandParentFromContext(ctx context.Context) (string, bool) {
	parent, _ := ctx.Value(errandSessionKey{}).(string)
	return parent, parent != ""
}

// applyErrandSessionIdentity stamps the errand role and parent on a session
// about to be inserted and keeps it out of primary selection.
func applyErrandSessionIdentity(ctx context.Context, session *models.TaskSession) (bool, error) {
	parent, requested := ctx.Value(errandSessionKey{}).(string)
	if !requested {
		return false, nil
	}
	identity, reserved, err := models.ReadReservedLaunchIdentity(ctx)
	if err != nil {
		return false, err
	}
	if !reserved {
		return false, fmt.Errorf("%w: errand context requires a reservation", models.ErrInvalidErrandLaunchAuthority)
	}
	session.Role = models.SessionRoleErrand
	session.ParentSessionID = parent
	session.IsPrimary = false
	if err := models.BindErrandLaunchAuthority(session, identity, parent); err != nil {
		return false, err
	}
	return true, nil
}
