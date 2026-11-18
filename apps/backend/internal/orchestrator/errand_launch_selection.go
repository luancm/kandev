package orchestrator

import (
	"context"
	"errors"

	"github.com/kandev/kandev/internal/task/models"
)

const legacyLocalExecutorType models.ExecutorType = "local_pc"

func supportsOwnedErrandWorkspace(provider string) bool {
	switch models.ExecutorType(provider) {
	case models.ExecutorTypeLocal, models.ExecutorTypeWorktree, legacyLocalExecutorType,
		models.ExecutorTypeLocalDocker, models.ExecutorTypeRemoteDocker, models.ExecutorTypeSSH, models.ExecutorTypeSprites,
		models.ExecutorTypeKubernetes:
		return true
	default:
		return false
	}
}

func (a *ErrandAdapter) hasLegacyErrandRuntime(ctx context.Context, errandID, taskID, parentID string) (bool, error) {
	session, err := a.svc.repo.GetTaskSession(ctx, errandID)
	if err != nil && !errors.Is(err, models.ErrTaskSessionNotFound) {
		return false, err
	}
	if session != nil {
		if session.ID != errandID || session.TaskID != taskID ||
			session.Role != models.SessionRoleErrand || session.ParentSessionID != parentID {
			return false, models.ErrInvalidErrand
		}
		return true, nil
	}
	row, err := a.svc.repo.GetExecutorRunningBySessionID(ctx, errandID)
	if errors.Is(err, models.ErrExecutorRunningNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if row == nil || row.TaskID != taskID || row.SessionID != errandID {
		return false, models.ErrInvalidErrand
	}
	return true, nil
}
