package orchestrator

import (
	"context"
	"fmt"
	"maps"

	runtimeapi "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/task/models"
)

type ownedErrandDockerAttachment struct {
	environment    *models.TaskEnvironment
	executorConfig map[string]string
}

func (a *ErrandAdapter) ownedErrandDockerWorkspace(ctx context.Context, info *runtimeapi.WorkspaceInfo) (*ownedErrandDockerAttachment, error) {
	if info.ExecutorType != string(models.ExecutorTypeLocalDocker) && info.ExecutorType != string(models.ExecutorTypeRemoteDocker) {
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
	config, err := a.svc.errandDockerExecutorConfig(ctx, env)
	if err != nil {
		return nil, err
	}
	return &ownedErrandDockerAttachment{environment: env, executorConfig: config}, nil
}

func (s *Service) errandDockerExecutorConfig(ctx context.Context, env *models.TaskEnvironment) (map[string]string, error) {
	if env.ExecutorType != string(models.ExecutorTypeRemoteDocker) {
		return nil, nil
	}
	if env.ExecutorID == "" {
		return nil, fmt.Errorf("%w: remote docker errand has no saved executor", models.ErrWorkspaceReuseUnsafe)
	}
	executor, err := s.repo.GetExecutor(ctx, env.ExecutorID)
	if err != nil {
		return nil, fmt.Errorf("read remote docker errand executor: %w", err)
	}
	if executor == nil || executor.ID != env.ExecutorID || executor.Type != models.ExecutorTypeRemoteDocker {
		return nil, fmt.Errorf("%w: remote docker errand executor identity changed", models.ErrWorkspaceReuseUnsafe)
	}
	return runtimeapi.SSHWorkspaceExecutorConfig(executor.Config), nil
}

func (a initialErrandAdmission) verifyContainerWorkspace(ctx context.Context) error {
	expected := a.container
	if expected == nil {
		return nil
	}
	env, err := a.service.repo.GetTaskEnvironment(ctx, expected.environment.ID)
	if err != nil {
		return fmt.Errorf("recheck docker errand workspace: %w", err)
	}
	if !attachableErrandContainer(env) || !sameErrandContainerWorkspace(env, expected.environment) {
		return fmt.Errorf("%w: docker errand workspace changed before attach", models.ErrWorkspaceReuseUnsafe)
	}
	config, err := a.service.errandDockerExecutorConfig(ctx, env)
	if err != nil {
		return err
	}
	if !maps.Equal(config, expected.executorConfig) {
		return fmt.Errorf("%w: remote docker errand target changed before attach", models.ErrWorkspaceReuseUnsafe)
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
		current.ExecutorID == expected.ExecutorID && current.ExecutorProfileID == expected.ExecutorProfileID &&
		current.WorkspacePath == expected.WorkspacePath && current.ContainerID == expected.ContainerID &&
		current.ContainerControlAuthTokenSecretID == expected.ContainerControlAuthTokenSecretID &&
		current.ContainerBootstrapNonceSecretID == expected.ContainerBootstrapNonceSecretID
}
