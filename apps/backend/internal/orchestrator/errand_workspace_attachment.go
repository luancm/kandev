package orchestrator

import (
	"context"
	"fmt"
	"maps"

	runtimeapi "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/task/models"
)

type ownedErrandWorkspaceAttachment struct {
	environment    *models.TaskEnvironment
	executorConfig map[string]string
}

func (a *ErrandAdapter) loadOwnedErrandWorkspaceAttachment(ctx context.Context, info *runtimeapi.WorkspaceInfo) (*ownedErrandWorkspaceAttachment, error) {
	switch models.ExecutorType(info.ExecutorType) {
	case models.ExecutorTypeLocalDocker, models.ExecutorTypeRemoteDocker, models.ExecutorTypeSSH, models.ExecutorTypeSprites:
	default:
		return nil, nil
	}
	if info.TaskEnvironmentID == "" {
		return nil, fmt.Errorf("%w: errand has no saved task environment", models.ErrWorkspaceReuseUnsafe)
	}
	env, err := a.svc.repo.GetTaskEnvironment(ctx, info.TaskEnvironmentID)
	if err != nil {
		return nil, fmt.Errorf("read errand workspace: %w", err)
	}
	if !attachableErrandWorkspace(env) || env.ID != info.TaskEnvironmentID || env.TaskID != info.EnvironmentOwnerTaskID ||
		env.OwnershipGeneration != info.OwnershipGeneration || env.ExecutorType != info.ExecutorType {
		return nil, fmt.Errorf("%w: errand workspace identity is unavailable or changed", models.ErrWorkspaceReuseUnsafe)
	}
	config, err := a.svc.errandWorkspaceExecutorConfig(ctx, env)
	if err != nil {
		return nil, err
	}
	return &ownedErrandWorkspaceAttachment{environment: env, executorConfig: config}, nil
}

func (s *Service) errandWorkspaceExecutorConfig(ctx context.Context, env *models.TaskEnvironment) (map[string]string, error) {
	provider := models.ExecutorType(env.ExecutorType)
	if provider != models.ExecutorTypeRemoteDocker && provider != models.ExecutorTypeSSH {
		return nil, nil
	}
	if env.ExecutorID == "" {
		return nil, fmt.Errorf("%w: remote errand workspace has no saved executor", models.ErrWorkspaceReuseUnsafe)
	}
	executor, err := s.repo.GetExecutor(ctx, env.ExecutorID)
	if err != nil {
		return nil, fmt.Errorf("read remote errand executor: %w", err)
	}
	if executor == nil || executor.ID != env.ExecutorID || executor.Type != provider {
		return nil, fmt.Errorf("%w: remote errand executor identity changed", models.ErrWorkspaceReuseUnsafe)
	}
	return runtimeapi.SSHWorkspaceExecutorConfig(executor.Config), nil
}

func (a initialErrandAdmission) verifyWorkspaceAttachment(ctx context.Context) error {
	expected := a.attachment
	if expected == nil {
		return nil
	}
	env, err := a.service.repo.GetTaskEnvironment(ctx, expected.environment.ID)
	if err != nil {
		return fmt.Errorf("recheck errand workspace: %w", err)
	}
	if !attachableErrandWorkspace(env) || !sameErrandWorkspace(env, expected.environment) {
		return fmt.Errorf("%w: errand workspace changed before attach", models.ErrWorkspaceReuseUnsafe)
	}
	config, err := a.service.errandWorkspaceExecutorConfig(ctx, env)
	if err != nil {
		return err
	}
	if !maps.Equal(config, expected.executorConfig) {
		return fmt.Errorf("%w: remote errand target changed before attach", models.ErrWorkspaceReuseUnsafe)
	}
	return nil
}

func attachableErrandWorkspace(env *models.TaskEnvironment) bool {
	if env == nil || env.TaskID == "" || env.WorkspacePath == "" ||
		(env.Status != models.TaskEnvironmentStatusReady && env.Status != models.TaskEnvironmentStatusStopped) {
		return false
	}
	switch models.ExecutorType(env.ExecutorType) {
	case models.ExecutorTypeSSH:
		return true
	case models.ExecutorTypeSprites:
		return env.SandboxID != "" && env.ExecutorID != "" && env.ExecutorProfileID != ""
	default:
		return env.ContainerID != "" && (env.ContainerControlAuthTokenSecretID != "" || env.ContainerBootstrapNonceSecretID != "")
	}
}

func sameErrandWorkspace(current, expected *models.TaskEnvironment) bool {
	return current.ID == expected.ID && current.TaskID == expected.TaskID &&
		current.OwnershipGeneration == expected.OwnershipGeneration && current.ExecutorType == expected.ExecutorType &&
		current.ExecutorID == expected.ExecutorID && current.ExecutorProfileID == expected.ExecutorProfileID &&
		current.WorkspacePath == expected.WorkspacePath && current.ContainerID == expected.ContainerID && current.SandboxID == expected.SandboxID &&
		current.ContainerControlAuthTokenSecretID == expected.ContainerControlAuthTokenSecretID &&
		current.ContainerBootstrapNonceSecretID == expected.ContainerBootstrapNonceSecretID
}
