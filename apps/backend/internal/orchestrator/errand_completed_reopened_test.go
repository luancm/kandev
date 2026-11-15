package orchestrator

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"

	kandevdb "github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/errands"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
)

// @covers AC-TASKS-ERRAND-COMPLETION-001.4
func TestErrandCompletionCleanupReopenedOwnerRetriesPendingResult(t *testing.T) {
	ctx := t.Context()
	seed := newErrandLaunchFixture(t)
	old := newCompletedConversationOwner(t, seed.repo)
	require.NoError(t, old.svc.messageQueue.SetAutoRun(ctx, seed.parent, false))
	now := time.Now().UTC()
	child := &models.TaskSession{ID: "reopened-child", TaskID: seed.taskID, ParentSessionID: seed.parent,
		Role: models.SessionRoleErrand, State: models.TaskSessionStateWaitingForInput,
		DownstreamACPSessionID: contractNativeConversation, StartedAt: now, UpdatedAt: now}
	require.NoError(t, old.repo.CreateTaskSession(ctx, child))
	require.NoError(t, old.repo.CreateErrand(ctx, &models.Errand{ID: child.ID, TaskID: seed.taskID, ParentSessionID: seed.parent,
		Title: "persist", Objective: "retain result", AgentProfileID: "errand-profile",
		FirstTurnID: "reopened-turn", SubmissionID: "prompt:errand:reopened-turn", Lifecycle: models.ErrandLifecycleLaunched,
		AwaitingResult: true, ResultState: models.ErrandResultNone, CreatedAt: now, UpdatedAt: now}))
	require.NoError(t, old.repo.CreateTurn(ctx, &models.Turn{ID: "reopened-turn", TaskID: seed.taskID,
		TaskSessionID: child.ID, StartedAt: now, CompletedAt: &now, CreatedAt: now, UpdatedAt: now}))
	require.NoError(t, old.repo.CreateMessage(ctx, &models.Message{ID: "reopened-output", TaskID: seed.taskID,
		TaskSessionID: child.ID, TurnID: "reopened-turn", AuthorType: models.MessageAuthorAgent,
		Content: "answer retained across owner replacement", CreatedAt: now}))
	old.retainExecution(t, child.ID, "reopened-execution")
	old.retainExecution(t, seed.parent, "contract-parent-execution")
	old.manager.stopAgentWithReasonFunc = func(context.Context, string, string, bool) error {
		return errors.New("provider close not acknowledged")
	}
	require.NoError(t, old.errands.OnErrandTurnSettled(ctx, errands.TurnSettlement{
		SessionID: child.ID, TurnID: "reopened-turn", Outcome: models.ErrandOutcomeDone,
		CompletionTarget: &models.ErrandTeardownTarget{TaskID: seed.taskID, SessionID: child.ID,
			SessionIncarnation: child.QueueIncarnationID, ExecutionID: "reopened-execution", TurnID: "reopened-turn"},
	}))
	require.ErrorContains(t, old.errands.RetryErrandCompletion(ctx, child.ID), "provider close not acknowledged")
	pending, err := old.repo.GetErrand(ctx, child.ID)
	require.NoError(t, err)
	require.Equal(t, models.ErrandResultPending, pending.ResultState)
	require.Equal(t, models.ErrandTeardownIntentPending, pending.TeardownIntent.State)
	queued := old.svc.messageQueue.GetStatus(ctx, seed.parent).Entries
	require.Len(t, queued, 1)
	var sequence int
	var name, path string
	require.NoError(t, old.repo.DB().QueryRow("PRAGMA database_list").Scan(&sequence, &name, &path))
	old.stop(t)
	require.NoError(t, old.repo.DB().Close())
	require.Error(t, old.repo.DB().Ping(), "old owners cannot keep using the closed pool")

	raw, err := kandevdb.OpenSQLite(path)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, raw.Close()) })
	opened := sqlx.NewDb(raw, "sqlite3")
	f := newCompletedConversationOwner(t, sqliterepo.NewWithInitializedDB(opened, opened, nil))
	var enabled atomic.Bool
	f.errands.SetEnabled(enabled.Load)
	f.svc.SetErrandsEnabled(enabled.Load)
	f.retainExecution(t, seed.parent, "contract-parent-execution")
	require.NoError(t, f.svc.messageQueue.SetAutoRun(ctx, seed.parent, true))
	require.NoError(t, f.errands.Reconcile(ctx))
	f.requireCompleted(t, child.ID, "reopened-turn")
	recovered, err := f.repo.GetErrand(ctx, child.ID)
	require.NoError(t, err)
	require.Equal(t, pending.ResultRevision, recovered.ResultRevision)
	require.Equal(t, pending.ResultText, recovered.ResultText)
	require.Equal(t, models.ErrandResultPending, recovered.ResultState)
	require.Equal(t, queued, f.svc.messageQueue.GetStatus(ctx, seed.parent).Entries)
	identity, err := f.svc.parentQueueIdentity(ctx, seed.parent)
	require.NoError(t, err)
	f.svc.CheckQueueAdmissionReadiness(ctx, identity)
	require.Empty(t, f.manager.stopAgentWithReasonArgs, "new authenticated inventory proves the original child absent")
	require.Empty(t, f.manager.launchedWith, "recovery must not replay the child")
	select {
	case prompt := <-f.prompts:
		t.Fatalf("disabled delivery dispatched %+v", prompt)
	default:
	}

	beforeDelivery := parentTerminalIdentityNotificationMetric(t)
	enabled.Store(true)
	require.NoError(t, f.errands.Reconcile(ctx))
	f.deliverOne(t, seed.parent, pending.ResultText)
	require.Equal(t, beforeDelivery+1, parentTerminalIdentityNotificationMetric(t))
	delivered, err := f.repo.GetErrand(ctx, child.ID)
	require.NoError(t, err)
	require.Equal(t, pending.ResultRevision, delivered.ResultRevision)
	require.Equal(t, models.ErrandResultDelivered, delivered.ResultState)
	require.Equal(t, models.ErrandDeliveryNotification, delivered.DeliveredRoute)
	parentTurns, err := f.repo.ListTurnsBySession(ctx, seed.parent)
	require.NoError(t, err)
	require.Len(t, parentTurns, 1)
	f.publishReady(t, seed.parent, "contract-parent-execution", parentTurns[0].ID)
	require.NoError(t, f.errands.Reconcile(ctx))
	require.Empty(t, f.svc.messageQueue.GetStatus(ctx, seed.parent).Entries)
	f.manager.mu.Lock()
	prompts := append([]string(nil), f.manager.capturedPrompts...)
	f.manager.mu.Unlock()
	require.Len(t, prompts, 1, "the reopened pending result is delivered once")
}
