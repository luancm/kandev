package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/statussummary"
)

// errandSummaryRow is one errand in the summary scenario. A session state
// empty means the errand has no session row yet.
type errandSummaryRow struct {
	id             string
	parentID       string
	lifecycle      models.ErrandLifecycle
	outcome        models.ErrandOutcome
	awaitingResult bool
	resultState    models.ErrandResultState
	sessionState   models.TaskSessionState
}

// exerciseErrandsSummary proves the bounded task projection: the live count
// follows the projected status, and waiting is set only when an idle parent
// owes an outcome, on the same rule as the parent's waiting_on_errands.
func exerciseErrandsSummary(t *testing.T, repo *Repository) {
	t.Helper()
	waiting := newErrandTask(t, repo, "summary-waiting")
	createErrandScenarioSession(t, repo, &models.TaskSession{ID: "sum-idle", TaskID: waiting, State: models.TaskSessionStateWaitingForInput})
	createErrandScenarioSession(t, repo, &models.TaskSession{ID: "sum-busy", TaskID: waiting, State: models.TaskSessionStateRunning})
	rows := []errandSummaryRow{
		// Settled and owed to the idle parent: not live, but waiting.
		{"sum-done", "sum-idle", models.ErrandLifecycleLaunched, models.ErrandOutcomeDone, true, models.ErrandResultPending, models.TaskSessionStateWaitingForInput},
		// Running under the busy parent: live, and the parent is not idle.
		{"sum-running", "sum-busy", models.ErrandLifecycleLaunched, "", false, models.ErrandResultNone, models.TaskSessionStateRunning},
		// Queued under the busy parent: live.
		{"sum-queued", "sum-busy", models.ErrandLifecycleQueued, "", true, models.ErrandResultNone, ""},
		// Cancelled and failed rows never count as live.
		{"sum-cancelled", "sum-busy", models.ErrandLifecycleCancelled, "", false, models.ErrandResultNone, models.TaskSessionStateCancelled},
		{"sum-failed", "sum-busy", models.ErrandLifecycleLaunched, models.ErrandOutcomeFailed, false, models.ErrandResultNone, models.TaskSessionStateFailed},
	}
	for index, row := range rows {
		createErrandSummaryRow(t, repo, waiting, row, index)
	}
	assertErrandsSummary(t, repo, waiting, statussummary.ErrandsSummary{Live: 2, Waiting: true})

	// Once the idle parent owes nothing, waiting clears while the live count
	// keeps tracking the running errands.
	settleErrandSummaryRow(t, repo, "sum-done", false, models.ErrandResultDelivered)
	assertErrandsSummary(t, repo, waiting, statussummary.ErrandsSummary{Live: 2, Waiting: false})

	quiet := newErrandTask(t, repo, "summary-quiet")
	createErrandScenarioSession(t, repo, &models.TaskSession{ID: "sum-quiet-parent", TaskID: quiet, State: models.TaskSessionStateWaitingForInput})
	createErrandSummaryRow(t, repo, quiet, errandSummaryRow{
		id: "sum-quiet-running", parentID: "sum-quiet-parent", lifecycle: models.ErrandLifecycleLaunched,
		awaitingResult: false, resultState: models.ErrandResultNone, sessionState: models.TaskSessionStateRunning,
	}, 0)
	assertErrandsSummary(t, repo, quiet, statussummary.ErrandsSummary{Live: 1, Waiting: false})

	assertErrandsSummary(t, repo, "task-without-errands-summary", statussummary.ErrandsSummary{})
}

func createErrandSummaryRow(t *testing.T, repo *Repository, taskID string, row errandSummaryRow, index int) {
	t.Helper()
	if row.sessionState != "" {
		createErrandScenarioSession(t, repo, &models.TaskSession{
			ID: row.id, TaskID: taskID, State: row.sessionState,
			Role: models.SessionRoleErrand, ParentSessionID: row.parentID,
		})
	}
	errand := sampleErrand(row.id, taskID, row.parentID, errandScenarioBase.Add(time.Duration(index)*time.Minute))
	errand.Lifecycle = row.lifecycle
	errand.LastOutcome = row.outcome
	errand.AwaitingResult = row.awaitingResult
	errand.ResultState = row.resultState
	mustCreateErrand(t, repo, errand)
}

func settleErrandSummaryRow(t *testing.T, repo *Repository, id string, awaiting bool, state models.ErrandResultState) {
	t.Helper()
	ctx := context.Background()
	errand, err := repo.GetErrand(ctx, id)
	if err != nil {
		t.Fatalf("GetErrand %s: %v", id, err)
	}
	errand.AwaitingResult = awaiting
	errand.ResultState = state
	if err := repo.UpdateErrand(ctx, errand); err != nil {
		t.Fatalf("UpdateErrand %s: %v", id, err)
	}
}

func assertErrandsSummary(t *testing.T, repo *Repository, taskID string, want statussummary.ErrandsSummary) {
	t.Helper()
	got, err := repo.ErrandsSummaryByTask(context.Background(), taskID)
	if err != nil {
		t.Fatalf("ErrandsSummaryByTask(%s): %v", taskID, err)
	}
	if got != want {
		t.Fatalf("ErrandsSummaryByTask(%s) = %+v, want %+v", taskID, got, want)
	}
}
