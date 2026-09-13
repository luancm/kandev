package backendapp

import (
	"context"
	"strings"
	"time"

	runtimeapi "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/task/models"
)

// gitOperationFeedbackStore is the task-service seam used to reconcile a
// persisted Git operation notice. Keeping the interface small makes the
// reconciliation logic independent from the service's other responsibilities
// and keeps it straightforward to exercise with an in-memory store.
type gitOperationFeedbackStore interface {
	ListMessages(context.Context, string) ([]*models.Message, error)
	ResolveGitOperationErrorMessage(context.Context, string, string) error
}

type gitOperationFeedbackTaskRepository interface {
	ListTaskRepositories(context.Context, string) ([]*models.TaskRepository, error)
}

type gitOperationRecoveryEvidence struct {
	RepositoryName    string
	Branch            string
	RemoteBranch      string
	HeadCommit        string
	RemoteHeadCommit  string
	RemoteAhead       int
	RemoteBehind      int
	RemoteAheadKnown  bool
	RemoteBehindKnown bool
	ObservedAt        time.Time
}

const (
	gitOperationPush                  = "push"
	gitOperationFeedbackVersion       = 1
	gitOperationResolutionPushSuccess = "push_success"
	gitOperationResolutionGitStatus   = "git_status"
)

func gitOperationRecoveryEvidenceFromStatus(status runtimeapi.GitStatusResult) gitOperationRecoveryEvidence {
	observedAt := time.Time{}
	if status.Timestamp != "" {
		observedAt, _ = time.Parse(time.RFC3339Nano, status.Timestamp)
	}
	return gitOperationRecoveryEvidence{
		RepositoryName:    status.RepositoryName,
		Branch:            status.Branch,
		RemoteBranch:      status.RemoteBranch,
		HeadCommit:        status.HeadCommit,
		RemoteHeadCommit:  status.RemoteHeadCommit,
		RemoteAhead:       status.RemoteAhead,
		RemoteBehind:      status.RemoteBehind,
		RemoteAheadKnown:  true,
		RemoteBehindKnown: true,
		ObservedAt:        observedAt,
	}
}

func gitOperationRecoveryEvidenceFromLifecycleStatus(status *runtimeapi.GitStatusData, observedAt time.Time) gitOperationRecoveryEvidence {
	if status == nil {
		return gitOperationRecoveryEvidence{}
	}
	return gitOperationRecoveryEvidence{
		RepositoryName:    status.RepositoryName,
		Branch:            status.Branch,
		RemoteBranch:      status.RemoteBranch,
		HeadCommit:        status.HeadCommit,
		RemoteHeadCommit:  status.RemoteHeadCommit,
		RemoteAhead:       status.RemoteAhead,
		RemoteBehind:      status.RemoteBehind,
		RemoteAheadKnown:  true,
		RemoteBehindKnown: true,
		ObservedAt:        observedAt,
	}
}

func gitOperationRecoveryEvidenceFromSnapshot(repositoryName string, snapshot *models.GitSnapshot) gitOperationRecoveryEvidence {
	if snapshot == nil {
		return gitOperationRecoveryEvidence{}
	}
	return gitOperationRecoveryEvidence{
		RepositoryName:    repositoryName,
		Branch:            snapshot.Branch,
		RemoteBranch:      snapshot.RemoteBranch,
		HeadCommit:        snapshot.HeadCommit,
		RemoteHeadCommit:  metadataString(snapshot.Metadata, "remote_head_commit"),
		RemoteAhead:       metadataInt(snapshot.Metadata, "remote_ahead"),
		RemoteBehind:      metadataInt(snapshot.Metadata, "remote_behind"),
		RemoteAheadKnown:  metadataIntKnown(snapshot.Metadata, "remote_ahead"),
		RemoteBehindKnown: metadataIntKnown(snapshot.Metadata, "remote_behind"),
		ObservedAt:        gitStatusObservationTime(snapshot),
	}
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
	}
	return 0
}

// resolveGitOperationErrorsForSuccessfulPushResult retires current-format
// errors for an explicit push destination. The fresh status endpoint reports
// the branch's configured upstream, which can differ from a destination named
// by the successful push, so direct-success matching uses that destination
// and the failure timestamp rather than the branch's configured upstream.
func resolveGitOperationErrorsForSuccessfulPushResult(
	ctx context.Context,
	taskRepo gitOperationFeedbackTaskRepository,
	messageStore gitOperationFeedbackStore,
	sessionID, taskID string,
	result *runtimeapi.GitOperationResult,
) (int, error) {
	if taskRepo == nil || messageStore == nil || sessionID == "" || taskID == "" || result == nil || !result.Success {
		return 0, nil
	}
	remote := strings.TrimSpace(result.PushedRemote)
	branch := strings.TrimSpace(result.PushedBranch)
	if remote == "" || branch == "" {
		return 0, nil
	}
	repositories, err := taskRepo.ListTaskRepositories(ctx, taskID)
	if err != nil {
		return 0, err
	}
	if len(repositories) != 1 {
		return 0, nil
	}
	observedAt := time.Now().UTC()
	return resolveGitOperationErrorsMatching(
		ctx,
		messageStore,
		sessionID,
		observedAt,
		gitOperationResolutionPushSuccess,
		func(message *models.Message) bool {
			return gitPushErrorMatchesSuccessfulDestination(message, remote, branch, observedAt)
		},
	)
}

// resolveGitOperationErrorsForStatus retires legacy and correlated notices
// only when the current upstream state proves that a named branch is
// synchronized. It deliberately requires upstream-relative counters and both
// commit IDs; base-branch-relative counts or defaulted zero values are not
// enough to establish that a push completed.
func resolveGitOperationErrorsForStatus(
	ctx context.Context,
	taskRepo gitOperationFeedbackTaskRepository,
	messageStore gitOperationFeedbackStore,
	sessionID, taskID string,
	evidence gitOperationRecoveryEvidence,
) (int, error) {
	if taskRepo == nil || messageStore == nil || sessionID == "" || taskID == "" || !validGitPushRecoveryEvidence(evidence) {
		return 0, nil
	}
	repositories, err := taskRepo.ListTaskRepositories(ctx, taskID)
	if err != nil {
		return 0, err
	}
	if len(repositories) != 1 {
		return 0, nil
	}
	observedAt := evidence.ObservedAt
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	}
	return resolveGitOperationErrors(ctx, messageStore, sessionID, observedAt, gitOperationResolutionGitStatus, &evidence)
}

func validGitPushRecoveryEvidence(evidence gitOperationRecoveryEvidence) bool {
	branch := strings.TrimSpace(evidence.Branch)
	remoteBranch := normalizeGitRemoteBranch(evidence.RemoteBranch)
	headCommit := strings.TrimSpace(evidence.HeadCommit)
	remoteHeadCommit := strings.TrimSpace(evidence.RemoteHeadCommit)
	return branch != "" && remoteBranch != "" && branch == remoteBranch &&
		headCommit != "" && remoteHeadCommit != "" && headCommit == remoteHeadCommit &&
		evidence.RemoteAheadKnown && evidence.RemoteBehindKnown &&
		evidence.RemoteAhead == 0 && evidence.RemoteBehind == 0
}

func normalizeGitRemoteBranch(remoteBranch string) string {
	_, branch := splitGitRemoteBranch(remoteBranch)
	return branch
}

func splitGitRemoteBranch(remoteBranch string) (string, string) {
	remoteBranch = strings.TrimSpace(remoteBranch)
	remoteBranch = strings.TrimPrefix(remoteBranch, "refs/remotes/")
	if slash := strings.IndexByte(remoteBranch, '/'); slash >= 0 {
		return remoteBranch[:slash], remoteBranch[slash+1:]
	}
	return "", remoteBranch
}

func resolveGitOperationErrors(
	ctx context.Context,
	messageStore gitOperationFeedbackStore,
	sessionID string,
	observedAt time.Time,
	resolution string,
	evidence *gitOperationRecoveryEvidence,
) (int, error) {
	var matches func(*models.Message) bool
	if evidence != nil {
		matches = func(message *models.Message) bool {
			return gitPushErrorMatchesEvidence(message, *evidence)
		}
	}
	return resolveGitOperationErrorsMatching(ctx, messageStore, sessionID, observedAt, resolution, matches)
}

func resolveGitOperationErrorsMatching(
	ctx context.Context,
	messageStore gitOperationFeedbackStore,
	sessionID string,
	observedAt time.Time,
	resolution string,
	matches func(*models.Message) bool,
) (int, error) {
	messages, err := messageStore.ListMessages(ctx, sessionID)
	if err != nil {
		return 0, err
	}
	resolved := 0
	var firstErr error
	for _, message := range messages {
		if !isUnresolvedGitPushError(message) || !messageCreatedBefore(message, observedAt) {
			continue
		}
		if matches != nil && !matches(message) {
			continue
		}
		if err := messageStore.ResolveGitOperationErrorMessage(ctx, message.ID, resolution); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		resolved++
	}
	return resolved, firstErr
}

func hasGitOperationFeedbackScope(message *models.Message) bool {
	if message == nil || message.Metadata == nil {
		return false
	}
	_, ok := message.Metadata["git_operation_error_version"]
	return ok
}

// gitPushErrorMatchesEvidence keeps the broad synchronized-state fallback for
// legacy rows, which have no operation scope. New rows carry the attempted
// remote, branch, and HEAD and must match all available identity before a
// status observation can retire them.
func gitPushErrorMatchesEvidence(message *models.Message, evidence gitOperationRecoveryEvidence) bool {
	if message == nil || message.Metadata == nil {
		return false
	}
	if !hasGitOperationFeedbackScope(message) {
		return true
	}
	metadata := message.Metadata
	attemptedRemote := strings.TrimSpace(metadataString(metadata, "git_operation_attempted_remote"))
	attemptedBranch := strings.TrimSpace(metadataString(metadata, "git_operation_attempted_branch"))
	attemptedHead := strings.TrimSpace(metadataString(metadata, "git_operation_attempted_head_commit"))
	if attemptedRemote == "" || attemptedBranch == "" || attemptedHead == "" {
		return false
	}
	remoteName, remoteBranch := splitGitRemoteBranch(evidence.RemoteBranch)
	return strings.EqualFold(attemptedRemote, remoteName) &&
		strings.EqualFold(attemptedBranch, strings.TrimSpace(evidence.Branch)) &&
		strings.EqualFold(attemptedBranch, remoteBranch) &&
		attemptedHead == strings.TrimSpace(evidence.HeadCommit)
}

// gitPushErrorMatchesSuccessfulDestination uses the affirmative push result
// as stronger evidence than a later status query. Legacy rows are deliberately
// excluded because they do not identify the destination, so only the
// same-remote-and-branch scope recorded by current rows can be matched here.
// A rewritten or amended HEAD is still a repair when it publishes to the same
// remote and branch, so this matcher deliberately omits HEAD equality while
// requiring the failure to predate the successful operation.
func gitPushErrorMatchesSuccessfulDestination(message *models.Message, remote, branch string, successfulAt time.Time) bool {
	if message == nil || message.Metadata == nil {
		return false
	}
	if !hasGitOperationFeedbackScope(message) {
		return false
	}
	metadata := message.Metadata
	attemptedRemote := strings.TrimSpace(metadataString(metadata, "git_operation_attempted_remote"))
	attemptedBranch := strings.TrimSpace(metadataString(metadata, "git_operation_attempted_branch"))
	if !strings.EqualFold(attemptedRemote, remote) || !strings.EqualFold(attemptedBranch, branch) {
		return false
	}
	return gitOperationFailurePredates(message, successfulAt)
}

func gitOperationFailurePredates(message *models.Message, successfulAt time.Time) bool {
	if message == nil || message.Metadata == nil || successfulAt.IsZero() {
		return false
	}
	raw, present := message.Metadata["git_operation_failed_at"]
	if !present {
		return true
	}
	failedAt, ok := raw.(string)
	if !ok || strings.TrimSpace(failedAt) == "" {
		return false
	}
	parsed, err := time.Parse(time.RFC3339Nano, failedAt)
	return err == nil && parsed.Before(successfulAt)
}

func isUnresolvedGitPushError(message *models.Message) bool {
	if message == nil || message.Metadata == nil {
		return false
	}
	metadata := message.Metadata
	failed, ok := metadata["git_operation_error"].(bool)
	if !ok || !failed {
		return false
	}
	operation, _ := metadata["operation"].(string)
	if !strings.EqualFold(strings.TrimSpace(operation), "push") {
		return false
	}
	resolved, _ := metadata["git_operation_resolved"].(bool)
	return !resolved
}

func messageCreatedBefore(message *models.Message, observedAt time.Time) bool {
	if message == nil || observedAt.IsZero() || message.CreatedAt.IsZero() {
		return true
	}
	return message.CreatedAt.Before(observedAt)
}
