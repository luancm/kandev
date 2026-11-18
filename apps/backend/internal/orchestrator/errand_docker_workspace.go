package orchestrator

import (
	"context"
	"fmt"

	runtimeapi "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/task/models"
)

func (a *ErrandAdapter) ownedErrandDockerWorkspace(ctx context.Context, info *runtimeapi.WorkspaceInfo) (*models.TaskEnvironment, error) {
	if info.ExecutorType != string(models.ExecutorTypeLocalDocker) {
		return nil, nil
	}
	if info.TaskEnvironmentID == "" {
		return nil, fmt.Errorf("%w: docker errand has no saved task environment", models.ErrWorkspaceReuseUnsafe)
	}
	env, err := a.svc.repo.GetTaskEnvironment(ctx, info.TaskEnvironmentID)
	if err != nil {
		return nil, fmt.Errorf("read docker errand workspace: %w", err)
	}
	if !attachableErrandContainer(env) || env.ID != info.TaskEnvironmentID || env.TaskID != info.EnvironmentOwnerTaskID ||
		env.OwnershipGeneration != info.OwnershipGeneration || env.ExecutorType != info.ExecutorType {
		return nil, fmt.Errorf("%w: docker errand workspace identity is unavailable or changed", models.ErrWorkspaceReuseUnsafe)
	}
	return env, nil
}

func (a initialErrandAdmission) verifyContainerWorkspace(ctx context.Context) error {
	expected := a.container
	if expected == nil {
		return nil
	}
	env, err := a.service.repo.GetTaskEnvironment(ctx, expected.ID)
	if err != nil {
		return fmt.Errorf("recheck docker errand workspace: %w", err)
	}
	if !attachableErrandContainer(env) || !sameErrandContainerWorkspace(env, expected) {
		return fmt.Errorf("%w: docker errand workspace changed before attach", models.ErrWorkspaceReuseUnsafe)
	}
	return nil
}

func attachableErrandContainer(env *models.TaskEnvironment) bool {
	return env != nil && env.TaskID != "" && env.WorkspacePath != "" && env.ContainerID != "" &&
		(env.Status == models.TaskEnvironmentStatusReady || env.Status == models.TaskEnvironmentStatusStopped) &&
		(env.ContainerControlAuthTokenSecretID != "" || env.ContainerBootstrapNonceSecretID != "")
}

func sameErrandContainerWorkspace(current, expected *models.TaskEnvironment) bool {
	return current.ID == expected.ID && current.TaskID == expected.TaskID &&
		current.OwnershipGeneration == expected.OwnershipGeneration && current.ExecutorType == expected.ExecutorType &&
		current.WorkspacePath == expected.WorkspacePath && current.ContainerID == expected.ContainerID &&
		current.ContainerControlAuthTokenSecretID == expected.ContainerControlAuthTokenSecretID &&
		current.ContainerBootstrapNonceSecretID == expected.ContainerBootstrapNonceSecretID
}
