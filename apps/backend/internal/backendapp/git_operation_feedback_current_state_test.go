package backendapp

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func TestGitPushFeedbackUsesOneCurrentAlert(t *testing.T) {
	now := time.Now().UTC()
	store := &gitOperationFeedbackStoreStub{messages: []*models.Message{
		legacyGitPushMessage("first-failure", now.Add(-2*time.Minute)),
		legacyGitPushMessage("latest-failure", now.Add(-time.Minute)),
	}}
	repo := &gitOperationFeedbackTaskRepositoryStub{repositories: []*models.TaskRepository{{ID: "task-repo-1"}}}

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
	if count != 1 || len(store.resolved) != 1 || store.resolved[0] != "latest-failure" {
		t.Fatalf("resolved = (%d, %v), want latest-failure only", count, store.resolved)
	}
}
