package executor

import (
	"context"

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
func applyErrandSessionIdentity(ctx context.Context, session *models.TaskSession) bool {
	parent, ok := errandParentFromContext(ctx)
	if !ok {
		return false
	}
	session.Role = models.SessionRoleErrand
	session.ParentSessionID = parent
	session.IsPrimary = false
	return true
}

// sessionIsErrand reports whether the stored session is an errand. A session
// that cannot be read is treated as an ordinary session.
func (e *Executor) sessionIsErrand(ctx context.Context, sessionID string) bool {
	if sessionID == "" {
		return false
	}
	session, err := e.repo.GetTaskSession(ctx, sessionID)
	return err == nil && models.IsErrandSession(session)
}
