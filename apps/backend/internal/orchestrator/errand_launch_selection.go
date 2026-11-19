package orchestrator

import "github.com/kandev/kandev/internal/task/models"

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
