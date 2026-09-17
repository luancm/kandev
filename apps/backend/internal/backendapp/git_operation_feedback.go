package backendapp

import (
	"context"
	"strings"
	"time"

	"go.uber.org/zap"

	runtimeapi "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	taskservice "github.com/kandev/kandev/internal/task/service"
	"github.com/kandev/kandev/internal/worktree"
)

const gitOperationPush = "push"

// gitPushAlertReconciler is the narrow adapter boundary used by the gateway,
// orchestrator, and hydration paths. Repository identity is resolved before a
// call crosses this boundary, so the task service never matches display names.
type gitPushAlertReconciler interface {
	RecordGitPushFailure(context.Context, string, taskservice.GitPushFailure) error
	RecordGitPushSuccess(context.Context, string, taskservice.GitPushSuccess) error
	ReconcileGitPushStatus(context.Context, string, taskservice.GitPushStatusObservation) error
}

func gitPushStatusObservationFromStatus(status runtimeapi.GitStatusResult, taskRepositoryID string, receivedAt ...time.Time) taskservice.GitPushStatusObservation {
	observedAt := parseGitObservationTimestamp(status.Timestamp)
	if observedAt.IsZero() && len(receivedAt) > 0 && !receivedAt[0].IsZero() {
		observedAt = receivedAt[0]
	}
	return taskservice.GitPushStatusObservation{
		TaskRepositoryID:  taskRepositoryID,
		Branch:            status.Branch,
		RemoteBranch:      status.RemoteBranch,
		HeadCommit:        status.HeadCommit,
		RemoteHeadCommit:  status.RemoteHeadCommit,
		RemoteAhead:       status.RemoteAhead,
		RemoteBehind:      status.RemoteBehind,
		RemoteAheadKnown:  status.RemoteAheadKnown || status.RemoteAhead != 0,
		RemoteBehindKnown: status.RemoteBehindKnown || status.RemoteBehind != 0,
		ObservedAt:        observedAt,
	}
}

func gitPushStatusObservationFromLifecycleStatus(status *runtimeapi.GitStatusData, taskRepositoryID string, observedAt time.Time) taskservice.GitPushStatusObservation {
	if status == nil {
		return taskservice.GitPushStatusObservation{}
	}
	return taskservice.GitPushStatusObservation{
		TaskRepositoryID:  taskRepositoryID,
		Branch:            status.Branch,
		RemoteBranch:      status.RemoteBranch,
		HeadCommit:        status.HeadCommit,
		RemoteHeadCommit:  status.RemoteHeadCommit,
		RemoteAhead:       status.RemoteAhead,
		RemoteBehind:      status.RemoteBehind,
		RemoteAheadKnown:  status.RemoteAheadKnown || status.RemoteAhead != 0,
		RemoteBehindKnown: status.RemoteBehindKnown || status.RemoteBehind != 0,
		ObservedAt:        observedAt,
	}
}

func gitPushStatusObservationFromSnapshot(snapshot *models.GitSnapshot, taskRepositoryID string) taskservice.GitPushStatusObservation {
	if snapshot == nil {
		return taskservice.GitPushStatusObservation{}
	}
	return taskservice.GitPushStatusObservation{
		TaskRepositoryID:  taskRepositoryID,
		Branch:            snapshot.Branch,
		RemoteBranch:      snapshot.RemoteBranch,
		HeadCommit:        snapshot.HeadCommit,
		RemoteHeadCommit:  metadataString(snapshot.Metadata, "remote_head_commit"),
		RemoteAhead:       metadataInt(snapshot.Metadata, "remote_ahead"),
		RemoteBehind:      metadataInt(snapshot.Metadata, "remote_behind"),
		RemoteAheadKnown:  metadataIntKnown(snapshot.Metadata, "remote_ahead"),
		RemoteBehindKnown: metadataIntKnown(snapshot.Metadata, "remote_behind"),
		ObservedAt:        snapshotObservationTime(snapshot),
	}
}

func snapshotObservationTime(snapshot *models.GitSnapshot) time.Time {
	return gitStatusObservationTime(snapshot)
}

func parseGitObservationTimestamp(value string) time.Time {
	if strings.TrimSpace(value) == "" {
		return time.Time{}
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}
	}
	return parsed
}

func metadataString(metadata map[string]interface{}, key string) string {
	if metadata == nil {
		return ""
	}
	value, _ := metadata[key].(string)
	return value
}

func metadataIntKnown(metadata map[string]interface{}, key string) bool {
	if metadata == nil {
		return false
	}
	switch metadata[key].(type) {
	case int, int64, float64, float32:
		return true
	default:
		return false
	}
}

func metadataInt(metadata map[string]interface{}, key string) int {
	if metadata == nil {
		return 0
	}
	switch value := metadata[key].(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	case float32:
		return int(value)
	default:
		return 0
	}
}

// resolveTaskRepositoryIDForSubpath maps an operation's runtime repository
// subpath to the task_repositories junction row. The junction ID is the
// canonical scope for a task alert; repositories.id is only a global source.
func resolveTaskRepositoryIDForSubpath(
	ctx context.Context,
	taskRepo *sqliterepo.Repository,
	taskID, subpath string,
	log *logger.Logger,
) string {
	if taskRepo == nil || strings.TrimSpace(taskID) == "" {
		return ""
	}
	links, err := taskRepo.ListTaskRepositories(ctx, taskID)
	if err != nil {
		log.Warn("task repositories lookup failed",
			zap.String("task_id", taskID), zap.Error(err))
		return ""
	}
	name := strings.TrimSpace(subpath)
	if name == "" {
		if len(links) == 1 && links[0] != nil {
			return links[0].ID
		}
		return ""
	}
	var match string
	for _, link := range links {
		if link == nil {
			continue
		}
		repository, lookupErr := taskRepo.GetRepository(ctx, link.RepositoryID)
		if lookupErr != nil || repository == nil {
			continue
		}
		if repository.Name != name && worktree.SanitizeRepoDirName(repository.Name) != name {
			continue
		}
		if match != "" {
			return ""
		}
		match = link.ID
	}
	return match
}

// resolveTaskRepositoryIDForStatus maps a status repository name through the
// session's environment worktrees and then back to the task repository link.
// Requiring both rows prevents a healthy status from an unrelated runtime
// repository from clearing the task's current alert.
func resolveTaskRepositoryIDForStatus(
	ctx context.Context,
	taskRepo *sqliterepo.Repository,
	sessionID, taskID, repositoryName string,
	log *logger.Logger,
) string {
	if taskRepo == nil || strings.TrimSpace(sessionID) == "" {
		return ""
	}
	session, err := taskRepo.GetTaskSession(ctx, sessionID)
	if err != nil || session == nil {
		return ""
	}
	taskID = statusTaskID(session, taskID)
	if taskID == "" {
		return ""
	}
	worktrees, err := taskRepo.ListTaskSessionWorktrees(ctx, sessionID)
	if err != nil {
		log.Warn("session worktrees lookup failed",
			zap.String("session_id", sessionID), zap.Error(err))
		return ""
	}
	links, err := taskRepo.ListTaskRepositories(ctx, taskID)
	if err != nil {
		log.Warn("task repositories lookup failed",
			zap.String("task_id", taskID), zap.Error(err))
		return ""
	}

	globalRepositoryID := statusEnvironmentRepositoryID(ctx, taskRepo, worktrees, repositoryName)
	if globalRepositoryID == "" {
		return ""
	}
	return uniqueTaskRepositoryLinkID(links, globalRepositoryID)
}

func statusTaskID(session *models.TaskSession, requestedTaskID string) string {
	if requestedTaskID == "" {
		return session.TaskID
	}
	if session.TaskID != requestedTaskID {
		return ""
	}
	return requestedTaskID
}

func statusEnvironmentRepositoryID(
	ctx context.Context,
	taskRepo *sqliterepo.Repository,
	worktrees []*models.TaskEnvironmentRepo,
	repositoryName string,
) string {
	name := strings.TrimSpace(repositoryName)
	if name == "" {
		return singleEnvironmentRepositoryID(worktrees)
	}
	var match string
	for _, worktreeRow := range worktrees {
		if worktreeRow == nil || !repositoryMatchesName(ctx, taskRepo, worktreeRow.RepositoryID, name) {
			continue
		}
		if match != "" {
			return ""
		}
		match = worktreeRow.RepositoryID
	}
	return match
}

func singleEnvironmentRepositoryID(worktrees []*models.TaskEnvironmentRepo) string {
	if len(worktrees) != 1 || worktrees[0] == nil {
		return ""
	}
	return worktrees[0].RepositoryID
}

func repositoryMatchesName(ctx context.Context, taskRepo *sqliterepo.Repository, repositoryID, name string) bool {
	repository, err := taskRepo.GetRepository(ctx, repositoryID)
	return err == nil && repository != nil &&
		(repository.Name == name || worktree.SanitizeRepoDirName(repository.Name) == name)
}

func uniqueTaskRepositoryLinkID(links []*models.TaskRepository, repositoryID string) string {
	var match string
	for _, link := range links {
		if link == nil || link.RepositoryID != repositoryID {
			continue
		}
		if match != "" {
			return ""
		}
		match = link.ID
	}
	return match
}
