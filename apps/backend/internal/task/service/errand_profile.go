package service

import (
	"context"
	"errors"
	"fmt"
)

// ValidateAgentProfileForTaskExecutor reports whether an enabled agent profile
// of the task's workspace can run on the task's effective executor, the check
// task creation applies. Errand start uses it before it admits a profile.
func (s *Service) ValidateAgentProfileForTaskExecutor(ctx context.Context, taskID, profileID string) error {
	task, err := s.tasks.GetTask(ctx, taskID)
	if err != nil {
		return err
	}
	profile, err := s.loadWorkflowChangeProfile(ctx, task.WorkspaceID, profileID, profileID)
	if err != nil {
		return err
	}
	if s.agentProfileExecutorValidator == nil {
		return errors.New("executor compatibility validation unavailable")
	}
	executor, executorProfile, err := s.workflowChangeExecutor(ctx, task)
	if err != nil {
		return fmt.Errorf("resolve task executor: %w", err)
	}
	return s.agentProfileExecutorValidator.ValidateAgentProfileForExecutor(ctx, profile, executor, executorProfile)
}
