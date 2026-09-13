package backendapp

import (
	"context"
	"errors"
	"testing"
	"time"

	client "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/task/models"
)

type gitOperationFeedbackTaskRepositoryStub struct {
	repositories []*models.TaskRepository
	err          error
}

func (s *gitOperationFeedbackTaskRepositoryStub) ListTaskRepositories(context.Context, string) ([]*models.TaskRepository, error) {
	return s.repositories, s.err
}

type gitOperationFeedbackStoreStub struct {
	messages   []*models.Message
	resolved   []string
	listErr    error
	resolveErr error
}

func (s *gitOperationFeedbackStoreStub) ListMessages(context.Context, string) ([]*models.Message, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	return s.messages, nil
}

func (s *gitOperationFeedbackStoreStub) ResolveGitOperationErrorMessage(_ context.Context, messageID, resolution string) error {
	if s.resolveErr != nil {
		return s.resolveErr
	}
	for _, message := range s.messages {
		if message != nil && message.ID == messageID {
			if message.Metadata == nil {
				message.Metadata = map[string]interface{}{}
			}
			message.Metadata["git_operation_resolved"] = true
			message.Metadata["resolution"] = resolution
		}
	}
	s.resolved = append(s.resolved, messageID)
	return nil
}

func legacyGitPushMessage(id string, createdAt time.Time) *models.Message {
	return &models.Message{
		ID:        id,
		CreatedAt: createdAt,
		Metadata: map[string]interface{}{
			"git_operation_error": true,
			"operation":           "push",
		},
	}
}

func TestResolveGitOperationErrorsForStatusResolvesOnlyOlderPushErrors(t *testing.T) {
	now := time.Now().UTC()
	store := &gitOperationFeedbackStoreStub{messages: []*models.Message{
		legacyGitPushMessage("old-push", now.Add(-time.Minute)),
		legacyGitPushMessage("new-push", now.Add(time.Minute)),
		{ID: "pull", CreatedAt: now.Add(-time.Minute), Metadata: map[string]interface{}{
			"git_operation_error": true,
			"operation":           "pull",
		}},
	}}
	repo := &gitOperationFeedbackTaskRepositoryStub{repositories: []*models.TaskRepository{{ID: "repo-1"}}}
	count, err := resolveGitOperationErrorsForStatus(context.Background(), repo, store, "session-1", "task-1", gitOperationRecoveryEvidence{
		Branch:            "main",
		RemoteBranch:      "origin/main",
		HeadCommit:        "abc",
		RemoteHeadCommit:  "abc",
		RemoteAheadKnown:  true,
		RemoteBehindKnown: true,
		ObservedAt:        now,
	})
	if err != nil {
		t.Fatalf("resolveGitOperationErrorsForStatus: %v", err)
	}
	if count != 1 || len(store.resolved) != 1 || store.resolved[0] != "old-push" {
		t.Fatalf("resolved = (%d, %v), want old-push only", count, store.resolved)
	}
	if store.messages[1].Metadata["git_operation_resolved"] == true {
		t.Fatal("newer push error must remain unresolved")
	}
	if store.messages[2].Metadata["git_operation_resolved"] == true {
		t.Fatal("pull error must remain unresolved")
	}
}

func TestResolveGitOperationErrorsForStatusRequiresCurrentAttemptScope(t *testing.T) {
	now := time.Now().UTC()
	message := legacyGitPushMessage("scoped-push", now.Add(-time.Minute))
	message.Metadata["git_operation_error_version"] = gitOperationFeedbackVersion
	message.Metadata["git_operation_attempted_remote"] = "origin"
	message.Metadata["git_operation_attempted_branch"] = "main"
	message.Metadata["git_operation_attempted_head_commit"] = "abc"
	store := &gitOperationFeedbackStoreStub{messages: []*models.Message{message}}
	repo := &gitOperationFeedbackTaskRepositoryStub{repositories: []*models.TaskRepository{{ID: "repo-1"}}}
	evidence := gitOperationRecoveryEvidence{
		Branch:            "main",
		RemoteBranch:      "origin/main",
		HeadCommit:        "abc",
		RemoteHeadCommit:  "abc",
		RemoteAheadKnown:  true,
		RemoteBehindKnown: true,
		ObservedAt:        now,
	}
	count, err := resolveGitOperationErrorsForStatus(context.Background(), repo, store, "session-1", "task-1", evidence)
	if err != nil {
		t.Fatalf("resolveGitOperationErrorsForStatus: %v", err)
	}
	if count != 1 || len(store.resolved) != 1 || store.resolved[0] != "scoped-push" {
		t.Fatalf("resolved = (%d, %v), want scoped-push", count, store.resolved)
	}
}

func TestResolveGitOperationErrorsForStatusLeavesMismatchedCurrentAttempt(t *testing.T) {
	now := time.Now().UTC()
	message := legacyGitPushMessage("scoped-push", now.Add(-time.Minute))
	message.Metadata["git_operation_error_version"] = gitOperationFeedbackVersion
	message.Metadata["git_operation_attempted_remote"] = "origin"
	message.Metadata["git_operation_attempted_branch"] = "feature"
	message.Metadata["git_operation_attempted_head_commit"] = "abc"
	store := &gitOperationFeedbackStoreStub{messages: []*models.Message{message}}
	repo := &gitOperationFeedbackTaskRepositoryStub{repositories: []*models.TaskRepository{{ID: "repo-1"}}}
	count, err := resolveGitOperationErrorsForStatus(context.Background(), repo, store, "session-1", "task-1", gitOperationRecoveryEvidence{
		Branch:            "main",
		RemoteBranch:      "origin/main",
		HeadCommit:        "abc",
		RemoteHeadCommit:  "abc",
		RemoteAheadKnown:  true,
		RemoteBehindKnown: true,
		ObservedAt:        now,
	})
	if err != nil {
		t.Fatalf("resolveGitOperationErrorsForStatus: %v", err)
	}
	if count != 0 || len(store.resolved) != 0 {
		t.Fatalf("resolved = (%d, %v), want none", count, store.resolved)
	}
}

func TestResolveGitOperationErrorsForStatusRejectsWeakEvidence(t *testing.T) {
	store := &gitOperationFeedbackStoreStub{messages: []*models.Message{legacyGitPushMessage("push", time.Now().Add(-time.Minute))}}
	repo := &gitOperationFeedbackTaskRepositoryStub{repositories: []*models.TaskRepository{{ID: "repo-1"}}}
	for name, evidence := range map[string]gitOperationRecoveryEvidence{
		"missing remote head": {
			Branch: "main", RemoteBranch: "origin/main", HeadCommit: "abc",
		},
		"base relative counters": {
			Branch: "main", RemoteBranch: "origin/main", HeadCommit: "abc", RemoteHeadCommit: "abc", RemoteAheadKnown: true, RemoteBehindKnown: true, RemoteAhead: 1,
		},
		"different branch": {
			Branch: "feature", RemoteBranch: "origin/main", HeadCommit: "abc", RemoteHeadCommit: "abc",
		},
		"different heads": {
			Branch: "main", RemoteBranch: "origin/main", HeadCommit: "abc", RemoteHeadCommit: "def",
		},
	} {
		t.Run(name, func(t *testing.T) {
			store.messages[0].Metadata["git_operation_resolved"] = false
			store.resolved = nil
			count, err := resolveGitOperationErrorsForStatus(context.Background(), repo, store, "session-1", "task-1", evidence)
			if err != nil {
				t.Fatalf("resolveGitOperationErrorsForStatus: %v", err)
			}
			if count != 0 || len(store.resolved) != 0 {
				t.Fatalf("resolved = (%d, %v), want none", count, store.resolved)
			}
		})
	}
}

func TestResolveGitOperationErrorsForSuccessfulPushRequiresOneRepository(t *testing.T) {
	message := legacyGitPushMessage("push", time.Now().Add(-time.Minute))
	store := &gitOperationFeedbackStoreStub{messages: []*models.Message{message}}
	multiRepo := &gitOperationFeedbackTaskRepositoryStub{repositories: []*models.TaskRepository{{ID: "repo-1"}, {ID: "repo-2"}}}
	count, err := resolveGitOperationErrorsForSuccessfulPush(context.Background(), multiRepo, store, "session-1", "task-1")
	if err != nil {
		t.Fatalf("resolveGitOperationErrorsForSuccessfulPush: %v", err)
	}
	if count != 0 || len(store.resolved) != 0 {
		t.Fatalf("resolved = (%d, %v), want none for ambiguous repositories", count, store.resolved)
	}

	oneRepo := &gitOperationFeedbackTaskRepositoryStub{repositories: []*models.TaskRepository{{ID: "repo-1"}}}
	count, err = resolveGitOperationErrorsForSuccessfulPush(context.Background(), oneRepo, store, "session-1", "task-1")
	if err != nil {
		t.Fatalf("resolveGitOperationErrorsForSuccessfulPush: %v", err)
	}
	if count != 1 || len(store.resolved) != 1 || store.resolved[0] != "push" {
		t.Fatalf("resolved = (%d, %v), want push", count, store.resolved)
	}
}

func TestResolveGitOperationErrorsForSuccessfulPushLeavesCurrentScopedErrorsForStatus(t *testing.T) {
	message := legacyGitPushMessage("scoped-push", time.Now().Add(-time.Minute))
	message.Metadata["git_operation_error_version"] = gitOperationFeedbackVersion
	store := &gitOperationFeedbackStoreStub{messages: []*models.Message{message}}
	repo := &gitOperationFeedbackTaskRepositoryStub{repositories: []*models.TaskRepository{{ID: "repo-1"}}}
	count, err := resolveGitOperationErrorsForSuccessfulPush(context.Background(), repo, store, "session-1", "task-1")
	if err != nil {
		t.Fatalf("resolveGitOperationErrorsForSuccessfulPush: %v", err)
	}
	if count != 0 || len(store.resolved) != 0 {
		t.Fatalf("resolved = (%d, %v), want none for status-scoped error", count, store.resolved)
	}
}

func TestResolveGitOperationErrorsForSuccessfulPushResultUsesExplicitDestination(t *testing.T) {
	message := legacyGitPushMessage("scoped-push", time.Now().Add(-time.Minute))
	message.Metadata["git_operation_error_version"] = gitOperationFeedbackVersion
	message.Metadata["git_operation_attempted_remote"] = "backup"
	message.Metadata["git_operation_attempted_branch"] = "feature/work"
	message.Metadata["git_operation_attempted_head_commit"] = "abc"
	store := &gitOperationFeedbackStoreStub{messages: []*models.Message{message}}
	repo := &gitOperationFeedbackTaskRepositoryStub{repositories: []*models.TaskRepository{{ID: "repo-1"}}}
	count, err := resolveGitOperationErrorsForSuccessfulPushResult(
		context.Background(), repo, store, "session-1", "task-1",
		&client.GitOperationResult{
			Success:          true,
			PushedRemote:     "backup",
			PushedBranch:     "feature/work",
			PushedHeadCommit: "abc",
		},
	)
	if err != nil {
		t.Fatalf("resolveGitOperationErrorsForSuccessfulPushResult: %v", err)
	}
	if count != 1 || len(store.resolved) != 1 || store.resolved[0] != "scoped-push" {
		t.Fatalf("resolved = (%d, %v), want scoped-push", count, store.resolved)
	}
}

func TestResolveGitOperationErrorsForSuccessfulPushResultLeavesMismatchedDestination(t *testing.T) {
	message := legacyGitPushMessage("scoped-push", time.Now().Add(-time.Minute))
	message.Metadata["git_operation_error_version"] = gitOperationFeedbackVersion
	message.Metadata["git_operation_attempted_remote"] = "origin"
	message.Metadata["git_operation_attempted_branch"] = "main"
	message.Metadata["git_operation_attempted_head_commit"] = "abc"
	store := &gitOperationFeedbackStoreStub{messages: []*models.Message{message}}
	repo := &gitOperationFeedbackTaskRepositoryStub{repositories: []*models.TaskRepository{{ID: "repo-1"}}}
	count, err := resolveGitOperationErrorsForSuccessfulPushResult(
		context.Background(), repo, store, "session-1", "task-1",
		&client.GitOperationResult{
			Success:          true,
			PushedRemote:     "backup",
			PushedBranch:     "feature/work",
			PushedHeadCommit: "different-head",
		},
	)
	if err != nil {
		t.Fatalf("resolveGitOperationErrorsForSuccessfulPushResult: %v", err)
	}
	if count != 0 || len(store.resolved) != 0 {
		t.Fatalf("resolved = (%d, %v), want none", count, store.resolved)
	}
}

func TestResolveGitOperationErrorsForSuccessfulPushResultAllowsRewrittenHead(t *testing.T) {
	message := legacyGitPushMessage("scoped-push", time.Now().Add(-time.Minute))
	message.Metadata["git_operation_error_version"] = gitOperationFeedbackVersion
	message.Metadata["git_operation_failed_at"] = time.Now().Add(-time.Minute).Format(time.RFC3339Nano)
	message.Metadata["git_operation_attempted_remote"] = "backup"
	message.Metadata["git_operation_attempted_branch"] = "feature/work"
	message.Metadata["git_operation_attempted_head_commit"] = "old-head"
	store := &gitOperationFeedbackStoreStub{messages: []*models.Message{message}}
	repo := &gitOperationFeedbackTaskRepositoryStub{repositories: []*models.TaskRepository{{ID: "repo-1"}}}
	count, err := resolveGitOperationErrorsForSuccessfulPushResult(
		context.Background(), repo, store, "session-1", "task-1",
		&client.GitOperationResult{
			Success:          true,
			PushedRemote:     "backup",
			PushedBranch:     "feature/work",
			PushedHeadCommit: "new-head",
		},
	)
	if err != nil {
		t.Fatalf("resolveGitOperationErrorsForSuccessfulPushResult: %v", err)
	}
	if count != 1 || len(store.resolved) != 1 || store.resolved[0] != "scoped-push" {
		t.Fatalf("resolved = (%d, %v), want scoped-push", count, store.resolved)
	}
}

func TestResolveGitOperationErrorsForSuccessfulPushResultLeavesFutureFailure(t *testing.T) {
	now := time.Now().UTC()
	message := legacyGitPushMessage("scoped-push", now.Add(-time.Minute))
	message.Metadata["git_operation_error_version"] = gitOperationFeedbackVersion
	message.Metadata["git_operation_failed_at"] = now.Add(time.Minute).Format(time.RFC3339Nano)
	message.Metadata["git_operation_attempted_remote"] = "backup"
	message.Metadata["git_operation_attempted_branch"] = "feature/work"
	store := &gitOperationFeedbackStoreStub{messages: []*models.Message{message}}
	repo := &gitOperationFeedbackTaskRepositoryStub{repositories: []*models.TaskRepository{{ID: "repo-1"}}}
	count, err := resolveGitOperationErrorsForSuccessfulPushResult(
		context.Background(), repo, store, "session-1", "task-1",
		&client.GitOperationResult{Success: true, PushedRemote: "backup", PushedBranch: "feature/work"},
	)
	if err != nil {
		t.Fatalf("resolveGitOperationErrorsForSuccessfulPushResult: %v", err)
	}
	if count != 0 || len(store.resolved) != 0 {
		t.Fatalf("resolved = (%d, %v), want none", count, store.resolved)
	}
}

func TestResolveGitOperationErrorsReturnsPersistenceErrorButContinues(t *testing.T) {
	now := time.Now().UTC()
	store := &gitOperationFeedbackStoreStub{
		messages:   []*models.Message{legacyGitPushMessage("push", now.Add(-time.Minute))},
		resolveErr: errors.New("write failed"),
	}
	repo := &gitOperationFeedbackTaskRepositoryStub{repositories: []*models.TaskRepository{{ID: "repo-1"}}}
	count, err := resolveGitOperationErrorsForSuccessfulPush(context.Background(), repo, store, "session-1", "task-1")
	if count != 0 || err == nil || err.Error() != "write failed" {
		t.Fatalf("result = (%d, %v), want persistence error", count, err)
	}
}
