package lifecycle

import (
	"context"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/gitremote"
)

type comparisonContextSetter interface {
	SetComparisonContexts(ctx context.Context, contexts map[string]gitremote.ComparisonContext) error
}

// ComparisonContextProvider hydrates a complete per-worktree observation for
// one task. A nil map means the provider could not make an authoritative
// observation and must not erase agentctl's last known value.
type ComparisonContextProvider func(ctx context.Context, taskID string) (map[string]gitremote.ComparisonContext, error)

func (m *Manager) SetComparisonContextProvider(fn ComparisonContextProvider) {
	m.comparisonContextProvider = fn
}

func (m *Manager) pushTaskComparisonContexts(ctx context.Context, taskID, executionID string, setter comparisonContextSetter) {
	if taskID == "" || setter == nil || m.comparisonContextProvider == nil {
		return
	}
	contexts, err := m.comparisonContextProvider(ctx, taskID)
	if err != nil {
		m.logger.Warn("failed to hydrate comparison contexts for workspace",
			zap.String("task_id", taskID), zap.String("execution_id", executionID), zap.Error(err))
		return
	}
	if contexts == nil {
		return
	}
	if err := setter.SetComparisonContexts(ctx, contexts); err != nil {
		m.logger.Warn("failed to seed comparison contexts on agentctl",
			zap.String("task_id", taskID), zap.String("execution_id", executionID), zap.Error(err))
	}
}

// PushComparisonContextsForTask sends a fresh full observation to every
// running execution. Empty non-nil maps intentionally clear stale targets.
func (m *Manager) PushComparisonContextsForTask(ctx context.Context, taskID string, contexts map[string]gitremote.ComparisonContext) {
	if taskID == "" {
		return
	}
	for _, execution := range m.executionStore.List() {
		if execution.TaskID != taskID || execution.GetAgentCtlClient() == nil {
			continue
		}
		if err := execution.GetAgentCtlClient().SetComparisonContexts(ctx, contexts); err != nil {
			m.logger.Warn("failed to push comparison contexts to agentctl",
				zap.String("task_id", taskID), zap.String("execution_id", execution.ID), zap.Error(err))
		}
	}
}
