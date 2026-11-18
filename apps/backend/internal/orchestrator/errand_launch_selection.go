package orchestrator

import (
	"context"
	"errors"

	runtimeapi "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/errands"
	"github.com/kandev/kandev/internal/task/models"
)

const legacyLocalExecutorType models.ExecutorType = "local_pc"

func (a *ErrandAdapter) selectOwnedErrandWorkspace(ctx context.Context, request errands.LaunchRequest) (*runtimeapi.WorkspaceInfo, error) {
	if !validErrandLaunchRequest(request) {
		return nil, models.ErrInvalidErrand
	}
	if err := a.svc.authorizeTask(ctx, request.TaskID); err != nil {
		return nil, err
	}
	legacy, err := a.hasLegacyErrandRuntime(ctx, request.ErrandID, request.TaskID, request.ParentSessionID)
	if err != nil || legacy {
		return nil, err
	}
	info, err := a.ownedErrandWorkspace(ctx, request)
	if err != nil {
		return nil, err
	}
	switch models.ExecutorType(info.ExecutorType) {
	case models.ExecutorTypeLocal, models.ExecutorTypeWorktree, legacyLocalExecutorType:
		return info, nil
	default:
		return nil, nil
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
