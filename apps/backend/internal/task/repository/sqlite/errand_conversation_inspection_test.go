package sqlite

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
)

func TestErrandConversationInspection(t *testing.T) {
	exerciseErrandConversationInspection(t, newRepoForSessionTests(t))
}

func TestPostgresErrandConversationInspection(t *testing.T) {
	exerciseErrandConversationInspection(t, newPostgresErrandRepo(t))
}

func exerciseErrandConversationInspection(t *testing.T, repo *Repository) {
	t.Helper()
	taskID := newErrandTask(t, repo, "owned-inspection")
	parent := &models.TaskSession{ID: "parent-owned-inspection", TaskID: taskID, State: models.TaskSessionStateWaitingForInput}
	createErrandScenarioSession(t, repo, parent)
	row := sampleErrand("owned-inspection", taskID, parent.ID, errandScenarioBase)
	mustCreateErrand(t, repo, row)
	inspection, err := repo.GetErrandInspection(t.Context(), taskID, row.ID)
	require.NoError(t, err)
	require.False(t, inspection.OwnsConversation)
	require.False(t, inspection.ChildSessionExists)

	_, err = repo.ReserveErrandExecutionGeneration(t.Context(), row.ID, 0)
	require.NoError(t, err)
	inspection, err = repo.GetErrandInspection(t.Context(), taskID, row.ID)
	require.NoError(t, err)
	require.True(t, inspection.OwnsConversation)
	require.False(t, inspection.ChildSessionExists)
	require.Equal(t, parent.ID, inspection.Parent.ID)
	_, err = repo.GetErrandInspection(t.Context(), "another-task", row.ID)
	require.ErrorIs(t, err, models.ErrErrandNotFound)
}
