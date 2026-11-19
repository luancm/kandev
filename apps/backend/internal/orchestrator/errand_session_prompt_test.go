package orchestrator

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/sysprompt"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type ordinaryReplayRepository struct {
	sessionExecutorStore
	errandLookups int
}

func (r *ordinaryReplayRepository) GetErrand(context.Context, string) (*models.Errand, error) {
	r.errandLookups++
	return nil, models.ErrErrandNotFound
}

func TestInitialSubmissionReplayOrdinarySessionIsUnchanged(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-ordinary", "session-ordinary", "")
	session, err := repo.GetTaskSession(ctx, "session-ordinary")
	require.NoError(t, err)
	rows := &ordinaryReplayRepository{sessionExecutorStore: repo}
	svc := &Service{repo: rows}

	got, err := svc.wrapInitialSubmissionReplayPrompt(ctx, "task-ordinary", "session-ordinary", "hello", nil, session, "")

	require.NoError(t, err)
	assert.Zero(t, rows.errandLookups)
	want := sysprompt.InjectKandevContextWithOptions("task-ordinary", "session-ordinary", "hello", sysprompt.KandevContextOptions{
		IncludeCoordinatorTaskControls: true,
		IncludeUserQuestionTool:        true,
	})
	assert.Equal(t, want, got)
}
