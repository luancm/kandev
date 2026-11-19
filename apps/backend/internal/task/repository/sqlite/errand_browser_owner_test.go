package sqlite

import (
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestErrandBrowserIgnoresOrdinarySessionIDCollision(t *testing.T) {
	exerciseErrandBrowserOwnerIdentity(t, newRepoForSessionTests(t))
}

func TestPostgresErrandBrowserIgnoresOrdinarySessionIDCollision(t *testing.T) {
	exerciseErrandBrowserOwnerIdentity(t, newPostgresErrandRepo(t))
}

func exerciseErrandBrowserOwnerIdentity(t *testing.T, repo *Repository) {
	t.Helper()
	ctx := t.Context()
	taskID := newErrandTask(t, repo, "owner-browser")
	otherTask := newErrandTask(t, repo, "ordinary-browser")
	parentID := "owner-browser-parent"
	errandID := "owner-browser-errand"
	createErrandScenarioSession(t, repo, &models.TaskSession{
		ID: parentID, TaskID: taskID, State: models.TaskSessionStateWaitingForInput,
	})
	createErrandScenarioSession(t, repo, &models.TaskSession{
		ID: errandID, TaskID: otherTask, State: models.TaskSessionStateRunning,
	})
	mustCreateErrand(t, repo, sampleErrand(errandID, taskID, parentID, errandScenarioBase))

	summary, err := repo.GetErrandBrowserSummary(ctx, taskID)
	require.NoError(t, err)
	require.Equal(t, []string{errandID}, browserErrandIDs(summary.Errands))
	require.Equal(t, 1, summary.Counts.Queued)
	statuses, err := repo.ReadErrandStatuses(ctx, taskID, errandID)
	require.NoError(t, err)
	require.Equal(t, models.ErrandStatusQueued, statuses[errandID].Status)
	view, err := repo.GetErrandInspection(ctx, taskID, errandID)
	require.NoError(t, err)
	require.Equal(t, errandID, view.Errand.ID)
	require.Equal(t, parentID, view.Parent.ID)
	_, err = repo.GetErrandInspection(ctx, otherTask, errandID)
	require.ErrorIs(t, err, models.ErrErrandNotFound)
}
